package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

func RentFlowGetReviews(c *gin.Context) {
	var reviews []models.RentFlowReview
	if err := config.DB.
		Order("created_at DESC").
		Limit(50).
		Find(&reviews).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรีวิวได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงรีวิวสำเร็จ", gin.H{
		"items": reviews,
		"total": len(reviews),
	})
}

func RentFlowCreateReview(c *gin.Context) {
	var payload struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Rating    int    `json:"rating"`
		Comment   string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลรีวิวไม่ถูกต้อง")
		return
	}

	firstName := strings.TrimSpace(payload.FirstName)
	lastName := strings.TrimSpace(payload.LastName)
	comment := strings.TrimSpace(payload.Comment)

	if len(firstName) < 2 || len(lastName) < 2 {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อและนามสกุลจริง")
		return
	}

	if payload.Rating < 1 || payload.Rating > 5 {
		rentFlowError(c, http.StatusBadRequest, "กรุณาให้คะแนนรีวิว 1 ถึง 5 ดาว")
		return
	}

	if len([]rune(comment)) > 1000 {
		rentFlowError(c, http.StatusBadRequest, "รีวิวยาวเกินไป")
		return
	}

	review := models.RentFlowReview{
		ID:        services.NewID("rev"),
		FirstName: firstName,
		LastName:  lastName,
		Rating:    payload.Rating,
		Comment:   comment,
	}

	if err := config.DB.Create(&review).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรีวิวได้")
		return
	}

	rentFlowSuccess(c, http.StatusCreated, "ส่งรีวิวสำเร็จ", review)
}
