// Package spec stores per-category gear "spec sheets" in MongoDB. A tent's
// meaningful attributes (capacity, season rating, packed weight) share
// nothing with a camera's (megapixels, lens mount, sensor size) — forcing
// this into MySQL would mean either a sparse wide table or an EAV
// (entity-attribute-value) schema, both of which break 3NF (see
// docs/erd/gearshare-erd.md). A document per listing, keyed by listing_id,
// is the natural fit.
package spec

import "time"

type Spec struct {
	ListingID  int64                  `bson:"listing_id" json:"listing_id"`
	Category   string                 `bson:"category" json:"category"`
	Attributes map[string]interface{} `bson:"attributes" json:"attributes"`
	UpdatedAt  time.Time              `bson:"updated_at" json:"updated_at"`
}
