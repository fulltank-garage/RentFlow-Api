package controllers

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

var (
	rentFlowAiBudgetPattern = regexp.MustCompile(`(\d[\d,]*)\s*(?:บาท|baht)`)
	rentFlowAiPartyPattern  = regexp.MustCompile(`(\d+)\s*(?:คน|ที่นั่ง|seats?)`)
)

type rentFlowStorefrontAssistantPayload struct {
	Query string `json:"query"`
}

type rentFlowAssistantCriteria struct {
	RawQuery          string `json:"rawQuery"`
	CarType           string `json:"carType,omitempty"`
	MinSeats          int    `json:"minSeats,omitempty"`
	BudgetPerDay      int64  `json:"budgetPerDay,omitempty"`
	PrioritizeBudget  bool   `json:"prioritizeBudget,omitempty"`
	PrioritizeComfort bool   `json:"prioritizeComfort,omitempty"`
}

type rentFlowAssistantScoredCar struct {
	Car     models.RentFlowCar
	Tenant  models.RentFlowTenant
	Image   string
	Score   int
	Reasons []string
}

func RentFlowStorefrontAssistant(c *gin.Context) {
	var payload rentFlowStorefrontAssistantPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลคำถามไม่ถูกต้อง")
		return
	}

	query := strings.TrimSpace(payload.Query)
	if query == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกคำถามหรือรายละเอียดทริป")
		return
	}

	marketplace := rentFlowIsMarketplaceRequest(c)
	var tenants []models.RentFlowTenant
	if marketplace {
		items, err := rentFlowMarketplaceTenants()
		if err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเตรียมข้อมูลร้านได้")
			return
		}
		tenants = items
	} else {
		tenant, ok := rentFlowRequireTenant(c)
		if !ok {
			return
		}
		tenants = []models.RentFlowTenant{*tenant}
	}

	if len(tenants) == 0 {
		rentFlowSuccess(c, http.StatusOK, "ยังไม่มีข้อมูลร้านสำหรับผู้ช่วยเลือก", gin.H{
			"provider":        "database-rules",
			"mode":            rentFlowAssistantModeLabel(marketplace),
			"summary":         "ยังไม่มีร้านหรือรถที่พร้อมใช้งานในระบบตอนนี้",
			"criteria":        rentFlowAssistantCriteria{RawQuery: query},
			"quickHints":      []string{"เพิ่มข้อมูลรถและสาขาในระบบก่อน แล้วค่อยลองถามใหม่"},
			"recommendedCars": []gin.H{},
			"generatedAt":     time.Now(),
		})
		return
	}

	tenantMap := rentFlowTenantMap(tenants)
	tenantIDs := rentFlowAssistantTenantIDs(tenants)

	var cars []models.RentFlowCar
	if err := config.DB.
		Where("tenant_id IN ? AND is_available = ?", tenantIDs, true).
		Where("status <> ?", "archived").
		Order("price_per_day ASC, created_at DESC").
		Find(&cars).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรถสำหรับผู้ช่วยเลือกได้")
		return
	}

	criteria := rentFlowAssistantCriteriaFromQuery(query)
	imageURLs, err := rentFlowCarImageURLsForTenants(c, tenantMap, cars)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรูปภาพรถสำหรับผู้ช่วยเลือกได้")
		return
	}

	recommendations, matched := rentFlowAssistantRecommendCars(cars, tenantMap, imageURLs, criteria)
	if len(recommendations) == 0 {
		recommendations = rentFlowAssistantFallbackCars(cars, tenantMap, imageURLs)
	}

	provider := "database-rules"
	summary := rentFlowAssistantStorefrontSummary(marketplace, criteria, recommendations, matched)
	if generated, err := rentFlowAIStorefrontSummary(c, marketplace, criteria, recommendations); err == nil && generated != "" {
		summary = generated
		provider = services.RentFlowAIProviderLabel()
	}

	rentFlowSuccess(c, http.StatusOK, "สร้างคำแนะนำสำหรับลูกค้าสำเร็จ", gin.H{
		"provider":        provider,
		"mode":            rentFlowAssistantModeLabel(marketplace),
		"summary":         summary,
		"criteria":        criteria,
		"quickHints":      rentFlowAssistantQuickHints(marketplace, criteria, len(recommendations), matched),
		"recommendedCars": recommendations,
		"generatedAt":     time.Now(),
	})
}

