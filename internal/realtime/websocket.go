package realtime

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/thvnhtai/gearshare/internal/availability"
)

// WebSocketHandler serves GET /ws/listings/{id} — live availability updates
// for the listing detail page (web/js/ws.js). Chosen over SSE here because
// a later iteration (in-app renter/owner chat, /ws/chat/{bookingId}) needs
// a bidirectional channel, and this keeps both real-time listing features
// on one consistent transport.
type WebSocketHandler struct {
	hub          *Hub
	availability *availability.Repository
	upgrader     websocket.Upgrader
}

func NewWebSocketHandler(hub *Hub, availabilityRepo *availability.Repository, allowedOrigins []string) *WebSocketHandler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}
	return &WebSocketHandler{
		hub:          hub,
		availability: availabilityRepo,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				_, ok := originSet[r.Header.Get("Origin")]
				return ok
			},
		},
	}
}

type availabilitySnapshot struct {
	ListingID int64                `json:"listing_id"`
	Blocks    []availability.Block `json:"blocks"`
}

func (h *WebSocketHandler) ListingAvailability(w http.ResponseWriter, r *http.Request) {
	listingID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid listing id", http.StatusBadRequest)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error response.
	}
	defer func() { _ = conn.Close() }()

	blocks, err := h.availability.ListForListing(r.Context(), listingID)
	if err == nil {
		if payload, err := json.Marshal(availabilitySnapshot{ListingID: listingID, Blocks: blocks}); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, payload)
		}
	}

	ch, unsubscribe := h.hub.SubscribeListing(listingID)
	var once sync.Once
	cancel := func() { once.Do(unsubscribe) }
	defer cancel()

	// Read pump: discards client messages but is required so gorilla/websocket
	// processes control frames (ping/pong/close) and detects a dead connection.
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	for payload := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}
