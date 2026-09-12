package realtime

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

// PollingHandler serves GET /api/v1/bookings/{id}/poll?since=<unix-millis> —
// the long-polling fallback for clients that can't hold a WebSocket or
// EventSource open. A genuine long poll: the request blocks (checking on an
// interval) until the booking's updated_at moves past `since`, or a server
// -side timeout elapses, whichever comes first — not a bare single-shot
// read, which would just be ordinary short polling wearing a longer name.
type PollingHandler struct {
	bookings *booking.Repository
	timeout  time.Duration
	interval time.Duration
}

func NewPollingHandler(bookings *booking.Repository) *PollingHandler {
	return &PollingHandler{bookings: bookings, timeout: 25 * time.Second, interval: 500 * time.Millisecond}
}

type pollResponse struct {
	Changed    bool              `json:"changed"`
	Booking    *booking.Response `json:"booking,omitempty"`
	ServerTime int64             `json:"server_time"`
}

func (h *PollingHandler) Poll(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	since := int64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		since, _ = strconv.ParseInt(v, 10, 64)
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		b, err := h.bookings.GetByID(ctx, id)
		if errors.Is(err, booking.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "booking not found")
			return
		}
		if err != nil {
			httputil.Error(w, http.StatusInternalServerError, "could not check booking status")
			return
		}

		if b.UpdatedAt.UnixMilli() > since {
			resp := booking.ToResponse(b)
			httputil.JSON(w, http.StatusOK, pollResponse{
				Changed:    true,
				Booking:    &resp,
				ServerTime: time.Now().UnixMilli(),
			})
			return
		}

		select {
		case <-ctx.Done():
			httputil.JSON(w, http.StatusOK, pollResponse{Changed: false, ServerTime: time.Now().UnixMilli()})
			return
		case <-ticker.C:
			// loop and re-check
		}
	}
}
