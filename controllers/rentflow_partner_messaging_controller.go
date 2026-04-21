package controllers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

const rentFlowLineAPIBaseURL = "https://api.line.me"

type rentFlowLineConnectionPayload struct {
	ChannelID     string `json:"channelId"`
	ChannelSecret string `json:"channelSecret"`
	AccessToken   string `json:"accessToken"`
}

type rentFlowLineWebhookTestPayload struct {
	ChannelID     string `json:"channelId"`
	ChannelSecret string `json:"channelSecret"`
	AccessToken   string `json:"accessToken"`
	Endpoint      string `json:"endpoint"`
}

type rentFlowLineBotInfo struct {
	UserID      string `json:"userId"`
	BasicID     string `json:"basicId"`
	DisplayName string `json:"displayName"`
	PictureURL  string `json:"pictureUrl"`
	ChatMode    string `json:"chatMode"`
	MarkAsRead  string `json:"markAsReadMode"`
	PremiumID   string `json:"premiumId"`
}

type rentFlowLineWebhookTestResult struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

type rentFlowLineAPIError struct {
	Message string `json:"message"`
	Details []struct {
		Message  string `json:"message"`
		Property string `json:"property"`
	} `json:"details"`
}

type rentFlowLineWebhookEnvelope struct {
	Destination string                     `json:"destination"`
	Events      []rentFlowLineWebhookEvent `json:"events"`
}

