// Command search-indexer is GearShare's other extracted microservice (see
// docs/adr/0003-service-boundaries.md). It owns the Elasticsearch index and
// has zero MySQL access: everything it knows about a listing comes from the
// listing.events Kafka topic, published with full content precisely because
// this service can't go look the rest up itself (internal/listing/events.go).
// booking.events is also consumed here as the audit-log-style side of
// Kafka's multi-consumer fan-out (see docs/architecture.md) — logged, not
// indexed, since a booking event carries no listing content to upsert.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/eventbus"
	appmiddleware "github.com/thvnhtai/gearshare/internal/middleware"
	"github.com/thvnhtai/gearshare/internal/observability"
)

type listingEvent struct {
	Type             string `json:"type"`
	ListingID        int64  `json:"listing_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	PricePerDayCents int64  `json:"price_per_day_cents"`
	CategoryName     string `json:"category_name"`
	Status           string `json:"status"`
}

type bookingEvent struct {
	Type      string `json:"type"`
	BookingID int64  `json:"booking_id"`
	ListingID int64  `json:"listing_id"`
	Status    string `json:"status"`
}

func main() {
	kafkaBrokers := strings.Split(envOrDefault("KAFKA_BROKERS", "127.0.0.1:9092"), ",")
	esAddr := envOrDefault("ELASTICSEARCH_ADDR", "http://127.0.0.1:9200")
	grpcAddr := envOrDefault("GRPC_ADDR", ":9092")
	metricsAddr := envOrDefault("METRICS_ADDR", ":9102")
	basicUser := envOrDefault("INTERNAL_BASIC_USER", "admin")
	basicPass := envOrDefault("INTERNAL_BASIC_PASS", "dev-only-change-me")

	index, err := NewIndex([]string{esAddr}, appmiddleware.NewBreaker("elasticsearch"))
	if err != nil {
		log.Fatalf("search-indexer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := index.EnsureIndex(ctx); err != nil {
		log.Printf("search-indexer: ensure index (will retry lazily on first write): %v", err)
	}

	listingConsumer := eventbus.NewConsumer(kafkaBrokers, eventbus.TopicListingEvents, "search-indexer", func(ctx context.Context, msg kafka.Message) error {
		var evt listingEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return err
		}
		switch evt.Type {
		case "ListingDeleted":
			return index.Delete(ctx, evt.ListingID)
		default: // Created or Updated: upsert is idempotent for both
			return index.Upsert(ctx, esDocument{
				ListingID: evt.ListingID, Title: evt.Title, Description: evt.Description,
				PricePerDayCents: evt.PricePerDayCents, CategoryName: evt.CategoryName, Status: evt.Status,
			})
		}
	})

	bookingConsumer := eventbus.NewConsumer(kafkaBrokers, eventbus.TopicBookingEvents, "search-indexer-audit", func(ctx context.Context, msg kafka.Message) error {
		var evt bookingEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return err
		}
		log.Printf("search-indexer: [audit] booking %d on listing %d -> %s", evt.BookingID, evt.ListingID, evt.Status)
		return nil
	})

	go func() {
		if err := listingConsumer.Run(ctx); err != nil {
			log.Printf("search-indexer: listing consumer stopped: %v", err)
		}
	}()
	go func() {
		if err := bookingConsumer.Run(ctx); err != nil {
			log.Printf("search-indexer: booking audit consumer stopped: %v", err)
		}
	}()

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", observability.Handler())
		metricsServer := &http.Server{Addr: metricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		log.Printf("search-indexer: metrics listening on %s", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil {
			log.Printf("search-indexer: metrics server stopped: %v", err)
		}
	}()

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(auth.BasicAuthInterceptor(basicUser, basicPass)))
	gearsharev1.RegisterSearchInternalServiceServer(grpcServer, &searchServer{index: index})

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("search-indexer: listen %s: %v", grpcAddr, err)
	}
	go func() {
		log.Printf("search-indexer: gRPC listening on %s", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("search-indexer: grpc serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("search-indexer: shutting down")
	cancel()
	grpcServer.GracefulStop()
}

type searchServer struct {
	gearsharev1.UnimplementedSearchInternalServiceServer
	index *Index
}

func (s *searchServer) Search(ctx context.Context, req *gearsharev1.SearchRequest) (*gearsharev1.SearchResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	results, err := s.index.Search(ctx, req.GetQuery(), limit)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "search backend unavailable")
	}
	return &gearsharev1.SearchResponse{Results: results}, nil
}

func (s *searchServer) IndexListing(ctx context.Context, req *gearsharev1.IndexListingRequest) (*gearsharev1.Ack, error) {
	err := s.index.Upsert(ctx, esDocument{
		ListingID: req.GetListingId(), Title: req.GetTitle(), Description: req.GetDescription(),
		PricePerDayCents: req.GetPricePerDayCents(), CategoryName: req.GetCategoryName(),
		OwnerName: req.GetOwnerName(), Status: req.GetStatus(),
	})
	if err != nil {
		return &gearsharev1.Ack{Ok: false}, status.Error(codes.Unavailable, "search backend unavailable")
	}
	return &gearsharev1.Ack{Ok: true}, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
