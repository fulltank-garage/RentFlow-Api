package config

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client
var Ctx = context.Background()

func ConnectRedis() {
	redisURL := os.Getenv("REDIS_PUBLIC_URL")

	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}

	if redisURL == "" {
		log.Println("ไม่พบ REDIS_URL ระบบจะทำงานโดยไม่ใช้ Redis (fallback DB)")
		return
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Println("แปลง REDIS_URL ไม่สำเร็จ:", err)
		return
	}

	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second

	RDB = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := RDB.Ping(ctx).Result(); err != nil {
		log.Println("เชื่อมต่อ Redis ไม่สำเร็จ จะใช้ DB แทน:", err)
		RDB = nil
		return
	}

	log.Println("เชื่อมต่อ Redis สำเร็จแล้ว")
}