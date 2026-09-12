package review

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/db"
)

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) Create(ctx context.Context, rv *Review) (int64, error) {
	const q = `INSERT INTO reviews (booking_id, reviewer_id, rating, comment) VALUES (?, ?, ?, ?)`
	res, err := r.db.Primary.ExecContext(ctx, q, rv.BookingID, rv.ReviewerID, rv.Rating, rv.Comment)
	if err != nil {
		return 0, fmt.Errorf("review: create: %w", err)
	}
	return res.LastInsertId()
}

// GetRatingSummary is the N+1 query issued once PER LISTING by the naive
// feed path (internal/listing/query_naive.go). Called in a loop, this is
// exactly the anti-pattern docs/performance/n-plus-one.md documents.
func (r *Repository) GetRatingSummary(ctx context.Context, listingID int64) (RatingSummary, error) {
	summary := RatingSummary{ListingID: listingID}
	const q = `SELECT COUNT(*) AS review_count, COALESCE(AVG(r.rating), 0) AS avg_rating
	           FROM reviews r JOIN bookings b ON b.id = r.booking_id
	           WHERE b.listing_id = ?`
	err := r.db.Reader().QueryRowxContext(ctx, q, listingID).Scan(&summary.ReviewCount, &summary.AvgRating)
	if err != nil {
		return summary, fmt.Errorf("review: rating summary: %w", err)
	}
	return summary, nil
}

// GetRatingSummaries is the batched equivalent used by the dataloader
// (internal/listing/loader.go): ONE query for every listing ID requested in
// a batch window, instead of one query per listing.
func (r *Repository) GetRatingSummaries(ctx context.Context, listingIDs []int64) (map[int64]RatingSummary, error) {
	result := make(map[int64]RatingSummary, len(listingIDs))
	if len(listingIDs) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(
		`SELECT b.listing_id AS listing_id, COUNT(*) AS review_count, AVG(r.rating) AS avg_rating
		 FROM reviews r JOIN bookings b ON b.id = r.booking_id
		 WHERE b.listing_id IN (?)
		 GROUP BY b.listing_id`, listingIDs)
	if err != nil {
		return nil, fmt.Errorf("review: build batched summary query: %w", err)
	}
	query = r.db.Reader().Rebind(query)

	var rows []RatingSummary
	if err := r.db.Reader().SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("review: batched rating summaries: %w", err)
	}
	for _, row := range rows {
		result[row.ListingID] = row
	}
	// Listings with zero reviews won't appear in the GROUP BY result —
	// callers should treat a missing key as ReviewCount 0 / AvgRating 0.
	return result, nil
}
