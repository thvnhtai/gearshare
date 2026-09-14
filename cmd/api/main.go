// Command api is the GearShare modular monolith: the primary REST API
// (chi, :8080) for the frontend, plus an internal gRPC server (:9090) for
// service-to-service calls from notification-service and search-indexer.
// This is the "monolith" side of the architectural-patterns requirement —
// see docs/architecture.md.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thvnhtai/gearshare/internal/app"
	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/availability"
	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/cache"
	"github.com/thvnhtai/gearshare/internal/category"
	"github.com/thvnhtai/gearshare/internal/config"
	"github.com/thvnhtai/gearshare/internal/damagereport"
	"github.com/thvnhtai/gearshare/internal/db"
	"github.com/thvnhtai/gearshare/internal/eventbus"
	"github.com/thvnhtai/gearshare/internal/listing"
	appmiddleware "github.com/thvnhtai/gearshare/internal/middleware"
	gsmongo "github.com/thvnhtai/gearshare/internal/mongo"
	"github.com/thvnhtai/gearshare/internal/notification"
	"github.com/thvnhtai/gearshare/internal/observability"
	"github.com/thvnhtai/gearshare/internal/partner"
	"github.com/thvnhtai/gearshare/internal/queue"
	"github.com/thvnhtai/gearshare/internal/realtime"
	"github.com/thvnhtai/gearshare/internal/review"
	"github.com/thvnhtai/gearshare/internal/search"
	"github.com/thvnhtai/gearshare/internal/user"
)

// emailQueueAdapter satisfies booking.NotificationQueuer over a real
// internal/queue.Publisher bound to the notifications exchange.
type emailQueueAdapter struct{ pub *queue.Publisher }

func (a *emailQueueAdapter) QueueEmail(ctx context.Context, payload []byte) error {
	return a.pub.Publish(ctx, queue.NotificationsTopology, payload, "application/json")
}

// syncNotifierAdapter satisfies booking.SyncNotifier over a real
// notification.GRPCClient, for the one urgent/synchronous path (disputes).
type syncNotifierAdapter struct{ client *notification.GRPCClient }

