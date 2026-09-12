package listing

import (
	"context"
	"encoding/json"
	"strconv"
)

// EventType names the listing.events Kafka topic's message types.
// search-indexer is the only consumer today. Unlike booking's events
// (internal/booking/events.go), these carry the FULL listing content, not
// just IDs — search-indexer never has a MySQL connection
// (docs/adr/0003-service-boundaries.md), so everything it needs to build a
// search document has to ride in the event itself.
type EventType string

const (
	EventListingCreated EventType = "ListingCreated"
	EventListingUpdated EventType = "ListingUpdated"
	EventListingDeleted EventType = "ListingDeleted"
)

type Event struct {
	Type             EventType `json:"type"`
	ListingID        int64     `json:"listing_id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	PricePerDayCents int64     `json:"price_per_day_cents"`
	CategoryName     string    `json:"category_name"`
	Status           Status    `json:"status"`
}

// EventPublisher mirrors booking.EventPublisher's shape (structural typing,
// same rationale: this package stays free of a direct eventbus dependency).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

const TopicListingEvents = "listing.events"

func publishEvent(ctx context.Context, publisher EventPublisher, evt Event) error {
	if publisher == nil {
		return nil
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return publisher.Publish(ctx, TopicListingEvents, []byte(strconv.FormatInt(evt.ListingID, 10)), payload)
}
