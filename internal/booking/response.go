package booking

import "time"

type Response struct {
	ID              int64     `json:"id"`
	ListingID       int64     `json:"listing_id"`
	RenterID        int64     `json:"renter_id"`
	StartDate       string    `json:"start_date"`
	EndDate         string    `json:"end_date"`
	Status          Status    `json:"status"`
	TotalPriceCents int64     `json:"total_price_cents"`
	CreatedAt       time.Time `json:"created_at"`
}

// ToResponse is the exported form of toResponse, reused by
// internal/realtime/polling.go so the long-poll response shape never drifts
// from the REST handler's shape.
func ToResponse(b *Booking) Response {
	return toResponse(b)
}

func toResponse(b *Booking) Response {
	const dateLayout = "2006-01-02"
	return Response{
		ID:              b.ID,
		ListingID:       b.ListingID,
		RenterID:        b.RenterID,
		StartDate:       b.StartDate.Format(dateLayout),
		EndDate:         b.EndDate.Format(dateLayout),
		Status:          b.Status,
		TotalPriceCents: b.TotalPriceCents,
		CreatedAt:       b.CreatedAt,
	}
}
