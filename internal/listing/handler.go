package listing

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/cache"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

type Handler struct {
	service *Service
	cache   *cache.ListingCache // nil-safe: a nil cache just always misses
}

func NewHandler(service *Service, listingCache *cache.ListingCache) *Handler {
	return &Handler{service: service, cache: listingCache}
}

// Feed serves GET /api/v1/listings: Redis cache-aside first
// (internal/cache/listing_cache.go), falling through to the optimized JOIN
// query on a miss or a Redis outage (graceful degradation), then layering
// HTTP client-side caching (ETag/Cache-Control) on top of whichever path
// produced the body — see internal/httputil/etag.go.
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	if h.cache != nil {
		if body, hit := h.cache.GetFeed(r.Context(), limit, offset); hit {
			httputil.WriteCacheableJSON(w, r, http.StatusOK, time.Now(), body)
			return
		}
	}

	items, err := h.service.Feed(r.Context(), limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not load listings")
		return
	}

	body, err := json.Marshal(items)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not encode listings")
		return
	}
	if h.cache != nil {
		h.cache.SetFeed(r.Context(), limit, offset, body)
	}
	httputil.WriteCacheableJSON(w, r, http.StatusOK, time.Now(), body)
}

func (h *Handler) GetDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid listing id")
		return
	}

	if h.cache != nil {
		if body, hit := h.cache.GetDetail(r.Context(), id); hit {
			httputil.WriteCacheableJSON(w, r, http.StatusOK, time.Now(), body)
			return
		}
	}

	item, err := h.service.GetDetail(r.Context(), id)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "listing not found")
		return
	}

	body, err := json.Marshal(item)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not encode listing")
		return
	}
	if h.cache != nil {
		h.cache.SetDetail(r.Context(), id, body)
	}
	httputil.WriteCacheableJSON(w, r, http.StatusOK, time.Now(), body)
}

type createRequest struct {
	CategoryID       int64  `json:"category_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	PricePerDayCents int64  `json:"price_per_day_cents"`
	DepositCents     int64  `json:"deposit_cents"`
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

	l, err := h.service.Create(r.Context(), CreateInput{
		OwnerID:          claims.UserID,
		CategoryID:       req.CategoryID,
		Title:            req.Title,
		Description:      req.Description,
		PricePerDayCents: req.PricePerDayCents,
		DepositCents:     req.DepositCents,
	})
	if errors.Is(err, ErrInvalidInput) {
		httputil.Error(w, http.StatusBadRequest, "title and a positive price_per_day_cents are required")
		return
	}
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not create listing")
		return
	}
	httputil.JSON(w, http.StatusCreated, l)
}
