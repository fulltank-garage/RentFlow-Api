package controllers

import (
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rentflow-api/config"
	"rentflow-api/middleware"
	"rentflow-api/models"
	"rentflow-api/services"
)

type rentFlowPlatformTenantItem struct {
	ID                string    `json:"id"`
	ShopName          string    `json:"shopName"`
	OwnerName         string    `json:"ownerName"`
	OwnerEmail        string    `json:"ownerEmail"`
	DomainSlug        string    `json:"domainSlug"`
	PublicDomain      string    `json:"publicDomain"`
	Status            string    `json:"status"`
	BookingMode       string    `json:"bookingMode"`
	Plan              string    `json:"plan"`
	LifecycleReason   string    `json:"lifecycleReason,omitempty"`
	Cars              int       `json:"cars"`
	TotalBookings     int       `json:"totalBookings"`
	BookingsThisMonth int       `json:"bookingsThisMonth"`
	RevenueThisMonth  int64     `json:"revenueThisMonth"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type rentFlowPlatformDomainItem struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenantId"`
	ShopName      string     `json:"shopName"`
	OwnerEmail    string     `json:"ownerEmail"`
	OwnerName     string     `json:"ownerName"`
	Domain        string     `json:"domain"`
	Target        string     `json:"target"`
	Status        string     `json:"status"`
	Source        string     `json:"source"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
}

func RentFlowAdminGetMe(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	user, _ := middleware.CurrentRentFlowUser(c)
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลผู้ดูแลระบบสำเร็จ", gin.H{
		"user": gin.H{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"name":      user.Name,
			"firstName": user.FirstName,
			"lastName":  user.LastName,
		},
		"hosts": rentFlowPlatformHosts(),
	})
}

func RentFlowAdminGetOverview(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	tenantItems, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงภาพรวมระบบได้")
		return
	}

	domainItems, err := rentFlowPlatformDomainItems(tenantItems)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลโดเมนได้")
		return
	}

	summary, err := rentFlowPlatformSummary(tenantItems, domainItems)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสรุปข้อมูลระบบได้")
		return
	}

	recentTenants := tenantItems
	if len(recentTenants) > 6 {
		recentTenants = recentTenants[:6]
	}
	recentDomains := domainItems
	if len(recentDomains) > 6 {
		recentDomains = recentDomains[:6]
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงภาพรวมระบบสำเร็จ", gin.H{
		"hosts":         rentFlowPlatformHosts(),
		"summary":       summary,
		"recentTenants": recentTenants,
		"recentDomains": recentDomains,
	})
}

func RentFlowAdminListPartners(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	items, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลเจ้าของร้านได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลเจ้าของร้านสำเร็จ", gin.H{
		"items": items,
		"total": len(items),
	})
}

func RentFlowAdminCreatePartner(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	var payload struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		FirstName  string `json:"firstName"`
		LastName   string `json:"lastName"`
		Phone      string `json:"phone"`
		ShopName   string `json:"shopName"`
		DomainSlug string `json:"domainSlug"`
		Plan       string `json:"plan"`
		Status     string `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลเจ้าของร้านไม่ถูกต้อง")
		return
	}

	username := strings.TrimSpace(strings.ToLower(payload.Username))
	firstName := strings.TrimSpace(payload.FirstName)
	lastName := strings.TrimSpace(payload.LastName)
	phone := strings.TrimSpace(payload.Phone)
	shopName := strings.TrimSpace(payload.ShopName)
	domainSlug := rentFlowNormalizeDomainSlug(payload.DomainSlug)
	plan := rentFlowNormalizePlatformPartnerPlan(payload.Plan)
	status := rentFlowNormalizePlatformTenantStatus(payload.Status)
	fullName := strings.TrimSpace(firstName + " " + lastName)

	if len(username) < 3 || len(strings.TrimSpace(payload.Password)) < 8 || len(firstName) < 2 || len(lastName) < 2 || shopName == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกข้อมูลเจ้าของร้านให้ครบถ้วน")
		return
	}
	if message := rentFlowValidateDomainSlug(domainSlug); message != "" {
		rentFlowError(c, http.StatusBadRequest, message)
		return
	}

	var existingUser models.RentFlowUser
	userErr := config.DB.Where("username = ? OR email = ?", username, username).First(&existingUser).Error
	switch {
	case userErr == nil:
		rentFlowError(c, http.StatusConflict, "ชื่อผู้ใช้นี้ถูกใช้งานแล้ว")
		return
	case !errors.Is(userErr, gorm.ErrRecordNotFound):
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบชื่อผู้ใช้ได้")
		return
	}

	publicDomain := rentFlowPublicDomain(domainSlug)
	var existingTenant models.RentFlowTenant
	tenantErr := config.DB.Where("domain_slug = ? OR public_domain = ?", domainSlug, publicDomain).First(&existingTenant).Error
	switch {
	case tenantErr == nil:
		rentFlowError(c, http.StatusConflict, "โดเมนร้านนี้ถูกใช้งานแล้ว")
		return
	case !errors.Is(tenantErr, gorm.ErrRecordNotFound):
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบโดเมนร้านได้")
		return
	}

	passwordHash, err := services.HashPasswordIfNeeded(payload.Password)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างรหัสผ่านได้")
		return
	}

	userID := services.NewID("usr")
	tenantID := services.NewID("tnt")
	memberID := services.NewID("mbr")
	now := time.Now()

	user := models.RentFlowUser{
		ID:           userID,
		Username:     username,
		FirstName:    firstName,
		LastName:     lastName,
		Name:         fullName,
		Email:        username,
		Phone:        phone,
		PasswordHash: passwordHash,
	}
	tenant := models.RentFlowTenant{
		ID:           tenantID,
		OwnerUserID:  &userID,
		OwnerEmail:   username,
		ShopName:     shopName,
		DomainSlug:   domainSlug,
		PublicDomain: publicDomain,
		Status:       status,
		BookingMode:  "payment",
		Plan:         plan,
	}
	member := models.RentFlowTenantMember{
		ID:        memberID,
		TenantID:  tenantID,
		UserID:    userID,
		Email:     username,
		Name:      fullName,
		Role:      "owner",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := tx.Create(&tenant).Error; err != nil {
			return err
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างเจ้าของร้านได้")
		return
	}

	rentFlowAudit(c, tenant.ID, "platform.partner.create", "tenant", tenant.ID, shopName+"|"+username)
	rentFlowCreateNotification(tenant.ID, &user.ID, user.Email, "บัญชี Partner พร้อมใช้งานแล้ว", "คุณสามารถเข้าสู่ระบบ Partner Dashboard เพื่อจัดการร้านได้ทันที")

	rentFlowSuccess(c, http.StatusCreated, "สร้างเจ้าของร้านสำเร็จ", gin.H{
		"tenant": rentFlowPlatformTenantItem{
			ID:                tenant.ID,
			ShopName:          tenant.ShopName,
			OwnerName:         user.Name,
			OwnerEmail:        tenant.OwnerEmail,
			DomainSlug:        tenant.DomainSlug,
			PublicDomain:      tenant.PublicDomain,
			Status:            tenant.Status,
			BookingMode:       rentFlowNormalizeBookingMode(tenant.BookingMode),
			Plan:              tenant.Plan,
			Cars:              0,
			TotalBookings:     0,
			BookingsThisMonth: 0,
			RevenueThisMonth:  0,
			CreatedAt:         tenant.CreatedAt,
			UpdatedAt:         tenant.UpdatedAt,
		},
		"user": rentFlowUserResponse(user),
	})
}

