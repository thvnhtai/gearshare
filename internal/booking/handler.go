package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/httputil"
	"github.com/thvnhtai/gearshare/internal/listing"
)

// OwnerNotifier is satisfied by *realtime.Hub's PublishOwner method
// (structural typing — this package can't import internal/realtime, which
// imports this package for its long-poll handler). Every successful
// booking mutation is pushed to the listing owner's SSE dashboard feed
// through this narrow interface.
type OwnerNotifier interface {
	PublishOwner(ownerID int64, payload []byte)
}

type Handler struct {
	service  *Service
	listings *listing.Repository
	notifier OwnerNotifier
}

func NewHandler(service *Service, listings *listing.Repository, notifier OwnerNotifier) *Handler {
	return &Handler{service: service, listings: listings, notifier: notifier}
}

func (h *Handler) notifyOwner(ctx context.Context, b *Booking) {
	if h.notifier == nil || h.listings == nil {
		return
	}
	l, err := h.listings.GetByID(ctx, b.ListingID)
	if err != nil {
		return
	}
	if payload, err := json.Marshal(toResponse(b)); err == nil {
		h.notifier.PublishOwner(l.OwnerID, payload)
	}
}

type createRequest struct {
	ListingID int64  `json:"listing_id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

const dateLayout = "2006-01-02"

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	start, err := time.Parse(dateLayout, req.StartDate)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "start_date must be YYYY-MM-DD")
		return
	}
	end, err := time.Parse(dateLayout, req.EndDate)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "end_date must be YYYY-MM-DD")
		return
	}

	b, err := h.service.CreateBooking(r.Context(), req.ListingID, claims.UserID, start, end)
	switch {
	case errors.Is(err, ErrListingUnavailable):
		httputil.Error(w, http.StatusConflict, "listing is not available for the requested dates")
	case errors.Is(err, ErrInvalidDateRange):
		httputil.Error(w, http.StatusBadRequest, err.Error())
	case err != nil:
		httputil.Error(w, http.StatusInternalServerError, "could not create booking")
	default:
		h.notifyOwner(r.Context(), b)
		httputil.JSON(w, http.StatusCreated, toResponse(b))
	}
}

func (h *Handler) transitionHandler(transition func(ctx *http.Request, id int64) (*Booking, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid booking id")
			return
		}
		b, err := transition(r, id)
		switch {
		case errors.Is(err, ErrNotFound):
			httputil.Error(w, http.StatusNotFound, "booking not found")
		case errors.Is(err, ErrInvalidTransition):
			httputil.Error(w, http.StatusConflict, "invalid status transition for this booking")
		case err != nil:
			httputil.Error(w, http.StatusInternalServerError, "could not update booking")
		default:
			h.notifyOwner(r.Context(), b)
			httputil.JSON(w, http.StatusOK, toResponse(b))
		}
	}
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler(func(r *http.Request, id int64) (*Booking, error) { return h.service.Approve(r.Context(), id) })(w, r)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler(func(r *http.Request, id int64) (*Booking, error) { return h.service.Reject(r.Context(), id) })(w, r)
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler(func(r *http.Request, id int64) (*Booking, error) { return h.service.Cancel(r.Context(), id) })(w, r)
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler(func(r *http.Request, id int64) (*Booking, error) { return h.service.Complete(r.Context(), id) })(w, r)
}

// ListMine returns bookings for listings the authenticated user owns —
// backs the dashboard.html initial load, before the SSE feed takes over
// for subsequent updates.
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	bookings, err := h.service.ListForOwner(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not load bookings")
		return
	}
	responses := make([]Response, 0, len(bookings))
	for i := range bookings {
		responses = append(responses, toResponse(&bookings[i]))
	}
	httputil.JSON(w, http.StatusOK, responses)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	b, err := h.service.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "booking not found")
		return
	}
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not fetch booking")
		return
	}
	httputil.JSON(w, http.StatusOK, toResponse(b))
}
