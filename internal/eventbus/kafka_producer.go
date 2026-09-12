package eventbus

import (
	"context"
	"fmt"

	kafka "github.com/segmentio/kafka-go"
)

// KafkaProducer implements booking.EventPublisher (structural typing —
// internal/booking never imports this package) and listing's equivalent
// event-publish path. One kafka.Writer per process, topic selected per-call
// so both booking.events and listing.events share a connection pool.
type KafkaProducer struct {
	writer *kafka.Writer
}

func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Balancer:               &kafka.Hash{}, // partition by key (booking_id/listing_id) for per-entity ordering
			AllowAutoTopicCreation: true,           // fine for local/dev; a real deployment would pre-provision topics
		},
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, topic string, key, value []byte) error {
	err := p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   key,
		Value: value,
	})
	if err != nil {
		return fmt.Errorf("eventbus: publish to %s: %w", topic, err)
	}
	return nil
}

func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}
