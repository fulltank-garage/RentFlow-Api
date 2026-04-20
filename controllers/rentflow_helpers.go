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
	secure := strings.EqualFold(os.Getenv("APP_ENV"), "production")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		services.RentFlowSessionCookieName,
		token,
		maxAge,
		"/",
		"",
		secure,
		true,
	)
}

func clearRentFlowSessionCookie(c *gin.Context) {
	secure := strings.EqualFold(os.Getenv("APP_ENV"), "production")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		services.RentFlowSessionCookieName,
		"",
		-1,
		"/",
		"",
		secure,
		true,
	)
}
