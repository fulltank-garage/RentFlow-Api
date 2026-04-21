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
	"rentflow-api/models"
	"rentflow-api/services"
)

type rentFlowPartnerCarPayload struct {
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Type         string `json:"type"`
	Seats        int    `json:"seats"`
	Transmission string `json:"transmission"`
	Fuel         string `json:"fuel"`
	PricePerDay  int64  `json:"pricePerDay"`
	Description  string `json:"description"`
	LocationID   string `json:"locationId"`
	Status       string `json:"status"`
	IsAvailable  *bool  `json:"isAvailable"`
}

type rentFlowPartnerBranchPayload struct {
	Name            string  `json:"name"`
	Address         string  `json:"address"`
	Phone           string  `json:"phone"`
	LocationID      string  `json:"locationId"`
	Type            string  `json:"type"`
	DisplayOrder    int     `json:"displayOrder"`
	Lat             float64 `json:"lat"`
	Lng             float64 `json:"lng"`
	OpenTime        string  `json:"openTime"`
	CloseTime       string  `json:"closeTime"`
	PickupAvailable *bool   `json:"pickupAvailable"`
	ReturnAvailable *bool   `json:"returnAvailable"`
	ExtraFee        int64   `json:"extraFee"`
	IsActive        *bool   `json:"isActive"`
}

func RentFlowPartnerDashboard(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var cars []models.RentFlowCar
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&cars).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรถได้")
		return
	}

	var branches []models.RentFlowBranch
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("display_order ASC, name ASC").Find(&branches).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสาขาได้")
		return
	}

	var bookings []models.RentFlowBooking
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Limit(100).Find(&bookings).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลการจองได้")
		return
	}

	var reviews []models.RentFlowReview
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Limit(20).Find(&reviews).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรีวิวได้")
		return
	}

	var payments []models.RentFlowPayment
	if err := config.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "paid").Find(&payments).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรายได้ได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลแดชบอร์ดสำเร็จ", rentFlowBuildPartnerDashboard(c, tenant, cars, branches, bookings, payments, reviews))
}

func RentFlowPartnerGetCars(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var cars []models.RentFlowCar
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&cars).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรถได้")
		return
	}

	imageURLs, err := rentFlowCarImageURLs(c, tenant, cars)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรูปภาพรถได้")
		return
	}

	items := make([]gin.H, 0, len(cars))
	for _, car := range cars {
		items = append(items, rentFlowPartnerCarResponse(c, tenant, car, imageURLs[car.ID]))
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลรถสำเร็จ", gin.H{
		"items": items,
		"total": len(items),
	})
}

func RentFlowPartnerCreateCar(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload rentFlowPartnerCarPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลรถไม่ถูกต้อง")
		return
	}

	car, ok := rentFlowBuildPartnerCarFromPayload(c, tenant.ID, "", payload)
	if !ok {
		return
	}

	if err := config.DB.Create(&car).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเพิ่มรถได้")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())

	rentFlowSuccess(c, http.StatusCreated, "เพิ่มรถสำเร็จ", rentFlowPartnerCarResponse(c, tenant, car, nil))
}

