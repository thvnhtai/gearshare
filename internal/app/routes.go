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
	"github.com/thvnhtai/gearshare/internal/httputil"
	"github.com/thvnhtai/gearshare/internal/observability"
	"github.com/thvnhtai/gearshare/internal/listing"
	appmiddleware "github.com/thvnhtai/gearshare/internal/middleware"
	"github.com/thvnhtai/gearshare/internal/partner"
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
	Admin    *AdminHandler
	Partner  *partner.Handler
	Dispute  *DisputeHandler
	OAuth    *user.OAuthHandler // nil when OAuth2/OIDC isn't configured (no client ID/secret)
	SAML     *auth.SAMLServiceProvider // nil when no IdP metadata URL is configured
}

type RouterConfig struct {
	CORSOrigins       []string
	JWTIssuer         *auth.JWTIssuer
	InternalBasicUser string
	InternalBasicPass string
	Sessions          *auth.SessionManager
	APIKeyRepo        *auth.APIKeyRepository
	APIKeyManager     *auth.APIKeyManager
}

func NewRouter(h Handlers, cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(appmiddleware.CSP)
	r.Use(appmiddleware.CORS(cfg.CORSOrigins))
	r.Use(observability.HTTPMiddleware)

	r.Get("/healthz", health.Handler)
	r.Handle("/metrics", observability.Handler())

	// Auth-rate-limited: 20 attempts/minute/IP guards login/register against
	// brute force without punishing normal usage.
	r.Route("/api/v1/auth", func(authRoutes chi.Router) {
		authRoutes.Use(appmiddleware.RateLimit(20, time.Minute))
		authRoutes.Post("/register", h.User.Register)
		authRoutes.Post("/login", h.User.Login)
		if h.OAuth != nil {
			authRoutes.Get("/oauth/google", h.OAuth.Start)
			authRoutes.Get("/oauth/google/callback", h.OAuth.Callback)
		}
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
			protected.Post("/bookings/{id}/activate", h.Booking.Activate)
			protected.Post("/bookings/{id}/complete", h.Booking.Complete)
			protected.Post("/bookings/{id}/dispute", h.Dispute.Dispute)
			protected.Get("/bookings/{id}/poll", h.Polling.Poll)

			protected.Post("/reviews", h.Review.Create)
		})
	})

	// WebSocket for live listing availability — top-level (not under
	// /api/v1) to match nginx.conf's separate /ws/ proxy_pass block, which
	// needs the Upgrade/Connection headers that a plain HTTP proxy_pass
	// location doesn't set.
	r.Get("/ws/listings/{id}", h.WS.ListingAvailability)

	// Partner/integration API — Token/API-Key auth, a distinct style from
	// both JWT (frontend) and cookie sessions (admin dashboard): no login
	// flow, no session, just a long-lived revocable key.
	r.Route("/partner/v1", func(p chi.Router) {
		p.Use(auth.RequireAPIKey(cfg.APIKeyRepo, cfg.APIKeyManager))
		p.Get("/listings", h.Listing.Feed)
		p.Post("/webhooks/insurance", h.Partner.InsuranceWebhook)
	})

	// Cookie session + CSRF admin dashboard — a classic server-rendered
	// surface, the auth style that actually needs CSRF protection (the
	// JSON API above doesn't send credentials via cookies, so it isn't
	// exposed to the same attack).
	r.Route("/admin", func(admin chi.Router) {
		admin.Get("/login", h.Admin.LoginForm)
		admin.With(auth.RequireCSRF).Post("/login", h.Admin.Login)
		admin.Route("/dashboard", func(dash chi.Router) {
			dash.Use(cfg.Sessions.RequireSession)
			dash.Get("/", h.Admin.Dashboard)
		})
		admin.With(cfg.Sessions.RequireSession, auth.RequireCSRF).Post("/logout", h.Admin.Logout)
	})

	// SAML SSO for the fictitious B2B business portal — the seventh and
	// last auth style. Registered only when an IdP is actually configured;
	// see internal/auth/saml.go's doc comment for why this repo doesn't
	// wire up a live third-party IdP by default.
	if h.SAML != nil {
		r.Get("/saml/metadata", h.SAML.MetadataHandler().ServeHTTP)
		r.Post("/saml/acs", h.SAML.ACSHandler().ServeHTTP)
		r.Route("/business/admin", func(business chi.Router) {
			business.Use(h.SAML.RequireSession)
			business.Get("/login", func(w http.ResponseWriter, r *http.Request) {
				httputil.JSON(w, http.StatusOK, map[string]string{"status": "authenticated via SAML"})
			})
		})
	}

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
