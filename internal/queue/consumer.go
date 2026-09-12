package queue

import (
	"context"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DeliveryHandler processes one message. Returning an error Nacks the
// delivery without requeue, which — combined with the queue's
// x-dead-letter-exchange argument (see rabbitmq.go's Declare) — routes it
// straight to the DLQ. There is no in-process retry loop here: retrying
// belongs to whatever re-drives the DLQ (an operator, or a scheduled
// redrive job), which is the standard RabbitMQ DLQ pattern rather than a
// hand-rolled backoff counter.
type DeliveryHandler func(ctx context.Context, d amqp.Delivery) error

// Consumer applies RabbitMQ QoS `prefetch` as GearShare's back-pressure
// mechanism (requirement 19): a consumer only holds `prefetch` unacked
// messages at a time, so a slow handler naturally throttles how fast
// RabbitMQ hands it more work, instead of the broker (or an unbounded
// in-process buffer) absorbing unlimited backlog.
type Consumer struct {
	channel  *amqp.Channel
	topology Topology
	handler  DeliveryHandler
}

func NewConsumer(conn *amqp.Connection, t Topology, prefetch int, handler DeliveryHandler) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := Declare(ch, t); err != nil {
		return nil, err
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return nil, err
	}
	return &Consumer{channel: ch, topology: t, handler: handler}, nil
}

func (c *Consumer) Run(ctx context.Context) error {
	deliveries, err := c.channel.Consume(c.topology.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return c.channel.Close()
		case d, ok := <-deliveries:
			if !ok {
				return nil
			}
			if err := c.handler(ctx, d); err != nil {
				log.Printf("queue: handler error on %s, routing to DLQ: %v", c.topology.Queue, err)
				_ = d.Nack(false, false) // no requeue -> dead-letter-exchange -> DLQ
				continue
			}
			_ = d.Ack(false)
		}
	}
}
