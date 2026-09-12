package middleware

import (
	"time"

	"github.com/sony/gobreaker"

	"github.com/thvnhtai/gearshare/internal/observability"
)

// NewBreaker wraps a downstream call with a circuit breaker: after
// MaxRequests consecutive failures within the rolling window, the breaker
// trips open and short-circuits further calls (returning immediately
// instead of waiting out a timeout against a known-bad dependency) until
// Timeout elapses, then allows a single trial request through (half-open)
// to test recovery. Used by internal/search/service.go around the
// search-indexer gRPC client and by search-indexer itself around its
// Elasticsearch client.
func NewBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Interval:    30 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		// Feeds the gearshare_circuit_breaker_state gauge (Grafana panel in
		// deployments/grafana/dashboards/gearshare-overview.json) — a
		// breaker sitting open is exactly the kind of thing "monitoring"
		// (as distinct from mere logging) should surface at a glance.
		OnStateChange: func(name string, from, to gobreaker.State) {
			observability.CircuitBreakerState.WithLabelValues(name).Set(float64(to))
		},
	})
}