func RentFlowPartnerAssistant(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var cars []models.RentFlowCar
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&cars).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรถสำหรับ AI ได้")
		return
	}

	var bookings []models.RentFlowBooking
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&bookings).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการจองสำหรับ AI ได้")
		return
	}

	var reviews []models.RentFlowReview
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Limit(30).Find(&reviews).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรีวิวสำหรับ AI ได้")
		return
	}

	var payments []models.RentFlowPayment
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&payments).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการชำระเงินสำหรับ AI ได้")
		return
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	recentThreshold := now.AddDate(0, 0, -30)

	availableCars := 0
	for _, car := range cars {
		if car.IsAvailable && strings.ToLower(strings.TrimSpace(car.Status)) != "maintenance" {
			availableCars++
		}
	}

	bookingsThisMonth := 0
	pendingBookings := 0
	bookingCountByCar := map[string]int{}
	revenueByCar := map[string]int64{}
	for _, booking := range bookings {
		bookingCountByCar[booking.CarID]++
		revenueByCar[booking.CarID] += booking.TotalAmount
		if booking.CreatedAt.After(monthStart) {
			bookingsThisMonth++
		}
		if booking.Status == "pending" {
			pendingBookings++
		}
	}

	revenueThisMonth := int64(0)
	pendingPayments := 0
	unverifiedPayments := 0
	for _, payment := range payments {
		if payment.CreatedAt.After(monthStart) && payment.Status == "paid" {
			revenueThisMonth += payment.Amount
		}
		if payment.Status == "pending" {
			pendingPayments++
		}
		if payment.Status == "paid" && payment.VerifiedAt == nil {
			unverifiedPayments++
		}
	}

	averageRating := 0.0
	lowRatingCount := 0
	for _, review := range reviews {
		averageRating += float64(review.Rating)
		if review.Rating <= 3 {
			lowRatingCount++
		}
	}
	if len(reviews) > 0 {
		averageRating = averageRating / float64(len(reviews))
	}

	idleCars := 0
	for _, car := range cars {
		lastSeen := time.Time{}
		for _, booking := range bookings {
			if booking.CarID == car.ID && booking.CreatedAt.After(lastSeen) {
				lastSeen = booking.CreatedAt
			}
		}
		if lastSeen.IsZero() || lastSeen.Before(recentThreshold) {
			idleCars++
		}
	}

	carNameByID := make(map[string]string, len(cars))
	for _, car := range cars {
		carNameByID[car.ID] = car.Name
	}

	topCars := make([]gin.H, 0, len(bookingCountByCar))
	for carID, count := range bookingCountByCar {
		topCars = append(topCars, gin.H{
			"carId":         carID,
			"carName":       carNameByID[carID],
			"bookings":      count,
			"revenue":       revenueByCar[carID],
			"isHighlighted": count >= 2,
		})
	}
	sort.Slice(topCars, func(i, j int) bool {
		if topCars[i]["bookings"].(int) == topCars[j]["bookings"].(int) {
			return topCars[i]["revenue"].(int64) > topCars[j]["revenue"].(int64)
		}
		return topCars[i]["bookings"].(int) > topCars[j]["bookings"].(int)
	})
	if len(topCars) > 5 {
		topCars = topCars[:5]
	}

	alerts := make([]gin.H, 0)
	if pendingBookings > 0 {
		alerts = append(alerts, gin.H{"tone": "warning", "title": "มีการจองรอดำเนินการ", "detail": fmt.Sprintf("ตอนนี้มี %d รายการที่ยังรอการยืนยันจากร้าน", pendingBookings)})
	}
	if pendingPayments > 0 || unverifiedPayments > 0 {
		alerts = append(alerts, gin.H{"tone": "warning", "title": "มีการชำระเงินที่ต้องตรวจต่อ", "detail": fmt.Sprintf("พบ %d รายการรอชำระและ %d รายการที่ชำระแล้วแต่ยังไม่ยืนยัน", pendingPayments, unverifiedPayments)})
	}
	if len(reviews) > 0 && averageRating < 4 {
		alerts = append(alerts, gin.H{"tone": "danger", "title": "คะแนนรีวิวเฉลี่ยต่ำกว่าเป้าหมาย", "detail": fmt.Sprintf("คะแนนเฉลี่ยล่าสุดอยู่ที่ %.1f/5 และมี %d รีวิวที่ให้ 3 ดาวหรือต่ำกว่า", averageRating, lowRatingCount)})
	}
	if idleCars > 0 {
		alerts = append(alerts, gin.H{"tone": "info", "title": "มีรถที่ควรเร่งปล่อยเช่า", "detail": fmt.Sprintf("พบ %d คันที่ไม่มี booking ในช่วง 30 วันที่ผ่านมา", idleCars)})
	}
	if len(alerts) == 0 {
		alerts = append(alerts, gin.H{"tone": "success", "title": "ภาพรวมร้านนิ่งดี", "detail": "ตอนนี้ยังไม่พบประเด็นเร่งด่วนที่ต้องจัดการทันที"})
	}

	actions := make([]gin.H, 0)
	if idleCars > 0 {
		actions = append(actions, gin.H{"title": "ทำโปรกับรถที่ว่างนาน", "detail": "เลือก 1-2 คันที่ไม่มี booking ล่าสุดไปทำโปรหรือดันโพสต์ขายก่อนสุดสัปดาห์", "priority": "high"})
	}
	if pendingBookings > 0 {
		actions = append(actions, gin.H{"title": "ไล่ยืนยัน booking ที่ค้างอยู่", "detail": "จัดการรายการรอดำเนินการก่อนเพื่อไม่ให้ลูกค้าหลุดและรายได้ตกหล่น", "priority": "high"})
	}
	if len(reviews) > 0 {
		actions = append(actions, gin.H{"title": "อ่านและตอบรีวิวล่าสุด", "detail": "ใช้ feedback ล่าสุดเพื่อแก้ pain point และเพิ่มความมั่นใจให้ลูกค้ารายใหม่", "priority": "medium"})
	}
	if len(actions) == 0 {
		actions = append(actions, gin.H{"title": "ต่อยอดการขายจากรถขายดี", "detail": "ใช้รถที่มีการจองสูงสุดเป็นพระเอกของแคมเปญหรือคอนเทนต์รอบถัดไป", "priority": "medium"})
	}

	metrics := []gin.H{
		{"label": "รายได้เดือนนี้", "value": revenueThisMonth, "displayValue": rentFlowFormatTHB(revenueThisMonth), "detail": "คำนวณจากรายการชำระที่สถานะ paid", "tone": "success"},
		{"label": "booking เดือนนี้", "value": bookingsThisMonth, "displayValue": bookingsThisMonth, "detail": "รายการจองที่สร้างในเดือนปัจจุบัน", "tone": "info"},
		{"label": "รถพร้อมปล่อย", "value": availableCars, "displayValue": fmt.Sprintf("%d / %d", availableCars, len(cars)), "detail": "นับเฉพาะรถที่พร้อมใช้งานในตอนนี้", "tone": "default"},
		{"label": "คะแนนรีวิวเฉลี่ย", "value": averageRating, "displayValue": fmt.Sprintf("%.1f/5", averageRating), "detail": fmt.Sprintf("อิงจาก %d รีวิวล่าสุด", len(reviews)), "tone": map[bool]string{true: "success", false: "warning"}[averageRating >= 4 || len(reviews) == 0]},
	}

	summary := fmt.Sprintf("%s มีรถทั้งหมด %d คัน พร้อมปล่อยเช่า %d คัน มี booking ใหม่เดือนนี้ %d รายการ และทำรายได้แล้ว %s", tenant.ShopName, len(cars), availableCars, bookingsThisMonth, rentFlowFormatTHB(revenueThisMonth))
	if len(reviews) > 0 {
		summary += fmt.Sprintf(" คะแนนรีวิวเฉลี่ยล่าสุดอยู่ที่ %.1f/5", averageRating)
	}

	provider := "database-rules"
	if generated, err := rentFlowAIPartnerSummary(c, tenant.ShopName, metrics, alerts, actions, topCars); err == nil && generated != "" {
		summary = generated
		provider = services.RentFlowAIProviderLabel()
	}

	rentFlowSuccess(c, http.StatusOK, "สร้างภาพรวม AI สำหรับร้านสำเร็จ", gin.H{
		"provider":    provider,
		"summary":     summary,
		"metrics":     metrics,
		"alerts":      alerts,
		"actions":     actions,
		"topCars":     topCars,
		"generatedAt": now,
	})
}

