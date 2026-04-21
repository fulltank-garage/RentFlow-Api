package controllers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rentflow-api/config"
	"rentflow-api/middleware"
	"rentflow-api/models"
	"rentflow-api/services"
)

type rentFlowLineUserProfile struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	PictureURL  string `json:"pictureUrl"`
}

func RentFlowPartnerGetSupport(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	response, err := rentFlowPartnerSupportResponse(tenant.ID, tenant.OwnerEmail)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลซัพพอร์ตได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลซัพพอร์ตสำเร็จ", response)
}

func RentFlowPartnerUpdateSupportTicket(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload struct {
		Status     string `json:"status"`
		Priority   string `json:"priority"`
		OwnerEmail string `json:"ownerEmail"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูล ticket ไม่ถูกต้อง")
		return
	}

	var ticket models.RentFlowSupportTicket
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("ticketId")).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบ ticket ที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหา ticket ได้")
		return
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if status := rentFlowNormalizeSupportStatus(payload.Status); status != "" {
		updates["status"] = status
		ticket.Status = status
	}
	if priority := rentFlowNormalizeSupportPriority(payload.Priority); priority != "" {
		updates["priority"] = priority
		ticket.Priority = priority
	}
	if ownerEmail := strings.TrimSpace(strings.ToLower(payload.OwnerEmail)); ownerEmail != "" || payload.OwnerEmail == "" {
		updates["owner_email"] = ownerEmail
		ticket.OwnerEmail = ownerEmail
	}

	if err := config.DB.Model(&models.RentFlowSupportTicket{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, ticket.ID).
		Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดต ticket ได้")
		return
	}

	rentFlowAudit(c, tenant.ID, "support.ticket.update", "support_ticket", ticket.ID, ticket.Status+"|"+ticket.Priority+"|"+ticket.OwnerEmail)
	rentFlowSuccess(c, http.StatusOK, "อัปเดต ticket สำเร็จ", gin.H{
		"id":         ticket.ID,
		"status":     ticket.Status,
		"priority":   ticket.Priority,
		"ownerEmail": ticket.OwnerEmail,
	})
}

func RentFlowPartnerCreateSupportMessage(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	user, _ := middleware.CurrentRentFlowUser(c)

	var payload struct {
		Message    string `json:"message"`
		IsInternal bool   `json:"isInternal"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อความซัพพอร์ตไม่ถูกต้อง")
		return
	}

	messageText := strings.TrimSpace(payload.Message)
	if messageText == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกข้อความก่อนส่ง")
		return
	}

	var ticket models.RentFlowSupportTicket
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("ticketId")).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบ ticket ที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหา ticket ได้")
		return
	}

	now := time.Now()
	messageStatus := "logged"
	messageFrom := "agent"
	if payload.IsInternal {
		messageFrom = "system"
	}

	if !payload.IsInternal && ticket.Channel == "line" && strings.TrimSpace(ticket.ExternalThreadID) != "" {
		channel, err := rentFlowLineChannelByTenant(tenant.ID)
		if err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบ LINE OA ของร้านได้")
			return
		}
		if channel == nil || strings.TrimSpace(channel.AccessToken) == "" {
			rentFlowError(c, http.StatusBadRequest, "ร้านนี้ยังไม่ได้เชื่อม LINE OA สำหรับส่งข้อความตอบกลับ")
			return
		}

		if err := rentFlowLinePushTextMessage(channel.AccessToken, ticket.ExternalThreadID, messageText); err != nil {
			messageStatus = "failed"
			_ = config.DB.Create(&models.RentFlowMessageLog{
				ID:           services.NewID("msg"),
				TenantID:     tenant.ID,
				Channel:      "line",
				Recipient:    ticket.ExternalThreadID,
				Subject:      "support.reply",
				Body:         messageText,
				Status:       "failed",
				ErrorMessage: err.Error(),
			}).Error
			rentFlowError(c, http.StatusBadRequest, "ส่งข้อความผ่าน LINE ไม่สำเร็จ: "+err.Error())
			return
		}

		messageStatus = "sent"
		_ = config.DB.Create(&models.RentFlowMessageLog{
			ID:        services.NewID("msg"),
			TenantID:  tenant.ID,
			Channel:   "line",
			Recipient: ticket.ExternalThreadID,
			Subject:   "support.reply",
			Body:      messageText,
			Status:    "sent",
		}).Error
	}

	message := models.RentFlowSupportMessage{
		ID:         services.NewID("supmsg"),
		TenantID:   tenant.ID,
		TicketID:   ticket.ID,
		FromType:   messageFrom,
		Message:    messageText,
		IsInternal: payload.IsInternal,
		Status:     messageStatus,
	}
	if err := config.DB.Create(&message).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกข้อความซัพพอร์ตได้")
		return
	}

	updates := map[string]interface{}{
		"last_message":    messageText,
		"last_message_at": &now,
		"updated_at":      now,
	}
	if ticket.Status == "new" || ticket.Status == "waiting" {
		updates["status"] = "open"
	}
	if !payload.IsInternal && strings.TrimSpace(ticket.OwnerEmail) == "" {
		updates["owner_email"] = strings.TrimSpace(strings.ToLower(user.Email))
	}
	if err := config.DB.Model(&models.RentFlowSupportTicket{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, ticket.ID).
		Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดต ticket หลังส่งข้อความได้")
		return
	}

	rentFlowAudit(c, tenant.ID, "support.message.create", "support_ticket", ticket.ID, messageFrom)
	rentFlowSuccess(c, http.StatusCreated, "บันทึกข้อความซัพพอร์ตสำเร็จ", gin.H{
		"id":         message.ID,
		"fromType":   message.FromType,
		"message":    message.Message,
		"isInternal": message.IsInternal,
		"status":     message.Status,
		"createdAt":  message.CreatedAt,
	})
}

