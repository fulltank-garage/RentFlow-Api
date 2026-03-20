package controllers

import (
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

const (
	RoleSuperAdmin = 1
	RoleAdmin      = 2
	RoleEmployee   = 3
)

func CreateUserRequestByAdmin(c *gin.Context) {
    if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่สามารถอ่านข้อมูลได้"})
        return
    }

    currentID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }

    var creator models.User
    if err := config.DB.First(&creator, currentID).Error; err != nil {
        log.Printf("CreateUserRequestByAdmin: ไม่พบผู้ใช้ ID %v: %v", currentID, err)
        c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่พบผู้ใช้"})
        return
    }

    gmail := c.PostForm("gmail")
    password := c.PostForm("password")
    firstName := c.PostForm("first_name")
    lastName := c.PostForm("last_name")
    roleStr := c.PostForm("role_id")

    roleIDInt, err := strconv.Atoi(roleStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "role_id ไม่ถูกต้อง"})
        return
    }
    roleID := uint(roleIDInt)

    if creator.RoleID == RoleAdmin && (roleID == RoleAdmin || roleID == RoleSuperAdmin) {
        c.JSON(http.StatusForbidden, gin.H{"error": "admin ไม่สามารถสร้าง admin หรือ superadmin ได้"})
        return
    }

    var count int64
    config.DB.Model(&models.User{}).Where("gmail = ?", gmail).Count(&count)
    if count > 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "อีเมลนี้ถูกใช้งานแล้ว"})
        return
    }

    file, _, err := c.Request.FormFile("profile_image")
    var imageData []byte
    if err != nil && err != http.ErrMissingFile {
        c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านไฟล์รูปภาพไม่สำเร็จ"})
        return
    }
    if file != nil {
        defer file.Close()
        imageData, err = io.ReadAll(file)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านไฟล์รูปภาพไม่สำเร็จ"})
            return
        }
    }

    hashedPass, err := services.HashPassword(password)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "เกิดข้อผิดพลาดในการเข้ารหัสรหัสผ่าน"})
        return
    }

    now := time.Now().In(time.FixedZone("Asia/Bangkok", 7*3600))

    err = config.DB.Exec(`
        INSERT INTO users (gmail, password, first_name, last_name, role_id, profile_image, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, gmail, hashedPass, firstName, lastName, roleID, imageData, now).Error
    if err != nil {
        log.Printf("CreateUserRequestByAdmin: insert users error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกได้"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "เพิ่มผู้ใช้เรียบร้อย"})
}

func VerifyAndActivateUser(c *gin.Context) {
	var input struct {
		Gmail string `json:"gmail" binding:"required,email"`
		OTP   string `json:"otp" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var verif struct {
		Gmail        string
		Password     string
		FirstName    string
		LastName     string
		RoleID       int
		Image        []byte `gorm:"column:profile_image"`
		OTP          string
		OtpExpiresAt time.Time
	}

	err := config.DB.Raw(`
        SELECT gmail, password, first_name, last_name, role_id, profile_image, otp, otp_expires_at
        FROM user_verifications
        WHERE gmail = ? ORDER BY id DESC LIMIT 1
    `, input.Gmail).Scan(&verif).Error

	if err != nil {
		log.Printf("VerifyAndActivateUser: query user_verifications error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบ OTP ได้"})
		return
	}

	if verif.OTP != input.OTP {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รหัส OTP ไม่ถูกต้อง"})
		return
	}

	if time.Now().After(verif.OtpExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รหัส OTP หมดอายุแล้ว"})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 🔹 ใช้ uint ตรง ๆ ไม่ต้อง pointer
	roleID := uint64(verif.RoleID)
	user := models.User{
		Gmail:        verif.Gmail,
		Password:     verif.Password,
		FirstName:    verif.FirstName,
		LastName:     verif.LastName,
		RoleID:       roleID,       // 🔹 แก้ตรงนี้
		ProfileImage: verif.Image,
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		log.Printf("VerifyAndActivateUser: create user error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้างบัญชีไม่สำเร็จ"})
		return
	}

	if err := tx.Exec("DELETE FROM user_verifications WHERE gmail = ?", input.Gmail).Error; err != nil {
		tx.Rollback()
		log.Printf("VerifyAndActivateUser: delete user_verifications error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ลบข้อมูลยืนยัน OTP ไม่สำเร็จ"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เกิดข้อผิดพลาดขณะบันทึกข้อมูล"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ยืนยันสำเร็จ บัญชีถูกสร้างแล้ว"})
}

