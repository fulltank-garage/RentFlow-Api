package controllers

import (
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
	var payload struct {
		CarID          string `json:"carId"`
		PickupDate     string `json:"pickupDate"`
		ReturnDate     string `json:"returnDate"`
		PickupLocation string `json:"pickupLocation"`
		ReturnLocation string `json:"returnLocation"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสำหรับคำนวณราคาไม่ถูกต้อง")
		return
	}

	car, pickupDate, returnDate, ok := rentFlowValidateBookingDatesAndCar(c, payload.CarID, payload.PickupDate, payload.ReturnDate)
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

	rentFlowSuccess(c, http.StatusOK, "คำนวณราคาเรียบร้อย", gin.H{
		"totalDays":   totalDays,
		"pricePerDay": car.PricePerDay,
		"subtotal":    subtotal,
		"extraCharge": extraCharge,
		"discount":    discount,
		"totalAmount": totalAmount,
	})
}

func RentFlowCreateBooking(c *gin.Context) {
	var payload struct {
		CarID          string `json:"carId"`
		PickupDate     string `json:"pickupDate"`
		ReturnDate     string `json:"returnDate"`
		PickupLocation string `json:"pickupLocation"`
		ReturnLocation string `json:"returnLocation"`
		PickupMethod   string `json:"pickupMethod"`
		ReturnMethod   string `json:"returnMethod"`
		CustomerName   string `json:"customerName"`
		CustomerEmail  string `json:"customerEmail"`
		CustomerPhone  string `json:"customerPhone"`
		Note           string `json:"note"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลการจองไม่ถูกต้อง")
		return
	}

	car, pickupDate, returnDate, ok := rentFlowValidateBookingDatesAndCar(c, payload.CarID, payload.PickupDate, payload.ReturnDate)
	if !ok {
		return
	}

	available, err := rentFlowCarIsAvailable(car.ID, pickupDate, returnDate)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
		return
	}
	if !available {
		rentFlowError(c, http.StatusConflict, "รถคันนี้ถูกจองในช่วงเวลาที่เลือกแล้ว")
		return
	}

	customerEmail := strings.TrimSpace(strings.ToLower(payload.CustomerEmail))
	if customerEmail == "" || strings.TrimSpace(payload.CustomerName) == "" || strings.TrimSpace(payload.CustomerPhone) == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกข้อมูลผู้จองให้ครบถ้วน")
		return
	}

	totalDays, subtotal, extraCharge, discount, totalAmount := services.ComputeBookingPrice(
		car.PricePerDay,
		pickupDate,
		returnDate,
		payload.PickupLocation,
		payload.ReturnLocation,
	)

	booking := models.RentFlowBooking{
		ID:             services.NewID("bok"),
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
		ExtraCharge:    extraCharge,
		Discount:       discount,
		TotalAmount:    totalAmount,
		Note:           strings.TrimSpace(payload.Note),
		CustomerName:   strings.TrimSpace(payload.CustomerName),
		CustomerEmail:  customerEmail,
		CustomerPhone:  strings.TrimSpace(payload.CustomerPhone),
		UserEmail:      customerEmail,
	}

	if user, ok := middleware.CurrentRentFlowUser(c); ok {
		booking.UserID = &user.ID
		booking.UserEmail = user.Email
	}

	if err := config.DB.Create(&booking).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างรายการจองได้")
		return
	}

	if booking.UserEmail != "" {
		rentFlowCreateNotification(booking.UserID, booking.UserEmail, "สร้างการจองใหม่", "การจอง "+booking.BookingCode+" ถูกสร้างเรียบร้อยแล้ว")
	}

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowSuccess(c, http.StatusCreated, "สร้างรายการจองสำเร็จ", booking)
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

	rentFlowSuccess(c, http.StatusOK, "ดึงรายการจองสำเร็จ", bookings)
}

func RentFlowGetBookingByID(c *gin.Context) {
	booking, ok := rentFlowLoadOwnedBooking(c, c.Param("bookingId"))
	if !ok {
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลการจองสำเร็จ", booking)
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
	rentFlowCreateNotification(booking.UserID, booking.CustomerEmail, "ยกเลิกการจอง", "การจอง "+booking.BookingCode+" ถูกยกเลิกแล้ว")
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowSuccess(c, http.StatusOK, "ยกเลิกรายการจองสำเร็จ", booking)
}

func RentFlowCreatePayment(c *gin.Context) {
	var payload struct {
		BookingID string `json:"bookingId"`
		Method    string `json:"method"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลการชำระเงินไม่ถูกต้อง")
		return
	}

	var booking models.RentFlowBooking
	if err := config.DB.Where("id = ? OR booking_code = ?", payload.BookingID, payload.BookingID).First(&booking).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรายการจองที่ต้องการชำระเงิน")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหารายการจองได้")
		return
	}

	method := strings.TrimSpace(payload.Method)
	if method == "bank_transfer" {
		method = "bank_transfer"
	}
	if method == "" {
		method = "promptpay"
	}

	payment := models.RentFlowPayment{
		ID:            services.NewID("pay"),
		BookingID:     booking.ID,
		Method:        method,
		Status:        "paid",
		Amount:        booking.TotalAmount,
		TransactionID: services.NewID("txn"),
	}
	if method == "promptpay" {
		payment.QRCodeURL = "/QR-CODE.jpg"
	}
	if method == "card" {
		payment.PaymentURL = "https://payments.example.com/checkout/" + payment.TransactionID
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

	rentFlowCreateNotification(booking.UserID, booking.CustomerEmail, "ชำระเงินสำเร็จ", "การจอง "+booking.BookingCode+" ชำระเงินเรียบร้อยแล้ว")
	rentFlowSuccess(c, http.StatusCreated, "สร้างรายการชำระเงินสำเร็จ", payment)
}

func RentFlowGetPaymentByBookingID(c *gin.Context) {
	var payment models.RentFlowPayment
	if err := config.DB.Where("booking_id = ?", c.Param("bookingId")).Order("created_at DESC").First(&payment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ยังไม่มีข้อมูลการชำระเงินสำหรับการจองนี้")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการชำระเงินได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลการชำระเงินสำเร็จ", payment)
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

func rentFlowValidateBookingDatesAndCar(c *gin.Context, carID, pickupDateRaw, returnDateRaw string) (models.RentFlowCar, time.Time, time.Time, bool) {
	var car models.RentFlowCar
	if err := config.DB.Where("id = ? AND is_available = ?", carID, true).First(&car).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรถที่ต้องการ")
		return models.RentFlowCar{}, time.Time{}, time.Time{}, false
	}

	pickupDate, err := services.ParseDateTime(pickupDateRaw)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "pickupDate ไม่ถูกต้อง")
		return models.RentFlowCar{}, time.Time{}, time.Time{}, false
	}
	returnDate, err := services.ParseDateTime(returnDateRaw)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "returnDate ไม่ถูกต้อง")
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

func rentFlowCreateNotification(userID *string, userEmail, title, message string) {
	if strings.TrimSpace(userEmail) == "" {
		return
	}
	notification := models.RentFlowNotification{
		ID:        services.NewID("ntf"),
		UserID:    userID,
		UserEmail: strings.TrimSpace(strings.ToLower(userEmail)),
		Title:     title,
		Message:   message,
		IsRead:    false,
	}
	_ = config.DB.Create(&notification).Error
}