func rentFlowPartnerSupportResponse(tenantID, ownerEmail string) (gin.H, error) {
	var tickets []models.RentFlowSupportTicket
	if err := config.DB.Where("tenant_id = ?", tenantID).Order("updated_at DESC").Find(&tickets).Error; err != nil {
		return nil, err
	}

	ticketIDs := make([]string, 0, len(tickets))
	bookingIDs := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		ticketIDs = append(ticketIDs, ticket.ID)
		if bookingID := strings.TrimSpace(ticket.BookingID); bookingID != "" {
			bookingIDs = append(bookingIDs, bookingID)
		}
	}

	messageMap := map[string][]models.RentFlowSupportMessage{}
	if len(ticketIDs) > 0 {
		var messages []models.RentFlowSupportMessage
		if err := config.DB.Where("tenant_id = ? AND ticket_id IN ?", tenantID, ticketIDs).Order("created_at ASC").Find(&messages).Error; err != nil {
			return nil, err
		}
		for _, message := range messages {
			messageMap[message.TicketID] = append(messageMap[message.TicketID], message)
		}
	}

	bookingCodeMap := map[string]string{}
	if len(bookingIDs) > 0 {
		var bookings []models.RentFlowBooking
		if err := config.DB.Where("tenant_id = ? AND id IN ?", tenantID, bookingIDs).Find(&bookings).Error; err == nil {
			for _, booking := range bookings {
				bookingCodeMap[booking.ID] = booking.BookingCode
			}
		}
	}

	items := make([]gin.H, 0, len(tickets))
	for _, ticket := range tickets {
		messages := messageMap[ticket.ID]
		customerMessages := make([]gin.H, 0, len(messages))
		internalNotes := make([]gin.H, 0, len(messages))
		for _, message := range messages {
			entry := gin.H{
				"id":         message.ID,
				"at":         message.CreatedAt,
				"from":       message.FromType,
				"text":       message.Message,
				"status":     message.Status,
				"isInternal": message.IsInternal,
			}
			if message.IsInternal {
				internalNotes = append(internalNotes, entry)
			} else {
				customerMessages = append(customerMessages, entry)
			}
		}

		items = append(items, gin.H{
			"id":               ticket.ID,
			"subject":          ticket.Subject,
			"customerName":     ticket.CustomerName,
			"email":            ticket.CustomerEmail,
			"phone":            ticket.CustomerPhone,
			"channel":          ticket.Channel,
			"status":           ticket.Status,
			"priority":         ticket.Priority,
			"ownerEmail":       ticket.OwnerEmail,
			"bookingId":        ticket.BookingID,
			"bookingCode":      bookingCodeMap[ticket.BookingID],
			"lastMessage":      ticket.LastMessage,
			"lastMessageAt":    ticket.LastMessageAt,
			"createdAt":        ticket.CreatedAt,
			"updatedAt":        ticket.UpdatedAt,
			"messages":         customerMessages,
			"internalNotes":    internalNotes,
			"externalThreadId": ticket.ExternalThreadID,
		})
	}

	owners, err := rentFlowPartnerSupportOwners(tenantID, ownerEmail)
	if err != nil {
		return nil, err
	}

	return gin.H{
		"items":  items,
		"owners": owners,
		"total":  len(items),
	}, nil
}

