package controllers

import (
	"errors"
	"net/http"
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

func RentFlowPartnerGetBookings(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var bookings []models.RentFlowBooking
	query := config.DB.Where("tenant_id = ?", tenant.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("created_at DESC").Find(&bookings).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการจองได้")
		return
	}

	carNames := rentFlowPartnerCarNames(tenant.ID)
	items := make([]gin.H, 0, len(bookings))
	for _, booking := range bookings {
		items = append(items, rentFlowPartnerBookingResponse(booking, carNames[booking.CarID]))
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลการจองสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowPartnerUpdateBookingStatus(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสถานะการจองไม่ถูกต้อง")
		return
	}
	status := rentFlowNormalizeBookingStatus(payload.Status)
	if status == "" {
		rentFlowError(c, http.StatusBadRequest, "สถานะการจองไม่ถูกต้อง")
		return
	}

	var booking models.RentFlowBooking
	if err := config.DB.Where("tenant_id = ? AND (id = ? OR booking_code = ?)", tenant.ID, c.Param("bookingId"), c.Param("bookingId")).First(&booking).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรายการจอง")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาการจองได้")
		return
	}

	updates := map[string]interface{}{"status": status, "updated_at": time.Now()}
	if strings.TrimSpace(payload.Note) != "" {
		updates["note"] = strings.TrimSpace(payload.Note)
	}
	if err := config.DB.Model(&models.RentFlowBooking{}).Where("tenant_id = ? AND id = ?", tenant.ID, booking.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตการจองได้")
		return
	}
	booking.Status = status
	if note := strings.TrimSpace(payload.Note); note != "" {
		booking.Note = note
	}
	rentFlowAudit(c, tenant.ID, "booking.update_status", "booking", booking.ID, "status="+status)
	rentFlowCreateNotification(tenant.ID, booking.UserID, booking.CustomerEmail, "อัปเดตสถานะการจอง", "การจอง "+booking.BookingCode+" เปลี่ยนสถานะเป็น "+status)

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowPublishBookingRealtime(services.RentFlowRealtimeEventBookingUpdated, booking)
	rentFlowSuccess(c, http.StatusOK, "อัปเดตการจองสำเร็จ", rentFlowPartnerBookingResponse(booking, rentFlowPartnerCarNames(tenant.ID)[booking.CarID]))
}

func RentFlowPartnerGetPayments(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payments []models.RentFlowPayment
	query := config.DB.Where("tenant_id = ?", tenant.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("created_at DESC").Find(&payments).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลชำระเงินได้")
		return
	}

	bookingByID := rentFlowPartnerBookingsByID(tenant.ID)
	items := make([]gin.H, 0, len(payments))
	for _, payment := range payments {
		items = append(items, rentFlowPartnerPaymentResponse(payment, bookingByID[payment.BookingID]))
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลชำระเงินสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowPartnerVerifyPayment(c *gin.Context) {
	rentFlowPartnerUpdatePayment(c, "paid", "payment.verify")
}

func RentFlowPartnerRefundPayment(c *gin.Context) {
	rentFlowPartnerUpdatePayment(c, "refunded", "payment.refund")
}

func RentFlowPartnerSettlePayment(c *gin.Context) {
	rentFlowPartnerUpdatePayment(c, "paid", "payment.settle")
}

func rentFlowPartnerUpdatePayment(c *gin.Context, status, action string) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	user, _ := middleware.CurrentRentFlowUser(c)

	var payload struct {
		SlipURL      string `json:"slipUrl"`
		RefundAmount int64  `json:"refundAmount"`
		Note         string `json:"note"`
	}
	_ = c.ShouldBindJSON(&payload)

	var payment models.RentFlowPayment
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("paymentId")).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรายการชำระเงิน")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาการชำระเงินได้")
		return
	}

	now := time.Now()
	updates := map[string]interface{}{"updated_at": now}
	switch action {
	case "payment.verify":
		updates["status"] = status
		updates["verified_by"] = user.ID
		updates["verified_at"] = &now
	case "payment.refund":
		updates["refund_status"] = "refunded"
		updates["refund_amount"] = payload.RefundAmount
		if payload.RefundAmount <= 0 || payload.RefundAmount > payment.Amount {
			updates["refund_amount"] = payment.Amount
		}
	case "payment.settle":
		updates["payout_status"] = "settled"
		updates["settled_at"] = &now
	}

	if err := config.DB.Model(&models.RentFlowPayment{}).Where("tenant_id = ? AND id = ?", tenant.ID, payment.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตการชำระเงินได้")
		return
	}
	rentFlowAudit(c, tenant.ID, action, "payment", payment.ID, strings.TrimSpace(payload.Note))
	if updatedPayment, err := rentFlowPaymentByID(tenant.ID, payment.ID); err == nil {
		rentFlowPublishPaymentRealtime(services.RentFlowRealtimeEventPaymentUpdated, updatedPayment)
	} else {
		rentFlowPublishPaymentRealtime(services.RentFlowRealtimeEventPaymentUpdated, payment)
	}
	rentFlowSuccess(c, http.StatusOK, "อัปเดตการชำระเงินสำเร็จ", nil)
}

