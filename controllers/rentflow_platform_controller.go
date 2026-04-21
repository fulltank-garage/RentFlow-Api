package controllers

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/middleware"
	"rentflow-api/models"
)

type rentFlowPlatformTenantItem struct {
	ID                string    `json:"id"`
	ShopName          string    `json:"shopName"`
	OwnerName         string    `json:"ownerName"`
	OwnerEmail        string    `json:"ownerEmail"`
	DomainSlug        string    `json:"domainSlug"`
	PublicDomain      string    `json:"publicDomain"`
	Status            string    `json:"status"`
	Plan              string    `json:"plan"`
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

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูล billing สำเร็จ", gin.H{
		"items": items,
		"plans": planItems,
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

	adminEmail := strings.TrimSpace(strings.ToLower(os.Getenv("RENTFLOW_SUPER_ADMIN_EMAIL")))
	policies := []gin.H{
		{
			"title":  "Platform admin",
			"detail": "ใช้บัญชีผู้ดูแลระบบกลางเพียงบัญชีเดียวในการควบคุม tenant และสถานะระบบ",
			"status": map[bool]string{true: "configured", false: "missing"}[adminEmail != ""],
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
			"platformAdminConfigured": adminEmail != "",
			"tenantOwners":            len(tenantItems),
			"tenantMembers":           len(members),
			"connectedLineChannels":   len(lineChannels),
			"verifiedCustomDomains":   rentFlowCountCustomDomainStatus(customDomains, "verified"),
			"suspendedTenants":        rentFlowPlatformCountTenantsByStatus(tenantItems, "suspended"),
		},
		"policies": policies,
	})
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
			Plan:              tenant.Plan,
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
