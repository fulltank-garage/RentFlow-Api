package middleware

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitBucket struct {
	Count     int
	ExpiresAt time.Time
}

var (
	rateLimitMu      sync.Mutex
	rateLimitBuckets = map[string]rateLimitBucket{}
)

func RateLimitFromEnv() gin.HandlerFunc {
	maxRequests := 300
	if value := strings.TrimSpace(os.Getenv("RENTFLOW_RATE_LIMIT_MAX")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			maxRequests = parsed
		}
	}

	window := time.Minute
	if value := strings.TrimSpace(os.Getenv("RENTFLOW_RATE_LIMIT_WINDOW_SECONDS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			window = time.Duration(parsed) * time.Second
		}
	}

	return RateLimit(maxRequests, window)
}

func RateLimit(maxRequests int, window time.Duration) gin.HandlerFunc {
	if maxRequests <= 0 {
		maxRequests = 120
	}
	if window <= 0 {
		window = time.Minute
	}

	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || strings.HasPrefix(c.Request.URL.Path, "/ws/") {
			c.Next()
			return
		}

		now := time.Now()
		key := c.ClientIP() + ":" + c.FullPath()

		rateLimitMu.Lock()
		for bucketKey, bucketValue := range rateLimitBuckets {
			if now.After(bucketValue.ExpiresAt) {
				delete(rateLimitBuckets, bucketKey)
			}
		}

		bucket := rateLimitBuckets[key]
		if now.After(bucket.ExpiresAt) {
			bucket = rateLimitBucket{ExpiresAt: now.Add(window)}
		}
		bucket.Count++
		rateLimitBuckets[key] = bucket
		rateLimitMu.Unlock()

		if bucket.Count > maxRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "มีการเรียกใช้งานมากเกินไป กรุณาลองใหม่อีกครั้ง",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