func RentFlowPartnerGetCustomers(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var bookings []models.RentFlowBooking
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&bookings).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลลูกค้าได้")
		return
	}

	customerMap := map[string]gin.H{}
	for _, booking := range bookings {
		key := strings.TrimSpace(strings.ToLower(booking.CustomerEmail))
		if key == "" {
			key = booking.CustomerPhone
		}
		row, ok := customerMap[key]
		if !ok {
			row = gin.H{
				"name":          booking.CustomerName,
				"email":         booking.CustomerEmail,
				"phone":         booking.CustomerPhone,
				"bookings":      0,
				"totalAmount":   int64(0),
				"lastBookingAt": booking.CreatedAt,
			}
			customerMap[key] = row
		}
		row["bookings"] = row["bookings"].(int) + 1
		row["totalAmount"] = row["totalAmount"].(int64) + booking.TotalAmount
		if booking.CreatedAt.After(row["lastBookingAt"].(time.Time)) {
			row["lastBookingAt"] = booking.CreatedAt
		}
	}

	items := make([]gin.H, 0, len(customerMap))
	for _, row := range customerMap {
		items = append(items, row)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["totalAmount"].(int64) > items[j]["totalAmount"].(int64)
	})
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลลูกค้าสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowPartnerGetReports(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var cars []models.RentFlowCar
	var branches []models.RentFlowBranch
	var bookings []models.RentFlowBooking
	var payments []models.RentFlowPayment
	var reviews []models.RentFlowReview
	_ = config.DB.Where("tenant_id = ?", tenant.ID).Find(&cars).Error
	_ = config.DB.Where("tenant_id = ?", tenant.ID).Find(&branches).Error
	_ = config.DB.Where("tenant_id = ?", tenant.ID).Find(&bookings).Error
	_ = config.DB.Where("tenant_id = ?", tenant.ID).Find(&payments).Error
	_ = config.DB.Where("tenant_id = ?", tenant.ID).Find(&reviews).Error

	rentFlowSuccess(c, http.StatusOK, "ดึงรายงานสำเร็จ", rentFlowBuildPartnerDashboard(c, tenant, cars, branches, bookings, payments, reviews))
}

func RentFlowPartnerGetCalendar(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var bookings []models.RentFlowBooking
	var blocks []models.RentFlowAvailabilityBlock
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("pickup_date ASC").Find(&bookings).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงปฏิทินได้")
		return
	}
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("start_date ASC").Find(&blocks).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงวันปิดรับจองได้")
		return
	}
	carNames := rentFlowPartnerCarNames(tenant.ID)
	items := make([]gin.H, 0, len(bookings))
	for _, booking := range bookings {
		items = append(items, rentFlowPartnerBookingResponse(booking, carNames[booking.CarID]))
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงปฏิทินสำเร็จ", gin.H{"bookings": items, "blocks": blocks})
}

