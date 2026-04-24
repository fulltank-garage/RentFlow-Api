package controllers

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rentflow-api/models"
)

func rentFlowUserAvatarURL(user models.RentFlowUser) string {
	if len(user.AvatarBlob) == 0 || strings.TrimSpace(user.AvatarMimeType) == "" {
		return ""
	}
	return "/users/" + url.PathEscape(user.ID) + "/avatar?v=" + url.QueryEscape(user.UpdatedAt.UTC().Format(time.RFC3339Nano))
}

func rentFlowTenantLogoURL(tenant models.RentFlowTenant) string {
	if len(tenant.LogoBlob) == 0 || strings.TrimSpace(tenant.LogoMimeType) == "" {
		return ""
	}
	return "/tenants/" + url.PathEscape(tenant.DomainSlug) + "/logo?v=" + url.QueryEscape(tenant.UpdatedAt.UTC().Format(time.RFC3339Nano))
}

func rentFlowTenantPromoImageURL(tenant models.RentFlowTenant) string {
	if len(tenant.PromoImageBlob) == 0 || strings.TrimSpace(tenant.PromoImageMimeType) == "" {
		return ""
	}
	return "/tenants/" + url.PathEscape(tenant.DomainSlug) + "/promo-image?v=" + url.QueryEscape(tenant.UpdatedAt.UTC().Format(time.RFC3339Nano))
}

func rentFlowPlatformImageURL(setting models.RentFlowPlatformSetting) string {
	if len(setting.ImageBlob) == 0 || strings.TrimSpace(setting.ImageMimeType) == "" {
		return ""
	}
	return "/platform/settings/marketplace-promo-image?v=" + url.QueryEscape(setting.UpdatedAt.UTC().Format(time.RFC3339Nano))
}

func rentFlowPaymentSlipURL(payment models.RentFlowPayment) string {
	if len(payment.SlipBlob) == 0 || strings.TrimSpace(payment.SlipMimeType) == "" {
		return ""
	}
	return "/payment-slips/" + url.PathEscape(payment.ID) + "?v=" + url.QueryEscape(payment.UpdatedAt.UTC().Format(time.RFC3339Nano))
}

func rentFlowUserResponse(user models.RentFlowUser) gin.H {
	return gin.H{
		"id":        user.ID,
		"username":  user.Username,
		"firstName": user.FirstName,
		"lastName":  user.LastName,
		"name":      user.Name,
		"email":     user.Email,
		"phone":     user.Phone,
		"avatarUrl": rentFlowUserAvatarURL(user),
		"createdAt": user.CreatedAt,
		"updatedAt": user.UpdatedAt,
	}
}

func rentFlowDecodeDataURLImage(source string) ([]byte, string, error) {
	if !strings.HasPrefix(source, "data:") {
		return nil, "", errors.New("ข้อมูลรูปภาพไม่ถูกต้อง")
	}

	commaIndex := strings.Index(source, ",")
	if commaIndex < 0 {
		return nil, "", errors.New("ข้อมูลรูปภาพไม่ถูกต้อง")
	}

	meta := source[:commaIndex]
	data := source[commaIndex+1:]
	if !strings.Contains(meta, ";base64") {
		return nil, "", errors.New("รูปภาพต้องอยู่ในรูปแบบ base64")
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", errors.New("ไม่สามารถอ่านข้อมูลรูปภาพได้")
	}

	return rentFlowValidateImageBlob(decoded)
}

func rentFlowFetchRemoteImage(source string) ([]byte, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", errors.New("ข้อมูลรูปภาพไม่ถูกต้อง")
	}

	client := &http.Client{Timeout: 12 * time.Second}
	request, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", errors.New("ไม่สามารถดาวน์โหลดรูปภาพได้")
	}
	request.Header.Set("User-Agent", "RentFlow-Api/1.0")

	response, err := client.Do(request)
	if err != nil {
		return nil, "", errors.New("ไม่สามารถดาวน์โหลดรูปภาพได้")
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", errors.New("ไม่สามารถดาวน์โหลดรูปภาพได้")
	}

	blob, err := io.ReadAll(io.LimitReader(response.Body, rentFlowMaxCarImageBytes+1))
	if err != nil {
		return nil, "", errors.New("ไม่สามารถอ่านรูปภาพได้")
	}

	return rentFlowValidateImageBlob(blob)
}

func rentFlowValidateImageBlob(blob []byte) ([]byte, string, error) {
	if len(blob) == 0 {
		return nil, "", errors.New("ไฟล์รูปภาพว่างเปล่า")
	}
	if len(blob) > rentFlowMaxCarImageBytes {
		return nil, "", errors.New("ไฟล์รูปภาพต้องมีขนาดไม่เกิน 5MB")
	}

	mimeType := http.DetectContentType(blob)
	if _, ok := rentFlowAllowedImageTypes[mimeType]; !ok {
		return nil, "", errors.New("รองรับเฉพาะไฟล์ JPG, PNG, WEBP หรือ GIF")
	}

	return blob, mimeType, nil
}

func rentFlowImageBlobFromSource(raw *string) ([]byte, string, error) {
	if raw == nil {
		return nil, "", nil
	}

	source := strings.TrimSpace(*raw)
	if source == "" {
		return []byte{}, "", nil
	}

	if strings.HasPrefix(source, "data:") {
		return rentFlowDecodeDataURLImage(source)
	}

	return rentFlowFetchRemoteImage(source)
}

func rentFlowSendImageBlob(c *gin.Context, mimeType string, blob []byte) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, blob)
}