func RentFlowAdminAssistant(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	tenantItems, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูล tenant สำหรับ AI ได้")
		return
	}

	domainItems, err := rentFlowPlatformDomainItems(tenantItems)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลโดเมนสำหรับ AI ได้")
		return
	}

	summaryData, err := rentFlowPlatformSummary(tenantItems, domainItems)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสรุปข้อมูล platform สำหรับ AI ได้")
		return
	}

	metrics := []gin.H{
		{"label": "tenant ทั้งหมด", "displayValue": summaryData["totalTenants"], "detail": "จำนวนร้านทั้งหมดในระบบ", "tone": "default"},
		{"label": "tenant ที่ active", "displayValue": summaryData["activeTenants"], "detail": "ร้านที่พร้อมใช้งานอยู่ตอนนี้", "tone": "success"},
		{"label": "โดเมนต้องดูแล", "displayValue": summaryData["domainsNeedingCare"], "detail": "โดเมนที่ยังไม่ verified หรือยังมีงานค้าง", "tone": "warning"},
		{"label": "รายได้เดือนนี้", "displayValue": rentFlowFormatTHB(summaryData["revenueThisMonth"].(int64)), "detail": "รวมทุก tenant ในระบบ", "tone": "info"},
	}

	growthTenants := make([]rentFlowPlatformTenantItem, len(tenantItems))
	copy(growthTenants, tenantItems)
	sort.Slice(growthTenants, func(i, j int) bool {
		if growthTenants[i].RevenueThisMonth == growthTenants[j].RevenueThisMonth {
			return growthTenants[i].BookingsThisMonth > growthTenants[j].BookingsThisMonth
		}
		return growthTenants[i].RevenueThisMonth > growthTenants[j].RevenueThisMonth
	})
	if len(growthTenants) > 5 {
		growthTenants = growthTenants[:5]
	}

	riskTenants := make([]gin.H, 0)
	for _, item := range tenantItems {
		reasons := make([]string, 0)
		if item.Status == "pending" {
			reasons = append(reasons, "ร้านยังอยู่ในสถานะรอตรวจสอบ")
		}
		if item.Status == "suspended" {
			reasons = append(reasons, "ร้านถูกระงับและควรตรวจสาเหตุ")
		}
		if item.Cars == 0 {
			reasons = append(reasons, "ยังไม่มีรถที่พร้อมขายในระบบ")
		}
		if item.BookingsThisMonth == 0 {
			reasons = append(reasons, "เดือนนี้ยังไม่มี booking ใหม่")
		}
		if len(reasons) == 0 {
			continue
		}
		riskTenants = append(riskTenants, gin.H{
			"tenantId":     item.ID,
			"shopName":     item.ShopName,
			"publicDomain": item.PublicDomain,
			"status":       item.Status,
			"reason":       strings.Join(reasons, " • "),
		})
	}
	if len(riskTenants) > 6 {
		riskTenants = riskTenants[:6]
	}

	alerts := make([]gin.H, 0)
	if summaryData["pendingTenants"].(int) > 0 {
		alerts = append(alerts, gin.H{"tone": "warning", "title": "มี tenant ที่รออนุมัติ", "detail": fmt.Sprintf("ตอนนี้มี %d ร้านที่ยังอยู่ในสถานะ pending", summaryData["pendingTenants"].(int))})
	}
	if summaryData["suspendedTenants"].(int) > 0 {
		alerts = append(alerts, gin.H{"tone": "danger", "title": "มี tenant ที่ถูกระงับ", "detail": fmt.Sprintf("พบ %d ร้านที่ต้องติดตามเหตุผลการระงับ", summaryData["suspendedTenants"].(int))})
	}
	if summaryData["domainsNeedingCare"].(int) > 0 {
		alerts = append(alerts, gin.H{"tone": "info", "title": "ยังมีโดเมนที่ต้องตรวจต่อ", "detail": fmt.Sprintf("พบ %d โดเมนที่ยังไม่พร้อมใช้งานเต็มรูปแบบ", summaryData["domainsNeedingCare"].(int))})
	}
	if len(alerts) == 0 {
		alerts = append(alerts, gin.H{"tone": "success", "title": "แพลตฟอร์มอยู่ในสถานะปกติ", "detail": "ตอนนี้ยังไม่พบสัญญาณผิดปกติที่ต้องจัดการทันที"})
	}

	actions := []gin.H{
		{"title": "เร่งปิดงาน tenant ที่ pending", "detail": "ช่วยเจ้าของร้านตั้งค่าโดเมนและเปิด storefront ให้เร็วที่สุดเพื่อไม่ให้ onboarding ค้าง", "priority": "high"},
		{"title": "ตามร้านที่ยอดนิ่ง", "detail": "ใช้หน้า risk tenants เพื่อเข้าไปดูร้านที่ยังไม่มี booking หรือยังไม่มีรถในระบบ", "priority": "medium"},
		{"title": "ดัน best-practice จากร้านที่โตดี", "detail": "หยิบ pattern ของร้านที่รายได้สูงสุดไปทำ onboarding หรือ playbook สำหรับร้านใหม่", "priority": "medium"},
	}

	summary := fmt.Sprintf("ตอนนี้ RentFlow มี tenant ทั้งหมด %d ร้าน เปิดใช้งานอยู่ %d ร้าน และสร้างรายได้รวมเดือนนี้ %s", summaryData["totalTenants"].(int), summaryData["activeTenants"].(int), rentFlowFormatTHB(summaryData["revenueThisMonth"].(int64)))

	provider := "database-rules"
	if generated, err := rentFlowAIAdminSummary(c, summaryData, alerts, actions, growthTenants, riskTenants); err == nil && generated != "" {
		summary = generated
		provider = services.RentFlowAIProviderLabel()
	}

	rentFlowSuccess(c, http.StatusOK, "สร้างภาพรวม AI สำหรับ platform สำเร็จ", gin.H{
		"provider":      provider,
		"summary":       summary,
		"metrics":       metrics,
		"alerts":        alerts,
		"actions":       actions,
		"growthTenants": growthTenants,
		"riskTenants":   riskTenants,
		"generatedAt":   time.Now(),
	})
}

