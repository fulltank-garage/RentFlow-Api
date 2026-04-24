package controllers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/models"
)

const rentFlowMarketplacePromoImageKey = "marketplace_home_promo_image"

func RentFlowGetPublicPlatformSettings(c *gin.Context) {
	setting := rentFlowMarketplacePromoImageSetting()
	rentFlowSuccess(c, http.StatusOK, "ดึงการตั้งค่าหน้ารวมสำเร็จ", rentFlowPlatformSettingsResponse(setting))
}

func RentFlowAdminGetPlatformSettings(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	setting := rentFlowMarketplacePromoImageSetting()
	rentFlowSuccess(c, http.StatusOK, "ดึงการตั้งค่าหน้ารวมสำเร็จ", rentFlowPlatformSettingsResponse(setting))
}

func RentFlowAdminUpdatePlatformSettings(c *gin.Context) {
	if !rentFlowRequirePlatformAdmin(c) {
		return
	}

	var payload struct {
		PromoImageURL *string `json:"promoImageUrl"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลการตั้งค่าไม่ถูกต้อง")
		return
	}

	setting := rentFlowMarketplacePromoImageSetting()
	if payload.PromoImageURL != nil {
		imageBlob, imageMimeType, err := rentFlowImageBlobFromSource(payload.PromoImageURL)
		if err != nil {
			rentFlowError(c, http.StatusBadRequest, err.Error())
			return
		}

		now := time.Now()
		setting.Key = rentFlowMarketplacePromoImageKey
		setting.ImageMimeType = imageMimeType
		setting.ImageBlob = imageBlob
		setting.UpdatedAt = now
		if setting.CreatedAt.IsZero() {
			setting.CreatedAt = now
		}

		if err := config.DB.Save(&setting).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถบันทึกรูปโปรโมชันหน้ารวมได้")
			return
		}
	}

	rentFlowSuccess(c, http.StatusOK, "บันทึกการตั้งค่าหน้ารวมสำเร็จ", rentFlowPlatformSettingsResponse(setting))
}

func RentFlowGetMarketplacePromoImage(c *gin.Context) {
	setting := rentFlowMarketplacePromoImageSetting()
	if len(setting.ImageBlob) == 0 || strings.TrimSpace(setting.ImageMimeType) == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรโมชันหน้ารวม")
		return
	}

	rentFlowSendImageBlob(c, setting.ImageMimeType, setting.ImageBlob)
}

func rentFlowMarketplacePromoImageSetting() models.RentFlowPlatformSetting {
	var setting models.RentFlowPlatformSetting
	_ = config.DB.Where("key = ?", rentFlowMarketplacePromoImageKey).First(&setting).Error
	return setting
}

func rentFlowPlatformSettingsResponse(setting models.RentFlowPlatformSetting) gin.H {
	return gin.H{
		"promoImageUrl": rentFlowPlatformImageURL(setting),
		"updatedAt":     setting.UpdatedAt,
	}
}
