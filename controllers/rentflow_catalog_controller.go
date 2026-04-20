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

func RentFlowGetCars(c *gin.Context) {
	cacheKey := services.CacheKey(
		services.RentFlowCarsCachePrefix(),
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

	var cars []models.RentFlowCar
	query := config.DB.Where("is_available = ?", true)

	if location := strings.TrimSpace(c.Query("location")); location != "" {
		query = query.Where("location_id = ?", location)
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
	for _, car := range cars {
		if pickupErr == nil && returnErr == nil && pickupDate.Before(returnDate) {
			available, err := rentFlowCarIsAvailable(car.ID, pickupDate, returnDate)
			if err != nil {
				rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
				return
			}
			if !available {
				continue
			}
		}

		visibleCars = append(visibleCars, car)
	}

	carImageURLs, err := rentFlowCarImageURLs(c, visibleCars)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรูปภาพรถได้")
		return
	}

	responseItems := make([]gin.H, 0, len(visibleCars))
	for _, car := range visibleCars {
		images := carImageURLs[car.ID]
		primaryImage := ""
		if len(images) > 0 {
			primaryImage = images[0]
		}

		responseItems = append(responseItems, gin.H{
			"id":           car.ID,
			"name":         car.Name,
			"image":        primaryImage,
			"brand":        car.Brand,
			"model":        car.Model,
			"year":         car.Year,
			"type":         car.Type,
			"seats":        car.Seats,
			"transmission": car.Transmission,
			"fuel":         car.Fuel,
			"grade":        rentFlowCarGrade(car.ID),
			"pricePerDay":  car.PricePerDay,
			"imageUrl":     primaryImage,
			"images":       images,
			"description":  car.Description,
			"locationId":   car.LocationID,
			"isAvailable":  car.IsAvailable,
			"createdAt":    car.CreatedAt,
			"updatedAt":    car.UpdatedAt,
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
	var image models.RentFlowCarImage
	if err := config.DB.
		Where("car_id = ?", c.Param("carId")).
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
	var image models.RentFlowCarImage
	if err := config.DB.
		Where("car_id = ? AND id = ?", c.Param("carId"), c.Param("imageId")).
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
	carID := strings.TrimSpace(c.Param("carId"))
	var car models.RentFlowCar
	if err := config.DB.Where("id = ?", carID).First(&car).Error; err != nil {
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
	maxSortOrder, err := rentFlowCurrentMaxImageSortOrder(carID)
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
		if err := tx.Where("car_id = ?", carID).Delete(&models.RentFlowCarImage{}).Error; err != nil {
			tx.Rollback()
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบรูปภาพเดิมได้")
			return
		}
	}

	images := make([]models.RentFlowCarImage, 0, len(files))
	for index, fileHeader := range files {
		image, err := rentFlowBuildCarImage(carID, maxSortOrder+index+1, fileHeader)
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
		items = append(items, rentFlowCarImageResponse(c, image))
	}

	rentFlowSuccess(c, http.StatusCreated, "อัปโหลดรูปภาพสำเร็จ", gin.H{
		"items": items,
		"total": len(items),
	})
}

func RentFlowGetBranches(c *gin.Context) {
	cacheKey := services.CacheKey("branches", "all")
	var cached rentFlowAPIResponse
	if services.CacheGetJSON(config.Ctx, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	var branches []models.RentFlowBranch
	if err := config.DB.Where("is_active = ?", true).Order("name ASC").Find(&branches).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสาขาได้")
		return
	}

	response := rentFlowAPIResponse{
		Success: true,
		Message: "ดึงข้อมูลสาขาสำเร็จ",
		Data:    branches,
	}
	services.CacheSetJSON(config.Ctx, cacheKey, response, 30*time.Minute)
	c.JSON(http.StatusOK, response)
}

func RentFlowGetBranchByID(c *gin.Context) {
	var branch models.RentFlowBranch
	if err := config.DB.Where("id = ? AND is_active = ?", c.Param("branchId"), true).First(&branch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			rentFlowError(c, http.StatusNotFound, "ไม่พบสาขาที่ต้องการ")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลสาขาได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลสาขาสำเร็จ", branch)
}

func RentFlowCheckAvailability(c *gin.Context) {
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

	available, err := rentFlowCarIsAvailable(payload.CarID, pickupDate, returnDate)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบคิวรถได้")
		return
	}

	unavailableDates, err := rentFlowUnavailableDates(payload.CarID)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงวันไม่ว่างได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ตรวจสอบคิวรถสำเร็จ", gin.H{
		"carId":            payload.CarID,
		"available":        available,
		"unavailableDates": unavailableDates,
	})
}

func RentFlowGetUnavailableDates(c *gin.Context) {
	dates, err := rentFlowUnavailableDates(c.Param("carId"))
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงวันไม่ว่างได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงวันไม่ว่างสำเร็จ", dates)
}

func rentFlowCarIsAvailable(carID string, pickupDate, returnDate time.Time) (bool, error) {
	var count int64
	err := config.DB.Model(&models.RentFlowBooking{}).
		Where("car_id = ?", carID).
		Where("status IN ?", []string{"pending", "confirmed", "paid"}).
		Where("pickup_date < ? AND return_date > ?", returnDate, pickupDate).
		Count(&count).Error
	return count == 0, err
}

func rentFlowUnavailableDates(carID string) ([]string, error) {
	var bookings []models.RentFlowBooking
	if err := config.DB.
		Where("car_id = ?", carID).
		Where("status IN ?", []string{"pending", "confirmed", "paid"}).
		Find(&bookings).Error; err != nil {
		return nil, err
	}

	var days []string
	for _, booking := range bookings {
		days = append(days, services.ExpandDateRange(booking.PickupDate, booking.ReturnDate)...)
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
	SortOrder int
}

func rentFlowCarImageURLs(c *gin.Context, cars []models.RentFlowCar) (map[string][]string, error) {
	result := make(map[string][]string, len(cars))
	if len(cars) == 0 {
		return result, nil
	}

	carIDs := make([]string, 0, len(cars))
	for _, car := range cars {
		carIDs = append(carIDs, car.ID)
	}

	var images []rentFlowCarImageRef
	if err := config.DB.
		Model(&models.RentFlowCarImage{}).
		Select("id, car_id, sort_order").
		Where("car_id IN ?", carIDs).
		Order("car_id ASC, sort_order ASC").
		Find(&images).Error; err != nil {
		return nil, err
	}

	for _, image := range images {
		result[image.CarID] = append(result[image.CarID], rentFlowCarImageURL(c, image.CarID, image.ID))
	}

	return result, nil
}

func rentFlowCarImageURL(_ *gin.Context, carID, imageID string) string {
	return "/cars/" + url.PathEscape(carID) + "/images/" + url.PathEscape(imageID)
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

func rentFlowCurrentMaxImageSortOrder(carID string) (int, error) {
	var maxSortOrder int
	row := config.DB.
		Model(&models.RentFlowCarImage{}).
		Select("COALESCE(MAX(sort_order), -1)").
		Where("car_id = ?", carID).
		Row()

	if err := row.Scan(&maxSortOrder); err != nil {
		return 0, err
	}
	return maxSortOrder, nil
}

func rentFlowBuildCarImage(carID string, sortOrder int, fileHeader *multipart.FileHeader) (models.RentFlowCarImage, error) {
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
		CarID:     carID,
		SortOrder: sortOrder,
		FileName:  filepath.Base(strings.TrimSpace(fileHeader.Filename)),
		MimeType:  mimeType,
		ImageBlob: imageBlob,
	}, nil
}

func rentFlowCarImageResponse(c *gin.Context, image models.RentFlowCarImage) gin.H {
	return gin.H{
		"id":        image.ID,
		"carId":     image.CarID,
		"imageUrl":  rentFlowCarImageURL(c, image.CarID, image.ID),
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
