package booking

import "time"

type Status string

const (
	StatusRequested Status = "requested"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
	StatusDisputed  Status = "disputed"
)

type Booking struct {
	ID               int64     `db:"id" json:"id"`
	ListingID        int64     `db:"listing_id" json:"listing_id"`
	RenterID         int64     `db:"renter_id" json:"renter_id"`
	StartDate        time.Time `db:"start_date" json:"start_date"`
	EndDate          time.Time `db:"end_date" json:"end_date"`
	Status           Status    `db:"status" json:"status"`
	TotalPriceCents  int64     `db:"total_price_cents" json:"total_price_cents"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

// validTransitions encodes the booking lifecycle state machine referenced
// throughout the requirement checklist (requested -> approved/rejected ->
// active -> completed/cancelled/disputed).
var validTransitions = map[Status][]Status{
	StatusRequested: {StatusApproved, StatusRejected, StatusCancelled},
	StatusApproved:  {StatusActive, StatusCancelled},
	StatusActive:    {StatusCompleted, StatusDisputed},
}

func CanTransition(from, to Status) bool {
	for _, allowed := range validTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
