package controllers

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rentflow-api/config"
	"rentflow-api/middleware"
	"rentflow-api/models"
	"rentflow-api/services"
)

const rentFlowDefaultTenantID = "tenant_fulltank"

var (
	rentFlowDomainSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,38}[a-z0-9])$`)
	rentFlowReservedSlugs     = map[string]struct{}{
		"admin":     {},
		"api":       {},
		"app":       {},
		"dashboard": {},
		"partner":   {},
		"partners":  {},
		"rentflow":  {},
		"support":   {},
		"www":       {},
	}
)

type rentFlowUploadedPromoImage struct {
	Blob     []byte
	MimeType string
}

func RentFlowResolveTenant(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลร้านสำเร็จ", rentFlowPublicTenantResponse(*tenant))
}

func RentFlowGetMyTenant(c *gin.Context) {
	tenant, err := rentFlowCurrentUserTenant(c)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ยังไม่ได้ตั้งค่าร้าน")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลร้านได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลร้านสำเร็จ", rentFlowOwnerTenantResponse(*tenant))
}

func RentFlowUpsertMyTenant(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	var payload struct {
		ShopName         string    `json:"shopName"`
		DomainSlug       string    `json:"domainSlug"`
		ChatThresholdTHB int64     `json:"chatThresholdTHB"`
		LogoURL          *string   `json:"logoUrl"`
		PromoImageURL    *string   `json:"promoImageUrl"`
		PromoImageURLs   *[]string `json:"promoImageUrls"`
		ClearPromoImages bool      `json:"clearPromoImages"`
	}

	var logoBlob []byte
	var logoMimeType string
	var promoImageBlob []byte
	var promoImageMimeType string
	var promoImages []rentFlowUploadedPromoImage
	logoProvided := false
	promoImageProvided := false
	promoImagesProvided := false
	clearPromoImages := false

	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	isMultipart := strings.Contains(contentType, "multipart/form-data") ||
		strings.Contains(strings.ToLower(c.ContentType()), "multipart/form-data") ||
		strings.Contains(contentType, "boundary=")
	if isMultipart {
		payload.ShopName = c.PostForm("shopName")
		payload.DomainSlug = c.PostForm("domainSlug")
		payload.ChatThresholdTHB = rentFlowParseThreshold(c.PostForm("chatThresholdTHB"))
		clearPromoImages = strings.EqualFold(strings.TrimSpace(c.PostForm("clearPromoImages")), "true")

		if value, exists := c.GetPostForm("logoUrl"); exists {
			payload.LogoURL = &value
			logoProvided = true
			var err error
			logoBlob, logoMimeType, err = rentFlowImageBlobFromSource(&value)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, "โลโก้ร้านไม่ถูกต้อง")
				return
			}
		}
		if fileHeader, err := c.FormFile("logo"); err == nil {
			logoProvided = true
			logoBlob, logoMimeType, err = rentFlowImageBlobFromUpload(fileHeader)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, err.Error())
				return
			}
		} else if !errors.Is(err, http.ErrMissingFile) {
			rentFlowError(c, http.StatusBadRequest, "โลโก้ร้านไม่ถูกต้อง")
			return
		}

		if value, exists := c.GetPostForm("promoImageUrl"); exists {
			payload.PromoImageURL = &value
			promoImageProvided = true
			var err error
			promoImageBlob, promoImageMimeType, err = rentFlowImageBlobFromSource(&value)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, "รูปโปรโมชันไม่ถูกต้อง")
				return
			}
		}
		if fileHeader, err := c.FormFile("promoImage"); err == nil {
			promoImageProvided = true
			promoImageBlob, promoImageMimeType, err = rentFlowImageBlobFromUpload(fileHeader)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, err.Error())
				return
			}
		} else if !errors.Is(err, http.ErrMissingFile) {
			rentFlowError(c, http.StatusBadRequest, "รูปโปรโมชันไม่ถูกต้อง")
			return
		}
		if form, err := c.MultipartForm(); err == nil && form != nil {
			for _, source := range rentFlowMultipartStringValues(form.Value["promoImageUrls"]) {
				blob, mimeType, err := rentFlowImageBlobFromSource(&source)
				if err != nil {
					rentFlowError(c, http.StatusBadRequest, "รูปโปรโมชันไม่ถูกต้อง")
					return
				}
				promoImages = append(promoImages, rentFlowUploadedPromoImage{
					Blob:     blob,
					MimeType: mimeType,
				})
			}
			for _, fileHeader := range form.File["promoImages"] {
				blob, mimeType, err := rentFlowImageBlobFromUpload(fileHeader)
				if err != nil {
					rentFlowError(c, http.StatusBadRequest, err.Error())
					return
				}
				promoImages = append(promoImages, rentFlowUploadedPromoImage{
					Blob:     blob,
					MimeType: mimeType,
				})
			}
		}
		if len(promoImages) > 0 {
			promoImagesProvided = true
			promoImageProvided = true
			promoImageBlob = promoImages[0].Blob
			promoImageMimeType = promoImages[0].MimeType
		}
	} else if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลร้านไม่ถูกต้อง")
		return
	} else {
		clearPromoImages = payload.ClearPromoImages
	}

	shopName := strings.TrimSpace(payload.ShopName)
	domainSlug := rentFlowNormalizeDomainSlug(payload.DomainSlug)
	chatThresholdTHB := payload.ChatThresholdTHB
	if chatThresholdTHB < 0 {
		chatThresholdTHB = 0
	}
	if len([]rune(shopName)) < 2 {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อร้านให้ถูกต้อง")
		return
	}
	if message := rentFlowValidateDomainSlug(domainSlug); message != "" {
		rentFlowError(c, http.StatusBadRequest, message)
		return
	}

	if !isMultipart {
		var err error
		logoProvided = payload.LogoURL != nil
		promoImageProvided = payload.PromoImageURL != nil
		logoBlob, logoMimeType, err = rentFlowImageBlobFromSource(payload.LogoURL)
		if err != nil {
			rentFlowError(c, http.StatusBadRequest, "โลโก้ร้านไม่ถูกต้อง")
			return
		}
		promoImageBlob, promoImageMimeType, err = rentFlowImageBlobFromSource(payload.PromoImageURL)
		if err != nil {
			rentFlowError(c, http.StatusBadRequest, "รูปโปรโมชันไม่ถูกต้อง")
			return
		}
		if payload.PromoImageURLs != nil {
			promoImagesProvided = true
			for _, source := range *payload.PromoImageURLs {
				value := strings.TrimSpace(source)
				if value == "" {
					continue
				}
				blob, mimeType, err := rentFlowImageBlobFromSource(&value)
				if err != nil {
					rentFlowError(c, http.StatusBadRequest, "รูปโปรโมชันไม่ถูกต้อง")
					return
				}
				promoImages = append(promoImages, rentFlowUploadedPromoImage{
					Blob:     blob,
					MimeType: mimeType,
				})
			}
			if len(promoImages) > 0 {
				promoImageProvided = true
				promoImageBlob = promoImages[0].Blob
				promoImageMimeType = promoImages[0].MimeType
			}
		}
	}

	publicDomain := rentFlowPublicDomain(domainSlug)

	var existing models.RentFlowTenant
	result := config.DB.
		Where("owner_user_id = ? OR owner_email = ?", user.ID, user.Email).
		First(&existing)

	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบข้อมูลร้านได้")
		return
	}

	conflictQuery := config.DB.
		Model(&models.RentFlowTenant{}).
		Where("(domain_slug = ? OR public_domain = ?)", domainSlug, publicDomain)
	if result.Error == nil {
		conflictQuery = conflictQuery.Where("id <> ?", existing.ID)
	}

	var conflictCount int64
	if err := conflictQuery.Count(&conflictCount).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบโดเมนได้")
		return
	}
	if conflictCount > 0 {
		rentFlowError(c, http.StatusConflict, "โดเมนนี้ถูกใช้งานแล้ว")
		return
	}

	ownerUserID := user.ID
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		tenant := models.RentFlowTenant{
			ID:                 services.NewID("tnt"),
			OwnerUserID:        &ownerUserID,
			OwnerEmail:         user.Email,
			ShopName:           shopName,
			DomainSlug:         domainSlug,
			PublicDomain:       publicDomain,
			LogoMimeType:       logoMimeType,
			LogoBlob:           logoBlob,
			PromoImageMimeType: promoImageMimeType,
			PromoImageBlob:     promoImageBlob,
			Status:             "active",
			BookingMode:        "payment",
			ChatThresholdTHB:   chatThresholdTHB,
			Plan:               "starter",
		}
		if err := config.DB.Create(&tenant).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างร้านได้")
			return
		}
		if promoImagesProvided && len(promoImages) > 0 {
			if err := rentFlowReplaceTenantPromoImages(tenant.ID, promoImages); err != nil {
				rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปโปรโมชันได้")
				return
			}
		} else if promoImageProvided && len(promoImageBlob) > 0 && promoImageMimeType != "" {
			if err := rentFlowReplaceTenantPromoImages(tenant.ID, []rentFlowUploadedPromoImage{{
				Blob:     promoImageBlob,
				MimeType: promoImageMimeType,
			}}); err != nil {
				rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปโปรโมชันได้")
				return
			}
		}
		services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
		services.CacheDeleteByPrefix(config.Ctx, services.RentFlowBranchesCachePrefix())
		rentFlowSuccess(c, http.StatusCreated, "บันทึกข้อมูลร้านสำเร็จ", rentFlowOwnerTenantResponse(tenant))
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"owner_user_id":      ownerUserID,
		"owner_email":        user.Email,
		"shop_name":          shopName,
		"domain_slug":        domainSlug,
		"public_domain":      publicDomain,
		"status":             "active",
		"chat_threshold_thb": chatThresholdTHB,
		"updated_at":         now,
	}
	if existing.Plan == "" {
		updates["plan"] = "starter"
	}
	if existing.BookingMode == "" {
		updates["booking_mode"] = "payment"
	}
	if logoProvided {
		updates["logo_mime_type"] = logoMimeType
		updates["logo_blob"] = logoBlob
	}
	if promoImageProvided {
		updates["promo_image_mime_type"] = promoImageMimeType
		updates["promo_image_blob"] = promoImageBlob
	} else if clearPromoImages {
		updates["promo_image_mime_type"] = ""
		updates["promo_image_blob"] = []byte{}
	}

	if err := config.DB.Model(&models.RentFlowTenant{}).
		Where("id = ?", existing.ID).
		Select(rentFlowUpdateColumns(updates)).
		Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลร้านได้")
		return
	}

	existing.OwnerUserID = &ownerUserID
	existing.OwnerEmail = user.Email
	existing.ShopName = shopName
	existing.DomainSlug = domainSlug
	existing.PublicDomain = publicDomain
	existing.Status = "active"
	existing.ChatThresholdTHB = chatThresholdTHB
	existing.UpdatedAt = now
	if existing.Plan == "" {
		existing.Plan = "starter"
	}
	if existing.BookingMode == "" {
		existing.BookingMode = "payment"
	}
	if logoProvided {
		existing.LogoMimeType = logoMimeType
		existing.LogoBlob = logoBlob
	}
	if promoImageProvided {
		existing.PromoImageMimeType = promoImageMimeType
		existing.PromoImageBlob = promoImageBlob
	} else if clearPromoImages {
		existing.PromoImageMimeType = ""
		existing.PromoImageBlob = nil
	}
	if promoImagesProvided {
		if err := rentFlowReplaceTenantPromoImages(existing.ID, promoImages); err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปโปรโมชันได้")
			return
		}
	} else if promoImageProvided && len(promoImageBlob) > 0 && promoImageMimeType != "" {
		if err := rentFlowReplaceTenantPromoImages(existing.ID, []rentFlowUploadedPromoImage{{
			Blob:     promoImageBlob,
			MimeType: promoImageMimeType,
		}}); err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปโปรโมชันได้")
			return
		}
	} else if clearPromoImages {
		if err := rentFlowReplaceTenantPromoImages(existing.ID, nil); err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปโปรโมชันได้")
			return
		}
	}

	_ = config.DB.Where("id = ?", existing.ID).First(&existing).Error

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowBranchesCachePrefix())
	rentFlowSuccess(c, http.StatusOK, "บันทึกข้อมูลร้านสำเร็จ", rentFlowOwnerTenantResponse(existing))
}

func rentFlowMultipartStringValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func rentFlowReplaceTenantPromoImages(tenantID string, images []rentFlowUploadedPromoImage) error {
	return config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&models.RentFlowTenantPromoImage{}).Error; err != nil {
			return err
		}
		for index, image := range images {
			if len(image.Blob) == 0 || strings.TrimSpace(image.MimeType) == "" {
				continue
			}
			item := models.RentFlowTenantPromoImage{
				ID:           services.NewID("tpi"),
				TenantID:     tenantID,
				MimeType:     image.MimeType,
				Blob:         image.Blob,
				DisplayOrder: index + 1,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func RentFlowPartnerReorderPromoImages(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var payload struct {
		ImageIDs []string `json:"imageIds"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || len(payload.ImageIDs) == 0 {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลลำดับรูปโปรโมชันไม่ถูกต้อง")
		return
	}
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		for index, imageID := range payload.ImageIDs {
			id := strings.TrimSpace(imageID)
			if id == "" {
				continue
			}
			if err := tx.Model(&models.RentFlowTenantPromoImage{}).
				Where("tenant_id = ? AND id = ?", tenant.ID, id).
				Updates(map[string]interface{}{"display_order": index + 1, "updated_at": time.Now()}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถจัดลำดับรูปโปรโมชันได้")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowAudit(c, tenant.ID, "promo_images.reorder", "tenant", tenant.ID, strings.Join(payload.ImageIDs, ","))
	rentFlowSuccess(c, http.StatusOK, "จัดลำดับรูปโปรโมชันสำเร็จ", rentFlowOwnerTenantResponse(*tenant))
}

func RentFlowPartnerDeletePromoImage(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	result := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("imageId")).Delete(&models.RentFlowTenantPromoImage{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรูปโปรโมชันได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowAudit(c, tenant.ID, "promo_images.delete", "tenant_promo_image", c.Param("imageId"), "")
	rentFlowSuccess(c, http.StatusOK, "ลบรูปโปรโมชันสำเร็จ", rentFlowOwnerTenantResponse(*tenant))
}

func rentFlowUpdateColumns(updates map[string]interface{}) []string {
	columns := make([]string, 0, len(updates))
	for key := range updates {
		columns = append(columns, key)
	}
	return columns
}

func rentFlowRequireTenant(c *gin.Context) (*models.RentFlowTenant, bool) {
	tenant, err := rentFlowTenantFromRequest(c, true)
	if err == nil {
		return tenant, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rentFlowError(c, http.StatusNotFound, "ไม่พบร้านที่ต้องการ")
		return nil, false
	}
	rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบร้านได้")
	return nil, false
}

func rentFlowTenantFromRequest(c *gin.Context, allowDefault bool) (*models.RentFlowTenant, error) {
	identity := rentFlowTenantIdentityFromRequest(c)
	host := rentFlowNormalizeTenantHost(identity)
	slug := rentFlowSlugFromTenantIdentity(identity)

	if slug == "" && host == "" && allowDefault {
		return rentFlowDefaultTenant()
	}

	query := config.DB.Where("status = ?", "active")
	switch {
	case slug != "" && host != "":
		query = query.Where("domain_slug = ? OR public_domain = ?", slug, host)
	case slug != "":
		query = query.Where("domain_slug = ?", slug)
	case host != "":
		query = query.Where("public_domain = ?", host)
	default:
		return nil, gorm.ErrRecordNotFound
	}

	var tenant models.RentFlowTenant
	if err := query.First(&tenant).Error; err != nil {
		return nil, err
	}
	return &tenant, nil
}

func rentFlowIsMarketplaceRequest(c *gin.Context) bool {
	for _, value := range []string{
		c.Query("marketplace"),
		c.GetHeader("X-RentFlow-Marketplace"),
	} {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "1", "true", "yes", "on":
			return true
		}
	}

	host := rentFlowNormalizeTenantHost(rentFlowTenantIdentityFromRequest(c))
	return host != "" && host == rentFlowRootDomain()
}

func rentFlowMarketplaceTenants() ([]models.RentFlowTenant, error) {
	var tenants []models.RentFlowTenant
	if err := config.DB.
		Where("status = ?", "active").
		Order("shop_name ASC").
		Find(&tenants).Error; err != nil {
		return nil, err
	}
	return tenants, nil
}

func rentFlowTenantMap(tenants []models.RentFlowTenant) map[string]models.RentFlowTenant {
	items := make(map[string]models.RentFlowTenant, len(tenants))
	for _, tenant := range tenants {
		items[tenant.ID] = tenant
	}
	return items
}

func rentFlowCurrentUserTenant(c *gin.Context) (*models.RentFlowTenant, error) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}

	var tenant models.RentFlowTenant
	if err := config.DB.
		Where("owner_user_id = ? OR owner_email = ?", user.ID, user.Email).
		First(&tenant).Error; err != nil {
		var member models.RentFlowTenantMember
		memberErr := config.DB.
			Where("status = ? AND (user_id = ? OR LOWER(email) = ?)", "active", user.ID, strings.ToLower(user.Email)).
			First(&member).Error
		if memberErr != nil {
			return nil, err
		}
		if err := config.DB.Where("id = ?", member.TenantID).First(&tenant).Error; err != nil {
			return nil, err
		}
	}
	return &tenant, nil
}