func GetUsers(c *gin.Context) {
	roleVal, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้รับอนุญาต"})
		return
	}

	var currentRole string
	switch v := roleVal.(type) {
	case string:
		currentRole = v
	case int:
		switch v {
		case 1:
			currentRole = "superadmin"
		case 2:
			currentRole = "admin"
		default:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ข้อมูล role ไม่ถูกต้อง"})
			return
		}
	default:
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ข้อมูล role ไม่ถูกต้อง"})
		return
	}

	var users []models.User
	if err := config.DB.Preload("Role").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถดึงข้อมูลผู้ใช้ได้"})
		return
	}

	var filteredUsers []models.User
	for _, user := range users {
		switch currentRole {
		case "superadmin":
			filteredUsers = append(filteredUsers, user)
		case "admin":
			if user.RoleID == RoleEmployee { // 🔹 ไม่ต้องเช็ค nil และไม่ต้อง dereference
				filteredUsers = append(filteredUsers, user)
			}
		default:
			c.JSON(http.StatusForbidden, gin.H{"error": "คุณไม่มีสิทธิ์ดูรายชื่อผู้ใช้"})
			return
		}
	}

	responses := make([]models.UserProfileResponse, 0, len(filteredUsers))
	for _, user := range filteredUsers {
		var profileImage string
		if len(user.ProfileImage) > 0 {
			profileImage = base64.StdEncoding.EncodeToString(user.ProfileImage)
		}

		responses = append(responses, models.UserProfileResponse{
			Gmail:        user.Gmail,
			FirstName:    user.FirstName,
			LastName:     user.LastName,
			RoleID:       uint32(user.RoleID), // 🔹 แค่ convert เป็น uint32
			ProfileImage: profileImage,
		})
	}

	c.JSON(http.StatusOK, gin.H{"users": responses})
}

func UpdateUser(c *gin.Context) {
	roleVal, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้รับอนุญาต"})
		return
	}
	currentRole, ok := roleVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ข้อมูล role ไม่ถูกต้อง"})
		return
	}

	gmail := c.Param("gmail")
	if gmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gmail ไม่ถูกต้อง"})
		return
	}

	var user models.User
	if err := config.DB.Preload("Role").Where("gmail = ?", gmail).First(&user).Error; err != nil {
		log.Printf("UpdateUser: ไม่พบ user gmail %v: %v", gmail, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบผู้ใช้"})
		return
	}

	if currentRole == "admin" && user.RoleID != RoleEmployee {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin แก้ไขได้เฉพาะ employee เท่านั้น"})
		return
	} else if currentRole != "admin" && currentRole != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "คุณไม่มีสิทธิ์แก้ไขข้อมูลนี้"})
		return
	}

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่สามารถอ่านข้อมูลฟอร์ม"})
		return
	}

	if firstName := c.PostForm("first_name"); firstName != "" {
		user.FirstName = firstName
	}
	if lastName := c.PostForm("last_name"); lastName != "" {
		user.LastName = lastName
	}

	if newPassword := c.PostForm("password"); newPassword != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "เข้ารหัสรหัสผ่านล้มเหลว"})
			return
		}
		user.Password = string(hashedPassword)
	}

	file, _, err := c.Request.FormFile("profile_image")
	if err == nil {
		defer file.Close()
		imageData, readErr := io.ReadAll(file)
		if readErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านไฟล์รูปภาพไม่สำเร็จ"})
			return
		}
		user.ProfileImage = imageData
	} else if err != http.ErrMissingFile {
		c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านไฟล์รูปภาพไม่สำเร็จ"})
		return
	}

	if roleIDStr := c.PostForm("role_id"); roleIDStr != "" {
		parsedRoleID, err := strconv.ParseUint(roleIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "role_id ไม่ถูกต้อง"})
			return
		}
		user.RoleID = uint64(parsedRoleID) // 🔹 assign ธรรมดา ไม่ต้อง pointer
	}

	if err := config.DB.Save(&user).Error; err != nil {
		log.Printf("UpdateUser: save user error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถอัปเดตผู้ใช้ได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "อัปเดตข้อมูลสำเร็จ",
		"user": gin.H{
			"gmail":      user.Gmail,
			"first_name": user.FirstName,
			"last_name":  user.LastName,
			"role_id":    user.RoleID,       // 🔹 ใช้ตรง ๆ
			"role_name":  user.Role.Name,
		},
	})
}

func DeleteUser(c *gin.Context) {
	// ดึง role จาก JWT
	roleVal, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้รับอนุญาต"})
		return
	}
	currentRole, ok := roleVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ข้อมูล role ไม่ถูกต้อง"})
		return
	}

	// ดึง gmail จาก param
	gmailParam := c.Param("gmail")
	if gmailParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gmail ไม่ถูกต้อง"})
		return
	}

	// ดึงข้อมูลผู้ใช้จาก DB ตาม Gmail
	var user models.User
	if err := config.DB.Preload("Role").Where("gmail = ?", gmailParam).First(&user).Error; err != nil {
		log.Printf("DeleteUser: ไม่พบ user Gmail %v: %v", gmailParam, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// ตรวจสอบสิทธิ์
	if currentRole == "admin" {
		if user.RoleID != RoleEmployee {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin ลบได้เฉพาะ employee เท่านั้น"})
			return
		}
	} else if currentRole != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "คุณไม่มีสิทธิ์ลบข้อมูลนี้"})
		return
	}

	// ลบผู้ใช้
	if err := config.DB.Delete(&user).Error; err != nil {
		log.Printf("DeleteUser: delete user error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถลบผู้ใช้ได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ลบผู้ใช้สำเร็จ"})
}