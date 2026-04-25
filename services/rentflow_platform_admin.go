package services

import (
	"errors"
	"log"
	"os"
	"strings"

	"gorm.io/gorm"
	"rentflow-api/config"
	"rentflow-api/models"
)

func RentFlowPlatformAdminEmail() string {
	return strings.TrimSpace(strings.ToLower(os.Getenv("RENTFLOW_SUPER_ADMIN_EMAIL")))
}

func RentFlowPlatformAdminUsername() string {
	return strings.TrimSpace(strings.ToLower(os.Getenv("RENTFLOW_SUPER_ADMIN_USERNAME")))
}

func IsRentFlowPlatformAdmin(user *models.RentFlowUser) bool {
	if user == nil {
		return false
	}

	adminEmail := RentFlowPlatformAdminEmail()
	adminUsername := RentFlowPlatformAdminUsername()
	userEmail := strings.TrimSpace(strings.ToLower(user.Email))
	userUsername := strings.TrimSpace(strings.ToLower(user.Username))

	if (adminEmail != "" && userEmail == adminEmail) ||
		(adminUsername != "" && userUsername == adminUsername) {
		return true
	}

	if config.DB == nil {
		return false
	}

	var member models.RentFlowPlatformMember
	err := config.DB.
		Where("status = ?", "active").
		Where("user_id = ? OR LOWER(email) = ?", user.ID, userEmail).
		First(&member).Error
	return err == nil
}

func RentFlowPlatformAdminConfigured() bool {
	if RentFlowPlatformAdminEmail() != "" || RentFlowPlatformAdminUsername() != "" {
		return true
	}
	if config.DB == nil {
		return false
	}
	var count int64
	if err := config.DB.Model(&models.RentFlowPlatformMember{}).
		Where("status = ?", "active").
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func EnsureRentFlowPlatformAdmin() {
	if config.DB == nil {
		return
	}

	email := RentFlowPlatformAdminEmail()
	username := RentFlowPlatformAdminUsername()
	password := strings.TrimSpace(os.Getenv("RENTFLOW_SUPER_ADMIN_PASSWORD"))

	if email == "" && username == "" {
		return
	}

	if username == "" {
		username = email
	}
	if email == "" {
		email = username
	}

	firstName := strings.TrimSpace(os.Getenv("RENTFLOW_SUPER_ADMIN_FIRST_NAME"))
	if firstName == "" {
		firstName = "Platform"
	}
	lastName := strings.TrimSpace(os.Getenv("RENTFLOW_SUPER_ADMIN_LAST_NAME"))
	if lastName == "" {
		lastName = "Admin"
	}
	name := strings.TrimSpace(firstName + " " + lastName)

	var user models.RentFlowUser
	err := config.DB.Where("username = ? OR email = ?", username, email).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Println("ตรวจสอบบัญชีผู้ดูแลระบบกลางไม่สำเร็จ:", err)
		return
	}

	updates := map[string]interface{}{
		"username":   username,
		"email":      email,
		"first_name": firstName,
		"last_name":  lastName,
		"name":       name,
	}

	if password != "" {
		hash, hashErr := HashPasswordIfNeeded(password)
		if hashErr != nil {
			log.Println("สร้างรหัสผ่านผู้ดูแลระบบกลางไม่สำเร็จ:", hashErr)
			return
		}
		updates["password_hash"] = hash
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		if password == "" {
			log.Println("ยังไม่ได้สร้างผู้ดูแลระบบกลาง เพราะไม่ได้กำหนด RENTFLOW_SUPER_ADMIN_PASSWORD")
			return
		}

		user = models.RentFlowUser{
			ID:           NewID("usr"),
			Username:     username,
			Email:        email,
			FirstName:    firstName,
			LastName:     lastName,
			Name:         name,
			PasswordHash: updates["password_hash"].(string),
		}
		if createErr := config.DB.Create(&user).Error; createErr != nil {
			log.Println("สร้างผู้ดูแลระบบกลางไม่สำเร็จ:", createErr)
			return
		}
		log.Println("สร้างผู้ดูแลระบบกลางแล้ว")
		return
	}

	if updateErr := config.DB.Model(&models.RentFlowUser{}).Where("id = ?", user.ID).Updates(updates).Error; updateErr != nil {
		log.Println("อัปเดตผู้ดูแลระบบกลางไม่สำเร็จ:", updateErr)
	}
}
