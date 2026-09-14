package eventbus

import (
	"context"
	"log"

	kafka "github.com/segmentio/kafka-go"
)

// Handler processes one message. Returning an error does not stop the
// consumer loop (see Consumer.Run's doc comment) — GearShare's consumers
// use upsert/idempotent semantics precisely so an at-least-once redelivery
// after a transient handler error is safe (docs/adr/0002-cap-tradeoffs.md).
type Handler func(ctx context.Context, msg kafka.Message) error

// Consumer wraps kafka-go's reader with a bounded channel between the fetch
// loop and the worker pool — the "back pressure" resilience pattern: if
// handlers fall behind, the channel fills and Fetch blocks (backing off the
// consumer's own commit rate) rather than buffering unboundedly in memory.
type Consumer struct {
	reader      *kafka.Reader
	handler     Handler
	workerCount int
	queueDepth  int
}

func NewConsumer(brokers []string, topic, groupID string, handler Handler) *Consumer {
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
			// A small QueueCapacity + MinBytes=1 keeps this responsive for a
			// low-volume demo workload rather than optimized for throughput.
			QueueCapacity: 100,
		}),
		handler:     handler,
		workerCount: 4,
		queueDepth:  16,
	}
}

// Run fetches messages and dispatches them to a bounded worker pool until
// ctx is cancelled. A handler error is logged, not fatal — see Handler's
// doc comment on why at-least-once + idempotent consumers make that safe.
func (c *Consumer) Run(ctx context.Context) error {
	defer func() { _ = c.reader.Close() }()

	jobs := make(chan kafka.Message, c.queueDepth)
	done := make(chan struct{})

	for i := 0; i < c.workerCount; i++ {
		go func() {
			for msg := range jobs {
				if err := c.handler(ctx, msg); err != nil {
					log.Printf("eventbus: handler error on topic %s: %v", msg.Topic, err)
					continue
				}
				if err := c.reader.CommitMessages(ctx, msg); err != nil {
					log.Printf("eventbus: commit error on topic %s: %v", msg.Topic, err)
				}
			}
			done <- struct{}{}
		}()
	}

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			close(jobs)
			for i := 0; i < c.workerCount; i++ {
				<-done
			}
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return err
		}
		select {
		case jobs <- msg:
		case <-ctx.Done():
			close(jobs)
			for i := 0; i < c.workerCount; i++ {
				<-done
			}
			return nil
		}
	}
}
