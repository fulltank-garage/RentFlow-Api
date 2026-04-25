package controllers

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

type rentFlowCarsResponse struct {
	Items []gin.H `json:"items"`
	Total int     `json:"total"`
}

const (
	rentFlowMaxCarImageBytes    = 5 * 1024 * 1024
	rentFlowMaxUploadImageCount = 10
)

var rentFlowAllowedImageTypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

type rentFlowCarAvailabilitySnapshot struct {
	UnitCount      int
	ReservedUnits  int
	AvailableUnits int
	Available      bool
}

func RentFlowGetCars(c *gin.Context) {
	marketplace := rentFlowIsMarketplaceRequest(c)

	tenantMap := make(map[string]models.RentFlowTenant)
	tenantIDs := make([]string, 0)
	cacheScope := ""
	if marketplace {
		tenants, err := rentFlowMarketplaceTenants()
		if err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลร้านได้")
			return
		}
		tenantMap = rentFlowTenantMap(tenants)
		for _, tenant := range tenants {
			tenantIDs = append(tenantIDs, tenant.ID)
		}
		cacheScope = "marketplace"
	} else {
		tenant, ok := rentFlowRequireTenant(c)
		if !ok {
			return
		}
		tenantMap[tenant.ID] = *tenant
		tenantIDs = append(tenantIDs, tenant.ID)
		cacheScope = tenant.ID
	}

	cacheKey := services.CacheKey(
		services.RentFlowCarsCachePrefix(),
		cacheScope,
		c.Query("q"),
		c.Query("type"),
		c.Query("location"),
		c.Query("pickupDate"),
		c.Query("returnDate"),
		c.Query("sort"),
	)

	var cached rentFlowCarsResponse
	if services.CacheGetJSON(config.Ctx, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	if len(tenantIDs) == 0 {
		response := rentFlowCarsResponse{
			Items: []gin.H{},
			Total: 0,
		}
		services.CacheSetJSON(config.Ctx, cacheKey, response, 5*time.Minute)
		c.JSON(http.StatusOK, response)
		return
	}

	var cars []models.RentFlowCar
	query := config.DB.Where(
		"tenant_id IN ? AND ((is_available = ? AND status = ?) OR status = ?)",
		tenantIDs,
		true,
		"available",
		"rented",
	)

	if location := strings.TrimSpace(c.Query("location")); location != "" {
		locationIDs := rentFlowLocationIDsForFilter(tenantIDs, location)
		if len(locationIDs) > 0 {
			query = query.Where("location_id IN ?", locationIDs)
		} else {
			query = query.Where("location_id = ?", location)
		}
	}

	if carType := strings.TrimSpace(c.Query("type")); carType != "" && !strings.EqualFold(carType, "all") {
		query = query.Where(`LOWER(type) = ?`, strings.ToLower(carType))
	}

	search := strings.TrimSpace(strings.ToLower(c.Query("q")))
	if search != "" {
		searchLike := "%" + search + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(brand) LIKE ? OR LOWER(model) LIKE ? OR LOWER(type) LIKE ?",
			searchLike,
			searchLike,
			searchLike,
			searchLike,
		)
	}

	if sortKey := strings.TrimSpace(c.Query("sort")); sortKey == "price_desc" {
		query = query.Order("price_per_day DESC")
	} else {
		query = query.Order("price_per_day ASC")
	}

	if err := query.Find(&cars).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลรถได้")
		return
	}

	pickupDate, pickupErr := services.ParseDateTime(c.Query("pickupDate"))
	returnDate, returnErr := services.ParseDateTime(c.Query("returnDate"))

	visibleCars := make([]models.RentFlowCar, 0, len(cars))
	availabilityByCarID := make(map[string]rentFlowCarAvailabilitySnapshot, len(cars))
	for _, car := range cars {
		snapshot := rentFlowBaseCarAvailability(car)
		if pickupErr == nil && returnErr == nil && pickupDate.Before(returnDate) {
			nextSnapshot, err := rentFlowCarAvailability(car.TenantID, car, pickupDate, returnDate)
			if err != nil {
				rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
				return
			}
			snapshot = nextSnapshot
		} else {
			now := time.Now()
			nextSnapshot, err := rentFlowCarAvailability(car.TenantID, car, now, now.Add(24*time.Hour))
			if err != nil {
				rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
				return
			}
			snapshot = nextSnapshot
		}

		availabilityByCarID[car.ID] = snapshot
		visibleCars = append(visibleCars, car)
	}

	carImageURLs, err := rentFlowCarImageURLsForTenants(c, tenantMap, visibleCars)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรูปภาพรถได้")
		return
	}

	responseItems := make([]gin.H, 0, len(visibleCars))
	for _, car := range visibleCars {
		tenant := tenantMap[car.TenantID]
		images := carImageURLs[car.ID]
		primaryImage := ""
		if len(images) > 0 {
			primaryImage = images[0]
		}
		availability := availabilityByCarID[car.ID]

		responseItems = append(responseItems, gin.H{
			"id":                  car.ID,
			"tenantId":            car.TenantID,
			"name":                car.Name,
			"image":               primaryImage,
			"brand":               car.Brand,
			"model":               car.Model,
			"year":                car.Year,
			"type":                car.Type,
			"seats":               car.Seats,
			"transmission":        car.Transmission,
			"fuel":                car.Fuel,
			"grade":               rentFlowCarGrade(car.ID),
			"pricePerDay":         car.PricePerDay,
			"unitCount":           availability.UnitCount,
			"reservedUnits":       availability.ReservedUnits,
			"availableUnits":      availability.AvailableUnits,
			"imageUrl":            primaryImage,
			"images":              images,
			"description":         car.Description,
			"locationId":          car.LocationID,
			"isAvailable":         availability.Available,
			"status":              car.Status,
			"createdAt":           car.CreatedAt,
			"updatedAt":           car.UpdatedAt,
			"shopName":            tenant.ShopName,
			"domainSlug":          tenant.DomainSlug,
			"publicDomain":        tenant.PublicDomain,
			"logoUrl":             rentFlowTenantLogoURL(tenant),
			"promoImageUrl":       rentFlowTenantPromoImageURL(tenant),
			"bookingMode":         rentFlowNormalizeBookingMode(tenant.BookingMode),
			"lineOfficialAccount": rentFlowPublicLineSummary(car.TenantID),
		})
	}

	response := rentFlowCarsResponse{
		Items: responseItems,
		Total: len(responseItems),
	}
	services.CacheSetJSON(config.Ctx, cacheKey, response, 5*time.Minute)
	c.JSON(http.StatusOK, response)
}

