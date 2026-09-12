package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// RateLimit implements the "throttling" resilience pattern: per-client-IP
// token bucket, protecting both the booking-creation hot path and the auth
// endpoints from brute-force/abuse. This in-memory limiter is per-process;
// internal/cache/ratelimiter.go provides the Redis-backed variant needed
// once the API runs as more than one replica (see docs/adr — CAP tradeoffs
// note this is an AP-leaning choice: under a network partition between
// replicas' Redis, we'd rather under- or over-throttle briefly than block
// all requests).
func RateLimit(requestsPerWindow int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.LimitByIP(requestsPerWindow, window)
}