func rentFlowAIStorefrontSummary(c *gin.Context, marketplace bool, criteria rentFlowAssistantCriteria, recommendations []gin.H) (string, error) {
	if !services.RentFlowAIEnabled() || len(recommendations) == 0 {
		return "", fmt.Errorf("ai disabled")
	}

	systemPrompt := "คุณคือผู้ช่วยเลือกรถเช่าของ RentFlow ตอบเป็นภาษาไทยแบบกระชับ มืออาชีพ และไม่ใช้ markdown"
	userPrompt := fmt.Sprintf(
		"โหมด: %s\nคำถามลูกค้า: %s\nเงื่อนไขที่ระบบตีความได้: %+v\nรถที่แนะนำ: %+v\n\nช่วยสรุปเป็น 2-3 ประโยคว่าเพราะอะไรตัวเลือกเหล่านี้ถึงเหมาะกับลูกค้า โดยอิงเฉพาะข้อมูลที่ให้มา ห้ามแต่งข้อมูลเพิ่ม",
		rentFlowAssistantModeLabel(marketplace),
		criteria.RawQuery,
		criteria,
		recommendations,
	)

	return services.RentFlowAIGenerateText(c.Request.Context(), systemPrompt, userPrompt)
}

func rentFlowAIPartnerSummary(c *gin.Context, shopName string, metrics, alerts, actions, topCars []gin.H) (string, error) {
	if !services.RentFlowAIEnabled() {
		return "", fmt.Errorf("ai disabled")
	}

	systemPrompt := "คุณคือ AI ที่ช่วยสรุปธุรกิจร้านรถเช่า ตอบภาษาไทยให้เจ้าของร้านอ่านเร็ว ชัด และเน้นสิ่งที่ควรลงมือทำ"
	userPrompt := fmt.Sprintf(
		"ร้าน: %s\nmetrics: %+v\nalerts: %+v\nactions: %+v\ntopCars: %+v\n\nช่วยสรุป 3-4 ประโยคว่า วันนี้ร้านควรโฟกัสอะไร จุดแข็งคืออะไร และเรื่องไหนควรทำก่อน ห้ามสร้างข้อมูลนอกเหนือจากนี้",
		shopName,
		metrics,
		alerts,
		actions,
		topCars,
	)

	return services.RentFlowAIGenerateText(c.Request.Context(), systemPrompt, userPrompt)
}

