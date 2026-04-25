package controllers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rentflow-api/services"
)

type rentFlowAPIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func rentFlowSuccess(c *gin.Context, status int, message string, data interface{}) {
	c.JSON(status, rentFlowAPIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func rentFlowError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}

func setRentFlowSessionCookie(c *gin.Context, token string) {
	maxAge := int((7 * 24 * time.Hour).Seconds())
	cookieName := rentFlowSessionCookieNameFromRequest(c)
	rentFlowWriteCookie(c, cookieName, token, maxAge, true)

	if cookieName != services.RentFlowLegacySessionCookieName {
		rentFlowWriteCookie(c, services.RentFlowLegacySessionCookieName, "", -1, true)
	}
}

func clearRentFlowSessionCookie(c *gin.Context) {
	rentFlowWriteCookie(c, rentFlowSessionCookieNameFromRequest(c), "", -1, true)
	rentFlowWriteCookie(c, services.RentFlowLegacySessionCookieName, "", -1, true)
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

func rentFlowSessionTokenFromRequest(c *gin.Context) string {
	cookieName := rentFlowSessionCookieNameFromRequest(c)
	if token, err := c.Cookie(cookieName); err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	return ""
}

func rentFlowWriteCookie(c *gin.Context, name, value string, maxAge int, httpOnly bool) {
	secure := rentFlowCookieSecure()
	cookieDomain := strings.TrimSpace(os.Getenv("RENTFLOW_COOKIE_DOMAIN"))
	c.SetSameSite(rentFlowCookieSameSite(secure))
	c.SetCookie(
		name, value, maxAge, "/", cookieDomain, secure, httpOnly,
	)
}

func rentFlowCookieSecure() bool {
	return strings.EqualFold(os.Getenv("APP_ENV"), "production") ||
		strings.EqualFold(os.Getenv("RENTFLOW_COOKIE_SECURE"), "true")
}

func rentFlowCookieSameSite(secure bool) http.SameSite {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("RENTFLOW_COOKIE_SAMESITE"))) {
	case "none":
		if secure {
			return http.SameSiteNoneMode
		}
		return http.SameSiteLaxMode
	case "lax":
		return http.SameSiteLaxMode
	case "strict", "":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteStrictMode
	}
}