func RentFlowPartnerUpdateCar(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var existing models.RentFlowCar
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("carId")).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรถที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาข้อมูลรถได้")
		return
	}

	var payload rentFlowPartnerCarPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลรถไม่ถูกต้อง")
		return
	}

	updated, ok := rentFlowBuildPartnerCarFromPayload(c, tenant.ID, existing.ID, payload)
	if !ok {
		return
	}

	updates := map[string]interface{}{
		"name":          updated.Name,
		"brand":         updated.Brand,
		"model":         updated.Model,
		"year":          updated.Year,
		"type":          updated.Type,
		"seats":         updated.Seats,
		"transmission":  updated.Transmission,
		"fuel":          updated.Fuel,
		"price_per_day": updated.PricePerDay,
		"description":   updated.Description,
		"location_id":   updated.LocationID,
		"status":        updated.Status,
		"is_available":  updated.IsAvailable,
		"updated_at":    time.Now(),
	}

	if err := config.DB.Model(&models.RentFlowCar{}).Where("tenant_id = ? AND id = ?", tenant.ID, existing.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลรถได้")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())

	existing.Name = updated.Name
	existing.Brand = updated.Brand
	existing.Model = updated.Model
	existing.Year = updated.Year
	existing.Type = updated.Type
	existing.Seats = updated.Seats
	existing.Transmission = updated.Transmission
	existing.Fuel = updated.Fuel
	existing.PricePerDay = updated.PricePerDay
	existing.Description = updated.Description
	existing.LocationID = updated.LocationID
	existing.Status = updated.Status
	existing.IsAvailable = updated.IsAvailable

	imageURLs, _ := rentFlowCarImageURLs(c, tenant, []models.RentFlowCar{existing})
	rentFlowSuccess(c, http.StatusOK, "บันทึกข้อมูลรถสำเร็จ", rentFlowPartnerCarResponse(c, tenant, existing, imageURLs[existing.ID]))
}

func RentFlowPartnerDeleteCar(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	carID := strings.TrimSpace(c.Param("carId"))
	tx := config.DB.Begin()
	if tx.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเริ่มลบรถได้")
		return
	}

	if err := tx.Where("tenant_id = ? AND car_id = ?", tenant.ID, carID).Delete(&models.RentFlowCarImage{}).Error; err != nil {
		tx.Rollback()
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรูปภาพรถได้")
		return
	}
	result := tx.Where("tenant_id = ? AND id = ?", tenant.ID, carID).Delete(&models.RentFlowCar{})
	if result.Error != nil {
		tx.Rollback()
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรถได้")
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		rentFlowError(c, http.StatusNotFound, "ไม่พบรถที่ต้องการ")
		return
	}
	if err := tx.Commit().Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรถได้")
		return
	}

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowSuccess(c, http.StatusOK, "ลบรถสำเร็จ", nil)
}

