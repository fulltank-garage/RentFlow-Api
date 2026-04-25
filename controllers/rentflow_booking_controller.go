package controllers

import (
	"encoding/json"
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

func RentFlowPreviewBookingPrice(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var payload struct {
		CarID          string                        `json:"carId"`
		PickupDate     string                        `json:"pickupDate"`
		ReturnDate     string                        `json:"returnDate"`
		PickupLocation string                        `json:"pickupLocation"`
		ReturnLocation string                        `json:"returnLocation"`
		Addons         []rentFlowBookingAddonPayload `json:"addons"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสำหรับคำนวณราคาไม่ถูกต้อง")
		return
	}

	car, pickupDate, returnDate, ok := rentFlowValidateBookingDatesAndCar(c, tenant.ID, payload.CarID, payload.PickupDate, payload.ReturnDate)
	if !ok {
		return
	}

	totalDays, subtotal, extraCharge, discount, totalAmount := services.ComputeBookingPrice(
		car.PricePerDay,
		pickupDate,
		returnDate,
		payload.PickupLocation,
		payload.ReturnLocation,
	)
	addons, addonsJSON, addonsTotal := rentFlowBookingAddonsSummary(tenant.ID, payload.Addons, totalDays)
	totalAmount += addonsTotal

	rentFlowSuccess(c, http.StatusOK, "คำนวณราคาเรียบร้อย", gin.H{
		"totalDays":   totalDays,
		"pricePerDay": car.PricePerDay,
		"subtotal":    subtotal,
		"addons":      addons,
		"addonsTotal": addonsTotal,
		"addonsJson":  addonsJSON,
		"extraCharge": extraCharge,
		"discount":    discount,
		"totalAmount": totalAmount,
	})
}

func RentFlowCreateBooking(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var payload struct {
		CarID          string                        `json:"carId"`
		PickupDate     string                        `json:"pickupDate"`
		ReturnDate     string                        `json:"returnDate"`
		PickupLocation string                        `json:"pickupLocation"`
		ReturnLocation string                        `json:"returnLocation"`
		PickupMethod   string                        `json:"pickupMethod"`
		ReturnMethod   string                        `json:"returnMethod"`
		CustomerName   string                        `json:"customerName"`
		CustomerEmail  string                        `json:"customerEmail"`
		CustomerPhone  string                        `json:"customerPhone"`
		Note           string                        `json:"note"`
		Addons         []rentFlowBookingAddonPayload `json:"addons"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลการจองไม่ถูกต้อง")
		return
	}

	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	car, pickupDate, returnDate, ok := rentFlowValidateBookingDatesAndCar(c, tenant.ID, payload.CarID, payload.PickupDate, payload.ReturnDate)
	if !ok {
		return
	}

	available, err := rentFlowCarIsAvailable(tenant.ID, car.ID, pickupDate, returnDate)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
		return
	}
	if !available {
		rentFlowError(c, http.StatusConflict, "รถคันนี้ถูกจองในช่วงเวลาที่เลือกแล้ว")
		return
	}

	customerEmail := strings.TrimSpace(strings.ToLower(payload.CustomerEmail))
	if strings.TrimSpace(payload.CustomerName) == "" || strings.TrimSpace(payload.CustomerPhone) == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกข้อมูลผู้จองให้ครบถ้วน")
		return
	}
	if customerEmail == "" {
		customerEmail = strings.TrimSpace(strings.ToLower(user.Email))
	}
	if customerEmail == "" {
		customerEmail = "no-email@rentflow.local"
	}

	totalDays, subtotal, extraCharge, discount, totalAmount := services.ComputeBookingPrice(
		car.PricePerDay,
		pickupDate,
		returnDate,
		payload.PickupLocation,
		payload.ReturnLocation,
	)
	_, addonsJSON, addonsTotal := rentFlowBookingAddonsSummary(tenant.ID, payload.Addons, totalDays)
	totalAmount += addonsTotal

	booking := models.RentFlowBooking{
		ID:             services.NewID("bok"),
		TenantID:       tenant.ID,
		BookingCode:    services.NewBookingCode(),
		CarID:          car.ID,
		Status:         "pending",
		PickupDate:     pickupDate,
		ReturnDate:     returnDate,
		PickupLocation: strings.TrimSpace(payload.PickupLocation),
		ReturnLocation: strings.TrimSpace(payload.ReturnLocation),
		PickupMethod:   rentFlowNormalizeMethod(payload.PickupMethod),
		ReturnMethod:   rentFlowNormalizeMethod(payload.ReturnMethod),
		TotalDays:      totalDays,
		Subtotal:       subtotal,
		AddonsJSON:     addonsJSON,
		AddonsTotal:    addonsTotal,
		ExtraCharge:    extraCharge,
		Discount:       discount,
		TotalAmount:    totalAmount,
		Note:           strings.TrimSpace(payload.Note),
		CustomerName:   strings.TrimSpace(payload.CustomerName),
		CustomerEmail:  customerEmail,
		CustomerPhone:  strings.TrimSpace(payload.CustomerPhone),
		UserID:         &user.ID,
		UserEmail:      user.Email,
	}

	if err := config.DB.Create(&booking).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างรายการจองได้")
		return
	}

	if booking.UserEmail != "" {
		rentFlowCreateNotification(tenant.ID, booking.UserID, booking.UserEmail, "สร้างการจองใหม่", "การจอง "+booking.BookingCode+" ถูกสร้างเรียบร้อยแล้ว")
	}

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowPublishBookingRealtime(services.RentFlowRealtimeEventBookingCreated, booking)
	rentFlowSuccess(c, http.StatusCreated, "สร้างรายการจองสำเร็จ", rentFlowBookingResponse(booking))
}

