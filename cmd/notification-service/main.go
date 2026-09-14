// Command notification-service is one of GearShare's two extracted
// microservices (see docs/adr/0003-service-boundaries.md). It owns zero
// MySQL access: contact details are resolved just-in-time via
// UserInternalService (gRPC, implemented by cmd/api), and work arrives
// either as a RabbitMQ notifications.email job (async, bulk path) or a
// direct NotificationService.SendTransactional gRPC call (sync, urgent
// path) from the monolith.
//
// Actual email delivery is simulated (logged), not wired to a real SMTP/SES
// provider — that integration is out of scope for this project and would
// only add an external account dependency without demonstrating a new
// pattern; the messaging/queueing/gRPC plumbing around it is what this
// service exists to show.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/observability"
	"github.com/thvnhtai/gearshare/internal/queue"
)

type emailJob struct {
	BookingID int64  `json:"booking_id"`
	RenterID  int64  `json:"renter_id"`
	Template  string `json:"template"`
}

func main() {
	rabbitURL := envOrDefault("RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/")
	grpcAddr := envOrDefault("GRPC_ADDR", ":9091")
	metricsAddr := envOrDefault("METRICS_ADDR", ":9101")
	userServiceAddr := envOrDefault("USER_SERVICE_ADDR", "127.0.0.1:9090")
	basicUser := envOrDefault("INTERNAL_BASIC_USER", "admin")
	basicPass := envOrDefault("INTERNAL_BASIC_PASS", "dev-only-change-me")

	userConn, err := grpc.NewClient(userServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.BasicAuthClientInterceptor(basicUser, basicPass)))
	if err != nil {
		log.Fatalf("notification-service: dial user service: %v", err)
	}
	defer func() { _ = userConn.Close() }()
	userClient := gearsharev1.NewUserInternalServiceClient(userConn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := queue.Connect(rabbitURL)
	if err != nil {
		log.Fatalf("notification-service: connect rabbitmq: %v", err)
	}
	defer func() { _ = conn.Close() }()

	consumer, err := queue.NewConsumer(conn, queue.NotificationsTopology, 10, func(ctx context.Context, d amqp.Delivery) error {
		var job emailJob
		if err := json.Unmarshal(d.Body, &job); err != nil {
			return err // malformed message -> DLQ, not silently dropped
		}
		return sendTransactional(ctx, userClient, job.RenterID, job.Template, nil)
	})
	if err != nil {
		log.Fatalf("notification-service: set up consumer: %v", err)
	}
	go func() {
		if err := consumer.Run(ctx); err != nil {
			log.Printf("notification-service: consumer stopped: %v", err)
		}
	}()

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", observability.Handler())
		// ReadHeaderTimeout guards against Slowloris-style connections that
		// send headers one byte at a time to exhaust server goroutines —
		// http.ListenAndServe's default has no such timeout.
		metricsServer := &http.Server{Addr: metricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		log.Printf("notification-service: metrics listening on %s", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil {
			log.Printf("notification-service: metrics server stopped: %v", err)
		}
	}()

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(auth.BasicAuthInterceptor(basicUser, basicPass)))
	gearsharev1.RegisterNotificationServiceServer(grpcServer, &notificationServer{userClient: userClient})

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("notification-service: listen %s: %v", grpcAddr, err)
	}
	go func() {
		log.Printf("notification-service: gRPC listening on %s", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("notification-service: grpc serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("notification-service: shutting down")
	cancel()
	grpcServer.GracefulStop()
}

type notificationServer struct {
	gearsharev1.UnimplementedNotificationServiceServer
	userClient gearsharev1.UserInternalServiceClient
}

func (s *notificationServer) SendTransactional(ctx context.Context, req *gearsharev1.SendTransactionalRequest) (*gearsharev1.Ack, error) {
	if err := sendTransactional(ctx, s.userClient, req.GetUserId(), req.GetTemplate(), req.GetData()); err != nil {
		return &gearsharev1.Ack{Ok: false}, err
	}
	return &gearsharev1.Ack{Ok: true}, nil
}

// sendTransactional resolves the recipient's contact details from the
// monolith (never carried in the queue payload itself — see the package
// doc) and "sends" the email. Swap this function's body for a real
// SMTP/SES/Postmark client to make delivery real; every caller above it
// (the RabbitMQ consumer, the gRPC handler) stays unchanged.
func sendTransactional(ctx context.Context, userClient gearsharev1.UserInternalServiceClient, userID int64, template string, data map[string]string) error {
	contact, err := userClient.GetUserContact(ctx, &gearsharev1.GetUserContactRequest{UserId: userID})
	if err != nil {
		return err
	}
	log.Printf("notification-service: [SIMULATED SEND] to=%s template=%s data=%v", contact.GetEmail(), template, data)
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