func RentFlowPartnerDeleteCarImage(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	result := config.DB.
		Where("tenant_id = ? AND car_id = ? AND id = ?", tenant.ID, c.Param("carId"), c.Param("imageId")).
		Delete(&models.RentFlowCarImage{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรูปภาพรถได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปภาพรถที่ต้องการ")
		return
	}

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowSuccess(c, http.StatusOK, "ลบรูปภาพรถสำเร็จ", nil)
}

func RentFlowPartnerReorderCarImages(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload struct {
		ImageIDs []string `json:"imageIds"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || len(payload.ImageIDs) == 0 {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลลำดับรูปภาพไม่ถูกต้อง")
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเริ่มอัปเดตรูปภาพได้")
		return
	}
	for index, imageID := range payload.ImageIDs {
		if err := tx.Model(&models.RentFlowCarImage{}).
			Where("tenant_id = ? AND car_id = ? AND id = ?", tenant.ID, c.Param("carId"), imageID).
			Updates(map[string]interface{}{"sort_order": index, "updated_at": time.Now()}).Error; err != nil {
			tx.Rollback()
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตลำดับรูปภาพได้")
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตลำดับรูปภาพได้")
		return
	}

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	rentFlowAudit(c, tenant.ID, "car_image.reorder", "car", c.Param("carId"), strings.Join(payload.ImageIDs, ","))
	rentFlowSuccess(c, http.StatusOK, "อัปเดตลำดับรูปภาพสำเร็จ", nil)
}

func RentFlowPartnerGetBranches(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var branches []models.RentFlowBranch
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("display_order ASC, name ASC").Find(&branches).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสาขาได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลสาขาสำเร็จ", gin.H{
		"items": branches,
		"total": len(branches),
	})
}

func RentFlowPartnerCreateBranch(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var payload rentFlowPartnerBranchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสาขาไม่ถูกต้อง")
		return
	}

	branch, ok := rentFlowBuildPartnerBranchFromPayload(c, tenant.ID, "", payload)
	if !ok {
		return
	}

	if err := config.DB.Create(&branch).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเพิ่มสาขาได้")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowBranchesCachePrefix())

	rentFlowSuccess(c, http.StatusCreated, "เพิ่มสาขาสำเร็จ", branch)
}

func RentFlowPartnerUpdateBranch(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	var existing models.RentFlowBranch
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("branchId")).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบสาขาที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาข้อมูลสาขาได้")
		return
	}

	var payload rentFlowPartnerBranchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสาขาไม่ถูกต้อง")
		return
	}

	updated, ok := rentFlowBuildPartnerBranchFromPayload(c, tenant.ID, existing.ID, payload)
	if !ok {
		return
	}

	updates := map[string]interface{}{
		"name":             updated.Name,
		"address":          updated.Address,
		"phone":            updated.Phone,
		"location_id":      updated.LocationID,
		"type":             updated.Type,
		"display_order":    updated.DisplayOrder,
		"lat":              updated.Lat,
		"lng":              updated.Lng,
		"open_time":        updated.OpenTime,
		"close_time":       updated.CloseTime,
		"pickup_available": updated.PickupAvailable,
		"return_available": updated.ReturnAvailable,
		"extra_fee":        updated.ExtraFee,
		"is_active":        updated.IsActive,
		"updated_at":       time.Now(),
	}

	if err := config.DB.Model(&models.RentFlowBranch{}).Where("tenant_id = ? AND id = ?", tenant.ID, existing.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกข้อมูลสาขาได้")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowBranchesCachePrefix())

	updated.CreatedAt = existing.CreatedAt
	rentFlowSuccess(c, http.StatusOK, "บันทึกข้อมูลสาขาสำเร็จ", updated)
}

func RentFlowPartnerDeleteBranch(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}

	result := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("branchId")).Delete(&models.RentFlowBranch{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบสาขาได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบสาขาที่ต้องการ")
		return
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowBranchesCachePrefix())

	rentFlowSuccess(c, http.StatusOK, "ลบสาขาสำเร็จ", nil)
}

func rentFlowRequireOwnerTenant(c *gin.Context) (*models.RentFlowTenant, bool) {
	tenant, err := rentFlowCurrentUserTenant(c)
	if err == nil {
		return tenant, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rentFlowError(c, http.StatusBadRequest, "กรุณาตั้งค่าร้านก่อนใช้งาน")
		return nil, false
	}
	rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบข้อมูลร้านได้")
	return nil, false
}

func rentFlowBuildPartnerCarFromPayload(c *gin.Context, tenantID, carID string, payload rentFlowPartnerCarPayload) (models.RentFlowCar, bool) {
	name := strings.TrimSpace(payload.Name)
	brand := strings.TrimSpace(payload.Brand)
	model := strings.TrimSpace(payload.Model)
	carType := strings.TrimSpace(payload.Type)
	transmission := strings.TrimSpace(payload.Transmission)
	fuel := strings.TrimSpace(payload.Fuel)
	status := rentFlowNormalizeCarStatus(payload.Status)

	if name == "" || brand == "" || model == "" || payload.Year < 1980 || carType == "" || payload.Seats < 1 || transmission == "" || fuel == "" || payload.PricePerDay < 0 {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกข้อมูลรถให้ครบถ้วน")
		return models.RentFlowCar{}, false
	}
	if carID == "" {
		carID = services.NewID("car")
	}

	isAvailable := status == "available"
	if payload.IsAvailable != nil {
		isAvailable = *payload.IsAvailable && status == "available"
	}

	locationID := strings.TrimSpace(payload.LocationID)
	if locationID == "" {
		locationID = "default"
	}

	return models.RentFlowCar{
		ID:           carID,
		TenantID:     tenantID,
		Name:         name,
		Brand:        brand,
		Model:        model,
		Year:         payload.Year,
		Type:         carType,
		Seats:        payload.Seats,
		Transmission: transmission,
		Fuel:         fuel,
		PricePerDay:  payload.PricePerDay,
		Description:  strings.TrimSpace(payload.Description),
		LocationID:   locationID,
		Status:       status,
		IsAvailable:  isAvailable,
	}, true
}

func rentFlowBuildPartnerBranchFromPayload(c *gin.Context, tenantID, branchID string, payload rentFlowPartnerBranchPayload) (models.RentFlowBranch, bool) {
	name := strings.TrimSpace(payload.Name)
	address := strings.TrimSpace(payload.Address)
	if name == "" || address == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อสาขาและที่อยู่")
		return models.RentFlowBranch{}, false
	}

	pickupAvailable := true
	if payload.PickupAvailable != nil {
		pickupAvailable = *payload.PickupAvailable
	}
	returnAvailable := true
	if payload.ReturnAvailable != nil {
		returnAvailable = *payload.ReturnAvailable
	}
	if !pickupAvailable && !returnAvailable {
		rentFlowError(c, http.StatusBadRequest, "อย่างน้อยต้องเปิดรับรถหรือคืนรถอย่างใดอย่างหนึ่ง")
		return models.RentFlowBranch{}, false
	}

	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}

	branchType := strings.TrimSpace(payload.Type)
	if branchType == "" {
		branchType = "storefront"
	}
	if branchType != "airport" && branchType != "storefront" && branchType != "meeting_point" {
		branchType = "storefront"
	}

	if branchID == "" {
		branchID = services.NewID("brn")
	}
	locationID := strings.TrimSpace(payload.LocationID)
	if locationID == "" {
		locationID = branchID
	}
	displayOrder := payload.DisplayOrder
	if displayOrder < 1 {
		displayOrder = 1
	}

	return models.RentFlowBranch{
		ID:              branchID,
		TenantID:        tenantID,
		Name:            name,
		Address:         address,
		Phone:           strings.TrimSpace(payload.Phone),
		LocationID:      locationID,
		Type:            branchType,
		DisplayOrder:    displayOrder,
		Lat:             payload.Lat,
		Lng:             payload.Lng,
		OpenTime:        strings.TrimSpace(payload.OpenTime),
		CloseTime:       strings.TrimSpace(payload.CloseTime),
		PickupAvailable: pickupAvailable,
		ReturnAvailable: returnAvailable,
		ExtraFee:        payload.ExtraFee,
		IsActive:        isActive,
	}, true
}

func rentFlowNormalizeCarStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "rented", "maintenance", "hidden":
		return strings.TrimSpace(strings.ToLower(status))
	default:
		return "available"
	}
}

func rentFlowPartnerCarResponse(c *gin.Context, tenant *models.RentFlowTenant, car models.RentFlowCar, images []string) gin.H {
	primaryImage := ""
	if len(images) > 0 {
		primaryImage = images[0]
	}

	return gin.H{
		"id":           car.ID,
		"tenantId":     car.TenantID,
		"name":         car.Name,
		"brand":        car.Brand,
		"model":        car.Model,
		"year":         car.Year,
		"type":         car.Type,
		"seats":        car.Seats,
		"transmission": car.Transmission,
		"fuel":         car.Fuel,
		"pricePerDay":  car.PricePerDay,
		"description":  car.Description,
		"locationId":   car.LocationID,
		"status":       car.Status,
		"isAvailable":  car.IsAvailable,
		"image":        primaryImage,
		"imageUrl":     primaryImage,
		"images":       images,
		"createdAt":    car.CreatedAt,
		"updatedAt":    car.UpdatedAt,
	}
}

func rentFlowBuildPartnerDashboard(c *gin.Context, tenant *models.RentFlowTenant, cars []models.RentFlowCar, branches []models.RentFlowBranch, bookings []models.RentFlowBooking, payments []models.RentFlowPayment, reviews []models.RentFlowReview) gin.H {
	carNameByID := make(map[string]string, len(cars))
	fleetStatus := map[string]int{"available": 0, "rented": 0, "maintenance": 0, "hidden": 0}
	for _, car := range cars {
		carNameByID[car.ID] = car.Name
		status := rentFlowNormalizeCarStatus(car.Status)
		fleetStatus[status]++
	}

	activeBranches := 0
	for _, branch := range branches {
		if branch.IsActive {
			activeBranches++
		}
	}

	totalRevenue := int64(0)
	for _, payment := range payments {
		totalRevenue += payment.Amount
	}

	statusCounts := map[string]int{"pending": 0, "confirmed": 0, "paid": 0, "completed": 0, "cancelled": 0}
	todayPickups := 0
	todayReturns := 0
	now := time.Now()
	today := now.Format("2006-01-02")
	for _, booking := range bookings {
		statusCounts[booking.Status]++
		if booking.PickupDate.Format("2006-01-02") == today {
			todayPickups++
		}
		if booking.ReturnDate.Format("2006-01-02") == today {
			todayReturns++
		}
	}

	revenueByBookingID := map[string]int64{}
	for _, payment := range payments {
		revenueByBookingID[payment.BookingID] += payment.Amount
	}

	topCarStats := map[string]gin.H{}
	for _, booking := range bookings {
		row, ok := topCarStats[booking.CarID]
		if !ok {
			row = gin.H{
				"id":       booking.CarID,
				"name":     carNameByID[booking.CarID],
				"bookings": 0,
				"revenue":  int64(0),
			}
			if row["name"] == "" {
				row["name"] = booking.CarID
			}
			topCarStats[booking.CarID] = row
		}
		row["bookings"] = row["bookings"].(int) + 1
		row["revenue"] = row["revenue"].(int64) + booking.TotalAmount
	}

	topCars := make([]gin.H, 0, len(topCarStats))
	for _, row := range topCarStats {
		topCars = append(topCars, row)
	}
	sort.Slice(topCars, func(i, j int) bool {
		return topCars[i]["revenue"].(int64) > topCars[j]["revenue"].(int64)
	})
	if len(topCars) > 5 {
		topCars = topCars[:5]
	}

	recentBookings := make([]gin.H, 0, min(len(bookings), 6))
	for _, booking := range bookings {
		if len(recentBookings) >= 6 {
			break
		}
		recentBookings = append(recentBookings, gin.H{
			"id":           booking.ID,
			"bookingCode":  booking.BookingCode,
			"carId":        booking.CarID,
			"carName":      carNameByID[booking.CarID],
			"customerName": booking.CustomerName,
			"pickupDate":   booking.PickupDate,
			"returnDate":   booking.ReturnDate,
			"status":       booking.Status,
			"totalAmount":  booking.TotalAmount,
			"revenue":      revenueByBookingID[booking.ID],
			"createdAt":    booking.CreatedAt,
		})
	}

	weeklySales := rentFlowBuildWeeklySales(bookings)

	return gin.H{
		"tenant": rentFlowOwnerTenantResponse(*tenant),
		"summary": gin.H{
			"totalCars":      len(cars),
			"availableCars":  fleetStatus["available"],
			"totalBranches":  len(branches),
			"activeBranches": activeBranches,
			"totalBookings":  len(bookings),
			"todayPickups":   todayPickups,
			"todayReturns":   todayReturns,
			"totalRevenue":   totalRevenue,
			"totalReviews":   len(reviews),
		},
		"bookingStatus":  statusCounts,
		"fleetStatus":    fleetStatus,
		"weeklySales":    weeklySales,
		"topCars":        topCars,
		"recentBookings": recentBookings,
		"recentReviews":  reviews,
	}
}

func rentFlowBuildWeeklySales(bookings []models.RentFlowBooking) []gin.H {
	now := time.Now()
	rows := make([]gin.H, 0, 7)
	indexByDay := map[string]int{}

	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		indexByDay[key] = len(rows)
		rows = append(rows, gin.H{
			"day":      day.Format("02 Jan"),
			"key":      key,
			"bookings": 0,
			"revenue":  int64(0),
		})
	}

	for _, booking := range bookings {
		key := booking.CreatedAt.Format("2006-01-02")
		index, ok := indexByDay[key]
		if !ok {
			continue
		}
		rows[index]["bookings"] = rows[index]["bookings"].(int) + 1
		rows[index]["revenue"] = rows[index]["revenue"].(int64) + booking.TotalAmount
	}

	return rows
}
