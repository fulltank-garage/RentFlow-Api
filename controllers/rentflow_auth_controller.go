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
	googleSub := strings.TrimSpace(payload.User.Sub)
	if payload.AccessToken == "" || email == "" || googleSub == "" {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลบัญชี Google ไม่ครบถ้วน")
		return
	}

	firstName := strings.TrimSpace(payload.User.GivenName)
	lastName := strings.TrimSpace(payload.User.FamilyName)
	name := strings.TrimSpace(payload.User.Name)
	if name == "" {
		name = strings.TrimSpace(firstName + " " + lastName)
	}
	if name == "" {
		name = email
	}

	avatarBlob, avatarMimeType, avatarErr := rentFlowImageBlobFromSource(&payload.User.Picture)

	var user models.RentFlowUser
	result := config.DB.Where("email = ?", email).First(&user)
	isNewUser := false
	switch {
	case result.Error == nil:
		isNewUser = false
	case errors.Is(result.Error, gorm.ErrRecordNotFound):
		isNewUser = true
	default:
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถค้นหาผู้ใช้ได้")
		return
	}

	if isNewUser {
		user = models.RentFlowUser{
			ID:             services.NewID("usr"),
			GoogleSub:      &googleSub,
			Username:       email,
			FirstName:      firstName,
			LastName:       lastName,
			Name:           name,
			Email:          email,
			AvatarMimeType: avatarMimeType,
			AvatarBlob:     avatarBlob,
		}
		if err := config.DB.Create(&user).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างบัญชีผู้ใช้ได้")
			return
		}
	} else {
		updates := map[string]interface{}{
			"username":   email,
			"first_name": firstName,
			"last_name":  lastName,
			"name":       name,
			"google_sub": googleSub,
			"updated_at": time.Now(),
		}
		if avatarErr == nil && len(avatarBlob) > 0 && avatarMimeType != "" {
			updates["avatar_mime_type"] = avatarMimeType
			updates["avatar_blob"] = avatarBlob
		}
		if err := config.DB.Model(&user).Updates(updates).Error; err != nil {
			rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตข้อมูลผู้ใช้ได้")
			return
		}
		user.Username = email
		user.FirstName = firstName
		user.LastName = lastName
		user.Name = name
		user.GoogleSub = &googleSub
		if avatarErr == nil && len(avatarBlob) > 0 && avatarMimeType != "" {
			user.AvatarMimeType = avatarMimeType
			user.AvatarBlob = avatarBlob
		}
	}

	sessionToken, err := services.CreateSession(config.Ctx, services.RentFlowSession{
		UserID:    user.ID,
		UserEmail: user.Email,
		App:       rentFlowAppFromRequest(c),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}, 7*24*time.Hour)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างเซสชันได้")
		return
	}

	setRentFlowSessionCookie(c, sessionToken)
	rentFlowRecordSessionAudit(c, user, "login")

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
		"user": rentFlowUserResponse(user),
	})
}

