package listing

import (
	"context"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/db"
)

// FeedServiceOptimized is the fix wired into the real GET /api/v1/listings
// handler (internal/listing/handler.go). One query, period: owner and
// category come in via JOIN, the rating aggregate via a subquery MySQL
// executes once as part of the same query plan — not once per row from the
// application. See internal/db/queries/listing.sql's
// ListActiveListingsFeedOptimized for the canonical SQL and
// docs/performance/n-plus-one.md for the EXPLAIN comparison against the
// naive path.
type FeedServiceOptimized struct {
	db *db.DB
}

func NewFeedServiceOptimized(database *db.DB) *FeedServiceOptimized {
	return &FeedServiceOptimized{db: database}
}

type feedRow struct {
	ListingID        int64   `db:"id"`
	Title            string  `db:"title"`
	PricePerDayCents int64   `db:"price_per_day_cents"`
	DepositCents     int64   `db:"deposit_cents"`
	OwnerID          int64   `db:"owner_id"`
	OwnerName        string  `db:"owner_name"`
	CategoryID       int64   `db:"category_id"`
	CategoryName     string  `db:"category_name"`
	ReviewCount      int64   `db:"review_count"`
	AvgRating        float64 `db:"avg_rating"`
}

const optimizedFeedQuery = `
SELECT
    gl.id, gl.title, gl.price_per_day_cents, gl.deposit_cents,
    u.id AS owner_id, u.display_name AS owner_name,
    c.id AS category_id, c.name AS category_name,
    COALESCE(rs.review_count, 0) AS review_count,
    COALESCE(rs.avg_rating, 0) AS avg_rating
FROM gear_listings gl
JOIN users u ON u.id = gl.owner_id
JOIN categories c ON c.id = gl.category_id
LEFT JOIN (
    SELECT b.listing_id, COUNT(*) AS review_count, AVG(r.rating) AS avg_rating
    FROM reviews r
    JOIN bookings b ON b.id = r.booking_id
    GROUP BY b.listing_id
) rs ON rs.listing_id = gl.id
WHERE gl.status = 'active'
ORDER BY gl.created_at DESC
LIMIT ? OFFSET ?`

const optimizedDetailQuery = `
SELECT
    gl.id, gl.title, gl.price_per_day_cents, gl.deposit_cents,
    u.id AS owner_id, u.display_name AS owner_name,
    c.id AS category_id, c.name AS category_name,
    COALESCE(rs.review_count, 0) AS review_count,
    COALESCE(rs.avg_rating, 0) AS avg_rating
FROM gear_listings gl
JOIN users u ON u.id = gl.owner_id
JOIN categories c ON c.id = gl.category_id
LEFT JOIN (
    SELECT b.listing_id, COUNT(*) AS review_count, AVG(r.rating) AS avg_rating
    FROM reviews r
    JOIN bookings b ON b.id = r.booking_id
    GROUP BY b.listing_id
) rs ON rs.listing_id = gl.id
WHERE gl.id = ?`

func (s *FeedServiceOptimized) GetByID(ctx context.Context, id int64) (*FeedItem, error) {
	var row feedRow
	if err := s.db.Reader().GetContext(ctx, &row, optimizedDetailQuery, id); err != nil {
		return nil, fmt.Errorf("listing: optimized detail: %w", err)
	}
	return &FeedItem{
		ListingID:        row.ListingID,
		Title:            row.Title,
		PricePerDayCents: row.PricePerDayCents,
		DepositCents:     row.DepositCents,
		OwnerID:          row.OwnerID,
		OwnerName:        row.OwnerName,
		CategoryID:       row.CategoryID,
		CategoryName:     row.CategoryName,
		ReviewCount:      row.ReviewCount,
		AvgRating:        row.AvgRating,
	}, nil
}

func (s *FeedServiceOptimized) GetFeed(ctx context.Context, limit, offset int) ([]FeedItem, error) {
	var rows []feedRow
	if err := s.db.Reader().SelectContext(ctx, &rows, optimizedFeedQuery, limit, offset); err != nil {
		return nil, fmt.Errorf("listing: optimized feed: %w", err)
	}

	items := make([]FeedItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, FeedItem{
			ListingID:        r.ListingID,
			Title:            r.Title,
			PricePerDayCents: r.PricePerDayCents,
			DepositCents:     r.DepositCents,
			OwnerID:          r.OwnerID,
			OwnerName:        r.OwnerName,
			CategoryID:       r.CategoryID,
			CategoryName:     r.CategoryName,
			ReviewCount:      r.ReviewCount,
			AvgRating:        r.AvgRating,
		})
	}
	return items, nil
}
