package middleware

import (
	"net"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// ResolveClientIP is the one place this app decides "who is the client",
// shared by both rate limiters (this file's in-process one and
// internal/cache.RateLimiter's Redis-backed one, wired together by
// cmd/api/main.go) so a request keys the same identity in either path.
// Prefers the trusted-proxy-resolved IP (internal/app/routes.go's
// ClientIPFromXFFTrustedProxies(1) middleware); falls back to the raw TCP
// peer address only when that's unavailable (e.g. this instance really is
// directly exposed, not behind Nginx, or a request slips through before
// any XFF header exists).
//
// This fallback matters concretely: without it, every request lacking a
// resolved XFF IP keys under the same empty string — one shared bucket for
// every such client instead of one each — found by checking the actual
// Redis keys after a real local request, not by inspection alone.
func ResolveClientIP(r *http.Request) string {
	if ip := chimiddleware.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// RateLimit implements the "throttling" resilience pattern: per-client-IP
// token bucket, protecting both the booking-creation hot path and the auth
// endpoints from brute-force/abuse. This in-memory limiter is per-process;
// internal/cache.RateLimiter provides the Redis-backed variant needed once
// the API runs as more than one replica (see docs/adr — CAP tradeoffs notes
// this is an AP-leaning choice: under a network partition between
// replicas' Redis, we'd rather under- or over-throttle briefly than block
// all requests).
func RateLimit(requestsPerWindow int, window time.Duration) func(http.Handler) http.Handler {
	// Not httprate.LimitByIP: it keys off r.RemoteAddr, which behind Nginx
	// is always the proxy's address, bucketing every client into one shared
	// limit.
	return httprate.LimitBy(requestsPerWindow, window, func(r *http.Request) (string, error) {
		return ResolveClientIP(r), nil
	})
}
