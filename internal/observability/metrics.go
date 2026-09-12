package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the "monitoring" pillar: a Prometheus /metrics endpoint on
// every service, scraped by docker-compose.yml's `prometheus` container
// (deployments/prometheus/prometheus.yml) and visualized in
// deployments/grafana/dashboards/gearshare-overview.json.
var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "gearshare_http_requests_total", Help: "Total HTTP requests processed."},
		[]string{"method", "path", "status"},
	)
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "gearshare_http_request_duration_seconds", Help: "HTTP request latency.", Buckets: prometheus.DefBuckets},
		[]string{"method", "path"},
	)
	BookingTransactionRetries = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "gearshare_booking_tx_retries_total", Help: "Deadlock/lock-wait retries in the booking transaction (internal/db/tx.go)."},
	)
	CircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "gearshare_circuit_breaker_state", Help: "0=closed, 1=half-open, 2=open."},
		[]string{"breaker"},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequestsTotal, HTTPRequestDuration, BookingTransactionRetries, CircuitBreakerState)
}

// Handler exposes the /metrics endpoint for Prometheus to scrape.
func Handler() http.Handler {
	return promhttp.Handler()
}

// HTTPMiddleware records request count and latency per method+route
// (chi's RoutePattern, not the raw URL, so /listings/{id} isn't a
// cardinality explosion of one label value per listing ID).
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := routePattern(r)
		HTTPRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// routePattern falls back to the raw path when chi's route context isn't
// available (e.g. a request that matched no route at all — a 404).
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}
