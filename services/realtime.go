package services

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	RentFlowRealtimeEventBookingCreated     = "booking.created"
	RentFlowRealtimeEventBookingUpdated     = "booking.updated"
	RentFlowRealtimeEventBookingCancelled   = "booking.cancelled"
	RentFlowRealtimeEventPaymentCreated     = "payment.created"
	RentFlowRealtimeEventPaymentUpdated     = "payment.updated"
	RentFlowRealtimeEventNotificationNew    = "notification.new"
	RentFlowRealtimeEventReviewCreated      = "review.created"
	RentFlowRealtimeEventCarChanged         = "car.changed"
	RentFlowRealtimeEventCarStatusChanged   = "car.status.changed"
	RentFlowRealtimeEventBranchChanged      = "branch.changed"
	RentFlowRealtimeEventAddonChanged       = "addon.changed"
	RentFlowRealtimeEventPromotionChanged   = "promotion.changed"
	RentFlowRealtimeEventLeadChanged        = "lead.changed"
	RentFlowRealtimeEventMemberChanged      = "member.changed"
	RentFlowRealtimeEventAvailabilityChange = "availability.changed"
	RentFlowRealtimeEventSupportChanged     = "support.changed"
	RentFlowRealtimeEventTenantUpdated      = "tenant.updated"
)

type RentFlowRealtimeEvent struct {
	Type      string      `json:"type"`
	TenantID  string      `json:"tenantId,omitempty"`
	UserID    string      `json:"userId,omitempty"`
	UserEmail string      `json:"userEmail,omitempty"`
	EntityID  string      `json:"entityId,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
}

type RentFlowRealtimeClientFilter struct {
	App         string
	TenantID    string
	UserID      string
	UserEmail   string
	Marketplace bool
}

type rentFlowRealtimeClient struct {
	hub    *rentFlowRealtimeHub
	conn   *websocket.Conn
	send   chan []byte
	filter RentFlowRealtimeClientFilter
}

type rentFlowRealtimeHub struct {
	mu      sync.RWMutex
	clients map[*rentFlowRealtimeClient]struct{}
}

var (
	rentFlowRealtime = &rentFlowRealtimeHub{
		clients: make(map[*rentFlowRealtimeClient]struct{}),
	}
	rentFlowRealtimeUpgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func RentFlowPublishRealtime(event RentFlowRealtimeEvent) {
	if strings.TrimSpace(event.Type) == "" {
		return
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	rentFlowRealtime.broadcast(event)
}

func RentFlowServeRealtime(w http.ResponseWriter, r *http.Request, filter RentFlowRealtimeClientFilter) error {
	filter.App = RentFlowNormalizeAppName(filter.App)
	filter.UserEmail = strings.TrimSpace(strings.ToLower(filter.UserEmail))
	conn, err := rentFlowRealtimeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	client := &rentFlowRealtimeClient{
		hub:    rentFlowRealtime,
		conn:   conn,
		send:   make(chan []byte, 32),
		filter: filter,
	}
	client.hub.register(client)

	go client.writePump()
	go client.readPump()
	return nil
}

func (h *rentFlowRealtimeHub) register(client *rentFlowRealtimeClient) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	client.enqueue(RentFlowRealtimeEvent{
		Type:      "connection.ready",
		TenantID:  client.filter.TenantID,
		UserID:    client.filter.UserID,
		UserEmail: client.filter.UserEmail,
		CreatedAt: time.Now(),
		Data: map[string]interface{}{
			"app":         client.filter.App,
			"marketplace": client.filter.Marketplace,
		},
	})
}

func (h *rentFlowRealtimeHub) unregister(client *rentFlowRealtimeClient) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
}

func (h *rentFlowRealtimeHub) broadcast(event RentFlowRealtimeEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*rentFlowRealtimeClient, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	for _, client := range clients {
		if rentFlowRealtimeMatches(client.filter, event) {
			client.enqueueBytes(payload)
		}
	}
}

func rentFlowRealtimeMatches(filter RentFlowRealtimeClientFilter, event RentFlowRealtimeEvent) bool {
	switch RentFlowNormalizeAppName(filter.App) {
	case RentFlowAppAdmin:
		return true
	case RentFlowAppPartner:
		return filter.TenantID != "" && filter.TenantID == event.TenantID
	default:
		if filter.UserID != "" && filter.UserID == event.UserID {
			return true
		}
		if filter.UserEmail != "" && strings.EqualFold(filter.UserEmail, event.UserEmail) {
			return true
		}
		if filter.TenantID != "" && filter.TenantID == event.TenantID {
			return true
		}
		if filter.Marketplace {
			return rentFlowRealtimeIsMarketplaceEvent(event.Type)
		}
		return false
	}
}

func rentFlowRealtimeIsMarketplaceEvent(eventType string) bool {
	switch eventType {
	case RentFlowRealtimeEventBookingCreated,
		RentFlowRealtimeEventBookingUpdated,
		RentFlowRealtimeEventBookingCancelled,
		RentFlowRealtimeEventReviewCreated,
		RentFlowRealtimeEventCarChanged,
		RentFlowRealtimeEventCarStatusChanged,
		RentFlowRealtimeEventBranchChanged,
		RentFlowRealtimeEventAddonChanged,
		RentFlowRealtimeEventPromotionChanged,
		RentFlowRealtimeEventAvailabilityChange,
		RentFlowRealtimeEventTenantUpdated:
		return true
	default:
		return false
	}
}

func (client *rentFlowRealtimeClient) enqueue(event RentFlowRealtimeEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	client.enqueueBytes(payload)
}

func (client *rentFlowRealtimeClient) enqueueBytes(payload []byte) {
	select {
	case client.send <- payload:
	default:
		client.hub.unregister(client)
	}
}

func (client *rentFlowRealtimeClient) readPump() {
	defer func() {
		client.hub.unregister(client)
		_ = client.conn.Close()
	}()

	client.conn.SetReadLimit(512)
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		if _, _, err := client.conn.NextReader(); err != nil {
			break
		}
	}
}

func (client *rentFlowRealtimeClient) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		_ = client.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
