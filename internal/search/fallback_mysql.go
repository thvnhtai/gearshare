package search

import (
	"context"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/db"
)

// FallbackMySQL is the "graceful degradation" path (requirement 19):
// exercised when the search-indexer gRPC call fails or the circuit breaker
// is open. A LIKE query is slower and cruder than Elasticsearch (no
// relevance ranking, no fuzzy matching), but it's better than a 5xx —
// GearShare would rather show imperfect search results than none.
type FallbackMySQL struct {
	db *db.DB
}

func NewFallbackMySQL(database *db.DB) *FallbackMySQL {
	return &FallbackMySQL{db: database}
}

type Result struct {
	ListingID        int64   `db:"id" json:"listing_id"`
	Title            string  `db:"title" json:"title"`
	PricePerDayCents int64   `db:"price_per_day_cents" json:"price_per_day_cents"`
	CategoryName     string  `db:"category_name" json:"category_name"`
	AvgRating        float64 `db:"avg_rating" json:"avg_rating"`
}

const fallbackQuery = `
SELECT gl.id, gl.title, gl.price_per_day_cents, c.name AS category_name,
       COALESCE(rs.avg_rating, 0) AS avg_rating
FROM gear_listings gl
JOIN categories c ON c.id = gl.category_id
LEFT JOIN (
    SELECT b.listing_id, AVG(r.rating) AS avg_rating
    FROM reviews r JOIN bookings b ON b.id = r.booking_id
    GROUP BY b.listing_id
) rs ON rs.listing_id = gl.id
WHERE gl.status = 'active' AND (gl.title LIKE ? OR gl.description LIKE ?)
ORDER BY gl.created_at DESC
LIMIT ?`

func (f *FallbackMySQL) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	like := "%" + query + "%"
	var results []Result
	if err := f.db.Reader().SelectContext(ctx, &results, fallbackQuery, like, like, limit); err != nil {
		return nil, fmt.Errorf("search: fallback query: %w", err)
	}
	return results, nil
}
