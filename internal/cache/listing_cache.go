package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ListingCache implements the cache-aside pattern for the two hottest,
// most expensive-to-compute reads: the browse feed and listing detail
// (both backed by the JOIN query in internal/listing/query_optimized.go).
// Callers (internal/listing/handler.go) look here first; on a miss they
// compute the real result and call Set before responding.
//
// Invalidation is explicit, not TTL-only: internal/booking/service.go calls
// InvalidateDetail whenever a booking changes a listing's availability, so
// a stale "still available" read is bounded by application logic, not just
// by however long the TTL happens to be.
type ListingCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewListingCache(client *redis.Client, ttl time.Duration) *ListingCache {
	return &ListingCache{client: client, ttl: ttl}
}

func feedKey(limit, offset int) string {
	return fmt.Sprintf("gearshare:listing:feed:%d:%d", limit, offset)
}

func detailKey(listingID int64) string {
	return fmt.Sprintf("gearshare:listing:detail:%d", listingID)
}

// GetFeed returns the cached JSON payload and true on a hit, or (nil,
// false) on a miss/Redis error — callers should treat a Redis outage as a
// miss and fall through to MySQL (graceful degradation, requirement 19),
// never as a hard failure.
func (c *ListingCache) GetFeed(ctx context.Context, limit, offset int) ([]byte, bool) {
	return c.get(ctx, feedKey(limit, offset))
}

func (c *ListingCache) SetFeed(ctx context.Context, limit, offset int, payload []byte) {
	c.set(ctx, feedKey(limit, offset), payload)
}

func (c *ListingCache) GetDetail(ctx context.Context, listingID int64) ([]byte, bool) {
	return c.get(ctx, detailKey(listingID))
}

func (c *ListingCache) SetDetail(ctx context.Context, listingID int64, payload []byte) {
	c.set(ctx, detailKey(listingID), payload)
}

// InvalidateDetail is called from internal/booking/service.go on every
// booking status transition. It also drops the feed pages, cheaply and
// crudely (a handful of keys, not a scan) — the feed's rating/count fields
// can go stale exactly the same way, and feed pages are few enough that
// evicting all of them costs nothing.
func (c *ListingCache) InvalidateDetail(ctx context.Context, listingID int64) {
	c.client.Del(ctx, detailKey(listingID))
	c.client.Del(ctx, feedKey(20, 0)) // the default, by-far-most-requested page
}

func (c *ListingCache) get(ctx context.Context, key string) ([]byte, bool) {
	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return val, true
}

func (c *ListingCache) set(ctx context.Context, key string, payload []byte) {
	_ = c.client.Set(ctx, key, payload, c.ttl).Err()
}
