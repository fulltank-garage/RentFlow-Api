package controllers

import (
	"log"
	"net/http"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"

	"github.com/gin-gonic/gin"
)

func ForgotPassword(c *gin.Context) {
	var input struct {
		Gmail       string `json:"gmail" binding:"required,email"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := config.DB.Where("gmail = ?", input.Gmail).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบัญชีผู้ใช้ที่ใช้อีเมลนี้"})
		return
	}

	hashed, err := services.HashPassword(input.NewPassword)
	if err != nil {
		log.Println("Error hashing password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เข้ารหัสรหัสผ่านไม่สำเร็จ"})
		return
	}

	if err := config.DB.Model(&user).Update("password", hashed).Error; err != nil {
		log.Println("Error updating password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถรีเซ็ทรหัสผ่านได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "รีเซ็ทรหัสผ่านเรียบร้อยแล้ว",
	})
}

func ChangeOwnPassword(c *gin.Context) {
	var input struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.MustGet("userID").(uint)
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		log.Println("User not found:", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบผู้ใช้"})
		return
	}

	if !services.CheckPasswordHash(input.OldPassword, user.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสผ่านเดิมไม่ถูกต้อง"})
		return
	}

	if input.OldPassword == input.NewPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสผ่านใหม่ต้องไม่เหมือนรหัสผ่านเก่า"})
		return
	}

	hashed, err := services.HashPassword(input.NewPassword)
	if err != nil {
		log.Println("Error hashing new password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เข้ารหัสรหัสผ่านไม่สำเร็จ"})
		return
	}

	if err := config.DB.Model(&user).Update("password", hashed).Error; err != nil {
		log.Println("Error updating password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เปลี่ยนรหัสผ่านไม่สำเร็จ"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "เปลี่ยนรหัสผ่านเรียบร้อยแล้ว"})
}

func AdminChangeUserPassword(c *gin.Context) {
	userID := c.MustGet("userID").(uint)

	var currentUser models.User
	if err := config.DB.First(&currentUser, userID).Error; err != nil {
		log.Println("Current user not found:", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if currentUser.RoleID > 2 {
		c.JSON(http.StatusForbidden, gin.H{"error": "คุณไม่มีสิทธิ์เปลี่ยนรหัสผ่านให้ผู้อื่น"})
		return
	}

	targetID := c.Param("id")
	var targetUser models.User
	if err := config.DB.First(&targetUser, targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบผู้ใช้เป้าหมาย"})
		return
	}

	if currentUser.RoleID == 2 && targetUser.RoleID <= 2 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin ไม่สามารถเปลี่ยนรหัสผ่านของ Admin หรือ Superadmin ได้"})
		return
	}

	var input struct {
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashed, err := services.HashPassword(input.NewPassword)
	if err != nil {
		log.Println("Error hashing password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เข้ารหัสรหัสผ่านไม่สำเร็จ"})
		return
	}

	if err := config.DB.Model(&targetUser).Update("password", hashed).Error; err != nil {
		log.Println("Error updating target user password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถเปลี่ยนรหัสผ่านได้"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "เปลี่ยนรหัสผ่านสำเร็จ"})
}
