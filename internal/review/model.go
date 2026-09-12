package review

import "time"

type Review struct {
	ID         int64     `db:"id" json:"id"`
	BookingID  int64     `db:"booking_id" json:"booking_id"`
	ReviewerID int64     `db:"reviewer_id" json:"reviewer_id"`
	Rating     int       `db:"rating" json:"rating"`
	Comment    string    `db:"comment" json:"comment"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

// RatingSummary is what the naive and optimized listing-feed paths both need
// to produce per listing — the shape differs only in how many round trips it
// costs to get there (see internal/listing/query_naive.go vs query_optimized.go).
type RatingSummary struct {
	ListingID   int64   `db:"listing_id"`
	ReviewCount int64   `db:"review_count"`
	AvgRating   float64 `db:"avg_rating"`
}
