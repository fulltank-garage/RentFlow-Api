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
		token := rentFlowSessionTokenFromRequest(c)
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
		if strings.EqualFold(user.Status, "locked") || strings.EqualFold(user.Status, "disabled") {
			_ = services.DeleteSession(config.Ctx, token)
			c.Next()
			return
		}

		c.Set(rentFlowSessionKey, *session)
		c.Set(rentFlowUserKey, user)
		c.Next()
	}
}

func rentFlowSessionTokenFromRequest(c *gin.Context) string {
	cookieName := rentFlowSessionCookieNameFromRequest(c)
	if token, err := c.Cookie(cookieName); err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	return ""
}

func rentFlowSessionCookieNameFromRequest(c *gin.Context) string {
	app := strings.TrimSpace(c.Query("app"))
	if app == "" {
		app = strings.TrimSpace(c.GetHeader(services.RentFlowAppHeaderName))
	}
	if app == "" {
		path := c.Request.URL.Path
		switch {
		case strings.HasPrefix(path, "/platform"):
			app = services.RentFlowAppAdmin
		case strings.HasPrefix(path, "/partner"), path == "/tenants/me":
			app = services.RentFlowAppPartner
		default:
			app = services.RentFlowAppStorefront
		}
	}

	return services.RentFlowSessionCookieNameForApp(app)
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