func rentFlowDefaultTenant() (*models.RentFlowTenant, error) {
	var tenant models.RentFlowTenant
	if err := config.DB.Where("id = ?", rentFlowDefaultTenantID).First(&tenant).Error; err != nil {
		return nil, err
	}
	return &tenant, nil
}

func rentFlowTenantIdentityFromRequest(c *gin.Context) string {
	for _, value := range []string{
		c.Query("tenant"),
		c.Query("host"),
		c.GetHeader("X-RentFlow-Tenant"),
		c.GetHeader("X-RentFlow-Host"),
		c.GetHeader("X-Forwarded-Host"),
		c.Request.Host,
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func rentFlowNormalizeDomainSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAllowed {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' && !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func rentFlowValidateDomainSlug(slug string) string {
	if slug == "" {
		return "กรุณากรอกโดเมนร้าน"
	}
	if !rentFlowDomainSlugPattern.MatchString(slug) {
		return "โดเมนต้องเป็นภาษาอังกฤษตัวพิมพ์เล็ก ตัวเลข หรือขีดกลาง ความยาว 3-40 ตัวอักษร"
	}
	if _, reserved := rentFlowReservedSlugs[slug]; reserved {
		return "โดเมนนี้ไม่สามารถใช้งานได้"
	}
	return ""
}

func rentFlowPublicDomain(slug string) string {
	return slug + "." + rentFlowRootDomain()
}

func rentFlowRootDomain() string {
	rootDomain := strings.Trim(strings.ToLower(os.Getenv("RENTFLOW_ROOT_DOMAIN")), ". ")
	if rootDomain == "" {
		rootDomain = "rentflow.com"
	}
	return rootDomain
}

func rentFlowNormalizeTenantHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	}
	value = strings.Split(value, "/")[0]
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else if strings.Count(value, ":") == 1 {
		value = strings.Split(value, ":")[0]
	}
	value = strings.Trim(value, ". ")

	if value == "localhost" || value == "127.0.0.1" || value == "::1" {
		return ""
	}
	return value
}

