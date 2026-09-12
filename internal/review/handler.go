package review

import (
	"errors"
	"net/http"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createRequest struct {
	BookingID int64  `json:"booking_id"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
}

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

	rv, err := h.service.Create(r.Context(), req.BookingID, claims.UserID, req.Rating, req.Comment)
	if errors.Is(err, ErrInvalidRating) {
		httputil.Error(w, http.StatusBadRequest, "rating must be between 1 and 5")
		return
	}
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not create review")
		return
	}
	httputil.JSON(w, http.StatusCreated, rv)
}
