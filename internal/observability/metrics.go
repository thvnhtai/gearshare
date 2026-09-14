package observability

import (
	"bufio"
	"fmt"
	"net"
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

// Hijack delegates to the underlying ResponseWriter's http.Hijacker, when
// it has one. Without this, this middleware being in the global chain
// silently broke every WebSocket upgrade in the app: gorilla/websocket
// v1.5.3 does a direct `w.(http.Hijacker)` type assertion (server.go), not
// the Go 1.20+ http.ResponseController/Unwrap protocol — so wrapping the
// ResponseWriter in *statusRecorder without an explicit Hijack method hid
// the underlying Hijacker completely, and every Upgrade() call failed with
// a bare 500. Found by actually opening the frontend in a browser and
// watching a real `Upgrade: websocket` handshake fail — curl-only API
// testing never exercises this path.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("observability: underlying ResponseWriter does not support Hijack")
	}
	return hijacker.Hijack()
}

// Flush lets Server-Sent Events flush each write immediately instead of
// waiting for Go's HTTP buffering — internal/realtime/sse.go depends on
// this via http.Flusher just like it depends on Hijack for WebSockets.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
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
