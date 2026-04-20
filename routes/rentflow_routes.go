package routes

import (
	"github.com/gin-gonic/gin"
	"rentflow-api/controllers"
	"rentflow-api/middleware"
)

func RegisterRentFlowRoutes(r *gin.Engine) {
	r.Use(middleware.AttachRentFlowSession())

	r.POST("/auth/google", controllers.RentFlowAuthWithGoogle)
	r.GET("/auth/me", controllers.RentFlowGetMe)
	r.POST("/auth/logout", controllers.RentFlowLogout)

	r.GET("/cars", controllers.RentFlowGetCars)
	r.GET("/branches", controllers.RentFlowGetBranches)
	r.GET("/branches/:branchId", controllers.RentFlowGetBranchByID)
	r.POST("/availability/check", controllers.RentFlowCheckAvailability)
	r.GET("/availability/:carId/unavailable-dates", controllers.RentFlowGetUnavailableDates)

	r.POST("/bookings/preview", controllers.RentFlowPreviewBookingPrice)
	r.POST("/bookings", controllers.RentFlowCreateBooking)
	r.POST("/payments", controllers.RentFlowCreatePayment)
	r.GET("/payments/booking/:bookingId", controllers.RentFlowGetPaymentByBookingID)

	protected := r.Group("/")
	protected.Use(middleware.RequireRentFlowSession())
	{
		protected.GET("/users/me", controllers.RentFlowUserMe)
		protected.PATCH("/users/me", controllers.RentFlowUpdateMe)
		protected.PATCH("/users/me/password", controllers.RentFlowChangePassword)

		protected.GET("/bookings/me", controllers.RentFlowGetMyBookings)
		protected.GET("/bookings/:bookingId", controllers.RentFlowGetBookingByID)
		protected.PATCH("/bookings/:bookingId/cancel", controllers.RentFlowCancelBooking)

		protected.GET("/notifications", controllers.RentFlowGetNotifications)
		protected.PATCH("/notifications/:notificationId/read", controllers.RentFlowMarkNotificationAsRead)
		protected.PATCH("/notifications/read-all", controllers.RentFlowMarkAllNotificationsAsRead)
	}
}
