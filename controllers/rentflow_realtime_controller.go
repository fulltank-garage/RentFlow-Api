package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"rentflow-api/middleware"
	"rentflow-api/models"
	"rentflow-api/services"
)

func RentFlowRealtimeSocket(c *gin.Context) {
	app := services.RentFlowNormalizeAppName(firstNonBlank(
		c.Query("app"),
		c.GetHeader(services.RentFlowAppHeaderName),
	))

	filter := services.RentFlowRealtimeClientFilter{
		App:         app,
		Marketplace: rentFlowIsMarketplaceRequest(c),
	}

	if user, ok := middleware.CurrentRentFlowUser(c); ok {
		filter.UserID = user.ID
		filter.UserEmail = user.Email
	}

	switch app {
	case services.RentFlowAppAdmin:
		user, ok := middleware.CurrentRentFlowUser(c)
		if !ok || !services.IsRentFlowPlatformAdmin(user) {
			rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบผู้ดูแลก่อน")
			return
		}
	case services.RentFlowAppPartner:
		tenant, err := rentFlowCurrentUserTenant(c)
		if err != nil {
			rentFlowError(c, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบ Partner ก่อน")
			return
		}
		filter.TenantID = tenant.ID
	default:
		if tenant, err := rentFlowTenantFromRequest(c, false); err == nil && tenant != nil {
			filter.TenantID = tenant.ID
		}
	}

	if err := services.RentFlowServeRealtime(c.Writer, c.Request, filter); err != nil {
		return
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func rentFlowPublishBookingRealtime(eventType string, booking models.RentFlowBooking) {
	userID := ""
	if booking.UserID != nil {
		userID = *booking.UserID
	}
	services.RentFlowPublishRealtime(services.RentFlowRealtimeEvent{
		Type:      eventType,
		TenantID:  booking.TenantID,
		UserID:    userID,
		UserEmail: booking.CustomerEmail,
		EntityID:  booking.ID,
		Data: gin.H{
			"id":          booking.ID,
			"bookingCode": booking.BookingCode,
			"carId":       booking.CarID,
			"status":      booking.Status,
		},
	})
	rentFlowPublishCarRealtime(booking.TenantID, booking.CarID, services.RentFlowRealtimeEventAvailabilityChange)
}

func rentFlowPublishPaymentRealtime(eventType string, payment models.RentFlowPayment) {
	services.RentFlowPublishRealtime(services.RentFlowRealtimeEvent{
		Type:     eventType,
		TenantID: payment.TenantID,
		EntityID: payment.ID,
		Data: gin.H{
			"id":        payment.ID,
			"bookingId": payment.BookingID,
			"status":    payment.Status,
			"amount":    payment.Amount,
		},
	})
}

func rentFlowPublishCarRealtime(tenantID, carID, eventType string) {
	if strings.TrimSpace(eventType) == "" {
		eventType = services.RentFlowRealtimeEventCarChanged
	}
	services.RentFlowPublishRealtime(services.RentFlowRealtimeEvent{
		Type:     eventType,
		TenantID: tenantID,
		EntityID: carID,
		Data: gin.H{
			"carId": carID,
		},
	})
}

func rentFlowPublishSupportRealtime(tenantID, ticketID, eventType string) {
	services.RentFlowPublishRealtime(services.RentFlowRealtimeEvent{
		Type:     eventType,
		TenantID: tenantID,
		EntityID: ticketID,
		Data: gin.H{
			"ticketId": ticketID,
		},
	})
}