func RentFlowGetCarPrimaryImage(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var image models.RentFlowCarImage
	if err := config.DB.
		Where("tenant_id = ? AND car_id = ?", tenant.ID, c.Param("carId")).
		Order("sort_order ASC").
		First(&image).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรูปภาพรถที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรูปภาพรถได้")
		return
	}

	rentFlowSendCarImage(c, image)
}

func RentFlowGetCarImage(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var image models.RentFlowCarImage
	if err := config.DB.
		Where("tenant_id = ? AND car_id = ? AND id = ?", tenant.ID, c.Param("carId"), c.Param("imageId")).
		First(&image).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรูปภาพรถที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรูปภาพรถได้")
		return
	}

	rentFlowSendCarImage(c, image)
}

func RentFlowUploadCarImages(c *gin.Context) {
	tenant, err := rentFlowCurrentUserTenant(c)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusBadRequest, "กรุณาตั้งค่าร้านก่อนอัปโหลดรูปภาพ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบข้อมูลร้านได้")
		return
	}

	carID := strings.TrimSpace(c.Param("carId"))
	var car models.RentFlowCar
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, carID).First(&car).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรถที่ต้องการอัปโหลดรูป")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาข้อมูลรถได้")
		return
	}

	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลไฟล์รูปภาพไม่ถูกต้อง")
		return
	}

	files := rentFlowUploadedImageFiles(c.Request.MultipartForm)
	if len(files) == 0 {
		rentFlowError(c, http.StatusBadRequest, "กรุณาแนบไฟล์รูปภาพ")
		return
	}
	if len(files) > rentFlowMaxUploadImageCount {
		rentFlowError(c, http.StatusBadRequest, "อัปโหลดรูปภาพได้ครั้งละไม่เกิน 10 รูป")
		return
	}

	replaceImages := strings.EqualFold(strings.TrimSpace(c.Query("replace")), "true")
	maxSortOrder, err := rentFlowCurrentMaxImageSortOrder(tenant.ID, carID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบลำดับรูปภาพได้")
		return
	}
	if replaceImages {
		maxSortOrder = -1
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเริ่มบันทึกรูปภาพได้")
		return
	}

	if replaceImages {
		if err := tx.Where("tenant_id = ? AND car_id = ?", tenant.ID, carID).Delete(&models.RentFlowCarImage{}).Error; err != nil {
			tx.Rollback()
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรูปภาพเดิมได้")
			return
		}
	}

	images := make([]models.RentFlowCarImage, 0, len(files))
	for index, fileHeader := range files {
		image, err := rentFlowBuildCarImage(tenant.ID, carID, maxSortOrder+index+1, fileHeader)
		if err != nil {
			tx.Rollback()
			rentFlowError(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := tx.Create(&image).Error; err != nil {
			tx.Rollback()
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปภาพได้")
			return
		}
		images = append(images, image)
	}

	if err := tx.Commit().Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปภาพได้")
		return
	}

	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())
	items := make([]gin.H, 0, len(images))
	for _, image := range images {
		items = append(items, rentFlowCarImageResponse(c, tenant, image))
	}

	rentFlowPublishCarRealtime(tenant.ID, carID, services.RentFlowRealtimeEventCarChanged)
	rentFlowSuccess(c, http.StatusCreated, "อัปโหลดรูปภาพสำเร็จ", gin.H{
		"items": items,
		"total": len(items),
	})
}

