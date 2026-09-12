package availability

import "time"

type Reason string

const (
	ReasonBooked      Reason = "booked"
	ReasonMaintenance Reason = "maintenance"
	ReasonBlocked     Reason = "blocked"
)

type Block struct {
	ID         int64     `db:"id" json:"id"`
	ListingID  int64     `db:"listing_id" json:"listing_id"`
	BookingID  *int64    `db:"booking_id" json:"booking_id,omitempty"`
	StartDate  time.Time `db:"start_date" json:"start_date"`
	EndDate    time.Time `db:"end_date" json:"end_date"`
	Reason     Reason    `db:"reason" json:"reason"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}
