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
	marketplace := rentFlowIsMarketplaceRequest(c)

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
	} else {
		tenant, ok := rentFlowRequireTenant(c)
		if !ok {
			return
		}
		tenantMap[tenant.ID] = *tenant
		tenantIDs = append(tenantIDs, tenant.ID)
	}

	if len(tenantIDs) == 0 {
		rentFlowSuccess(c, http.StatusOK, "ดึงรีวิวสำเร็จ", gin.H{
			"items": []gin.H{},
			"total": 0,
		})
		return
	}

	var reviews []models.RentFlowReview
	if err := config.DB.
		Where("tenant_id IN ?", tenantIDs).
		Order("created_at DESC").
		Limit(50).
		Find(&reviews).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงรีวิวได้")
		return
	}

	items := make([]gin.H, 0, len(reviews))
	for _, review := range reviews {
		tenant := tenantMap[review.TenantID]
		items = append(items, gin.H{
			"id":           review.ID,
			"tenantId":     review.TenantID,
			"firstName":    review.FirstName,
			"lastName":     review.LastName,
			"rating":       review.Rating,
			"comment":      review.Comment,
			"createdAt":    review.CreatedAt,
			"updatedAt":    review.UpdatedAt,
			"shopName":     tenant.ShopName,
			"domainSlug":   tenant.DomainSlug,
			"publicDomain": tenant.PublicDomain,
		})
	}

	rentFlowSuccess(c, http.StatusOK, "ดึงรีวิวสำเร็จ", gin.H{
		"items": items,
		"total": len(items),
	})
}

func RentFlowCreateReview(c *gin.Context) {
	tenant, ok := rentFlowRequireTenant(c)
	if !ok {
		return
	}

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
		TenantID:  tenant.ID,
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