func rentFlowSlugFromTenantIdentity(value string) string {
	raw := strings.TrimSpace(strings.ToLower(value))
	if raw == "" {
		return ""
	}
	host := rentFlowNormalizeTenantHost(value)
	if host == "" {
		if strings.Contains(raw, "localhost") || strings.Contains(raw, "127.0.0.1") || strings.Contains(raw, "::1") {
			return ""
		}
		return rentFlowNormalizeDomainSlug(value)
	}

	rootDomain := rentFlowRootDomain()
	if host == rootDomain {
		return ""
	}
	if strings.HasSuffix(host, ".localhost") {
		slug := strings.TrimSuffix(host, ".localhost")
		if strings.Contains(slug, ".") {
			parts := strings.Split(slug, ".")
			slug = parts[len(parts)-1]
		}
		return rentFlowNormalizeDomainSlug(slug)
	}
	if strings.HasSuffix(host, "."+rootDomain) {
		slug := strings.TrimSuffix(host, "."+rootDomain)
		if strings.Contains(slug, ".") {
			parts := strings.Split(slug, ".")
			slug = parts[len(parts)-1]
		}
		return rentFlowNormalizeDomainSlug(slug)
	}
	if !strings.Contains(host, ".") {
		return rentFlowNormalizeDomainSlug(host)
	}
	return ""
}

