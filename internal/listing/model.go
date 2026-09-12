package listing

import "time"

type Status string

const (
	StatusDraft    Status = "draft"
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Listing is the raw gear_listings row.
type Listing struct {
	ID               int64     `db:"id" json:"id"`
	OwnerID          int64     `db:"owner_id" json:"owner_id"`
	CategoryID       int64     `db:"category_id" json:"category_id"`
	Title            string    `db:"title" json:"title"`
	Description      string    `db:"description" json:"description"`
	PricePerDayCents int64     `db:"price_per_day_cents" json:"price_per_day_cents"`
	DepositCents     int64     `db:"deposit_cents" json:"deposit_cents"`
	Status           Status    `db:"status" json:"status"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

// FeedItem is the enriched shape the browse/search page actually renders:
// listing + owner + category + rating summary, however it was assembled
// (naively or optimized — see query_naive.go / query_optimized.go).
type FeedItem struct {
	ListingID        int64   `json:"listing_id"`
	Title            string  `json:"title"`
	PricePerDayCents int64   `json:"price_per_day_cents"`
	DepositCents     int64   `json:"deposit_cents"`
	OwnerID          int64   `json:"owner_id"`
	OwnerName        string  `json:"owner_name"`
	CategoryID       int64   `json:"category_id"`
	CategoryName     string  `json:"category_name"`
	ReviewCount      int64   `json:"review_count"`
	AvgRating        float64 `json:"avg_rating"`
}
