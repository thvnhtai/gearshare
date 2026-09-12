// Package queue wraps RabbitMQ (amqp091-go) for GearShare's transactional,
// task-queue-shaped work: sending confirmation emails and generating
// thumbnails. This is the deliberate counterpart to internal/eventbus's
// Kafka usage — see its package doc for the durable-event-stream side.
// RabbitMQ is chosen here specifically for its native dead-letter-exchange
// support, giving each queue a genuine retry-then-DLQ policy rather than
// hand-rolled retry bookkeeping.
package queue

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Connect(url string) (*amqp.Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("queue: connect: %w", err)
	}
	return conn, nil
}

// Topology names every exchange/queue/DLX pair GearShare declares.
// Declared once at startup by both the publisher and consumer sides (amqp
// declarations are idempotent), so either side booting first is safe.
type Topology struct {
	Exchange     string
	Queue        string
	RoutingKey   string
	DeadExchange string
	DeadQueue    string
}

var (
	NotificationsTopology = Topology{
		Exchange:     "notifications",
		Queue:        "notifications.email",
		RoutingKey:   "email",
		DeadExchange: "notifications.dlx",
		DeadQueue:    "notifications.email.dlq",
	}
	MediaTopology = Topology{
		Exchange:     "media",
		Queue:        "media.thumbnail",
		RoutingKey:   "thumbnail",
		DeadExchange: "media.dlx",
		DeadQueue:    "media.thumbnail.dlq",
	}
)

// Declare sets up the exchange, the dead-letter exchange/queue, and the
// main queue (with its `x-dead-letter-exchange` argument pointing at the
// DLX) — a message a handler Nacks without requeue lands in the DLQ
// automatically, no application-level retry-counter bookkeeping needed.
func Declare(ch *amqp.Channel, t Topology) error {
	if err := ch.ExchangeDeclare(t.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare exchange %s: %w", t.Exchange, err)
	}
	if err := ch.ExchangeDeclare(t.DeadExchange, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare dead exchange %s: %w", t.DeadExchange, err)
	}
	if _, err := ch.QueueDeclare(t.DeadQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare dead queue %s: %w", t.DeadQueue, err)
	}
	if err := ch.QueueBind(t.DeadQueue, "", t.DeadExchange, false, nil); err != nil {
		return fmt.Errorf("queue: bind dead queue %s: %w", t.DeadQueue, err)
	}

	_, err := ch.QueueDeclare(t.Queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": t.DeadExchange,
	})
	if err != nil {
		return fmt.Errorf("queue: declare queue %s: %w", t.Queue, err)
	}
	if err := ch.QueueBind(t.Queue, t.RoutingKey, t.Exchange, false, nil); err != nil {
		return fmt.Errorf("queue: bind queue %s: %w", t.Queue, err)
	}
	return nil
}
