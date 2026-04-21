package routes

import (
	"time"

	"rentflow-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.RateLimit(300, time.Minute))
	RegisterRentFlowRoutes(r)
}