func RentFlowAdminListDomains(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	tenantItems, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลโดเมนได้")
		return
	}

	items, err := rentFlowPlatformDomainItems(tenantItems)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลโดเมนได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลโดเมนสำเร็จ", gin.H{
		"hosts": rentFlowPlatformHosts(),
		"items": items,
		"total": len(items),
	})
}

func RentFlowAdminGetBilling(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	items, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรายได้ได้")
		return
	}

	plans := []string{"starter", "growth", "enterprise"}
	planItems := make([]gin.H, 0, len(plans))
	totalRevenue := int64(0)
	for _, item := range items {
		totalRevenue += item.RevenueThisMonth
	}
	for _, plan := range plans {
		count := 0
		revenue := int64(0)
		for _, item := range items {
			if strings.EqualFold(item.Plan, plan) {
				count++
				revenue += item.RevenueThisMonth
			}
		}
		planItems = append(planItems, gin.H{
			"plan":             plan,
			"count":            count,
			"revenueThisMonth": revenue,
		})
	}
	invoices, err := rentFlowPlatformEnsureInvoices(items)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเตรียมใบแจ้งหนี้ได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูล billing สำเร็จ", gin.H{
		"items":    items,
		"plans":    planItems,
		"invoices": invoices,
		"summary": gin.H{
			"totalTenants":      len(items),
			"revenueThisMonth":  totalRevenue,
			"activeTenantCount": rentFlowPlatformCountTenantsByStatus(items, "active"),
		},
	})
}