func RentFlowRegister(c *gin.Context) {
	var payload struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Name      string `json:"name"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลสมัครสมาชิกไม่ถูกต้อง")
		return
	}

	username := strings.TrimSpace(strings.ToLower(payload.Username))
	firstName := strings.TrimSpace(payload.FirstName)
	lastName := strings.TrimSpace(payload.LastName)
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = strings.TrimSpace(firstName + " " + lastName)
	}

	if len(username) < 3 || len(payload.Password) < 8 || len(firstName) < 2 || len(lastName) < 2 {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกข้อมูลสมัครสมาชิกให้ครบถ้วน")
		return
	}

	var existing models.RentFlowUser
	err := config.DB.Where("username = ? OR email = ?", username, username).First(&existing).Error
	switch {
	case err == nil:
		rentFlowError(c, http.StatusConflict, "ชื่อผู้ใช้นี้ถูกใช้งานแล้ว")
		return
	case !errors.Is(err, gorm.ErrRecordNotFound):
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบข้อมูลผู้ใช้ได้")
		return
	}

	hash, err := services.HashPasswordIfNeeded(payload.Password)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างบัญชีผู้ใช้ได้")
		return
	}

	user := models.RentFlowUser{
		ID:           services.NewID("usr"),
		Username:     username,
		FirstName:    firstName,
		LastName:     lastName,
		Name:         name,
		Email:        username,
		PasswordHash: hash,
	}

	if err := config.DB.Create(&user).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างบัญชีผู้ใช้ได้")
		return
	}

	sessionToken, err := services.CreateSession(config.Ctx, services.RentFlowSession{
		UserID:    user.ID,
		UserEmail: user.Email,
		App:       rentFlowAppFromRequest(c),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}, 7*24*time.Hour)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างเซสชันได้")
		return
	}

	setRentFlowSessionCookie(c, sessionToken)
	rentFlowRecordSessionAudit(c, user, "register")
	rentFlowCreateNotification("", &user.ID, user.Email, "ยินดีต้อนรับสู่ RentFlow", "บัญชีของคุณพร้อมใช้งานแล้ว")

	rentFlowSuccess(c, http.StatusCreated, "สมัครสมาชิกสำเร็จ", gin.H{
		"user": rentFlowUserResponse(user),
	})
}

func RentFlowLogin(c *gin.Context) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลเข้าสู่ระบบไม่ถูกต้อง")
		return
	}

	username := strings.TrimSpace(strings.ToLower(payload.Username))
	if username == "" || payload.Password == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อผู้ใช้และรหัสผ่าน")
		return
	}

	var user models.RentFlowUser
	if err := config.DB.Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
		rentFlowError(c, http.StatusUnauthorized, "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	if strings.EqualFold(user.Status, "locked") || strings.EqualFold(user.Status, "disabled") {
		rentFlowError(c, http.StatusForbidden, "บัญชีนี้ถูกระงับการใช้งาน กรุณาติดต่อผู้ดูแลระบบ")
		return
	}

	if !services.CheckPassword(payload.Password, user.PasswordHash) {
		now := time.Now()
		failedCount := user.FailedLoginCount + 1
		updates := map[string]interface{}{
			"failed_login_count":   failedCount,
			"last_failed_login_at": &now,
			"updated_at":           now,
		}
		if failedCount >= 5 {
			updates["status"] = "locked"
			updates["locked_reason"] = "กรอกรหัสผ่านผิดเกินจำนวนครั้งที่กำหนด"
			updates["locked_at"] = &now
		}
		_ = config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(updates).Error
		rentFlowError(c, http.StatusUnauthorized, "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง")
		return
	}

	sessionToken, err := services.CreateSession(config.Ctx, services.RentFlowSession{
		UserID:    user.ID,
		UserEmail: user.Email,
		App:       rentFlowAppFromRequest(c),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}, 7*24*time.Hour)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถสร้างเซสชันได้")
		return
	}

	setRentFlowSessionCookie(c, sessionToken)
	_ = config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"failed_login_count":   0,
		"last_failed_login_at": nil,
		"updated_at":           time.Now(),
	}).Error
	rentFlowRecordSessionAudit(c, user, "login")
	rentFlowSuccess(c, http.StatusOK, "เข้าสู่ระบบสำเร็จ", gin.H{
		"user":               rentFlowUserResponse(user),
		"mustChangePassword": user.MustChangePassword,
	})
}

func RentFlowForgotPassword(c *gin.Context) {
	var payload struct {
		Username    string `json:"username"`
		Phone       string `json:"phone"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลเปลี่ยนรหัสผ่านไม่ถูกต้อง")
		return
	}

	username := strings.TrimSpace(strings.ToLower(payload.Username))
	phone := rentFlowNormalizePhone(payload.Phone)
	if len(username) < 3 || len(phone) < 9 || len(strings.TrimSpace(payload.NewPassword)) < 8 {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อผู้ใช้ เบอร์โทรศัพท์ และรหัสผ่านใหม่ให้ถูกต้อง")
		return
	}

	hash, err := services.HashPasswordIfNeeded(payload.NewPassword)
	if err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้")
		return
	}

	var user models.RentFlowUser
	if err := config.DB.Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rentFlowError(c, http.StatusNotFound, "ไม่พบบัญชีผู้ใช้")
			return
		}
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถตรวจสอบบัญชีผู้ใช้ได้")
		return
	}
	if rentFlowNormalizePhone(user.Phone) == "" || rentFlowNormalizePhone(user.Phone) != phone {
		rentFlowError(c, http.StatusUnauthorized, "เบอร์โทรศัพท์ไม่ตรงกับข้อมูลในระบบ")
		return
	}

	if err := config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"password_hash":        hash,
		"password_changed_at":  time.Now(),
		"must_change_password": false,
		"failed_login_count":   0,
		"last_failed_login_at": nil,
		"locked_reason":        "",
		"updated_at":           time.Now(),
	}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเปลี่ยนรหัสผ่านได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "เปลี่ยนรหัสผ่านสำเร็จ", nil)
}

func RentFlowGetMe(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลผู้ใช้สำเร็จ", gin.H{
		"user": rentFlowUserResponse(*user),
	})
}

func RentFlowLogout(c *gin.Context) {
	if user, ok := middleware.CurrentRentFlowUser(c); ok {
		rentFlowRecordSessionAudit(c, *user, "logout")
	}
	token := rentFlowSessionTokenFromRequest(c)
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
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลโปรไฟล์สำเร็จ", rentFlowUserResponse(*user))
}

