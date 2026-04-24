package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"rentflow-api/models"
)

var DB *gorm.DB

func ConnectDatabase() {
	if err := godotenv.Load(); err != nil {
		log.Println("ไม่พบไฟล์ .env กำลังใช้ค่าจากสภาพแวดล้อมของระบบแทน")
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Bangkok",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("เชื่อมต่อฐานข้อมูลไม่สำเร็จ:", err)
	}

	DB = db

	db.AutoMigrate(
		&models.RentFlowUser{},
		&models.RentFlowTenant{},
		&models.RentFlowTenantPromoImage{},
		&models.RentFlowPlatformSetting{},
		&models.RentFlowBranch{},
		&models.RentFlowCar{},
		&models.RentFlowCarImage{},
		&models.RentFlowBooking{},
		&models.RentFlowPayment{},
		&models.RentFlowNotification{},
		&models.RentFlowMessageLog{},
		&models.RentFlowReview{},
		&models.RentFlowTenantMember{},
		&models.RentFlowCustomDomain{},
		&models.RentFlowAuditLog{},
		&models.RentFlowAvailabilityBlock{},
		&models.RentFlowPromotion{},
		&models.RentFlowAddon{},
		&models.RentFlowLead{},
		&models.RentFlowLineChannel{},
		&models.RentFlowSupportTicket{},
		&models.RentFlowSupportMessage{},
	)
}