func RentFlowPartnerCreateAvailabilityBlock(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var payload struct {
		CarID     string `json:"carId"`
		BranchID  string `json:"branchId"`
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
		Reason    string `json:"reason"`
		Note      string `json:"note"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลวันปิดรับจองไม่ถูกต้อง")
		return
	}
	startDate, err := services.ParseDateTime(payload.StartDate)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "วันเริ่มต้นไม่ถูกต้อง")
		return
	}
	endDate, err := services.ParseDateTime(payload.EndDate)
	if err != nil || !endDate.After(startDate) {
		rentFlowError(c, http.StatusBadRequest, "วันสิ้นสุดไม่ถูกต้อง")
		return
	}
	reason := strings.TrimSpace(payload.Reason)
	if reason == "" {
		reason = "maintenance"
	}
	block := models.RentFlowAvailabilityBlock{
		ID:        services.NewID("blk"),
		TenantID:  tenant.ID,
		CarID:     strings.TrimSpace(payload.CarID),
		BranchID:  strings.TrimSpace(payload.BranchID),
		StartDate: startDate,
		EndDate:   endDate,
		Reason:    reason,
		Note:      strings.TrimSpace(payload.Note),
	}
	if err := config.DB.Create(&block).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกวันปิดรับจองได้")
		return
	}
	rentFlowAudit(c, tenant.ID, "availability_block.create", "availability_block", block.ID, block.Reason)
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowPublishCarRealtime(tenant.ID, block.CarID, services.RentFlowRealtimeEventAvailabilityChange)
	rentFlowSuccess(c, http.StatusCreated, "บันทึกวันปิดรับจองสำเร็จ", block)
}

func RentFlowPartnerDeleteAvailabilityBlock(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	result := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("blockId")).Delete(&models.RentFlowAvailabilityBlock{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบวันปิดรับจองได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบวันปิดรับจอง")
		return
	}
	rentFlowAudit(c, tenant.ID, "availability_block.delete", "availability_block", c.Param("blockId"), "")
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowPublishCarRealtime(tenant.ID, "", services.RentFlowRealtimeEventAvailabilityChange)
	rentFlowSuccess(c, http.StatusOK, "ลบวันปิดรับจองสำเร็จ", nil)
}

func RentFlowPartnerListDomains(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var domains []models.RentFlowCustomDomain
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&domains).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงโดเมนได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงโดเมนสำเร็จ", gin.H{"items": domains, "total": len(domains)})
}

func RentFlowPartnerCreateDomain(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var payload struct {
		Domain string `json:"domain"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลโดเมนไม่ถูกต้อง")
		return
	}
	domain := strings.Trim(strings.ToLower(payload.Domain), ". ")
	if domain == "" || strings.Contains(domain, "/") {
		rentFlowError(c, http.StatusBadRequest, "ชื่อโดเมนไม่ถูกต้อง")
		return
	}
	item := models.RentFlowCustomDomain{
		ID:              services.NewID("dom"),
		TenantID:        tenant.ID,
		Domain:          domain,
		Status:          "pending",
		VerificationTXT: "rentflow-verify=" + services.NewID("verify"),
	}
	if err := config.DB.Create(&item).Error; err != nil {
		rentFlowError(c, http.StatusConflict, "โดเมนนี้ถูกใช้งานแล้วหรือบันทึกไม่ได้")
		return
	}
	rentFlowAudit(c, tenant.ID, "domain.create", "domain", item.ID, domain)
	rentFlowSuccess(c, http.StatusCreated, "เพิ่มโดเมนสำเร็จ", item)
}

