// Package eventbus wraps segmentio/kafka-go for the durable event-stream
// side of GearShare's messaging (see docs/adr/0002-cap-tradeoffs.md for why
// Kafka, not RabbitMQ, is used here — durability and multi-consumer fan-out
// over transactional task-queue semantics). internal/queue is the RabbitMQ
// counterpart for work-queue-shaped jobs (email, thumbnails).
package eventbus

// Topic names are the wire contract between producers (the monolith, via
// internal/booking/events.go) and consumers (search-indexer, an audit-log
// consumer) — kept here as the canonical source even though the producer
// side also defines its own copy (internal/booking.TopicBookingEvents),
// because in a real multi-repo/multi-language deployment the topic name,
// not a shared Go constant, is the actual contract.
const (
	TopicBookingEvents = "booking.events"
	TopicListingEvents = "listing.events"
)
