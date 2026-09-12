//go:build !lambda

// Default build target: `go run ./cmd/thumbnail-fn` runs this local
// RabbitMQ-consumer entrypoint instead of the Lambda one (main_lambda.go,
// built only with `-tags lambda`). Both call the exact same Handler
// (handler.go) — this file is just a different way of invoking the same
// function, which is the point: the function itself doesn't know or care
// whether it's running behind API Gateway/Lambda or a local queue.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/thvnhtai/gearshare/internal/queue"
)

func main() {
	rabbitURL := envOrDefault("RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/")

	conn, err := queue.Connect(rabbitURL)
	if err != nil {
		log.Fatalf("thumbnail-fn: connect rabbitmq: %v", err)
	}
	defer conn.Close()

	consumer, err := queue.NewConsumer(conn, queue.MediaTopology, 4, func(ctx context.Context, d amqp.Delivery) error {
		var req ThumbnailRequest
		if err := json.Unmarshal(d.Body, &req); err != nil {
			return err
		}
		resp, err := Handler(req)
		if err != nil {
			return err
		}
		// A real deployment would upload resp.ThumbnailBase64 to object
		// storage and update the listing's image URL; that integration is
		// out of scope here for the same reason SMTP is in
		// notification-service — it's an external account dependency, not
		// a new pattern this project needs to demonstrate.
		log.Printf("thumbnail-fn: generated %dx%d thumbnail for listing %d", resp.Width, resp.Height, resp.ListingID)
		return nil
	})
	if err != nil {
		log.Fatalf("thumbnail-fn: set up consumer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := consumer.Run(ctx); err != nil {
			log.Printf("thumbnail-fn: consumer stopped: %v", err)
		}
	}()

	log.Println("thumbnail-fn: worker running, consuming media.thumbnail")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("thumbnail-fn: shutting down")
	cancel()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
