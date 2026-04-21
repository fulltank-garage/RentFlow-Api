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
		ShopName   string `json:"shopName"`
		DomainSlug string `json:"domainSlug"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลร้านไม่ถูกต้อง")
		return
	}

	shopName := strings.TrimSpace(payload.ShopName)
	domainSlug := rentFlowNormalizeDomainSlug(payload.DomainSlug)
	if len([]rune(shopName)) < 2 {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อร้านให้ถูกต้อง")
		return
	}
	if message := rentFlowValidateDomainSlug(domainSlug); message != "" {
		rentFlowError(c, http.StatusBadRequest, message)
		return
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
			ID:           services.NewID("tnt"),
			OwnerUserID:  &ownerUserID,
			OwnerEmail:   user.Email,
			ShopName:     shopName,
			DomainSlug:   domainSlug,
			PublicDomain: publicDomain,
			Status:       "active",
			Plan:         "starter",
		}
		if err := config.DB.Create(&tenant).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างร้านได้")
			return
		}
		rentFlowSuccess(c, http.StatusCreated, "บันทึกข้อมูลร้านสำเร็จ", rentFlowOwnerTenantResponse(tenant))
		return
	}

	updates := map[string]interface{}{
		"owner_user_id": ownerUserID,
		"owner_email":   user.Email,
		"shop_name":     shopName,
		"domain_slug":   domainSlug,
		"public_domain": publicDomain,
		"status":        "active",
		"updated_at":    time.Now(),
	}
	if existing.Plan == "" {
		updates["plan"] = "starter"
	}

	if err := config.DB.Model(&models.RentFlowTenant{}).
		Where("id = ?", existing.ID).
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
	if existing.Plan == "" {
		existing.Plan = "starter"
	}

	rentFlowSuccess(c, http.StatusOK, "บันทึกข้อมูลร้านสำเร็จ", rentFlowOwnerTenantResponse(existing))
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

func rentFlowCurrentUserTenant(c *gin.Context) (*models.RentFlowTenant, error) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}

	var tenant models.RentFlowTenant
	if err := config.DB.
		Where("owner_user_id = ? OR owner_email = ?", user.ID, user.Email).
		First(&tenant).Error; err != nil {
		return nil, err
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
	response := gin.H{
		"id":           tenant.ID,
		"shopName":     tenant.ShopName,
		"domainSlug":   tenant.DomainSlug,
		"publicDomain": tenant.PublicDomain,
		"status":       tenant.Status,
		"plan":         tenant.Plan,
		"createdAt":    tenant.CreatedAt,
		"updatedAt":    tenant.UpdatedAt,
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