func RentFlowGetBranches(c *gin.Context) {
	marketplace := rentFlowIsMarketplaceRequest(c)
	cacheScope := ""
	tenantMap := make(map[string]models.RentFlowTenant)
	tenantIDs := make([]string, 0)
	if marketplace {
		tenants, err := rentFlowMarketplaceTenants()
		if err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลร้านได้")
			return
		}
		tenantMap = rentFlowTenantMap(tenants)
		for _, tenant := range tenants {
			tenantIDs = append(tenantIDs, tenant.ID)
		}
		cacheScope = "marketplace"
	} else {
		tenant, ok := rentFlowRequireTenant(c)
		if !ok {
			return
		}
		tenantMap[tenant.ID] = *tenant
		tenantIDs = append(tenantIDs, tenant.ID)
		cacheScope = tenant.ID
	}

	cacheKey := services.CacheKey(services.RentFlowBranchesCachePrefix(), cacheScope, "all")
	var cached rentFlowAPIResponse
	if services.CacheGetJSON(config.Ctx, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	if len(tenantIDs) == 0 {
		response := rentFlowAPIResponse{
			Success: true,
			Message: "ดึงข้อมูลสาขาสำเร็จ",
			Data:    []gin.H{},
		}
		services.CacheSetJSON(config.Ctx, cacheKey, response, 30*time.Minute)
		c.JSON(http.StatusOK, response)
		return
	}

	var branches []models.RentFlowBranch
	if err := config.DB.Where("tenant_id IN ? AND is_active = ?", tenantIDs, true).Order("name ASC").Find(&branches).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสาขาได้")
		return
	}

	responseItems := make([]gin.H, 0, len(branches))
	for _, branch := range branches {
		tenant := tenantMap[branch.TenantID]
		responseItems = append(responseItems, gin.H{
			"id":            branch.ID,
			"tenantId":      branch.TenantID,
			"name":          rentFlowBranchDisplayName(branch),
			"rawName":       branch.Name,
			"address":       branch.Address,
			"phone":         branch.Phone,
			"locationId":    branch.LocationID,
			"lat":           branch.Lat,
			"lng":           branch.Lng,
			"openTime":      branch.OpenTime,
			"closeTime":     branch.CloseTime,
			"isActive":      branch.IsActive,
			"shopName":      tenant.ShopName,
			"domainSlug":    tenant.DomainSlug,
			"publicDomain":  tenant.PublicDomain,
			"logoUrl":       rentFlowTenantLogoURL(tenant),
			"promoImageUrl": rentFlowTenantPromoImageURL(tenant),
		})
	}

	response := rentFlowAPIResponse{
		Success: true,
		Message: "ดึงข้อมูลสาขาสำเร็จ",
		Data:    responseItems,
	}
	services.CacheSetJSON(config.Ctx, cacheKey, response, 30*time.Minute)
	c.JSON(http.StatusOK, response)
}

