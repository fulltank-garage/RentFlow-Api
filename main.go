package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/routes"
	"rentflow-api/services"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("ไม่พบไฟล์ .env กำลังอ่านค่าจากสภาพแวดล้อมของระบบแทน")
	}

	config.ConnectDatabase()
	db := config.DB

	config.ConnectRedis()
	if config.RDB == nil {
		log.Println("Redis ยังไม่ได้เชื่อมต่อ")
	} else {
		log.Println("Redis เชื่อมต่อแล้ว")
	}

	if err := models.SeedRentFlowData(db); err != nil {
		log.Fatal("เตรียมข้อมูลเริ่มต้นของ RentFlow ไม่สำเร็จ: ", err)
	}
	services.CacheDeleteByPrefix(config.Ctx, services.RentFlowCarsCachePrefix())

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	routes.SetupRoutes(router)
	router.Run(":8080")
}