func (a *syncNotifierAdapter) SendTransactional(ctx context.Context, userID int64, template string) error {
	return a.client.SendTransactional(ctx, userID, template, nil)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Shared by this process's own gRPC server (app.NewGRPCServer) AND
	// every outbound internal gRPC client below (search, notification) —
	// they must agree, or every internal call gets rejected as
	// Unauthenticated regardless of whether the "password" half is right.
	internalBasicUser := envOrDefault("INTERNAL_BASIC_USER", "admin")
	internalBasicPass := envOrDefault("INTERNAL_BASIC_PASS", "dev-only-change-me")

	shutdownTracing, err := observability.SetupTracing(context.Background(), cfg.OTel)
	if err != nil {
		log.Printf("observability: tracing setup failed, continuing without it: %v", err)
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := db.Connect(ctx, cfg.MySQL)
	cancel()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Redis is treated as an optional dependency for local dev ergonomics:
	// if it's unreachable, listingCache stays nil and every cache lookup in
	// internal/listing/handler.go becomes an explicit miss (graceful
	// degradation straight through to MySQL) rather than a boot-time crash.
	var listingCache *cache.ListingCache
	redisCtx, redisCancel := context.WithTimeout(context.Background(), 5*time.Second)
	redisClient, err := cache.Connect(redisCtx, cfg.Redis)
	redisCancel()
	if err != nil {
		log.Printf("cache: redis unavailable, running without cache-aside: %v", err)
	} else {
		listingCache = cache.NewListingCache(redisClient, 30*time.Second)
	}

	// Rate limiting: Redis-backed (correct across the multiple replicas
	// deployments/k8s/base/api.yaml runs) when Redis is reachable, the
	// in-process limiter otherwise — same graceful-degradation shape as
	// everything else keyed off listingCache's success above.
	var authRateLimit, apiRateLimit func(http.Handler) http.Handler
	if redisClient != nil {
		limiter := cache.NewRateLimiter(redisClient)
		authRateLimit = limiter.Middleware(20, time.Minute, appmiddleware.ResolveClientIP)
		apiRateLimit = limiter.Middleware(300, time.Minute, appmiddleware.ResolveClientIP)
	} else {
		authRateLimit = appmiddleware.RateLimit(20, time.Minute)
		apiRateLimit = appmiddleware.RateLimit(300, time.Minute)
	}

	// MongoDB: same optional-dependency pattern as Redis/Kafka/RabbitMQ —
	// nil damageReportRepo makes the dispute endpoint still mark a booking
	// disputed in MySQL, just without persisting the incident write-up
	// (internal/app/dispute.go's documented degradation path).
	var damageReportRepo *damagereport.Repository
	mongoCtx, mongoCancel := context.WithTimeout(context.Background(), 5*time.Second)
	mongoDB, err := gsmongo.Connect(mongoCtx, cfg.Mongo)
	mongoCancel()
	if err != nil {
		log.Printf("mongo: unavailable, disputes won't persist damage reports: %v", err)
	} else {
		damageReportRepo = damagereport.NewRepository(mongoDB)
	}

	// --- Repositories ---
	userRepo := user.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	listingRepo := listing.NewRepository(database)
	availabilityRepo := availability.NewRepository(database)
	bookingRepo := booking.NewRepository(database)
	reviewRepo := review.NewRepository(database)

	// --- Auth primitives ---
	bcryptHasher := auth.NewBcryptHasher(cfg.Auth.BcryptCost)
	jwtIssuer := auth.NewJWTIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTAccessTTL)

	// Kafka is treated the same way as Redis above: optional at boot, with
	// a documented no-op fallback (internal/booking/events.go's
	// publishEvent) rather than a crash, so `go run ./cmd/api` still works
	// against a bare MySQL+Redis dev setup with no broker running.
	kafkaProducer := eventbus.NewKafkaProducer(cfg.Kafka.Brokers)
	defer func() { _ = kafkaProducer.Close() }()

	// --- Services ---
	userService := user.NewService(userRepo, bcryptHasher, jwtIssuer)
	feedOptimized := listing.NewFeedServiceOptimized(database)
	listingService := listing.NewService(listingRepo, feedOptimized, categoryRepo, kafkaProducer)
	bookingTxRunner := booking.NewTxRunner(database, bookingRepo, availabilityRepo, listingRepo)

	// RabbitMQ: same optional-dependency treatment. A nil emailQueuer means
	// booking.Service.queueEmail is a documented no-op (see its own guard).
	var emailQueuer booking.NotificationQueuer
	rabbitConn, err := queue.Connect(cfg.RabbitMQ.URL)
	if err != nil {
		log.Printf("queue: rabbitmq unavailable, running without email notifications: %v", err)
	} else {
		defer func() { _ = rabbitConn.Close() }()
		publisher, err := queue.NewPublisher(rabbitConn)
		if err != nil {
			log.Printf("queue: could not set up publisher: %v", err)
		} else {
			defer func() { _ = publisher.Close() }()
			emailQueuer = &emailQueueAdapter{pub: publisher}
		}
	}

	// A nil *cache.ListingCache boxed directly into the
	// booking.ListingCacheInvalidator interface would be a non-nil interface
	// wrapping a nil pointer — the classic Go typed-nil trap, which would
	// make Service's `s.cache != nil` check pass and then panic on first
	// use. Keep the interface value itself nil when Redis wasn't reachable.
	var bookingCache booking.ListingCacheInvalidator
	if listingCache != nil {
		bookingCache = listingCache
	}
	// notification-service's gRPC client, for the synchronous dispute path
	// (booking.SyncNotifier) — optional, same nil-safe pattern as everything
	// else above.
	var syncNotifier booking.SyncNotifier
	if cfg.GRPC.NotificationAddr != "" {
		notificationClient, err := notification.NewGRPCClient(cfg.GRPC.NotificationAddr, internalBasicUser, internalBasicPass)
		if err != nil {
			log.Printf("notification: could not set up sync client: %v", err)
		} else {
			defer func() { _ = notificationClient.Close() }()
			syncNotifier = &syncNotifierAdapter{client: notificationClient}
		}
	}

	bookingService := booking.NewService(bookingRepo, bookingTxRunner, kafkaProducer, bookingCache, emailQueuer, syncNotifier)
	reviewService := review.NewService(reviewRepo)

	// --- Search: gRPC to search-indexer behind a circuit breaker, MySQL
	// LIKE-query fallback on failure/open-breaker (graceful degradation) ---
	var searchGRPCClient *search.GRPCClient
	if cfg.GRPC.SearchIndexerAddr != "" {
		searchGRPCClient, err = search.NewGRPCClient(cfg.GRPC.SearchIndexerAddr, internalBasicUser, internalBasicPass)
		if err != nil {
			log.Printf("search: could not set up search-indexer client, falling back to MySQL only: %v", err)
		}
	}
	searchService := search.NewService(searchGRPCClient, search.NewFallbackMySQL(database), appmiddleware.NewBreaker("search-indexer"))

	// --- Cookie-session admin dashboard + API-key partner auth (the
	// remaining two of the seven auth styles wired into the monolith) ---
	secureCookies := cfg.Env == "production"
	sessionManager := auth.NewSessionManager(cfg.Auth.SessionSecret, 12*time.Hour)
	apiKeyRepo := auth.NewAPIKeyRepository(database)
	apiKeyManager := auth.NewAPIKeyManager(cfg.Auth.APIKeyPepper)
	adminHandler := app.NewAdminHandler(sessionManager, userRepo, bcryptHasher, bookingRepo, secureCookies)
	partnerHandler := partner.NewHandler()
	disputeHandler := app.NewDisputeHandler(bookingService, damageReportRepo)

	// OAuth2 + OIDC "Log in with Google" — optional at boot like Redis/Kafka/
	// RabbitMQ above. NewOIDCVerifier does an OIDC discovery HTTP call, so
	// it's skipped entirely (not just given a short timeout) when no client
	// ID is configured, rather than slowing every boot down for a feature
	// most local dev runs won't exercise.
	var oauthHandler *user.OAuthHandler
	if cfg.Auth.OAuthGoogleClientID != "" {
		oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 5*time.Second)
		oidcVerifier, err := auth.NewOIDCVerifier(oidcCtx, cfg.Auth.OAuthGoogleClientID)
		oidcCancel()
		if err != nil {
			log.Printf("oauth: OIDC discovery failed, Google login disabled: %v", err)
		} else {
			googleOAuth := auth.NewGoogleOAuth(cfg.Auth.OAuthGoogleClientID, cfg.Auth.OAuthGoogleClientSecret, cfg.Auth.OAuthGoogleRedirectURL)
			oauthHandler = user.NewOAuthHandler(googleOAuth, oidcVerifier, database, userRepo, jwtIssuer, secureCookies, cfg.HTTP.CORSOrigins[0])
		}
	}

	// SAML SSO — optional, same pattern. Skipped entirely unless a real
	// IdP metadata URL is configured (see internal/auth/saml.go).
	var samlSP *auth.SAMLServiceProvider
	if cfg.Auth.SAMLIDPMetadataURL != "" {
		key, cert, err := auth.GenerateSelfSignedCert()
		if err != nil {
			log.Printf("saml: could not generate SP certificate, SAML disabled: %v", err)
		} else {
			samlCtx, samlCancel := context.WithTimeout(context.Background(), 10*time.Second)
			samlSP, err = auth.NewSAMLServiceProvider(samlCtx, "https://localhost", cfg.Auth.SAMLIDPMetadataURL, cert, key)
			samlCancel()
			if err != nil {
				log.Printf("saml: could not set up service provider, SAML disabled: %v", err)
				samlSP = nil
			}
		}
	}

	// --- Real-time hub (SSE + WebSocket + long-poll; see internal/realtime) ---
	hub := realtime.NewHub()
	sseHandler := realtime.NewSSEHandler(hub, jwtIssuer)
	wsHandler := realtime.NewWebSocketHandler(hub, availabilityRepo, cfg.HTTP.CORSOrigins)
	pollingHandler := realtime.NewPollingHandler(bookingRepo)

	// --- Handlers ---
	handlers := app.Handlers{
		User:     user.NewHandler(userService),
		Category: category.NewHandler(categoryRepo),
		Listing:  listing.NewHandler(listingService, listingCache),
		Booking:  booking.NewHandler(bookingService, listingRepo, hub),
		Review:   review.NewHandler(reviewService),
		SSE:      sseHandler,
		WS:       wsHandler,
		Polling:  pollingHandler,
		Search:   searchService.Handler,
		Admin:    adminHandler,
		Partner:  partnerHandler,
		Dispute:  disputeHandler,
		OAuth:    oauthHandler,
		SAML:     samlSP,
	}

	router := app.NewRouter(handlers, app.RouterConfig{
		CORSOrigins:       cfg.HTTP.CORSOrigins,
		JWTIssuer:         jwtIssuer,
		InternalBasicUser: internalBasicUser,
		InternalBasicPass: internalBasicPass,
		Sessions:          sessionManager,
		APIKeyRepo:        apiKeyRepo,
		APIKeyManager:     apiKeyManager,
		AuthRateLimit:     authRateLimit,
		APIRateLimit:      apiRateLimit,
	})

	server := app.NewServer(router, cfg.HTTP)

	grpcServer := app.NewGRPCServer(userRepo, internalBasicUser, internalBasicPass)

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()
	go func() {
		if err := app.ServeGRPC(grpcServer, cfg.GRPC.Addr); err != nil {
			log.Fatalf("grpc server: %v", err)
		}
	}()
	log.Printf("gearshare-api: gRPC listening on %s", cfg.GRPC.Addr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("gearshare-api: shutting down")
	grpcServer.GracefulStop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("gearshare-api: shutdown error: %v", err)
	}
	if searchGRPCClient != nil {
		_ = searchGRPCClient.Close()
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