func rentFlowPublicTenantResponse(tenant models.RentFlowTenant) gin.H {
	promoImageUrls := rentFlowTenantPromoImageURLs(tenant)
	promoImageUrl := ""
	if len(promoImageUrls) > 0 {
		promoImageUrl = promoImageUrls[0]
	}

	response := gin.H{
		"id":               tenant.ID,
		"shopName":         tenant.ShopName,
		"domainSlug":       tenant.DomainSlug,
		"publicDomain":     tenant.PublicDomain,
		"logoUrl":          rentFlowTenantLogoURL(tenant),
		"promoImageUrl":    promoImageUrl,
		"promoImageUrls":   promoImageUrls,
		"status":           tenant.Status,
		"bookingMode":      rentFlowNormalizeBookingMode(tenant.BookingMode),
		"chatThresholdTHB": tenant.ChatThresholdTHB,
		"plan":             tenant.Plan,
		"createdAt":        tenant.CreatedAt,
		"updatedAt":        tenant.UpdatedAt,
	}
	if lineSummary := rentFlowPublicLineSummary(tenant.ID); lineSummary != nil {
		response["lineOfficialAccount"] = lineSummary
	}
	return response
}

func rentFlowOwnerTenantResponse(tenant models.RentFlowTenant) gin.H {
	response := rentFlowPublicTenantResponse(tenant)
	response["ownerEmail"] = tenant.OwnerEmail
	return response
}

