package middleware

import (
	"net/http"
	"strings"
	"rentflow-api/services"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "กรุณาใส่ Authorization Header"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims := &services.JWTClaims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return services.JwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token ไม่ถูกต้องหรือหมดอายุ"})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func RoleAuthorization(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "ไม่พบข้อมูล role"})
			c.Abort()
			return
		}

		userRole, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "ข้อมูล role ไม่ถูกต้อง"})
			c.Abort()
			return
		}

		for _, r := range requiredRoles {
			if userRole == r {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "คุณไม่มีสิทธิ์เข้าถึง"})
		c.Abort()
	}
}

func DBMiddleware(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Set("DB", db)
        c.Next()
    }
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้รับอนุญาต"})
			c.Abort()
			return
		}
		userRole, ok := role.(string)
		if !ok || userRole != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "อนุญาตเฉพาะ admin เท่านั้น"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func SuperAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้รับอนุญาต"})
			c.Abort()
			return
		}
		userRole, ok := role.(string)
		if !ok || userRole != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "อนุญาตเฉพาะ superadmin เท่านั้น"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func AdminOrSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้รับอนุญาต"})
			c.Abort()
			return
		}
		userRole, ok := role.(string)
		if !ok || (userRole != "admin" && userRole != "superadmin") {
			c.JSON(http.StatusForbidden, gin.H{"error": "อนุญาตเฉพาะ admin หรือ superadmin เท่านั้น"})
			c.Abort()
			return
		}
		c.Next()
	}
}