func RentFlowGetMyBookings(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	var bookings []models.RentFlowBooking
	if err := config.DB.
		Where("user_id = ? OR customer_email = ?", user.ID, user.Email).
		Order("created_at DESC").
		Find(&bookings).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรายการจองได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงรายการจองสำเร็จ", rentFlowBookingResponses(bookings))
}

func RentFlowGetBookingByID(c *gin.Context) {
	booking, ok := rentFlowLoadOwnedBooking(c, c.Param("bookingId"))
	if !ok {
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลการจองสำเร็จ", rentFlowBookingResponse(*booking))
}

func RentFlowCancelBooking(c *gin.Context) {
	booking, ok := rentFlowLoadOwnedBooking(c, c.Param("bookingId"))
	if !ok {
		return
	}

	if booking.Status == "cancelled" {
		rentFlowError(c, http.StatusBadRequest, "รายการจองนี้ถูกยกเลิกไปแล้ว")
		return
	}
	if booking.Status == "completed" {
		rentFlowError(c, http.StatusBadRequest, "ไม่สามารถยกเลิกงานที่เสร็จสิ้นแล้วได้")
		return
	}

	if err := config.DB.Model(&models.RentFlowBooking{}).
		Where("id = ?", booking.ID).
		Updates(map[string]interface{}{
			"status":     "cancelled",
			"updated_at": time.Now(),
		}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถยกเลิกรายการจองได้")
		return
	}

	booking.Status = "cancelled"
	rentFlowCreateNotification(booking.TenantID, booking.UserID, booking.CustomerEmail, "ยกเลิกการจอง", "การจอง "+booking.BookingCode+" ถูกยกเลิกแล้ว")
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowPublishBookingRealtime(services.RentFlowRealtimeEventBookingCancelled, *booking)
	rentFlowSuccess(c, http.StatusOK, "ยกเลิกรายการจองสำเร็จ", rentFlowBookingResponse(*booking))
}

func RentFlowCreatePayment(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var payload struct {
		BookingID  string  `json:"bookingId"`
		Method     string  `json:"method"`
		SlipImage  *string `json:"slipImage"`
		CardHolder string  `json:"cardHolder"`
		CardNumber string  `json:"cardNumber"`
		CardExpiry string  `json:"cardExpiry"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลการชำระเงินไม่ถูกต้อง")
		return
	}

	var booking models.RentFlowBooking
	if err := config.DB.Where("tenant_id = ? AND (id = ? OR booking_code = ?)", tenant.ID, payload.BookingID, payload.BookingID).First(&booking).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรายการจองที่ต้องการชำระเงิน")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหารายการจองได้")
		return
	}

	method := rentFlowNormalizePaymentMethod(payload.Method)
	if method == "" {
		method = "promptpay"
	}
	if method == "card" {
		if !rentFlowValidateInternalCard(payload.CardNumber, payload.CardExpiry) {
			rentFlowError(c, http.StatusBadRequest, "ข้อมูลบัตรไม่ถูกต้อง")
			return
		}
	}

	slipBlob, slipMimeType, err := rentFlowImageBlobFromSource(payload.SlipImage)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "ไฟล์สลิปไม่ถูกต้อง")
		return
	}
	if method == "bank_transfer" && len(slipBlob) == 0 {
		rentFlowError(c, http.StatusBadRequest, "กรุณาแนบสลิปโอนเงิน")
		return
	}

	now := time.Now()
	payment := models.RentFlowPayment{
		ID:               services.NewID("pay"),
		TenantID:         tenant.ID,
		BookingID:        booking.ID,
		Method:           method,
		Status:           "paid",
		Amount:           booking.TotalAmount,
		TransactionID:    services.NewID("txn_int"),
		Processor:        "internal",
		ProcessedAt:      &now,
		SlipMimeType:     slipMimeType,
		SlipBlob:         slipBlob,
		SettlementPeriod: time.Now().Format("2006-01"),
	}
	if method == "promptpay" {
		payment.QRCodeURL = "/QR-CODE.jpg"
	}
	if method == "card" {
		payment.CardHolder = strings.TrimSpace(payload.CardHolder)
		payment.CardLast4 = rentFlowCardLast4(payload.CardNumber)
	}

	if err := config.DB.Create(&payment).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างรายการชำระเงินได้")
		return
	}

	if err := config.DB.Model(&models.RentFlowBooking{}).
		Where("id = ?", booking.ID).
		Updates(map[string]interface{}{
			"status":     "paid",
			"updated_at": time.Now(),
		}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตสถานะการจองได้")
		return
	}

	rentFlowCreateNotification(tenant.ID, booking.UserID, booking.CustomerEmail, "ชำระเงินสำเร็จ", "การจอง "+booking.BookingCode+" ชำระเงินเรียบร้อยแล้ว")
	booking.Status = "paid"
	rentFlowPublishPaymentRealtime(services.RentFlowRealtimeEventPaymentCreated, payment)
	rentFlowPublishBookingRealtime(services.RentFlowRealtimeEventBookingUpdated, booking)
	rentFlowSuccess(c, http.StatusCreated, "สร้างรายการชำระเงินสำเร็จ", rentFlowPaymentResponse(payment))
}

func RentFlowGetPaymentByBookingID(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var payment models.RentFlowPayment
	if err := config.DB.Where("tenant_id = ? AND booking_id = ?", tenant.ID, c.Param("bookingId")).Order("created_at DESC").First(&payment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ยังไม่มีข้อมูลการชำระเงินสำหรับการจองนี้")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการชำระเงินได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลการชำระเงินสำเร็จ", rentFlowPaymentResponse(payment))
}

func RentFlowGetPaymentSlip(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var payment models.RentFlowPayment
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("paymentId")).First(&payment).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบสลิปชำระเงิน")
		return
	}

	if len(payment.SlipBlob) == 0 || strings.TrimSpace(payment.SlipMimeType) == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบสลิปชำระเงิน")
		return
	}

	rentFlowSendImageBlob(c, payment.SlipMimeType, payment.SlipBlob)
}

func RentFlowGetNotifications(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	var notifications []models.RentFlowNotification
	if err := config.DB.
		Where("user_id = ? OR user_email = ?", user.ID, user.Email).
		Order("created_at DESC").
		Find(&notifications).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงการแจ้งเตือนได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงการแจ้งเตือนสำเร็จ", gin.H{
		"items": notifications,
		"total": len(notifications),
	})
}

func RentFlowMarkNotificationAsRead(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	if err := config.DB.Model(&models.RentFlowNotification{}).
		Where("id = ? AND (user_id = ? OR user_email = ?)", c.Param("notificationId"), user.ID, user.Email).
		Updates(map[string]interface{}{
			"is_read":    true,
			"updated_at": time.Now(),
		}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตการแจ้งเตือนได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "อัปเดตการแจ้งเตือนสำเร็จ", nil)
}

func RentFlowMarkAllNotificationsAsRead(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	if err := config.DB.Model(&models.RentFlowNotification{}).
		Where("user_id = ? OR user_email = ?", user.ID, user.Email).
		Updates(map[string]interface{}{
			"is_read":    true,
			"updated_at": time.Now(),
		}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตการแจ้งเตือนได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "อัปเดตการแจ้งเตือนทั้งหมดสำเร็จ", nil)
}

func rentFlowValidateBookingDatesAndCar(c *gin.Context, tenantID, carID, pickupDateRaw, returnDateRaw string) (models.RentFlowCar, time.Time, time.Time, bool) {
	var car models.RentFlowCar
	if err := config.DB.Where("tenant_id = ? AND id = ? AND is_available = ? AND status = ?", tenantID, carID, true, "available").First(&car).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรถที่ต้องการ")
		return models.RentFlowCar{}, time.Time{}, time.Time{}, false
	}

	pickupDate, err := services.ParseDateTime(pickupDateRaw)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "วันรับรถไม่ถูกต้อง")
		return models.RentFlowCar{}, time.Time{}, time.Time{}, false
	}
	returnDate, err := services.ParseDateTime(returnDateRaw)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "วันคืนรถไม่ถูกต้อง")
		return models.RentFlowCar{}, time.Time{}, time.Time{}, false
	}
	if !returnDate.After(pickupDate) {
		rentFlowError(c, http.StatusBadRequest, "วันคืนรถต้องหลังวันรับรถ")
		return models.RentFlowCar{}, time.Time{}, time.Time{}, false
	}

	return car, pickupDate, returnDate, true
}

func rentFlowNormalizeMethod(method string) string {
	method = strings.TrimSpace(method)
	if method == "custom" {
		return "custom"
	}
	return "branch"
}

type rentFlowBookingAddonPayload struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	Price   int64  `json:"price"`
	Pricing string `json:"pricing"`
}

type rentFlowBookingAddonItem struct {
	ID        string `json:"id,omitempty"`
	Key       string `json:"key,omitempty"`
	Name      string `json:"name"`
	Price     int64  `json:"price"`
	Pricing   string `json:"pricing"`
	Quantity  int    `json:"quantity"`
	LineTotal int64  `json:"lineTotal"`
}

func rentFlowBookingAddonsSummary(tenantID string, payload []rentFlowBookingAddonPayload, totalDays int) ([]rentFlowBookingAddonItem, string, int64) {
	if len(payload) == 0 {
		return []rentFlowBookingAddonItem{}, "", 0
	}

	days := totalDays
	if days < 1 {
		days = 1
	}

	ids := make([]string, 0, len(payload))
	names := make([]string, 0, len(payload)*3)
	for _, addon := range payload {
		if value := strings.TrimSpace(addon.ID); value != "" {
			ids = append(ids, value)
		}
		for _, value := range []string{addon.Name, addon.Title, addon.Key} {
			value = strings.TrimSpace(value)
			if value != "" {
				names = append(names, strings.ToLower(value))
			}
		}
	}

	dbAddonsByID := map[string]models.RentFlowAddon{}
	dbAddonsByName := map[string]models.RentFlowAddon{}
	if len(ids) > 0 || len(names) > 0 {
		var dbAddons []models.RentFlowAddon
		query := config.DB.Where("tenant_id = ? AND is_active = ?", tenantID, true)
		if len(ids) > 0 && len(names) > 0 {
			query = query.Where("id IN ? OR LOWER(name) IN ?", ids, names)
		} else if len(ids) > 0 {
			query = query.Where("id IN ?", ids)
		} else {
			query = query.Where("LOWER(name) IN ?", names)
		}
		_ = query.Find(&dbAddons).Error
		for _, addon := range dbAddons {
			dbAddonsByID[addon.ID] = addon
			dbAddonsByName[strings.ToLower(strings.TrimSpace(addon.Name))] = addon
		}
	}

	items := make([]rentFlowBookingAddonItem, 0, len(payload))
	seen := map[string]struct{}{}
	var total int64

	for _, input := range payload {
		dbAddon, hasDBAddon := dbAddonsByID[strings.TrimSpace(input.ID)]
		if !hasDBAddon {
			for _, value := range []string{input.Name, input.Title, input.Key} {
				value = strings.ToLower(strings.TrimSpace(value))
				if value == "" {
					continue
				}
				if addon, ok := dbAddonsByName[value]; ok {
					dbAddon = addon
					hasDBAddon = true
					break
				}
			}
		}

		id := strings.TrimSpace(input.ID)
		key := strings.TrimSpace(input.Key)
		name := strings.TrimSpace(input.Title)
		if name == "" {
			name = strings.TrimSpace(input.Name)
		}
		price := input.Price
		pricing := rentFlowNormalizeAddonPricing(input.Pricing)

		if hasDBAddon {
			id = dbAddon.ID
			if name == "" {
				name = dbAddon.Name
			}
			price = dbAddon.Price
			pricing = rentFlowNormalizeAddonPricing(dbAddon.Unit)
		}
		if name == "" {
			name = key
		}
		if name == "" || price < 0 {
			continue
		}

		identity := strings.ToLower(strings.TrimSpace(id))
		if identity == "" {
			identity = strings.ToLower(strings.TrimSpace(key))
		}
		if identity == "" {
			identity = strings.ToLower(name)
		}
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}

		quantity := 1
		if pricing == "perDay" {
			quantity = days
		}
		lineTotal := price * int64(quantity)
		total += lineTotal
		items = append(items, rentFlowBookingAddonItem{
			ID:        id,
			Key:       key,
			Name:      name,
			Price:     price,
			Pricing:   pricing,
			Quantity:  quantity,
			LineTotal: lineTotal,
		})
	}

	if len(items) == 0 {
		return []rentFlowBookingAddonItem{}, "", 0
	}

	raw, err := json.Marshal(items)
	if err != nil {
		return items, "", total
	}
	return items, string(raw), total
}

func rentFlowNormalizeAddonPricing(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "day", "daily", "per_day", "perday":
		return "perDay"
	default:
		return "perTrip"
	}
}

func rentFlowBookingAddonsFromJSON(raw string) []rentFlowBookingAddonItem {
	if strings.TrimSpace(raw) == "" {
		return []rentFlowBookingAddonItem{}
	}
	var items []rentFlowBookingAddonItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []rentFlowBookingAddonItem{}
	}
	return items
}

func rentFlowLoadOwnedBooking(c *gin.Context, bookingID string) (*models.RentFlowBooking, bool) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return nil, false
	}

	var booking models.RentFlowBooking
	if err := config.DB.
		Where("(id = ? OR booking_code = ?) AND (user_id = ? OR customer_email = ?)", bookingID, bookingID, user.ID, user.Email).
		First(&booking).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรายการจองที่ต้องการ")
			return nil, false
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการจองได้")
		return nil, false
	}

	return &booking, true
}

func rentFlowBookingResponses(bookings []models.RentFlowBooking) []gin.H {
	carIDs := make([]string, 0, len(bookings))
	tenantIDs := make([]string, 0, len(bookings))
	locationValues := make([]string, 0, len(bookings)*2)
	seenCars := map[string]struct{}{}
	seenTenants := map[string]struct{}{}
	seenLocations := map[string]struct{}{}

	for _, booking := range bookings {
		if booking.CarID != "" {
			if _, ok := seenCars[booking.CarID]; !ok {
				seenCars[booking.CarID] = struct{}{}
				carIDs = append(carIDs, booking.CarID)
			}
		}
		if booking.TenantID != "" {
			if _, ok := seenTenants[booking.TenantID]; !ok {
				seenTenants[booking.TenantID] = struct{}{}
				tenantIDs = append(tenantIDs, booking.TenantID)
			}
		}
		for _, value := range []string{booking.PickupLocation, booking.ReturnLocation} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seenLocations[key]; ok {
				continue
			}
			seenLocations[key] = struct{}{}
			locationValues = append(locationValues, value)
		}
	}

	carMap := map[string]models.RentFlowCar{}
	if len(carIDs) > 0 {
		var cars []models.RentFlowCar
		_ = config.DB.Unscoped().Where("id IN ?", carIDs).Find(&cars).Error
		for _, car := range cars {
			carMap[car.ID] = car
		}
	}

	tenantMap := map[string]models.RentFlowTenant{}
	if len(tenantIDs) > 0 {
		var tenants []models.RentFlowTenant
		_ = config.DB.Where("id IN ?", tenantIDs).Find(&tenants).Error
		for _, tenant := range tenants {
			tenantMap[tenant.ID] = tenant
		}
	}

	branchNameMap := map[string]string{}
	if len(tenantIDs) > 0 && len(locationValues) > 0 {
		var branches []models.RentFlowBranch
		_ = config.DB.
			Where("tenant_id IN ? AND (id IN ? OR location_id IN ? OR name IN ?)", tenantIDs, locationValues, locationValues, locationValues).
			Find(&branches).Error
		for _, branch := range branches {
			displayName := rentFlowBranchDisplayName(branch)
			for _, value := range []string{branch.ID, branch.LocationID, branch.Name} {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				branchNameMap[branch.TenantID+"|"+strings.ToLower(value)] = displayName
			}
		}
	}

	items := make([]gin.H, 0, len(bookings))
	for _, booking := range bookings {
		items = append(items, rentFlowBookingResponseWithMaps(booking, carMap, tenantMap, branchNameMap))
	}
	return items
}

func rentFlowBookingResponse(booking models.RentFlowBooking) gin.H {
	return rentFlowBookingResponses([]models.RentFlowBooking{booking})[0]
}

func rentFlowBookingResponseWithMaps(booking models.RentFlowBooking, carMap map[string]models.RentFlowCar, tenantMap map[string]models.RentFlowTenant, branchNameMap map[string]string) gin.H {
	car := carMap[booking.CarID]
	tenant := tenantMap[booking.TenantID]
	pickupLocation := rentFlowDisplayBranchName(booking.TenantID, booking.PickupLocation, branchNameMap)
	returnLocation := rentFlowDisplayBranchName(booking.TenantID, booking.ReturnLocation, branchNameMap)
	carName := strings.TrimSpace(car.Name)
	if carName == "" {
		carName = booking.CarID
	}

	return gin.H{
		"id":                  booking.ID,
		"tenantId":            booking.TenantID,
		"bookingCode":         booking.BookingCode,
		"userId":              booking.UserID,
		"carId":               booking.CarID,
		"carName":             carName,
		"status":              booking.Status,
		"pickupDate":          booking.PickupDate,
		"returnDate":          booking.ReturnDate,
		"pickupLocation":      pickupLocation,
		"returnLocation":      returnLocation,
		"pickupLocationValue": booking.PickupLocation,
		"returnLocationValue": booking.ReturnLocation,
		"pickupMethod":        booking.PickupMethod,
		"returnMethod":        booking.ReturnMethod,
		"totalDays":           booking.TotalDays,
		"subtotal":            booking.Subtotal,
		"addons":              rentFlowBookingAddonsFromJSON(booking.AddonsJSON),
		"addonsTotal":         booking.AddonsTotal,
		"extraCharge":         booking.ExtraCharge,
		"discount":            booking.Discount,
		"totalAmount":         booking.TotalAmount,
		"note":                booking.Note,
		"customerName":        booking.CustomerName,
		"customerEmail":       booking.CustomerEmail,
		"customerPhone":       booking.CustomerPhone,
		"createdAt":           booking.CreatedAt,
		"updatedAt":           booking.UpdatedAt,
		"shopName":            tenant.ShopName,
		"domainSlug":          tenant.DomainSlug,
		"publicDomain":        tenant.PublicDomain,
		"logoUrl":             rentFlowTenantLogoURL(tenant),
	}
}

func rentFlowDisplayBranchName(tenantID, value string, branchNameMap map[string]string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if name := branchNameMap[tenantID+"|"+strings.ToLower(trimmed)]; strings.TrimSpace(name) != "" {
		return name
	}
	return trimmed
}

func rentFlowPaymentResponse(payment models.RentFlowPayment) gin.H {
	return gin.H{
		"id":               payment.ID,
		"tenantId":         payment.TenantID,
		"bookingId":        payment.BookingID,
		"method":           payment.Method,
		"status":           payment.Status,
		"amount":           payment.Amount,
		"transactionId":    payment.TransactionID,
		"paymentUrl":       payment.PaymentURL,
		"qrCodeUrl":        payment.QRCodeURL,
		"processor":        payment.Processor,
		"cardLast4":        payment.CardLast4,
		"cardHolder":       payment.CardHolder,
		"processedAt":      payment.ProcessedAt,
		"failureReason":    payment.FailureReason,
		"slipUrl":          rentFlowPaymentSlipURL(payment),
		"verifiedBy":       payment.VerifiedBy,
		"verifiedAt":       payment.VerifiedAt,
		"refundStatus":     payment.RefundStatus,
		"refundAmount":     payment.RefundAmount,
		"payoutStatus":     payment.PayoutStatus,
		"settledAt":        payment.SettledAt,
		"settlementPeriod": payment.SettlementPeriod,
		"settlementNote":   payment.SettlementNote,
		"createdAt":        payment.CreatedAt,
		"updatedAt":        payment.UpdatedAt,
	}
}

func rentFlowNormalizePaymentMethod(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "card", "promptpay", "cash":
		return strings.TrimSpace(strings.ToLower(value))
	case "transfer", "bank-transfer", "bank_transfer":
		return "bank_transfer"
	default:
		return ""
	}
}

func rentFlowValidateInternalCard(number, expiry string) bool {
	digits := rentFlowDigitsOnly(number)
	expiry = strings.TrimSpace(expiry)
	return len(digits) >= 12 && len(digits) <= 19 && len(expiry) >= 4
}

func rentFlowCardLast4(number string) string {
	digits := rentFlowDigitsOnly(number)
	if len(digits) <= 4 {
		return digits
	}
	return digits[len(digits)-4:]
}

func rentFlowDigitsOnly(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func rentFlowCreateNotification(tenantID string, userID *string, userEmail, title, message string) {
	if strings.TrimSpace(userEmail) == "" {
		return
	}
	notification := models.RentFlowNotification{
		ID:        services.NewID("ntf"),
		TenantID:  tenantID,
		UserID:    userID,
		UserEmail: strings.TrimSpace(strings.ToLower(userEmail)),
		Title:     title,
		Message:   message,
		IsRead:    false,
	}
	_ = config.DB.Create(&notification).Error
	_ = config.DB.Create(&models.RentFlowMessageLog{
		ID:        services.NewID("msg"),
		TenantID:  tenantID,
		Channel:   "email",
		Recipient: strings.TrimSpace(strings.ToLower(userEmail)),
		Subject:   title,
		Body:      message,
		Status:    "queued",
	}).Error
	userIDValue := ""
	if userID != nil {
		userIDValue = *userID
	}
	services.RentFlowPublishRealtime(services.RentFlowRealtimeEvent{
		Type:      services.RentFlowRealtimeEventNotificationNew,
		TenantID:  tenantID,
		UserID:    userIDValue,
		UserEmail: notification.UserEmail,
		EntityID:  notification.ID,
		Data: gin.H{
			"id":      notification.ID,
			"title":   notification.Title,
			"message": notification.Message,
			"isRead":  notification.IsRead,
		},
	})
}