func RentFlowPartnerVerifyDomain(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	now := time.Now()
	result := config.DB.Model(&models.RentFlowCustomDomain{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("domainId")).
		Updates(map[string]interface{}{"status": "verified", "verified_at": &now, "updated_at": now})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถยืนยันโดเมนได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบโดเมน")
		return
	}
	rentFlowAudit(c, tenant.ID, "domain.verify", "domain", c.Param("domainId"), "")
	rentFlowSuccess(c, http.StatusOK, "ยืนยันโดเมนสำเร็จ", nil)
}

func RentFlowPartnerDeleteDomain(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	result := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("domainId")).Delete(&models.RentFlowCustomDomain{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบโดเมนได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบโดเมน")
		return
	}
	rentFlowAudit(c, tenant.ID, "domain.delete", "domain", c.Param("domainId"), "")
	rentFlowSuccess(c, http.StatusOK, "ลบโดเมนสำเร็จ", nil)
}

func RentFlowPartnerGetAuditLogs(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var logs []models.RentFlowAuditLog
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Limit(100).Find(&logs).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงประวัติการใช้งานได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงประวัติการใช้งานสำเร็จ", gin.H{"items": logs, "total": len(logs)})
}

func RentFlowAdminListTenants(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}
	items, err := rentFlowPlatformTenantItems()
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลร้านได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลร้านสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowAdminUpdateTenantStatus(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}
	var payload struct {
		Status      string `json:"status"`
		BookingMode string `json:"bookingMode"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลร้านไม่ถูกต้อง")
		return
	}

	var tenant models.RentFlowTenant
	if err := config.DB.Where("id = ?", c.Param("tenantId")).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบร้านที่ต้องการอัปเดต")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาข้อมูลร้านได้")
		return
	}

	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = tenant.Status
	}
	if status != "active" && status != "suspended" && status != "pending" {
		rentFlowError(c, http.StatusBadRequest, "สถานะร้านไม่ถูกต้อง")
		return
	}

	bookingMode := rentFlowNormalizeBookingMode(tenant.BookingMode)
	if strings.TrimSpace(payload.BookingMode) != "" {
		bookingMode = rentFlowNormalizeBookingMode(payload.BookingMode)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":       status,
		"booking_mode": bookingMode,
		"updated_at":   now,
	}
	if err := config.DB.Model(&models.RentFlowTenant{}).Where("id = ?", tenant.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตร้านได้")
		return
	}

	tenant.Status = status
	tenant.BookingMode = bookingMode
	tenant.UpdatedAt = now
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowAudit(c, tenant.ID, "platform.tenant_settings", "tenant", tenant.ID, status+"|"+bookingMode)
	services.RentFlowPublishRealtime(services.RentFlowRealtimeEvent{
		Type:     services.RentFlowRealtimeEventTenantUpdated,
		TenantID: tenant.ID,
		EntityID: tenant.ID,
		Data: gin.H{
			"id":          tenant.ID,
			"status":      tenant.Status,
			"bookingMode": tenant.BookingMode,
			"domainSlug":  tenant.DomainSlug,
		},
	})
	rentFlowSuccess(c, http.StatusOK, "อัปเดตร้านสำเร็จ", gin.H{
		"tenant": gin.H{
			"id":           tenant.ID,
			"shopName":     tenant.ShopName,
			"domainSlug":   tenant.DomainSlug,
			"publicDomain": tenant.PublicDomain,
			"status":       tenant.Status,
			"bookingMode":  rentFlowNormalizeBookingMode(tenant.BookingMode),
			"plan":         tenant.Plan,
			"updatedAt":    tenant.UpdatedAt,
		},
	})
}

func rentFlowPaymentByID(tenantID, paymentID string) (models.RentFlowPayment, error) {
	var payment models.RentFlowPayment
	err := config.DB.Where("tenant_id = ? AND id = ?", tenantID, paymentID).First(&payment).Error
	return payment, err
}

func rentFlowNormalizeBookingStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "pending", "confirmed", "paid", "completed", "cancelled":
		return strings.TrimSpace(strings.ToLower(status))
	default:
		return ""
	}
}

func rentFlowPartnerCarNames(tenantID string) map[string]string {
	var cars []models.RentFlowCar
	_ = config.DB.Where("tenant_id = ?", tenantID).Find(&cars).Error
	result := map[string]string{}
	for _, car := range cars {
		result[car.ID] = car.Name
	}
	return result
}

func rentFlowPartnerBookingsByID(tenantID string) map[string]models.RentFlowBooking {
	var bookings []models.RentFlowBooking
	_ = config.DB.Where("tenant_id = ?", tenantID).Find(&bookings).Error
	result := map[string]models.RentFlowBooking{}
	for _, booking := range bookings {
		result[booking.ID] = booking
	}
	return result
}

func rentFlowPartnerBookingResponse(booking models.RentFlowBooking, carName string) gin.H {
	return gin.H{
		"id":             booking.ID,
		"tenantId":       booking.TenantID,
		"bookingCode":    booking.BookingCode,
		"carId":          booking.CarID,
		"carName":        carName,
		"status":         booking.Status,
		"pickupDate":     booking.PickupDate,
		"returnDate":     booking.ReturnDate,
		"pickupLocation": booking.PickupLocation,
		"returnLocation": booking.ReturnLocation,
		"pickupMethod":   booking.PickupMethod,
		"returnMethod":   booking.ReturnMethod,
		"totalDays":      booking.TotalDays,
		"totalAmount":    booking.TotalAmount,
		"customerName":   booking.CustomerName,
		"customerEmail":  booking.CustomerEmail,
		"customerPhone":  booking.CustomerPhone,
		"note":           booking.Note,
		"createdAt":      booking.CreatedAt,
		"updatedAt":      booking.UpdatedAt,
	}
}

func rentFlowPartnerPaymentResponse(payment models.RentFlowPayment, booking models.RentFlowBooking) gin.H {
	return gin.H{
		"id":            payment.ID,
		"tenantId":      payment.TenantID,
		"bookingId":     payment.BookingID,
		"bookingCode":   booking.BookingCode,
		"customerName":  booking.CustomerName,
		"method":        payment.Method,
		"status":        payment.Status,
		"amount":        payment.Amount,
		"transactionId": payment.TransactionID,
		"paymentUrl":    payment.PaymentURL,
		"qrCodeUrl":     payment.QRCodeURL,
		"slipUrl":       rentFlowPaymentSlipURL(payment),
		"verifiedBy":    payment.VerifiedBy,
		"verifiedAt":    payment.VerifiedAt,
		"refundStatus":  payment.RefundStatus,
		"refundAmount":  payment.RefundAmount,
		"payoutStatus":  payment.PayoutStatus,
		"settledAt":     payment.SettledAt,
		"createdAt":     payment.CreatedAt,
		"updatedAt":     payment.UpdatedAt,
	}
}

func rentFlowAudit(c *gin.Context, tenantID, action, entity, entityID, detail string) {
	user, _ := middleware.CurrentRentFlowUser(c)
	log := models.RentFlowAuditLog{
		ID:         services.NewID("aud"),
		TenantID:   tenantID,
		ActorID:    user.ID,
		ActorEmail: user.Email,
		Action:     action,
		Entity:     entity,
		EntityID:   entityID,
		Detail:     strings.TrimSpace(detail),
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	}
	_ = config.DB.Create(&log).Error
}

func rentFlowRequirePlatformAdmin(c *gin.Context) bool {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return false
	}
	if !services.IsRentFlowPlatformAdmin(user) {
		rentFlowError(c, http.StatusForbidden, "ไม่มีสิทธิ์จัดการระบบกลาง")
		return false
	}
	return true
}
