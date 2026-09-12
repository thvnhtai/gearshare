// Package app is the composition root: it wires every internal/<capability>
// package's handler into one chi.Router (REST) and, in grpc_server.go, one
// gRPC server — the two API styles the requirement checklist asks for,
// served out of the same binary (cmd/api/main.go).
package app

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/category"
	"github.com/thvnhtai/gearshare/internal/health"
	"github.com/thvnhtai/gearshare/internal/listing"
	appmiddleware "github.com/thvnhtai/gearshare/internal/middleware"
	"github.com/thvnhtai/gearshare/internal/realtime"
	"github.com/thvnhtai/gearshare/internal/review"
	"github.com/thvnhtai/gearshare/internal/user"
)

// Handlers bundles every REST handler the router needs. Built in
// cmd/api/main.go's dependency-wiring block.
type Handlers struct {
	User     *user.Handler
	Category *category.Handler
	Listing  *listing.Handler
	Booking  *booking.Handler
	Review   *review.Handler
	SSE      *realtime.SSEHandler
	WS       *realtime.WebSocketHandler
	Polling  *realtime.PollingHandler
	Search   http.HandlerFunc
}

type RouterConfig struct {
	CORSOrigins    []string
	JWTIssuer      *auth.JWTIssuer
	InternalBasicUser string
	InternalBasicPass string
}

func NewRouter(h Handlers, cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(appmiddleware.CSP)
	r.Use(appmiddleware.CORS(cfg.CORSOrigins))

	r.Get("/healthz", health.Handler)

	// Auth-rate-limited: 20 attempts/minute/IP guards login/register against
	// brute force without punishing normal usage.
	r.Route("/api/v1/auth", func(authRoutes chi.Router) {
		authRoutes.Use(appmiddleware.RateLimit(20, time.Minute))
		authRoutes.Post("/register", h.User.Register)
		authRoutes.Post("/login", h.User.Login)
	})

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(appmiddleware.RateLimit(300, time.Minute))

		v1.Get("/categories", h.Category.List)
		v1.Get("/listings", h.Listing.Feed)
		v1.Get("/listings/{id}", h.Listing.GetDetail)
		v1.Get("/search", h.Search)

		// SSE: EventSource can't set an Authorization header, so this route
		// sits outside the JWT-middleware group — the handler itself
		// verifies the access token from the query string (web/js/sse.js).
		v1.Get("/bookings/events", h.SSE.BookingEvents)

		v1.Group(func(protected chi.Router) {
			protected.Use(auth.RequireJWT(cfg.JWTIssuer))

			protected.Post("/listings", h.Listing.Create)

			protected.Post("/bookings", h.Booking.Create)
			protected.Get("/bookings", h.Booking.ListMine)
			protected.Get("/bookings/{id}", h.Booking.Get)
			protected.Post("/bookings/{id}/approve", h.Booking.Approve)
			protected.Post("/bookings/{id}/reject", h.Booking.Reject)
			protected.Post("/bookings/{id}/cancel", h.Booking.Cancel)
			protected.Post("/bookings/{id}/complete", h.Booking.Complete)
			protected.Get("/bookings/{id}/poll", h.Polling.Poll)

			protected.Post("/reviews", h.Review.Create)
		})
	})

	// WebSocket for live listing availability — top-level (not under
	// /api/v1) to match nginx.conf's separate /ws/ proxy_pass block, which
	// needs the Upgrade/Connection headers that a plain HTTP proxy_pass
	// location doesn't set.
	r.Get("/ws/listings/{id}", h.WS.ListingAvailability)

	// Internal/operational endpoints use Basic Auth — a deliberately
	// different auth style from the public API (see internal/auth/middleware.go).
	r.Route("/internal", func(internal chi.Router) {
		internal.Use(auth.RequireBasicAuth(cfg.InternalBasicUser, cfg.InternalBasicPass))
		internal.Get("/admin/health-detailed", func(w http.ResponseWriter, r *http.Request) {
			health.Handler(w, r)
		})
	})

	return r
}
