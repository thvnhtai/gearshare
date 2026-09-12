// Package damagereport stores booking-dispute incident write-ups in
// MongoDB: freeform description, a variable-length array of photo URLs,
// and whatever metadata the reporting party attaches. This is the same
// "naturally document-shaped, not relational" reasoning as internal/spec —
// see docs/architecture.md.
package damagereport

import "time"

type Report struct {
	ID          string    `bson:"_id,omitempty" json:"id,omitempty"`
	BookingID   int64     `bson:"booking_id" json:"booking_id"`
	ReportedBy  int64     `bson:"reported_by" json:"reported_by"`
	Description string    `bson:"description" json:"description"`
	PhotoURLs   []string  `bson:"photo_urls" json:"photo_urls"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
}