func rentFlowLocationIDsForFilter(tenantIDs []string, location string) []string {
	location = strings.TrimSpace(location)
	if location == "" || len(tenantIDs) == 0 {
		return nil
	}

	var branches []models.RentFlowBranch
	if err := config.DB.
		Where("tenant_id IN ? AND (id = ? OR location_id = ? OR name = ?)", tenantIDs, location, location, location).
		Find(&branches).Error; err != nil {
		return nil
	}

	seen := map[string]struct{}{location: {}}
	values := []string{location}
	for _, branch := range branches {
		for _, value := range []string{branch.LocationID, branch.ID, branch.Name} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values
}

func rentFlowBranchDisplayName(branch models.RentFlowBranch) string {
	name := strings.TrimSpace(branch.Name)
	if name != "" && !rentFlowLooksGeneratedID(name, "brn") {
		return name
	}

	locationID := strings.TrimSpace(branch.LocationID)
	if locationID != "" && !rentFlowLooksGeneratedID(locationID, "brn") {
		return rentFlowThaiLocationName(locationID)
	}

	address := strings.TrimSpace(branch.Address)
	if address != "" {
		return address
	}

	if name != "" {
		return "สาขาหลัก"
	}

	return "สาขาหลัก"
}

func rentFlowLooksGeneratedID(value, prefix string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, prefix+"_") ||
		strings.HasPrefix(normalized, prefix+"-")
}

func rentFlowThaiLocationName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", "-")

	labels := map[string]string{
		"bangkok":             "กรุงเทพฯ",
		"pattaya":             "พัทยา",
		"phuket":              "ภูเก็ต",
		"chiangmai":           "เชียงใหม่",
		"chiang-mai":          "เชียงใหม่",
		"khonkaen":            "ขอนแก่น",
		"khon-kaen":           "ขอนแก่น",
		"korat":               "นครราชสีมา",
		"nakhonratchasima":    "นครราชสีมา",
		"nakhon-ratchasima":   "นครราชสีมา",
		"udonthani":           "อุดรธานี",
		"udon-thani":          "อุดรธานี",
		"suratthani":          "สุราษฎร์ธานี",
		"surat-thani":         "สุราษฎร์ธานี",
		"huahin":              "หัวหิน",
		"hua-hin":             "หัวหิน",
		"suphanburi":          "สุพรรณบุรี",
		"suphan-buri":         "สุพรรณบุรี",
		"suphanburi-downtown": "สุพรรณบุรี (ในเมือง)",
	}

	if label, ok := labels[normalized]; ok {
		return label
	}

	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == '-' || r == ' '
	})
	for index, part := range parts {
		if label, ok := labels[part]; ok {
			parts[index] = label
			continue
		}
		if part != "" {
			parts[index] = rentFlowTitleLocationPart(part)
		}
	}

	if len(parts) == 0 {
		return strings.TrimSpace(value)
	}
	return strings.Join(parts, " ")
}

func rentFlowTitleLocationPart(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}