func rentFlowAIAdminSummary(c *gin.Context, summaryData gin.H, alerts, actions []gin.H, growthTenants []rentFlowPlatformTenantItem, riskTenants []gin.H) (string, error) {
	if !services.RentFlowAIEnabled() {
		return "", fmt.Errorf("ai disabled")
	}

	systemPrompt := "คุณคือ AI analyst ของแพลตฟอร์ม RentFlow ตอบภาษาไทยแบบผู้บริหารอ่านเร็ว กระชับ และชี้ประเด็นสำคัญ"
	userPrompt := fmt.Sprintf(
		"platformSummary: %+v\nalerts: %+v\nactions: %+v\ngrowthTenants: %+v\nriskTenants: %+v\n\nช่วยสรุป 3-4 ประโยคว่า ภาพรวมแพลตฟอร์มตอนนี้เป็นอย่างไร ร้านกลุ่มไหนน่าสนใจ ร้านกลุ่มไหนเสี่ยง และทีม platform ควรทำอะไรก่อน ห้ามแต่งข้อมูลนอกเหนือจากนี้",
		summaryData,
		alerts,
		actions,
		growthTenants,
		riskTenants,
	)

	return services.RentFlowAIGenerateText(c.Request.Context(), systemPrompt, userPrompt)
}

