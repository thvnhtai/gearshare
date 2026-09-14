package cache

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/thvnhtai/gearshare/internal/httputil"
)

// RateLimiter is the distributed counterpart to
// internal/middleware.RateLimit's in-process limiter: once the API runs as
// more than one replica (deployments/k8s/base/api.yaml requests 2), an
// in-process bucket under-throttles — a client hitting two different pods
// gets 2x the intended limit, since neither pod's counter knows about the
// other's requests.
//
// This is a fixed-window counter (INCR + EXPIRE on a key scoped to the
// current window), not a token bucket: simpler to implement correctly with
// a single round trip, and — given the honest tradeoff — allows a client to
// burst up to ~2x the limit across a window boundary (all their budget at
// the end of one window, all of it again at the start of the next). That's
// an accepted, disclosed limitation, not an oversight; a sliding-window-log
// or token-bucket Lua script would close it at the cost of more Redis
// round trips/complexity than this project's traffic justifies.
type RateLimiter struct {
	client *redis.Client
}

func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Allow reports whether the given key (e.g. "ip:<addr>" or "route:<pattern>:ip:<addr>")
// may proceed under a limit of `max` requests per `window`. A Redis error
// here fails OPEN (returns true, allowed) — this is requirement 19's
// graceful degradation again: an unreachable rate limiter should never
// itself become the reason legitimate traffic gets rejected.
func (r *RateLimiter) Allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	windowKey := fmt.Sprintf("gearshare:ratelimit:%s:%d", key, time.Now().Unix()/int64(window.Seconds()))

	count, err := r.client.Incr(ctx, windowKey).Result()
	if err != nil {
		return true, fmt.Errorf("ratelimiter: incr: %w", err)
	}
	if count == 1 {
		// Only the request that created this window's key needs to set its
		// expiry — every subsequent Incr in the same window is a no-op here.
		r.client.Expire(ctx, windowKey, window)
	}
	return count <= int64(max), nil
}

// Middleware wraps a handler with the distributed limiter, keyed by chi's
// resolved client IP (internal/app/routes.go's ClientIPFromXFFTrustedProxies)
// so it shares the same spoofing-resistant IP resolution as everything
// else that makes a per-client decision.
func (r *RateLimiter) Middleware(max int, window time.Duration, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			allowed, err := r.Allow(req.Context(), keyFunc(req), max, window)
			if err != nil {
				// Logged by the caller's own observability stack via the
				// standard request-duration/error metrics; degrade open.
				next.ServeHTTP(w, req)
				return
			}
			if !allowed {
				httputil.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