func rentFlowPartnerSupportOwners(tenantID, ownerEmail string) ([]gin.H, error) {
	seen := map[string]struct{}{}
	items := make([]gin.H, 0, 4)

	addItem := func(email, name string) {
		email = strings.TrimSpace(strings.ToLower(email))
		if email == "" {
			return
		}
		if _, ok := seen[email]; ok {
			return
		}
		seen[email] = struct{}{}
		items = append(items, gin.H{
			"email": email,
			"name":  strings.TrimSpace(name),
		})
	}

	if strings.TrimSpace(ownerEmail) != "" {
		var owner models.RentFlowUser
		if err := config.DB.Where("LOWER(email) = ?", strings.ToLower(ownerEmail)).First(&owner).Error; err == nil {
			addItem(owner.Email, owner.Name)
		} else {
			addItem(ownerEmail, ownerEmail)
		}
	}

	var members []models.RentFlowTenantMember
	if err := config.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").Order("role DESC, email ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	for _, member := range members {
		addItem(member.Email, member.Name)
	}
	return items, nil
}

func rentFlowNormalizeSupportStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "new", "open", "waiting", "resolved", "closed":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func rentFlowNormalizeSupportPriority(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "low", "normal", "high", "urgent":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func rentFlowSupportIngestLineEvent(tenant *models.RentFlowTenant, channel *models.RentFlowLineChannel, event rentFlowLineWebhookEvent) {
	threadID := strings.TrimSpace(rentFlowLineEventRecipient(event))
	if threadID == "" || threadID == "unknown" {
		return
	}

	providerRef := strings.TrimSpace(event.WebhookEventID)
	if providerRef != "" {
		var existingMessage models.RentFlowSupportMessage
		if err := config.DB.Where("tenant_id = ? AND provider_ref = ?", tenant.ID, providerRef).First(&existingMessage).Error; err == nil {
			return
		}
	}

	customerName := "ลูกค้า LINE"
	if strings.TrimSpace(event.Source.UserID) != "" && strings.TrimSpace(channel.AccessToken) != "" {
		if profile, err := rentFlowLineGetUserProfile(channel.AccessToken, event.Source.UserID); err == nil && strings.TrimSpace(profile.DisplayName) != "" {
			customerName = strings.TrimSpace(profile.DisplayName)
		}
	}
	if threadID != "" && customerName == "ลูกค้า LINE" {
		customerName = "ลูกค้า " + threadID
	}

	subject := strings.TrimSpace(rentFlowLineEventSummary(event))
	if len([]rune(subject)) > 120 {
		subject = string([]rune(subject)[:120])
	}
	if subject == "" {
		subject = "ข้อความจากลูกค้า"
	}

	now := time.Now()
	var ticket models.RentFlowSupportTicket
	err := config.DB.Where("tenant_id = ? AND channel = ? AND external_thread_id = ?", tenant.ID, "line", threadID).First(&ticket).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		ticket = models.RentFlowSupportTicket{
			ID:               services.NewID("supt"),
			TenantID:         tenant.ID,
			Channel:          "line",
			ExternalThreadID: threadID,
			Subject:          subject,
			CustomerName:     customerName,
			Status:           "new",
			Priority:         "normal",
			LastMessage:      subject,
			LastMessageAt:    &now,
		}
		_ = config.DB.Create(&ticket).Error
	case err == nil:
		updates := map[string]interface{}{
			"subject":         subject,
			"customer_name":   customerName,
			"last_message":    subject,
			"last_message_at": &now,
			"updated_at":      now,
		}
		if ticket.Status == "resolved" || ticket.Status == "closed" {
			updates["status"] = "open"
		}
		_ = config.DB.Model(&models.RentFlowSupportTicket{}).
			Where("tenant_id = ? AND id = ?", tenant.ID, ticket.ID).
			Updates(updates).Error
		ticket.Subject = subject
		ticket.CustomerName = customerName
		ticket.LastMessage = subject
		ticket.LastMessageAt = &now
	default:
		return
	}

	_ = config.DB.Create(&models.RentFlowSupportMessage{
		ID:          services.NewID("supmsg"),
		TenantID:    tenant.ID,
		TicketID:    ticket.ID,
		FromType:    "customer",
		Message:     subject,
		IsInternal:  false,
		Status:      "received",
		ProviderRef: providerRef,
	}).Error
}

func rentFlowLineGetUserProfile(accessToken, userID string) (*rentFlowLineUserProfile, error) {
	var response rentFlowLineUserProfile
	if err := rentFlowLineRequest(http.MethodGet, "/v2/bot/profile/"+userID, accessToken, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func rentFlowLinePushTextMessage(accessToken, to, message string) error {
	return rentFlowLineRequest(http.MethodPost, "/v2/bot/message/push", accessToken, gin.H{
		"to": strings.TrimSpace(to),
		"messages": []gin.H{
			{
				"type": "text",
				"text": strings.TrimSpace(message),
			},
		},
	}, nil)
}
