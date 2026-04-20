package controllers

import (
	"errors"
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

type rentFlowGooglePayload struct {
	AccessToken string `json:"accessToken"`
	User        struct {
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Picture       string `json:"picture"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	} `json:"user"`
}

func RentFlowAuthWithGoogle(c *gin.Context) {
	var payload rentFlowGooglePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสำหรับเข้าสู่ระบบไม่ถูกต้อง")
		return
	}

	email := strings.TrimSpace(strings.ToLower(payload.User.Email))
	if payload.AccessToken == "" || email == "" || strings.TrimSpace(payload.User.Sub) == "" {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูล Google account ไม่ครบถ้วน")
		return
	}

	name := strings.TrimSpace(payload.User.Name)
	if name == "" {
		name = strings.TrimSpace(strings.TrimSpace(payload.User.GivenName + " " + payload.User.FamilyName))
	}
	if name == "" {
		name = email
	}

	var user models.RentFlowUser
	result := config.DB.Where("email = ?", email).First(&user)
	isNewUser := result.Error != nil
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาผู้ใช้ได้")
		return
	}

	if isNewUser {
		user = models.RentFlowUser{
			ID:        services.NewID("usr"),
			GoogleSub: payload.User.Sub,
			Name:      name,
			Email:     email,
			AvatarURL: strings.TrimSpace(payload.User.Picture),
		}
		if err := config.DB.Create(&user).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างบัญชีผู้ใช้ได้")
			return
		}
	} else {
		updates := map[string]interface{}{
			"name":       name,
			"google_sub": payload.User.Sub,
			"avatar_url": strings.TrimSpace(payload.User.Picture),
			"updated_at": time.Now(),
		}
		if err := config.DB.Model(&user).Updates(updates).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตข้อมูลผู้ใช้ได้")
			return
		}
		user.Name = name
		user.GoogleSub = payload.User.Sub
		user.AvatarURL = strings.TrimSpace(payload.User.Picture)
	}

	sessionToken, err := services.CreateSession(config.Ctx, services.RentFlowSession{
		UserID:    user.ID,
		UserEmail: user.Email,
	}, 7*24*time.Hour)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้าง session ได้")
		return
	}

	setRentFlowSessionCookie(c, sessionToken)

	if isNewUser {
		notification := models.RentFlowNotification{
			ID:        services.NewID("ntf"),
			UserID:    &user.ID,
			UserEmail: user.Email,
			Title:     "ยินดีต้อนรับสู่ RentFlow",
			Message:   "บัญชีของคุณพร้อมใช้งานแล้ว สามารถเริ่มค้นหารถและทำรายการจองได้ทันที",
			IsRead:    false,
		}
		_ = config.DB.Create(&notification).Error
	}

	rentFlowSuccess(c, http.StatusOK, "เข้าสู่ระบบสำเร็จ", gin.H{
		"user": user,
	})
}

func RentFlowGetMe(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลผู้ใช้สำเร็จ", gin.H{
		"user": user,
	})
}

func RentFlowLogout(c *gin.Context) {
	token, _ := c.Cookie(services.RentFlowSessionCookieName)
	if token != "" {
		_ = services.DeleteSession(config.Ctx, token)
	}
	clearRentFlowSessionCookie(c)
	rentFlowSuccess(c, http.StatusOK, "ออกจากระบบสำเร็จ", nil)
}

func RentFlowUserMe(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลโปรไฟล์สำเร็จ", user)
}

func RentFlowUpdateMe(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	var payload struct {
		Name      *string `json:"name"`
		Phone     *string `json:"phone"`
		AvatarURL *string `json:"avatarUrl"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลโปรไฟล์ไม่ถูกต้อง")
		return
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if payload.Name != nil {
		updates["name"] = strings.TrimSpace(*payload.Name)
		user.Name = strings.TrimSpace(*payload.Name)
	}
	if payload.Phone != nil {
		updates["phone"] = strings.TrimSpace(*payload.Phone)
		user.Phone = strings.TrimSpace(*payload.Phone)
	}
	if payload.AvatarURL != nil {
		updates["avatar_url"] = strings.TrimSpace(*payload.AvatarURL)
		user.AvatarURL = strings.TrimSpace(*payload.AvatarURL)
	}

	if err := config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตโปรไฟล์ได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "อัปเดตโปรไฟล์สำเร็จ", user)
}

func RentFlowChangePassword(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	var payload struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลรหัสผ่านไม่ถูกต้อง")
		return
	}

	if len(strings.TrimSpace(payload.NewPassword)) < 6 {
		rentFlowError(c, http.StatusBadRequest, "รหัสผ่านใหม่ต้องมีอย่างน้อย 6 ตัวอักษร")
		return
	}

	if user.PasswordHash != "" && !services.CheckPassword(payload.CurrentPassword, user.PasswordHash) {
		rentFlowError(c, http.StatusBadRequest, "รหัสผ่านปัจจุบันไม่ถูกต้อง")
		return
	}

	hash, err := services.HashPasswordIfNeeded(payload.NewPassword)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตรหัสผ่านได้")
		return
	}

	if err := config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"password_hash": hash,
		"updated_at":    time.Now(),
	}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตรหัสผ่านได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "อัปเดตรหัสผ่านสำเร็จ", nil)
}