func RentFlowAdminGetSecurity(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	tenantItems, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลความปลอดภัยได้")
		return
	}

	var members []models.RentFlowTenantMember
	if err := config.DB.Where("status = ?", "active").Find(&members).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสมาชิกได้")
		return
	}
	memberItems := make([]gin.H, 0, len(members))
	for _, member := range members {
		memberItems = append(memberItems, gin.H{
			"id":       member.ID,
			"tenantId": member.TenantID,
			"userId":   member.UserID,
			"email":    member.Email,
			"name":     member.Name,
			"role":     member.Role,
			"status":   member.Status,
		})
	}

	var lineChannels []models.RentFlowLineChannel
	if err := config.DB.Where("status = ?", "connected").Find(&lineChannels).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูล LINE OA ได้")
		return
	}

	var customDomains []models.RentFlowCustomDomain
	if err := config.DB.Find(&customDomains).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลโดเมนได้")
		return
	}

	policies := []gin.H{
		{
			"title":  "Platform admin",
			"detail": "ใช้บัญชีผู้ดูแลระบบกลางเพียงบัญชีเดียวในการควบคุม tenant และสถานะระบบ",
			"status": map[bool]string{true: "configured", false: "missing"}[services.RentFlowPlatformAdminConfigured()],
		},
		{
			"title":  "Tenant isolation",
			"detail": "รถ การจอง การชำระเงิน รีวิว และ LINE OA ถูกแยกตาม tenant ทั้งหมด",
			"status": "active",
		},
		{
			"title":  "Store messaging",
			"detail": "ร้านที่เชื่อม LINE OA จะรับข้อความผ่าน webhook ของตัวเองและจัดการแยกตามร้าน",
			"status": map[bool]string{true: "active", false: "pending"}[len(lineChannels) > 0],
		},
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลความปลอดภัยสำเร็จ", gin.H{
		"summary": gin.H{
			"platformAdminConfigured": services.RentFlowPlatformAdminConfigured(),
			"tenantOwners":            len(tenantItems),
			"tenantMembers":           len(members),
			"connectedLineChannels":   len(lineChannels),
			"verifiedCustomDomains":   rentFlowCountCustomDomainStatus(customDomains, "verified"),
			"suspendedTenants":        rentFlowPlatformCountTenantsByStatus(tenantItems, "suspended"),
		},
		"policies": policies,
		"members":  memberItems,
	})
}

func RentFlowAdminGetAuditLogs(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}
	var logs []models.RentFlowAuditLog
	query := config.DB.Order("created_at DESC").Limit(200)
	if tenantID := strings.TrimSpace(c.Query("tenantId")); tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		query = query.Where("action = ?", action)
	}
	if err := query.Find(&logs).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึง audit log ได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึง audit log สำเร็จ", gin.H{"items": logs, "total": len(logs)})
}

