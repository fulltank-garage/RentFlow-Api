package middleware

import (
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORSMiddleware() gin.HandlerFunc {
	allowedOrigins := map[string]bool{
		"http://localhost:3000":            true,
		"http://127.0.0.1:3000":            true,
		"https://sci-stock-app.vercel.app": true,
	}

	if extraOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); extraOrigins != "" {
		for _, origin := range strings.Split(extraOrigins, ",") {
			trimmed := strings.TrimSpace(origin)
			if trimmed != "" {
				allowedOrigins[trimmed] = true
			}
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if allowedOrigins[origin] || isLocalDevelopmentOrigin(origin) || isRentFlowSubdomainOrigin(origin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, PATCH, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cookie, X-RentFlow-Host, X-RentFlow-Tenant, X-RentFlow-Marketplace")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func isLocalDevelopmentOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := strings.Trim(strings.ToLower(parsed.Hostname()), ".")
	return (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		(host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1")
}

func isRentFlowSubdomainOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := strings.Trim(strings.ToLower(parsed.Hostname()), ".")
	rootDomain := strings.Trim(strings.ToLower(os.Getenv("RENTFLOW_ROOT_DOMAIN")), ". ")
	if rootDomain == "" {
		rootDomain = "rentflow.com"
	}

	return parsed.Scheme == "https" && (host == rootDomain || strings.HasSuffix(host, "."+rootDomain))
}