type rentFlowLineWebhookEvent struct {
	Type           string `json:"type"`
	Mode           string `json:"mode"`
	ReplyToken     string `json:"replyToken"`
	WebhookEventID string `json:"webhookEventId"`
	Timestamp      int64  `json:"timestamp"`
	Source         struct {
		Type    string `json:"type"`
		UserID  string `json:"userId"`
		GroupID string `json:"groupId"`
		RoomID  string `json:"roomId"`
	} `json:"source"`
	Message struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"message"`
}

type rentFlowLineRecentEvent struct {
	ID          string    `json:"id"`
	Recipient   string    `json:"recipient"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body,omitempty"`
	Status      string    `json:"status"`
	ProviderRef string    `json:"providerRef,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type rentFlowLineConnectionResponse struct {
	TenantID              string                    `json:"tenantId,omitempty"`
	ShopName              string                    `json:"shopName,omitempty"`
	DomainSlug            string                    `json:"domainSlug,omitempty"`
	WebhookURL            string                    `json:"webhookUrl"`
	Status                string                    `json:"status"`
	IsConnected           bool                      `json:"isConnected"`
	ChannelID             string                    `json:"channelId,omitempty"`
	HasChannelSecret      bool                      `json:"hasChannelSecret"`
	HasAccessToken        bool                      `json:"hasAccessToken"`
	DisplayName           string                    `json:"displayName,omitempty"`
	BasicID               string                    `json:"basicId,omitempty"`
	BotUserID             string                    `json:"botUserId,omitempty"`
	PictureURL            string                    `json:"pictureUrl,omitempty"`
	LastVerifiedAt        *time.Time                `json:"lastVerifiedAt,omitempty"`
	LastWebhookTestAt     *time.Time                `json:"lastWebhookTestAt,omitempty"`
	LastWebhookTestStatus string                    `json:"lastWebhookTestStatus,omitempty"`
	LastError             string                    `json:"lastError,omitempty"`
	RecentEvents          []rentFlowLineRecentEvent `json:"recentEvents,omitempty"`
}

func RentFlowPartnerGetLineMessaging(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	channel, err := rentFlowLineChannelByTenant(tenant.ID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูล LINE OA ได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูล LINE OA สำเร็จ", rentFlowBuildLineConnectionResponse(c, tenant, channel))
}

func RentFlowPartnerSaveLineMessaging(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload rentFlowLineConnectionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูล LINE OA ไม่ถูกต้อง")
		return
	}

	channelID := strings.TrimSpace(payload.ChannelID)
	channelSecret := strings.TrimSpace(payload.ChannelSecret)
	accessToken := strings.TrimSpace(payload.AccessToken)
	if channelID == "" || channelSecret == "" || accessToken == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอก Channel ID, Channel Secret และ Access Token ให้ครบ")
		return
	}

	botInfo, err := rentFlowLineGetBotInfo(accessToken)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "ตรวจสอบ LINE OA ไม่ผ่าน: "+err.Error())
		return
	}

	channel, err := rentFlowLineChannelByTenant(tenant.ID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูล LINE OA ได้")
		return
	}

	now := time.Now()
	if channel == nil {
		channel = &models.RentFlowLineChannel{
			ID:       services.NewID("line"),
			TenantID: tenant.ID,
		}
	}

	channel.ChannelID = channelID
	channel.ChannelSecret = channelSecret
	channel.AccessToken = accessToken
	channel.DisplayName = strings.TrimSpace(botInfo.DisplayName)
	channel.BasicID = strings.TrimSpace(botInfo.BasicID)
	channel.BotUserID = strings.TrimSpace(botInfo.UserID)
	channel.PictureURL = strings.TrimSpace(botInfo.PictureURL)
	channel.WebhookURL = rentFlowLineWebhookURL(c, tenant)
	channel.Status = "connected"
	channel.LastVerifiedAt = &now
	channel.LastError = ""

	if err := config.DB.Save(channel).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูล LINE OA ได้")
		return
	}

	rentFlowAudit(c, tenant.ID, "line.save", "line_channel", channel.ID, channel.ChannelID)
	rentFlowSuccess(c, http.StatusOK, "บันทึก LINE OA สำเร็จ", rentFlowBuildLineConnectionResponse(c, tenant, channel))
}

func RentFlowPartnerTestLineMessaging(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload rentFlowLineConnectionPayload
	_ = c.ShouldBindJSON(&payload)

	channel, err := rentFlowLineChannelByTenant(tenant.ID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบ LINE OA ได้")
		return
	}

	channelID, channelSecret, accessToken, err := rentFlowLineResolveCredentials(payload.ChannelID, payload.ChannelSecret, payload.AccessToken, channel)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, err.Error())
		return
	}

	botInfo, err := rentFlowLineGetBotInfo(accessToken)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "ตรวจสอบ LINE OA ไม่ผ่าน: "+err.Error())
		return
	}

	response := rentFlowBuildLineConnectionResponse(c, tenant, channel)
	now := time.Now()
	response.ChannelID = channelID
	response.HasChannelSecret = channelSecret != ""
	response.HasAccessToken = accessToken != ""
	response.Status = "connected"
	response.IsConnected = true
	response.DisplayName = strings.TrimSpace(botInfo.DisplayName)
	response.BasicID = strings.TrimSpace(botInfo.BasicID)
	response.BotUserID = strings.TrimSpace(botInfo.UserID)
	response.PictureURL = strings.TrimSpace(botInfo.PictureURL)
	response.LastVerifiedAt = &now
	response.LastError = ""

	rentFlowAudit(c, tenant.ID, "line.test_connection", "line_channel", tenant.ID, channelID)
	rentFlowSuccess(c, http.StatusOK, "ตรวจสอบการเชื่อมต่อ LINE OA สำเร็จ", gin.H{
		"connection": response,
	})
}

func RentFlowPartnerTestLineWebhook(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload rentFlowLineWebhookTestPayload
	_ = c.ShouldBindJSON(&payload)

	channel, err := rentFlowLineChannelByTenant(tenant.ID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถทดสอบ Webhook LINE ได้")
		return
	}

	_, _, accessToken, err := rentFlowLineResolveCredentials(payload.ChannelID, payload.ChannelSecret, payload.AccessToken, channel)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, err.Error())
		return
	}

	endpoint := strings.TrimSpace(payload.Endpoint)
	if endpoint == "" {
		if channel != nil {
			endpoint = strings.TrimSpace(channel.WebhookURL)
		}
		if endpoint == "" {
			endpoint = rentFlowLineWebhookURL(c, tenant)
		}
	}

	result, err := rentFlowLineTestWebhook(accessToken, endpoint)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "ทดสอบ Webhook LINE ไม่ผ่าน: "+err.Error())
		return
	}

	now := time.Now()
	if channel != nil {
		updates := map[string]interface{}{
			"webhook_url":              endpoint,
			"last_webhook_test_at":     &now,
			"last_webhook_test_status": "failed",
			"updated_at":               now,
		}
		if result.Success {
			updates["last_webhook_test_status"] = "passed"
			updates["last_error"] = ""
		} else {
			updates["last_error"] = strings.TrimSpace(result.Reason)
		}
		_ = config.DB.Model(&models.RentFlowLineChannel{}).
			Where("tenant_id = ?", tenant.ID).
			Updates(updates).Error
		channel.WebhookURL = endpoint
		channel.LastWebhookTestAt = &now
		channel.LastWebhookTestStatus = updates["last_webhook_test_status"].(string)
		if result.Success {
			channel.LastError = ""
		} else {
			channel.LastError = strings.TrimSpace(result.Reason)
		}
	}

	logStatus := "test_failed"
	if result.Success {
		logStatus = "test_passed"
	}
	_ = config.DB.Create(&models.RentFlowMessageLog{
		ID:           services.NewID("msg"),
		TenantID:     tenant.ID,
		Channel:      "line",
		Recipient:    endpoint,
		Subject:      "webhook.test",
		Body:         "ผลการทดสอบ Webhook LINE",
		Status:       logStatus,
		ErrorMessage: strings.TrimSpace(result.Reason),
	}).Error

	rentFlowAudit(c, tenant.ID, "line.test_webhook", "line_channel", tenant.ID, endpoint)
	rentFlowSuccess(c, http.StatusOK, "ทดสอบ Webhook LINE สำเร็จ", gin.H{
		"connection":  rentFlowBuildLineConnectionResponse(c, tenant, channel),
		"webhookTest": gin.H{"success": result.Success, "reason": result.Reason, "endpoint": endpoint},
	})
}

func RentFlowPartnerDeleteLineMessaging(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	result := config.DB.Where("tenant_id = ?", tenant.ID).Delete(&models.RentFlowLineChannel{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบการเชื่อมต่อ LINE OA ได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ยังไม่มี LINE OA ที่เชื่อมไว้")
		return
	}

	rentFlowAudit(c, tenant.ID, "line.delete", "line_channel", tenant.ID, "")
	rentFlowSuccess(c, http.StatusOK, "ลบการเชื่อมต่อ LINE OA สำเร็จ", nil)
}

func RentFlowLineWebhook(c *gin.Context) {
	slug := rentFlowNormalizeDomainSlug(c.Param("tenantSlug"))
	if slug == "" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ไม่พบร้าน"})
		return
	}

	var tenant models.RentFlowTenant
	if err := config.DB.Where("domain_slug = ?", slug).First(&tenant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ไม่พบร้าน"})
		return
	}

	channel, err := rentFlowLineChannelByTenant(tenant.ID)
	if err != nil || channel == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ร้านนี้ยังไม่ได้เชื่อม LINE OA"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "อ่านข้อมูล Webhook ไม่สำเร็จ"})
		return
	}

	signature := strings.TrimSpace(c.GetHeader("X-Line-Signature"))
	if !rentFlowLineVerifySignature(channel.ChannelSecret, body, signature) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "ลายเซ็น Webhook ไม่ถูกต้อง"})
		return
	}

	var envelope rentFlowLineWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "ข้อมูล Webhook ไม่ถูกต้อง"})
		return
	}

	if len(envelope.Events) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	now := time.Now()
	for _, event := range envelope.Events {
		logEntry := models.RentFlowMessageLog{
			ID:          services.NewID("msg"),
			TenantID:    tenant.ID,
			Channel:     "line",
			Recipient:   rentFlowLineEventRecipient(event),
			Subject:     "line." + strings.TrimSpace(event.Type),
			Body:        rentFlowLineEventSummary(event),
			Status:      "received",
			ProviderRef: strings.TrimSpace(event.WebhookEventID),
		}
		_ = config.DB.Create(&logEntry).Error
		rentFlowSupportIngestLineEvent(&tenant, channel, event)
	}

	_ = config.DB.Model(&models.RentFlowLineChannel{}).
		Where("tenant_id = ?", tenant.ID).
		Updates(map[string]interface{}{
			"last_webhook_test_at":     &now,
			"last_webhook_test_status": "live",
			"last_error":               "",
			"updated_at":               now,
		}).Error

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func rentFlowLineChannelByTenant(tenantID string) (*models.RentFlowLineChannel, error) {
	var channel models.RentFlowLineChannel
	if err := config.DB.Where("tenant_id = ?", tenantID).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &channel, nil
}

func rentFlowLineResolveCredentials(channelID, channelSecret, accessToken string, channel *models.RentFlowLineChannel) (string, string, string, error) {
	resolvedChannelID := strings.TrimSpace(channelID)
	resolvedChannelSecret := strings.TrimSpace(channelSecret)
	resolvedAccessToken := strings.TrimSpace(accessToken)

	if channel != nil {
		if resolvedChannelID == "" {
			resolvedChannelID = strings.TrimSpace(channel.ChannelID)
		}
		if resolvedChannelSecret == "" {
			resolvedChannelSecret = strings.TrimSpace(channel.ChannelSecret)
		}
		if resolvedAccessToken == "" {
			resolvedAccessToken = strings.TrimSpace(channel.AccessToken)
		}
	}

	if resolvedChannelID == "" || resolvedChannelSecret == "" || resolvedAccessToken == "" {
		return "", "", "", errors.New("กรุณากรอก Channel ID, Channel Secret และ Access Token ให้ครบก่อนทดสอบ")
	}
	return resolvedChannelID, resolvedChannelSecret, resolvedAccessToken, nil
}

func rentFlowBuildLineConnectionResponse(c *gin.Context, tenant *models.RentFlowTenant, channel *models.RentFlowLineChannel) rentFlowLineConnectionResponse {
	response := rentFlowLineConnectionResponse{
		TenantID:     tenant.ID,
		ShopName:     tenant.ShopName,
		DomainSlug:   tenant.DomainSlug,
		WebhookURL:   rentFlowLineWebhookURL(c, tenant),
		Status:       "not_connected",
		IsConnected:  false,
		RecentEvents: rentFlowLineRecentEvents(tenant.ID),
	}
	if channel == nil {
		return response
	}

	response.Status = strings.TrimSpace(channel.Status)
	if response.Status == "" {
		response.Status = "draft"
	}
	response.IsConnected = response.Status == "connected"
	response.ChannelID = strings.TrimSpace(channel.ChannelID)
	response.HasChannelSecret = strings.TrimSpace(channel.ChannelSecret) != ""
	response.HasAccessToken = strings.TrimSpace(channel.AccessToken) != ""
	response.DisplayName = strings.TrimSpace(channel.DisplayName)
	response.BasicID = strings.TrimSpace(channel.BasicID)
	response.BotUserID = strings.TrimSpace(channel.BotUserID)
	response.PictureURL = strings.TrimSpace(channel.PictureURL)
	response.LastVerifiedAt = channel.LastVerifiedAt
	response.LastWebhookTestAt = channel.LastWebhookTestAt
	response.LastWebhookTestStatus = strings.TrimSpace(channel.LastWebhookTestStatus)
	response.LastError = strings.TrimSpace(channel.LastError)
	if webhookURL := strings.TrimSpace(channel.WebhookURL); webhookURL != "" {
		response.WebhookURL = webhookURL
	}
	return response
}

func rentFlowLineRecentEvents(tenantID string) []rentFlowLineRecentEvent {
	var logs []models.RentFlowMessageLog
	if err := config.DB.
		Where("tenant_id = ? AND channel = ?", tenantID, "line").
		Order("created_at DESC").
		Limit(10).
		Find(&logs).Error; err != nil {
		return nil
	}

	items := make([]rentFlowLineRecentEvent, 0, len(logs))
	for _, item := range logs {
		items = append(items, rentFlowLineRecentEvent{
			ID:          item.ID,
			Recipient:   item.Recipient,
			Subject:     item.Subject,
			Body:        item.Body,
			Status:      item.Status,
			ProviderRef: item.ProviderRef,
			CreatedAt:   item.CreatedAt,
		})
	}
	return items
}

func rentFlowLineWebhookURL(c *gin.Context, tenant *models.RentFlowTenant) string {
	return rentFlowPublicAPIBaseURL(c) + "/webhooks/line/" + tenant.DomainSlug
}

func rentFlowPublicAPIBaseURL(c *gin.Context) string {
	if value := strings.TrimSpace(os.Getenv("RENTFLOW_PUBLIC_API_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	host = strings.Trim(host, "/ ")
	if host == "" {
		return "http://localhost:8080"
	}

	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if strings.Contains(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
			scheme = "http"
		} else {
			scheme = "https"
		}
	}
	return scheme + "://" + host
}

func rentFlowLineVerifySignature(channelSecret string, body []byte, signature string) bool {
	channelSecret = strings.TrimSpace(channelSecret)
	signature = strings.TrimSpace(signature)
	if channelSecret == "" || signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(channelSecret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func rentFlowLineGetBotInfo(accessToken string) (*rentFlowLineBotInfo, error) {
	var response rentFlowLineBotInfo
	if err := rentFlowLineRequest(http.MethodGet, "/v2/bot/info", accessToken, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func rentFlowLineTestWebhook(accessToken, endpoint string) (*rentFlowLineWebhookTestResult, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, errors.New("ไม่พบ Webhook URL สำหรับทดสอบ")
	}

	var response rentFlowLineWebhookTestResult
	if err := rentFlowLineRequest(http.MethodPost, "/v2/bot/channel/webhook/test", accessToken, gin.H{
		"endpoint": endpoint,
	}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func rentFlowLineRequest(method, path, accessToken string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, rentFlowLineAPIBaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var apiErr rentFlowLineAPIError
		if json.Unmarshal(raw, &apiErr) == nil {
			message := strings.TrimSpace(apiErr.Message)
			if message != "" {
				return errors.New(message)
			}
			if len(apiErr.Details) > 0 {
				return errors.New(strings.TrimSpace(apiErr.Details[0].Message))
			}
		}
		if len(raw) > 0 {
			return errors.New(strings.TrimSpace(string(raw)))
		}
		return errors.New("LINE API ตอบกลับด้วยสถานะไม่สำเร็จ")
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func rentFlowLineEventRecipient(event rentFlowLineWebhookEvent) string {
	if userID := strings.TrimSpace(event.Source.UserID); userID != "" {
		return userID
	}
	if groupID := strings.TrimSpace(event.Source.GroupID); groupID != "" {
		return groupID
	}
	if roomID := strings.TrimSpace(event.Source.RoomID); roomID != "" {
		return roomID
	}
	return "unknown"
}

func rentFlowLineEventSummary(event rentFlowLineWebhookEvent) string {
	switch strings.TrimSpace(event.Type) {
	case "message":
		if text := strings.TrimSpace(event.Message.Text); text != "" {
			return text
		}
		if messageType := strings.TrimSpace(event.Message.Type); messageType != "" {
			return "ได้รับข้อความประเภท " + messageType
		}
		return "ได้รับข้อความใหม่จากลูกค้า"
	case "follow":
		return "ลูกค้าเพิ่มเพื่อน LINE OA"
	case "unfollow":
		return "ลูกค้ายกเลิกการติดตาม LINE OA"
	case "postback":
		return "ลูกค้ากดปุ่มจากข้อความ LINE OA"
	default:
		if eventType := strings.TrimSpace(event.Type); eventType != "" {
			return "ได้รับ event ประเภท " + eventType
		}
		return "ได้รับ event ใหม่จาก LINE OA"
	}
}
