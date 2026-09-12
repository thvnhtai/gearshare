// Package partner backs the API-Key-authenticated /partner/v1/* surface —
// GearShare's token/API-key auth style, for machine-to-machine integrations
// with no login flow or session (see internal/auth/apikey.go,
// internal/auth/middleware.go's RequireAPIKey).
package partner

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

type insuranceWebhookPayload struct {
	BookingID int64  `json:"booking_id"`
	EventType string `json:"event_type"` // e.g. "policy_bound", "claim_filed"
	PolicyRef string `json:"policy_ref"`
}

// InsuranceWebhook receives events from a fictitious insurance partner
// covering rental bookings. Authenticated by API key rather than JWT —
// there is no human, and therefore no login session, on the other end of
// this call.
func (h *Handler) InsuranceWebhook(w http.ResponseWriter, r *http.Request) {
	owner, _ := auth.APIKeyOwnerFromContext(r.Context())

	var payload insuranceWebhookPayload
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&payload); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid webhook payload")
		return
	}

	// A real integration would record this against the booking and notify
	// the renter/owner; logging it is enough to demonstrate the
	// authenticated-webhook pattern itself.
	log.Printf("partner: webhook from %q, booking=%d event=%s", owner, payload.BookingID, payload.EventType)

	httputil.JSON(w, http.StatusAccepted, map[string]string{"status": "received"})
}
