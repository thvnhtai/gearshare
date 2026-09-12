package search

import (
	"context"
	"net/http"
	"strconv"

	"github.com/sony/gobreaker"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

// Service is the monolith-side entry point for listing search: try
// search-indexer over gRPC behind a circuit breaker, fall back to a MySQL
// LIKE query (fallback_mysql.go) on any failure or an open breaker. This is
// requirement 19's "graceful degradation" pattern, made concrete.
type Service struct {
	grpcClient *GRPCClient // nil-safe: nil means "no search-indexer configured", always use the fallback
	fallback   *FallbackMySQL
	breaker    *gobreaker.CircuitBreaker
}

func NewService(grpcClient *GRPCClient, fallback *FallbackMySQL, breaker *gobreaker.CircuitBreaker) *Service {
	return &Service{grpcClient: grpcClient, fallback: fallback, breaker: breaker}
}

type SearchItem struct {
	ListingID        int64   `json:"listing_id"`
	Title            string  `json:"title"`
	PricePerDayCents int64   `json:"price_per_day_cents"`
	CategoryName     string  `json:"category_name"`
	AvgRating        float64 `json:"avg_rating"`
	// Source is surfaced so a degraded response is observable by the
	// caller, not a silently different result shape.
	Source string `json:"source"` // "elasticsearch" | "mysql_fallback"
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]SearchItem, error) {
	if s.grpcClient != nil {
		result, err := s.breaker.Execute(func() (interface{}, error) {
			return s.grpcClient.Search(ctx, query, limit)
		})
		if err == nil {
			resp := result.(*gearsharev1.SearchResponse)
			items := make([]SearchItem, 0, len(resp.GetResults()))
			for _, r := range resp.GetResults() {
				items = append(items, SearchItem{
					ListingID: r.GetListingId(), Title: r.GetTitle(),
					PricePerDayCents: r.GetPricePerDayCents(), CategoryName: r.GetCategoryName(),
					AvgRating: r.GetAvgRating(), Source: "elasticsearch",
				})
			}
			return items, nil
		}
		// Any error here — including gobreaker.ErrOpenState when the
		// breaker has tripped — falls through to MySQL below. This IS the
		// degradation path, not an error to propagate to the caller.
	}

	rows, err := s.fallback.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	items := make([]SearchItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, SearchItem{
			ListingID: r.ListingID, Title: r.Title, PricePerDayCents: r.PricePerDayCents,
			CategoryName: r.CategoryName, AvgRating: r.AvgRating, Source: "mysql_fallback",
		})
	}
	return items, nil
}

func (s *Service) Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	items, err := s.Search(r.Context(), query, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "search failed")
		return
	}
	httputil.JSON(w, http.StatusOK, items)
}