func RentFlowGetBranchByID(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var branch models.RentFlowBranch
	if err := config.DB.Where("tenant_id = ? AND id = ? AND is_active = ?", tenant.ID, c.Param("branchId"), true).First(&branch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบสาขาที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสาขาได้")
		return
	}
	tenantValue := *tenant
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลสาขาสำเร็จ", gin.H{
		"id":            branch.ID,
		"tenantId":      branch.TenantID,
		"name":          rentFlowBranchDisplayName(branch),
		"rawName":       branch.Name,
		"address":       branch.Address,
		"phone":         branch.Phone,
		"locationId":    branch.LocationID,
		"lat":           branch.Lat,
		"lng":           branch.Lng,
		"openTime":      branch.OpenTime,
		"closeTime":     branch.CloseTime,
		"isActive":      branch.IsActive,
		"shopName":      tenantValue.ShopName,
		"domainSlug":    tenantValue.DomainSlug,
		"publicDomain":  tenantValue.PublicDomain,
		"logoUrl":       rentFlowTenantLogoURL(tenantValue),
		"promoImageUrl": rentFlowTenantPromoImageURL(tenantValue),
	})
}

func RentFlowCheckAvailability(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	var payload struct {
		CarID      string `json:"carId"`
		PickupDate string `json:"pickupDate"`
		ReturnDate string `json:"returnDate"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลตรวจสอบคิวรถไม่ถูกต้อง")
		return
	}

	pickupDate, err := services.ParseDateTime(payload.PickupDate)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "วันรับรถไม่ถูกต้อง")
		return
	}
	returnDate, err := services.ParseDateTime(payload.ReturnDate)
	if err != nil {
		rentFlowError(c, http.StatusBadRequest, "วันคืนรถไม่ถูกต้อง")
		return
	}

	var car models.RentFlowCar
	if err := config.DB.Where("tenant_id = ? AND id = ? AND is_available = ? AND status = ?", tenant.ID, payload.CarID, true, "available").First(&car).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบรถที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาข้อมูลรถได้")
		return
	}

	availability, err := rentFlowCarAvailability(tenant.ID, car, pickupDate, returnDate)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
		return
	}

	unavailableDates, err := rentFlowUnavailableDates(tenant.ID, payload.CarID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงวันไม่ว่างได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ตรวจสอบคิวรถสำเร็จ", gin.H{
		"carId":            payload.CarID,
		"available":        availability.Available,
		"unitCount":        availability.UnitCount,
		"reservedUnits":    availability.ReservedUnits,
		"availableUnits":   availability.AvailableUnits,
		"unavailableDates": unavailableDates,
	})
}

func RentFlowGetUnavailableDates(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

	dates, err := rentFlowUnavailableDates(tenant.ID, c.Param("carId"))
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงวันไม่ว่างได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงวันไม่ว่างสำเร็จ", dates)
}

func rentFlowNormalizeBookingMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "chat", "chat_first", "line", "line_chat":
		return "chat"
	default:
		return "payment"
	}
}

func rentFlowCarUnitCount(car models.RentFlowCar) int {
	if car.UnitCount < 1 {
		return 1
	}
	return car.UnitCount
}

func rentFlowBaseCarAvailability(car models.RentFlowCar) rentFlowCarAvailabilitySnapshot {
	unitCount := rentFlowCarUnitCount(car)
	available := car.IsAvailable && strings.TrimSpace(strings.ToLower(car.Status)) == "available"
	availableUnits := unitCount
	if !available {
		availableUnits = 0
	}
	return rentFlowCarAvailabilitySnapshot{
		UnitCount:      unitCount,
		ReservedUnits:  0,
		AvailableUnits: availableUnits,
		Available:      available,
	}
}

func rentFlowCarAvailability(tenantID string, car models.RentFlowCar, pickupDate, returnDate time.Time) (rentFlowCarAvailabilitySnapshot, error) {
	base := rentFlowBaseCarAvailability(car)
	if !base.Available {
		return base, nil
	}

	var reservedCount int64
	err := config.DB.Model(&models.RentFlowBooking{}).
		Where("tenant_id = ? AND car_id = ?", tenantID, car.ID).
		Where("status IN ?", []string{"pending", "confirmed", "paid"}).
		Where("pickup_date < ? AND return_date > ?", returnDate, pickupDate).
		Count(&reservedCount).Error
	if err != nil {
		return base, err
	}

	var blockCount int64
	err = config.DB.Model(&models.RentFlowAvailabilityBlock{}).
		Where("tenant_id = ? AND (car_id = ? OR car_id = '')", tenantID, car.ID).
		Where("start_date < ? AND end_date > ?", returnDate, pickupDate).
		Count(&blockCount).Error
	if err != nil {
		return base, err
	}
	if blockCount > 0 {
		return rentFlowCarAvailabilitySnapshot{
			UnitCount:      base.UnitCount,
			ReservedUnits:  int(reservedCount),
			AvailableUnits: 0,
			Available:      false,
		}, nil
	}

	availableUnits := base.UnitCount - int(reservedCount)
	if availableUnits < 0 {
		availableUnits = 0
	}
	return rentFlowCarAvailabilitySnapshot{
		UnitCount:      base.UnitCount,
		ReservedUnits:  int(reservedCount),
		AvailableUnits: availableUnits,
		Available:      availableUnits > 0,
	}, nil
}

func rentFlowCarIsAvailable(tenantID, carID string, pickupDate, returnDate time.Time) (bool, error) {
	var car models.RentFlowCar
	if err := config.DB.Where("tenant_id = ? AND id = ?", tenantID, carID).First(&car).Error; err != nil {
		return false, err
	}
	availability, err := rentFlowCarAvailability(tenantID, car, pickupDate, returnDate)
	return availability.Available, err
}

func rentFlowUnavailableDates(tenantID, carID string) ([]string, error) {
	var bookings []models.RentFlowBooking
	if err := config.DB.
		Where("tenant_id = ? AND car_id = ?", tenantID, carID).
		Where("status IN ?", []string{"pending", "confirmed", "paid"}).
		Find(&bookings).Error; err != nil {
		return nil, err
	}

	var days []string
	for _, booking := range bookings {
		days = append(days, services.ExpandDateRange(booking.PickupDate, booking.ReturnDate)...)
	}

	var blocks []models.RentFlowAvailabilityBlock
	if err := config.DB.
		Where("tenant_id = ? AND (car_id = ? OR car_id = '')", tenantID, carID).
		Find(&blocks).Error; err != nil {
		return nil, err
	}
	for _, block := range blocks {
		days = append(days, services.ExpandDateRange(block.StartDate, block.EndDate)...)
	}
	return services.UniqueSortedStrings(days), nil
}

func rentFlowCarGrade(carID string) int {
	switch carID {
	case "bmw-x3-m50", "bmw-i7-xdrive60-m-sport":
		return 1
	case "bmw-i5-edrive40-m-sport":
		return 2
	case "bmw-320d-m-sport", "bmw-i5-m60-xdrive":
		return 3
	case "bmw-330e-m-sport":
		return 4
	default:
		return 3
	}
}

type rentFlowCarImageRef struct {
	ID        string
	CarID     string
	TenantID  string
	SortOrder int
}

func rentFlowCarImageURLs(c *gin.Context, tenant *models.RentFlowTenant, cars []models.RentFlowCar) (map[string][]string, error) {
	if tenant == nil {
		return rentFlowCarImageURLsForTenants(c, nil, cars)
	}
	return rentFlowCarImageURLsForTenants(c, map[string]models.RentFlowTenant{
		tenant.ID: *tenant,
	}, cars)
}

func rentFlowCarImageURLsForTenants(c *gin.Context, tenantMap map[string]models.RentFlowTenant, cars []models.RentFlowCar) (map[string][]string, error) {
	result := make(map[string][]string, len(cars))
	if len(cars) == 0 {
		return result, nil
	}

	carIDs := make([]string, 0, len(cars))
	tenantIDs := make([]string, 0, len(cars))
	tenantSeen := make(map[string]struct{})
	for _, car := range cars {
		carIDs = append(carIDs, car.ID)
		if _, ok := tenantSeen[car.TenantID]; !ok {
			tenantIDs = append(tenantIDs, car.TenantID)
			tenantSeen[car.TenantID] = struct{}{}
		}
	}

	var images []rentFlowCarImageRef
	if err := config.DB.
		Model(&models.RentFlowCarImage{}).
		Select("id, car_id, tenant_id, sort_order").
		Where("tenant_id IN ? AND car_id IN ?", tenantIDs, carIDs).
		Order("tenant_id ASC, car_id ASC, sort_order ASC").
		Find(&images).Error; err != nil {
		return nil, err
	}

	for _, image := range images {
		tenant := tenantMap[image.TenantID]
		result[image.CarID] = append(result[image.CarID], rentFlowCarImageURL(c, &tenant, image.CarID, image.ID))
	}

	return result, nil
}

func rentFlowCarImageURL(_ *gin.Context, tenant *models.RentFlowTenant, carID, imageID string) string {
	imagePath := "/cars/" + url.PathEscape(carID) + "/images/" + url.PathEscape(imageID)
	if tenant == nil || tenant.DomainSlug == "" {
		return imagePath
	}
	return imagePath + "?tenant=" + url.QueryEscape(tenant.DomainSlug)
}

func rentFlowUploadedImageFiles(form *multipart.Form) []*multipart.FileHeader {
	if form == nil {
		return nil
	}

	files := make([]*multipart.FileHeader, 0)
	files = append(files, form.File["image"]...)
	files = append(files, form.File["images"]...)
	if len(files) > 0 {
		return files
	}

	for _, group := range form.File {
		files = append(files, group...)
	}
	return files
}

func rentFlowCurrentMaxImageSortOrder(tenantID, carID string) (int, error) {
	var maxSortOrder int
	row := config.DB.
		Model(&models.RentFlowCarImage{}).
		Select("COALESCE(MAX(sort_order), -1)").
		Where("tenant_id = ? AND car_id = ?", tenantID, carID).
		Row()

	if err := row.Scan(&maxSortOrder); err != nil {
		return 0, err
	}
	return maxSortOrder, nil
}

func rentFlowBuildCarImage(tenantID, carID string, sortOrder int, fileHeader *multipart.FileHeader) (models.RentFlowCarImage, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return models.RentFlowCarImage{}, errors.New("ไม่สามารถอ่านไฟล์รูปภาพได้")
	}
	defer file.Close()

	imageBlob, err := io.ReadAll(io.LimitReader(file, rentFlowMaxCarImageBytes+1))
	if err != nil {
		return models.RentFlowCarImage{}, errors.New("ไม่สามารถอ่านไฟล์รูปภาพได้")
	}
	if len(imageBlob) == 0 {
		return models.RentFlowCarImage{}, errors.New("ไฟล์รูปภาพว่างเปล่า")
	}
	if len(imageBlob) > rentFlowMaxCarImageBytes {
		return models.RentFlowCarImage{}, errors.New("ไฟล์รูปภาพต้องมีขนาดไม่เกิน 5MB")
	}

	mimeType := http.DetectContentType(imageBlob)
	if _, ok := rentFlowAllowedImageTypes[mimeType]; !ok {
		return models.RentFlowCarImage{}, errors.New("รองรับเฉพาะไฟล์ JPG, PNG, WEBP หรือ GIF")
	}

	return models.RentFlowCarImage{
		ID:        services.NewID("carimg"),
		TenantID:  tenantID,
		CarID:     carID,
		SortOrder: sortOrder,
		FileName:  filepath.Base(strings.TrimSpace(fileHeader.Filename)),
		MimeType:  mimeType,
		ImageBlob: imageBlob,
	}, nil
}

func rentFlowCarImageResponse(c *gin.Context, tenant *models.RentFlowTenant, image models.RentFlowCarImage) gin.H {
	return gin.H{
		"id":        image.ID,
		"tenantId":  image.TenantID,
		"carId":     image.CarID,
		"imageUrl":  rentFlowCarImageURL(c, tenant, image.CarID, image.ID),
		"sortOrder": image.SortOrder,
		"fileName":  image.FileName,
		"mimeType":  image.MimeType,
		"size":      len(image.ImageBlob),
		"createdAt": image.CreatedAt,
		"updatedAt": image.UpdatedAt,
	}
}

func rentFlowSendCarImage(c *gin.Context, image models.RentFlowCarImage) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, image.MimeType, image.ImageBlob)
}
