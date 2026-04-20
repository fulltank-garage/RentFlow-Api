package controllers

import (
	"net/http"
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

	responseItems := make([]gin.H, 0, len(cars))
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

		responseItems = append(responseItems, gin.H{
			"id":           car.ID,
			"name":         car.Name,
			"image":        car.ImageURL,
			"brand":        car.Brand,
			"model":        car.Model,
			"year":         car.Year,
			"type":         car.Type,
			"seats":        car.Seats,
			"transmission": car.Transmission,
			"fuel":         car.Fuel,
			"grade":        rentFlowCarGrade(car.ID),
			"pricePerDay":  car.PricePerDay,
			"imageUrl":     car.ImageURL,
			"images":       car.Images(),
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
