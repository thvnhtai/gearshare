package booking

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// EventType names the booking.events Kafka topic's message types (see
// internal/eventbus/topics.go). search-indexer and the audit-log consumer
// both subscribe to this topic and switch on Type.
type EventType string

const (
	EventBookingCreated   EventType = "BookingCreated"
	EventBookingApproved  EventType = "BookingApproved"
	EventBookingRejected  EventType = "BookingRejected"
	EventBookingCancelled EventType = "BookingCancelled"
	EventBookingCompleted EventType = "BookingCompleted"
	EventBookingDisputed  EventType = "BookingDisputed"
)

type Event struct {
	Type       EventType `json:"type"`
	BookingID  int64     `json:"booking_id"`
	ListingID  int64     `json:"listing_id"`
	RenterID   int64     `json:"renter_id"`
	Status     Status    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventPublisher is deliberately a narrow interface owned by the consumer
// (this package), not the producer — internal/eventbus.KafkaProducer
// implements it without booking importing eventbus, avoiding a dependency
// cycle and keeping this package's tests free of any Kafka dependency
// (see service_test-style usage: a fake publisher in tests, the real one
// wired in cmd/api/main.go).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
}

const TopicBookingEvents = "booking.events"

func publishEvent(ctx context.Context, publisher EventPublisher, evt Event) error {
	if publisher == nil {
		return nil // events are best-effort in dev when no broker is configured
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	key := []byte(strconv.FormatInt(evt.BookingID, 10))
	return publisher.Publish(ctx, TopicBookingEvents, key, payload)
}
