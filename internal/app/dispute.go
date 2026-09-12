package app

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/damagereport"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

// DisputeHandler lives at the composition root, not inside internal/booking
// or internal/damagereport, because filing a dispute genuinely spans both
// domains: the booking's status machine (MySQL, transactional) and the
// incident write-up (MongoDB, document-shaped) — see
// docs/architecture.md's "data store boundaries" for why those two facts
// live in different databases at all. Neither domain package should import
// the other just to serve this one composite action.
type DisputeHandler struct {
	bookings *booking.Service
	reports  *damagereport.Repository // nil-safe: Mongo is an optional dependency, same as Redis/Kafka/RabbitMQ
}

func NewDisputeHandler(bookings *booking.Service, reports *damagereport.Repository) *DisputeHandler {
	return &DisputeHandler{bookings: bookings, reports: reports}
}

type disputeRequest struct {
	Description string   `json:"description"`
	PhotoURLs   []string `json:"photo_urls"`
}

func (h *DisputeHandler) Dispute(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var req disputeRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Description == "" {
		httputil.Error(w, http.StatusBadRequest, "description is required")
		return
	}

	b, err := h.bookings.Dispute(r.Context(), id)
	switch {
	case errors.Is(err, booking.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "booking not found")
		return
	case errors.Is(err, booking.ErrInvalidTransition):
		httputil.Error(w, http.StatusConflict, "booking cannot be disputed from its current status")
		return
	case err != nil:
		httputil.Error(w, http.StatusInternalServerError, "could not dispute booking")
		return
	}

	var reportID string
	if h.reports != nil {
		reportID, err = h.reports.Create(r.Context(), &damagereport.Report{
			BookingID:   b.ID,
			ReportedBy:  claims.UserID,
			Description: req.Description,
			PhotoURLs:   req.PhotoURLs,
		})
		if err != nil {
			// The booking is already marked disputed (durably, in MySQL) —
			// a Mongo write failure here shouldn't un-do that. Logged, not
			// fatal, the same graceful-degradation stance as every other
			// non-critical side effect in this codebase.
			httputil.JSON(w, http.StatusCreated, map[string]interface{}{
				"booking": b, "damage_report_id": nil, "warning": "booking disputed, but damage report could not be saved",
			})
			return
		}
	}

	httputil.JSON(w, http.StatusCreated, map[string]interface{}{
		"booking": b, "damage_report_id": reportID,
	})
}
