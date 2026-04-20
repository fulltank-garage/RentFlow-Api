package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

const (
	rentFlowSessionKey = "rentflow.session"
	rentFlowUserKey    = "rentflow.user"
)

func AttachRentFlowSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := c.Cookie(services.RentFlowSessionCookieName)
		if token == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			}
		}

		if token == "" {
			c.Next()
			return
		}

		session, err := services.GetSession(config.Ctx, token)
		if err != nil || session == nil {
			c.Next()
			return
		}

		var user models.RentFlowUser
		if err := config.DB.Where("id = ?", session.UserID).First(&user).Error; err != nil {
			_ = services.DeleteSession(config.Ctx, token)
			c.Next()
			return
		}

		c.Set(rentFlowSessionKey, *session)
		c.Set(rentFlowUserKey, user)
		c.Next()
	}
}

func RequireRentFlowSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get(rentFlowUserKey); !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "กรุณาเข้าสู่ระบบก่อน",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func CurrentRentFlowUser(c *gin.Context) (*models.RentFlowUser, bool) {
	value, ok := c.Get(rentFlowUserKey)
	if !ok {
		return nil, false
	}

	user, ok := value.(models.RentFlowUser)
	if !ok {
		return nil, false
	}
	return &user, true
}
