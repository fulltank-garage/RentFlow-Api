package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/routes"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("No .env file found, reading environment variables from system")
	}

	config.ConnectDatabase()
	db := config.DB

	config.ConnectRedis()
	if config.RDB == nil {
		log.Println("❌ Redis = NIL (ไม่ได้เชื่อม)")
	} else {
		log.Println("✅ Redis = CONNECTED")
	}

	if err := db.AutoMigrate(&models.Role{}, &models.User{}); err != nil {
		log.Fatal("Migration failed: ", err)
	}

	roles := []string{"superadmin", "admin", "employee", "user"}
	for _, r := range roles {
		db.FirstOrCreate(&models.Role{}, models.Role{Name: r})
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	routes.SetupRoutes(router)
	router.Run(":8080")
}
