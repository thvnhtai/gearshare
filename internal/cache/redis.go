// Package cache holds the Redis-backed server-side cache-aside layer and
// the distributed rate limiter used once the API runs as more than one
// replica (internal/middleware/ratelimit.go's in-process limiter is
// per-process only).
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/thvnhtai/gearshare/internal/config"
)

func Connect(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cache: connect: %w", err)
	}
	return client, nil
}