func RentFlowGetTenantLogo(c *gin.Context) {
	slug := rentFlowNormalizeDomainSlug(c.Param("tenantSlug"))
	if slug == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบโลโก้ร้าน")
		return
	}

	var tenant models.RentFlowTenant
	if err := config.DB.Where("status = ? AND domain_slug = ?", "active", slug).First(&tenant).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบโลโก้ร้าน")
		return
	}

	if len(tenant.LogoBlob) == 0 || strings.TrimSpace(tenant.LogoMimeType) == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบโลโก้ร้าน")
		return
	}

	rentFlowSendImageBlob(c, tenant.LogoMimeType, tenant.LogoBlob)
}

func RentFlowGetTenantPromoImage(c *gin.Context) {
	slug := rentFlowNormalizeDomainSlug(c.Param("tenantSlug"))
	if slug == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	var tenant models.RentFlowTenant
	if err := config.DB.Where("status = ? AND domain_slug = ?", "active", slug).First(&tenant).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	if len(tenant.PromoImageBlob) == 0 || strings.TrimSpace(tenant.PromoImageMimeType) == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	rentFlowSendImageBlob(c, tenant.PromoImageMimeType, tenant.PromoImageBlob)
}

func RentFlowGetTenantPromoImageByID(c *gin.Context) {
	slug := rentFlowNormalizeDomainSlug(c.Param("tenantSlug"))
	imageID := strings.TrimSpace(c.Param("imageId"))
	if slug == "" || imageID == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	var tenant models.RentFlowTenant
	if err := config.DB.Where("status = ? AND domain_slug = ?", "active", slug).First(&tenant).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	var image models.RentFlowTenantPromoImage
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, imageID).First(&image).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	if len(image.Blob) == 0 || strings.TrimSpace(image.MimeType) == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชัน")
		return
	}

	rentFlowSendImageBlob(c, image.MimeType, image.Blob)
}

func rentFlowPublicLineSummary(tenantID string) gin.H {
	channel, err := rentFlowLineChannelByTenant(tenantID)
	if err != nil || channel == nil {
		return nil
	}

	basicID := strings.TrimSpace(channel.BasicID)
	encodedID := url.PathEscape(basicID)
	chatURL := ""
	shareURL := ""
	if basicID != "" {
		chatURL = "https://line.me/R/oaMessage/" + encodedID + "/"
		shareURL = "https://line.me/R/ti/p/" + encodedID
	}

	return gin.H{
		"displayName": channel.DisplayName,
		"basicId":     basicID,
		"pictureUrl":  channel.PictureURL,
		"chatUrl":     chatURL,
		"shareUrl":    shareURL,
		"isConnected": channel.Status == "connected",
	}
}
