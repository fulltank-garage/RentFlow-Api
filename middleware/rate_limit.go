package middleware

import (
	"net/http"
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

func RateLimit(maxRequests int, window time.Duration) gin.HandlerFunc {
	if maxRequests <= 0 {
		maxRequests = 120
	}
	if window <= 0 {
		window = time.Minute
	}

	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP() + ":" + c.FullPath()

		rateLimitMu.Lock()
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