func rentFlowAssistantModeLabel(marketplace bool) string {
	if marketplace {
		return "marketplace"
	}
	return "storefront"
}

func rentFlowAssistantTenantIDs(tenants []models.RentFlowTenant) []string {
	ids := make([]string, 0, len(tenants))
	for _, tenant := range tenants {
		ids = append(ids, tenant.ID)
	}
	return ids
}

func rentFlowAssistantCriteriaFromQuery(query string) rentFlowAssistantCriteria {
	criteria := rentFlowAssistantCriteria{RawQuery: strings.TrimSpace(query)}
	lower := strings.ToLower(query)

	switch {
	case strings.Contains(lower, "van") || strings.Contains(lower, "รถตู้") || strings.Contains(lower, "หลายคน"):
		criteria.CarType = "Van"
	case strings.Contains(lower, "suv") || strings.Contains(lower, "เอสยูวี") || strings.Contains(lower, "ขึ้นดอย"):
		criteria.CarType = "SUV"
	case strings.Contains(lower, "sedan") || strings.Contains(lower, "ซีดาน"):
		criteria.CarType = "Sedan"
	case strings.Contains(lower, "economy") || strings.Contains(lower, "eco") || strings.Contains(lower, "ประหยัด"):
		criteria.CarType = "Economy"
	}

	if matches := rentFlowAiPartyPattern.FindStringSubmatch(lower); len(matches) == 2 {
		if value, err := strconv.Atoi(strings.TrimSpace(matches[1])); err == nil {
			criteria.MinSeats = value
		}
	}

	if matches := rentFlowAiBudgetPattern.FindStringSubmatch(lower); len(matches) == 2 {
		clean := strings.ReplaceAll(matches[1], ",", "")
		if value, err := strconv.ParseInt(clean, 10, 64); err == nil {
			criteria.BudgetPerDay = value
		}
	}

	if strings.Contains(lower, "ประหยัด") || strings.Contains(lower, "คุ้ม") || strings.Contains(lower, "budget") || strings.Contains(lower, "ถูก") {
		criteria.PrioritizeBudget = true
	}
	if strings.Contains(lower, "ครอบครัว") || strings.Contains(lower, "เดินทางไกล") || strings.Contains(lower, "นั่งสบาย") || strings.Contains(lower, "สัมภาระ") || strings.Contains(lower, "ของเยอะ") {
		criteria.PrioritizeComfort = true
	}

	return criteria
}

