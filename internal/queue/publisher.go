package queue

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher is the "load shifting" resilience pattern in code: anything
// published here (email sends, thumbnail jobs) is deliberately deferred out
// of the request path that triggered it — see
// internal/booking/service.go's event publish for the synchronous audit
// trail (Kafka) versus this queue for the actual side-effectful work.
type Publisher struct {
	channel *amqp.Channel
}

func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("queue: open channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("queue: enable confirms: %w", err)
	}
	for _, t := range []Topology{NotificationsTopology, MediaTopology} {
		if err := Declare(ch, t); err != nil {
			return nil, err
		}
	}
	return &Publisher{channel: ch}, nil
}

func (p *Publisher) Publish(ctx context.Context, t Topology, body []byte, contentType string) error {
	return p.channel.PublishWithContext(ctx, t.Exchange, t.RoutingKey, false, false, amqp.Publishing{
		ContentType:  contentType,
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
}

func (p *Publisher) Close() error {
	return p.channel.Close()
}