func RentFlowAdminUpdateUserSecurity(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}
	var payload struct {
		Status        string `json:"status"`
		Reason        string `json:"reason"`
		RevokeSession bool   `json:"revokeSession"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลความปลอดภัยผู้ใช้ไม่ถูกต้อง")
		return
	}
	status := strings.TrimSpace(strings.ToLower(payload.Status))
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "locked" && status != "disabled" {
		rentFlowError(c, http.StatusBadRequest, "สถานะผู้ใช้ไม่ถูกต้อง")
		return
	}
	var user models.RentFlowUser
	if err := config.DB.Where("id = ? OR username = ? OR email = ?", c.Param("userId"), c.Param("userId"), c.Param("userId")).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบผู้ใช้")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาผู้ใช้ได้")
		return
	}
	now := time.Now()
	updates := map[string]interface{}{
		"status":        status,
		"locked_reason": strings.TrimSpace(payload.Reason),
		"updated_at":    now,
	}
	if status == "locked" || status == "disabled" {
		updates["locked_at"] = &now
	} else {
		updates["locked_at"] = nil
		updates["locked_reason"] = ""
	}
	if err := config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตผู้ใช้ได้")
		return
	}
	if payload.RevokeSession || status == "locked" || status == "disabled" {
		_ = services.DeleteUserSessions(config.Ctx, user.ID)
	}
	rentFlowAudit(c, "", "platform.user_security.update", "user", user.ID, status+"|"+strings.TrimSpace(payload.Reason))
	rentFlowSuccess(c, http.StatusOK, "อัปเดตความปลอดภัยผู้ใช้สำเร็จ", gin.H{"user": gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"email":        user.Email,
		"name":         user.Name,
		"status":       status,
		"lockedReason": strings.TrimSpace(payload.Reason),
	}})
}

func rentFlowPlatformSummary(tenantItems []rentFlowPlatformTenantItem, domainItems []rentFlowPlatformDomainItem) (gin.H, error) {
	totalRevenue := int64(0)
	for _, item := range tenantItems {
		totalRevenue += item.RevenueThisMonth
	}

	verifiedDomains := 0
	for _, item := range domainItems {
		if item.Status == "verified" {
			verifiedDomains++
		}
	}

	return gin.H{
		"totalTenants":       len(tenantItems),
		"activeTenants":      rentFlowPlatformCountTenantsByStatus(tenantItems, "active"),
		"pendingTenants":     rentFlowPlatformCountTenantsByStatus(tenantItems, "pending"),
		"suspendedTenants":   rentFlowPlatformCountTenantsByStatus(tenantItems, "suspended"),
		"verifiedDomains":    verifiedDomains,
		"domainsNeedingCare": len(domainItems) - verifiedDomains,
		"revenueThisMonth":   totalRevenue,
	}, nil
}

func rentFlowPlatformCountTenantsByStatus(items []rentFlowPlatformTenantItem, status string) int {
	total := 0
	for _, item := range items {
		if item.Status == status {
			total++
		}
	}
	return total
}

func rentFlowCountCustomDomainStatus(items []models.RentFlowCustomDomain, status string) int {
	total := 0
	for _, item := range items {
		if item.Status == status {
			total++
		}
	}
	return total
}

func rentFlowNormalizePlatformPartnerPlan(plan string) string {
	switch strings.TrimSpace(strings.ToLower(plan)) {
	case "growth", "enterprise":
		return strings.TrimSpace(strings.ToLower(plan))
	default:
		return "starter"
	}
}

func rentFlowNormalizePlatformTenantStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "active", "suspended", "rejected":
		return strings.TrimSpace(strings.ToLower(status))
	default:
		return "pending"
	}
}

func rentFlowPlatformEnsureInvoices(tenants []rentFlowPlatformTenantItem) ([]models.RentFlowPlatformInvoice, error) {
	period := time.Now().Format("2006-01")
	planPrices := map[string]int64{
		"starter":    0,
		"growth":     990,
		"enterprise": 2990,
	}
	for _, tenant := range tenants {
		amount := planPrices[rentFlowNormalizePlatformPartnerPlan(tenant.Plan)]
		var existing models.RentFlowPlatformInvoice
		err := config.DB.Where("tenant_id = ? AND period = ?", tenant.ID, period).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			now := time.Now()
			status := "draft"
			if amount == 0 {
				status = "paid"
			}
			invoice := models.RentFlowPlatformInvoice{
				ID:       services.NewID("inv"),
				TenantID: tenant.ID,
				Period:   period,
				Plan:     rentFlowNormalizePlatformPartnerPlan(tenant.Plan),
				Amount:   amount,
				Status:   status,
				IssuedAt: &now,
			}
			if status == "paid" {
				invoice.PaidAt = &now
			}
			if err := config.DB.Create(&invoice).Error; err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	var invoices []models.RentFlowPlatformInvoice
	if err := config.DB.Order("created_at DESC").Limit(200).Find(&invoices).Error; err != nil {
		return nil, err
	}
	return invoices, nil
}

func rentFlowPlatformHosts() gin.H {
	rootDomain := rentFlowRootDomain()
	target := strings.TrimSpace(os.Getenv("RENTFLOW_STOREFRONT_TARGET"))
	if target == "" {
		target = "storefront." + rootDomain
	}
	return gin.H{
		"admin":              "admin." + rootDomain,
		"partner":            "partner." + rootDomain,
		"wildcardStorefront": "*." + rootDomain,
		"cnameTarget":        target,
	}
}

func rentFlowPlatformTenantItems() ([]rentFlowPlatformTenantItem, error) {
	var tenants []models.RentFlowTenant
	if err := config.DB.Order("created_at DESC").Find(&tenants).Error; err != nil {
		return nil, err
	}
	if len(tenants) == 0 {
		return []rentFlowPlatformTenantItem{}, nil
	}

	tenantIDs := make([]string, 0, len(tenants))
	ownerUserIDs := make([]string, 0, len(tenants))
	ownerEmails := make([]string, 0, len(tenants))
	tenantByID := make(map[string]models.RentFlowTenant, len(tenants))
	for _, tenant := range tenants {
		tenantIDs = append(tenantIDs, tenant.ID)
		tenantByID[tenant.ID] = tenant
		if tenant.OwnerUserID != nil && strings.TrimSpace(*tenant.OwnerUserID) != "" {
			ownerUserIDs = append(ownerUserIDs, strings.TrimSpace(*tenant.OwnerUserID))
		}
		if email := strings.TrimSpace(strings.ToLower(tenant.OwnerEmail)); email != "" {
			ownerEmails = append(ownerEmails, email)
		}
	}

	ownerNamesByUserID := map[string]string{}
	ownerNamesByEmail := map[string]string{}
	if len(ownerUserIDs) > 0 || len(ownerEmails) > 0 {
		query := config.DB.Model(&models.RentFlowUser{})
		if len(ownerUserIDs) > 0 {
			query = query.Where("id IN ?", ownerUserIDs)
		}
		if len(ownerEmails) > 0 {
			if len(ownerUserIDs) > 0 {
				query = query.Or("LOWER(email) IN ?", ownerEmails)
			} else {
				query = query.Where("LOWER(email) IN ?", ownerEmails)
			}
		}
		var users []models.RentFlowUser
		if err := query.Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			if name := strings.TrimSpace(user.Name); name != "" {
				ownerNamesByUserID[user.ID] = name
				ownerNamesByEmail[strings.ToLower(user.Email)] = name
			}
		}
	}

	carCount := map[string]int{}
	var cars []models.RentFlowCar
	if err := config.DB.Where("tenant_id IN ?", tenantIDs).Find(&cars).Error; err != nil {
		return nil, err
	}
	for _, car := range cars {
		carCount[car.TenantID]++
	}

	totalBookings := map[string]int{}
	bookingsThisMonth := map[string]int{}
	var bookings []models.RentFlowBooking
	if err := config.DB.Where("tenant_id IN ?", tenantIDs).Find(&bookings).Error; err != nil {
		return nil, err
	}
	monthStart := time.Now().In(time.Local)
	monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
	for _, booking := range bookings {
		totalBookings[booking.TenantID]++
		if !booking.CreatedAt.Before(monthStart) {
			bookingsThisMonth[booking.TenantID]++
		}
	}

	revenueThisMonth := map[string]int64{}
	var payments []models.RentFlowPayment
	if err := config.DB.Where("tenant_id IN ? AND status = ?", tenantIDs, "paid").Find(&payments).Error; err != nil {
		return nil, err
	}
	for _, payment := range payments {
		if !payment.CreatedAt.Before(monthStart) {
			revenueThisMonth[payment.TenantID] += payment.Amount
		}
	}

	items := make([]rentFlowPlatformTenantItem, 0, len(tenants))
	for _, tenant := range tenants {
		ownerName := strings.TrimSpace(ownerNamesByEmail[strings.ToLower(tenant.OwnerEmail)])
		if ownerName == "" && tenant.OwnerUserID != nil {
			ownerName = strings.TrimSpace(ownerNamesByUserID[*tenant.OwnerUserID])
		}
		if ownerName == "" {
			ownerName = strings.TrimSpace(tenant.OwnerEmail)
		}

		items = append(items, rentFlowPlatformTenantItem{
			ID:                tenant.ID,
			ShopName:          tenant.ShopName,
			OwnerName:         ownerName,
			OwnerEmail:        tenant.OwnerEmail,
			DomainSlug:        tenant.DomainSlug,
			PublicDomain:      tenant.PublicDomain,
			Status:            tenant.Status,
			BookingMode:       rentFlowNormalizeBookingMode(tenant.BookingMode),
			Plan:              tenant.Plan,
			LifecycleReason:   tenant.LifecycleReason,
			Cars:              carCount[tenant.ID],
			TotalBookings:     totalBookings[tenant.ID],
			BookingsThisMonth: bookingsThisMonth[tenant.ID],
			RevenueThisMonth:  revenueThisMonth[tenant.ID],
			CreatedAt:         tenant.CreatedAt,
			UpdatedAt:         tenant.UpdatedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func rentFlowPlatformDomainItems(tenantItems []rentFlowPlatformTenantItem) ([]rentFlowPlatformDomainItem, error) {
	items := make([]rentFlowPlatformDomainItem, 0, len(tenantItems))
	for _, tenant := range tenantItems {
		status := "verified"
		switch tenant.Status {
		case "pending":
			status = "pending_dns"
		case "suspended":
			status = "suspended"
		}
		lastCheckedAt := tenant.UpdatedAt
		items = append(items, rentFlowPlatformDomainItem{
			ID:            "sub_" + tenant.ID,
			TenantID:      tenant.ID,
			ShopName:      tenant.ShopName,
			OwnerEmail:    tenant.OwnerEmail,
			OwnerName:     tenant.OwnerName,
			Domain:        tenant.PublicDomain,
			Target:        rentFlowPlatformHosts()["cnameTarget"].(string),
			Status:        status,
			Source:        "subdomain",
			LastCheckedAt: &lastCheckedAt,
		})
	}

	var customDomains []models.RentFlowCustomDomain
	if err := config.DB.Order("created_at DESC").Find(&customDomains).Error; err != nil {
		return nil, err
	}
	tenantMap := make(map[string]rentFlowPlatformTenantItem, len(tenantItems))
	for _, item := range tenantItems {
		tenantMap[item.ID] = item
	}
	for _, domain := range customDomains {
		tenant := tenantMap[domain.TenantID]
		lastCheckedAt := domain.UpdatedAt
		if domain.VerifiedAt != nil {
			lastCheckedAt = *domain.VerifiedAt
		}
		items = append(items, rentFlowPlatformDomainItem{
			ID:            domain.ID,
			TenantID:      domain.TenantID,
			ShopName:      tenant.ShopName,
			OwnerEmail:    tenant.OwnerEmail,
			OwnerName:     tenant.OwnerName,
			Domain:        domain.Domain,
			Target:        rentFlowPlatformHosts()["cnameTarget"].(string),
			Status:        domain.Status,
			Source:        "custom",
			LastCheckedAt: &lastCheckedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		left := time.Time{}
		right := time.Time{}
		if items[i].LastCheckedAt != nil {
			left = *items[i].LastCheckedAt
		}
		if items[j].LastCheckedAt != nil {
			right = *items[j].LastCheckedAt
		}
		return left.After(right)
	})
	return items, nil
}