func rentFlowAssistantRecommendCars(cars []models.RentFlowCar, tenantMap map[string]models.RentFlowTenant, imageURLs map[string][]string, criteria rentFlowAssistantCriteria) ([]gin.H, bool) {
	scored := make([]rentFlowAssistantScoredCar, 0, len(cars))
	matched := false
	lowerQuery := strings.ToLower(criteria.RawQuery)

	for _, car := range cars {
		score := 1
		reasons := make([]string, 0, 4)

		if criteria.CarType != "" {
			if strings.EqualFold(car.Type, criteria.CarType) {
				score += 5
				reasons = append(reasons, "ตรงกับประเภทรถที่ถาม")
				matched = true
			} else {
				continue
			}
		}

		if criteria.MinSeats > 0 {
			if car.Seats >= criteria.MinSeats {
				score += 3
				reasons = append(reasons, fmt.Sprintf("รองรับอย่างน้อย %d คน", criteria.MinSeats))
				matched = true
			} else {
				continue
			}
		}

		if criteria.BudgetPerDay > 0 {
			if car.PricePerDay <= criteria.BudgetPerDay {
				score += 3
				reasons = append(reasons, "อยู่ในงบต่อวัน")
				matched = true
			} else {
				score -= 2
			}
		}

		searchable := strings.ToLower(strings.Join([]string{car.Name, car.Brand, car.Model}, " "))
		if lowerQuery != "" && strings.Contains(searchable, lowerQuery) {
			score += 2
			reasons = append(reasons, "ชื่อรถหรือรุ่นตรงกับคำค้นหา")
		}

		if criteria.PrioritizeBudget {
			score += rentFlowAssistantBudgetBonus(car.PricePerDay)
			if !containsString(reasons, "ราคาต่อวันเหมาะกับสายคุ้มค่า") {
				reasons = append(reasons, "ราคาต่อวันเหมาะกับสายคุ้มค่า")
			}
		}
		if criteria.PrioritizeComfort && (car.Type == "SUV" || car.Type == "Van" || car.Seats >= 5) {
			score += 2
			reasons = append(reasons, "เหมาะกับทริปที่ต้องการพื้นที่และความสบาย")
		}

		images := imageURLs[car.ID]
		image := ""
		if len(images) > 0 {
			image = images[0]
		}

		scored = append(scored, rentFlowAssistantScoredCar{
			Car:     car,
			Tenant:  tenantMap[car.TenantID],
			Image:   image,
			Score:   score,
			Reasons: servicesUniqueStrings(reasons),
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Car.PricePerDay < scored[j].Car.PricePerDay
		}
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > 4 {
		scored = scored[:4]
	}

	result := make([]gin.H, 0, len(scored))
	for _, item := range scored {
		result = append(result, gin.H{
			"id":           item.Car.ID,
			"name":         item.Car.Name,
			"brand":        item.Car.Brand,
			"model":        item.Car.Model,
			"type":         item.Car.Type,
			"seats":        item.Car.Seats,
			"transmission": item.Car.Transmission,
			"fuel":         item.Car.Fuel,
			"pricePerDay":  item.Car.PricePerDay,
			"image":        item.Image,
			"shopName":     item.Tenant.ShopName,
			"domainSlug":   item.Tenant.DomainSlug,
			"publicDomain": item.Tenant.PublicDomain,
			"reasons":      item.Reasons,
		})
	}

	return result, matched
}

func rentFlowAssistantFallbackCars(cars []models.RentFlowCar, tenantMap map[string]models.RentFlowTenant, imageURLs map[string][]string) []gin.H {
	items := make([]models.RentFlowCar, len(cars))
	copy(items, cars)
	sort.Slice(items, func(i, j int) bool {
		return items[i].PricePerDay < items[j].PricePerDay
	})
	if len(items) > 4 {
		items = items[:4]
	}

	result := make([]gin.H, 0, len(items))
	for _, car := range items {
		images := imageURLs[car.ID]
		image := ""
		if len(images) > 0 {
			image = images[0]
		}
		tenant := tenantMap[car.TenantID]
		result = append(result, gin.H{
			"id":           car.ID,
			"name":         car.Name,
			"brand":        car.Brand,
			"model":        car.Model,
			"type":         car.Type,
			"seats":        car.Seats,
			"transmission": car.Transmission,
			"fuel":         car.Fuel,
			"pricePerDay":  car.PricePerDay,
			"image":        image,
			"shopName":     tenant.ShopName,
			"domainSlug":   tenant.DomainSlug,
			"publicDomain": tenant.PublicDomain,
			"reasons":      []string{"ยังไม่เจอรถที่ตรงเงื่อนไขแบบเป๊ะ เลยคัดตัวเลือกที่ราคาน่าเริ่มดูที่สุดให้ก่อน"},
		})
	}
	return result
}

func rentFlowAssistantBudgetBonus(pricePerDay int64) int {
	switch {
	case pricePerDay <= 1200:
		return 3
	case pricePerDay <= 2000:
		return 2
	case pricePerDay <= 3000:
		return 1
	default:
		return 0
	}
}

func rentFlowAssistantStorefrontSummary(marketplace bool, criteria rentFlowAssistantCriteria, recommendations []gin.H, matched bool) string {
	if len(recommendations) == 0 {
		return "ตอนนี้ยังไม่มีรถที่พร้อมใช้งานในระบบสำหรับช่วยแนะนำ"
	}

	if !matched {
		if marketplace {
			return "ยังไม่เจอรถที่ตรงเงื่อนไขแบบเป๊ะจากหลายร้าน ผมเลยคัดตัวเลือกเริ่มต้นที่ราคาน่าเริ่มดูและพร้อมใช้งานให้ก่อน"
		}
		return "ยังไม่เจอรถที่ตรงเงื่อนไขแบบเป๊ะในร้านนี้ ผมเลยหยิบตัวเลือกที่น่าเริ่มดูที่สุดให้ก่อน"
	}

	parts := make([]string, 0, 3)
	if criteria.CarType != "" {
		parts = append(parts, criteria.CarType)
	}
	if criteria.MinSeats > 0 {
		parts = append(parts, fmt.Sprintf("รองรับ %d คนขึ้นไป", criteria.MinSeats))
	}
	if criteria.BudgetPerDay > 0 {
		parts = append(parts, fmt.Sprintf("งบไม่เกิน %s/วัน", rentFlowFormatTHB(criteria.BudgetPerDay)))
	}

	scope := "ร้านนี้"
	if marketplace {
		scope = "หลายร้านใน marketplace"
	}
	if len(parts) == 0 {
		return fmt.Sprintf("ผมคัดรถที่น่าเริ่มดูจาก%sให้แล้ว %d คัน โดยเน้นรถที่พร้อมใช้งานและราคาไล่ดูง่าย", scope, len(recommendations))
	}
	return fmt.Sprintf("จากเงื่อนไข %s ผมคัดรถจาก%sให้ %d คันที่ใกล้เคียงที่สุด", strings.Join(parts, " • "), scope, len(recommendations))
}

func rentFlowAssistantQuickHints(marketplace bool, criteria rentFlowAssistantCriteria, recommendationCount int, matched bool) []string {
	hints := make([]string, 0, 4)
	if marketplace {
		hints = append(hints, "ผลลัพธ์ของ marketplace จะแสดงชื่อร้านกำกับทุกคันเพื่อช่วยเทียบราคาและสไตล์บริการ")
	} else {
		hints = append(hints, "ผลลัพธ์ใน storefront นี้จะโฟกัสเฉพาะรถของร้านเดียวเพื่อให้ตัดสินใจได้ไว")
	}
	if criteria.BudgetPerDay == 0 {
		hints = append(hints, "ถ้าระบุงบต่อวัน ระบบจะช่วยคัดรถได้แคบและตรงขึ้นมาก")
	}
	if criteria.MinSeats == 0 {
		hints = append(hints, "บอกจำนวนผู้โดยสารหรือจำนวนที่นั่ง จะช่วยให้ AI เลือกรถได้แม่นขึ้น")
	}
	if !matched && recommendationCount > 0 {
		hints = append(hints, "ถ้ายังไม่เจอแบบที่ใช่ ลองเปลี่ยนประเภทรถหรืองบประมาณอีกนิดแล้วถามใหม่ได้")
	}
	return hints
}

func rentFlowFormatTHB(value int64) string {
	formatted := strconv.FormatInt(value, 10)
	if value == 0 {
		return "0 บาท"
	}
	parts := make([]byte, 0, len(formatted)+len(formatted)/3)
	count := 0
	for i := len(formatted) - 1; i >= 0; i-- {
		parts = append(parts, formatted[i])
		count++
		if count%3 == 0 && i != 0 {
			parts = append(parts, ',')
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return string(parts) + " บาท"
}

func servicesUniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
