package routes

import (
	"rentflow-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.RateLimitFromEnv())
	RegisterRentFlowRoutes(r)
}