func RentFlowUpdateMe(c *gin.Context) {
	user, ok := middleware.CurrentRentFlowUser(c)
	if !ok {
		rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"updated_at": now,
	}

	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	isMultipart := strings.Contains(contentType, "multipart/form-data") ||
		strings.HasPrefix(strings.ToLower(c.ContentType()), "multipart/form-data")

	if isMultipart {
		if value, exists := c.GetPostForm("name"); exists {
			updates["name"] = strings.TrimSpace(value)
			user.Name = strings.TrimSpace(value)
		}
		if value, exists := c.GetPostForm("phone"); exists {
			normalizedPhone := rentFlowNormalizePhone(value)
			updates["phone"] = normalizedPhone
			user.Phone = normalizedPhone
		}
		if strings.EqualFold(strings.TrimSpace(c.PostForm("clearAvatar")), "true") {
			updates["avatar_mime_type"] = ""
			updates["avatar_blob"] = []byte{}
			user.AvatarMimeType = ""
			user.AvatarBlob = []byte{}
		}
		if fileHeader, err := c.FormFile("avatar"); err == nil {
			avatarBlob, avatarMimeType, err := rentFlowImageBlobFromUpload(fileHeader)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, "รูปโปรไฟล์ไม่ถูกต้อง")
				return
			}
			updates["avatar_mime_type"] = avatarMimeType
			updates["avatar_blob"] = avatarBlob
			user.AvatarMimeType = avatarMimeType
			user.AvatarBlob = avatarBlob
		} else if !errors.Is(err, http.ErrMissingFile) {
			rentFlowError(c, http.StatusBadRequest, "รูปโปรไฟล์ไม่ถูกต้อง")
			return
		}
		if value, exists := c.GetPostForm("avatarUrl"); exists {
			avatarBlob, avatarMimeType, err := rentFlowImageBlobFromSource(&value)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, "รูปโปรไฟล์ไม่ถูกต้อง")
				return
			}
			updates["avatar_mime_type"] = avatarMimeType
			updates["avatar_blob"] = avatarBlob
			user.AvatarMimeType = avatarMimeType
			user.AvatarBlob = avatarBlob
		}
	} else {
		var payload struct {
			Name      *string `json:"name"`
			Phone     *string `json:"phone"`
			AvatarURL *string `json:"avatarUrl"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			rentFlowError(c, http.StatusBadRequest, "ข้อมูลโปรไฟล์ไม่ถูกต้อง")
			return
		}

		if payload.Name != nil {
			updates["name"] = strings.TrimSpace(*payload.Name)
			user.Name = strings.TrimSpace(*payload.Name)
		}
		if payload.Phone != nil {
			normalizedPhone := rentFlowNormalizePhone(*payload.Phone)
			updates["phone"] = normalizedPhone
			user.Phone = normalizedPhone
		}
		if payload.AvatarURL != nil {
			avatarBlob, avatarMimeType, err := rentFlowImageBlobFromSource(payload.AvatarURL)
			if err != nil {
				rentFlowError(c, http.StatusBadRequest, "รูปโปรไฟล์ไม่ถูกต้อง")
				return
			}
			updates["avatar_mime_type"] = avatarMimeType
			updates["avatar_blob"] = avatarBlob
			user.AvatarMimeType = avatarMimeType
			user.AvatarBlob = avatarBlob
		}
	}

	if err := config.DB.Model(&models.RentFlowUser{}).
		Where("id = ?", user.ID).
		Select(rentFlowUpdateColumns(updates)).
		Updates(updates).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตโปรไฟล์ได้")
		return
	}

	_ = config.DB.Where("id = ?", user.ID).First(user).Error
	rentFlowSuccess(c, http.StatusOK, "อัปเดตโปรไฟล์สำเร็จ", rentFlowUserResponse(*user))
}

func RentFlowGetUserAvatar(c *gin.Context) {
	var user models.RentFlowUser
	if err := config.DB.Where("id = ?", strings.TrimSpace(c.Param("userId"))).First(&user).Error; err != nil {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรไฟล์")
		return
	}

	if len(user.AvatarBlob) == 0 || strings.TrimSpace(user.AvatarMimeType) == "" {
		rentFlowError(c, http.StatusNotFound, "ไม่พบรูปโปรไฟล์")
		return
	}

	rentFlowSendImageBlob(c, user.AvatarMimeType, user.AvatarBlob)
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
		"password_hash":        hash,
		"password_changed_at":  time.Now(),
		"must_change_password": false,
		"updated_at":           time.Now(),
	}).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตรหัสผ่านได้")
		return
	}

	rentFlowSuccess(c, http.StatusOK, "อัปเดตรหัสผ่านสำเร็จ", nil)
}

func rentFlowRecordSessionAudit(c *gin.Context, user models.RentFlowUser, action string) {
	if strings.TrimSpace(user.ID) == "" || config.DB == nil {
		return
	}
	audit := models.RentFlowSessionAudit{
		ID:        services.NewID("ses"),
		UserID:    user.ID,
		UserEmail: user.Email,
		App:       rentFlowAppFromRequest(c),
		Action:    strings.TrimSpace(strings.ToLower(action)),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	_ = config.DB.Create(&audit).Error
}
