package realtime

import (
	"fmt"
	"net/http"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

// SSEHandler serves GET /api/v1/bookings/events — the owner dashboard's
// live booking-status feed. SSE is chosen over WebSockets for this one
// because it's one-directional and gets automatic browser reconnection for
// free; internal/booking/handler.go (via the hub passed at construction)
// calls Hub.PublishOwner after every booking mutation.
type SSEHandler struct {
	hub    *Hub
	issuer *auth.JWTIssuer
}

func NewSSEHandler(hub *Hub, issuer *auth.JWTIssuer) *SSEHandler {
	return &SSEHandler{hub: hub, issuer: issuer}
}

func (h *SSEHandler) BookingEvents(w http.ResponseWriter, r *http.Request) {
	// EventSource cannot set an Authorization header, so this one endpoint
	// also accepts the access token as a query parameter — see web/js/sse.js.
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		token := r.URL.Query().Get("access_token")
		verified, err := h.issuer.Verify(token)
		if err != nil {
			httputil.Error(w, http.StatusUnauthorized, "invalid or missing access token")
			return
		}
		claims = verified
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.Error(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ch, cancel := h.hub.SubscribeOwner(claims.UserID)
	defer cancel()

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "event: booking-update\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
