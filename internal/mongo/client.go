// Package mongo owns the shared MongoDB client used by internal/spec and
// internal/damagereport — the two collections GearShare deliberately keeps
// out of MySQL because their shape is naturally document-like, not
// relational (see docs/architecture.md's "data store boundaries").
package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/thvnhtai/gearshare/internal/config"
)

func Connect(ctx context.Context, cfg config.MongoConfig) (*mongo.Database, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("mongo: connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("mongo: ping: %w", err)
	}
	return client.Database(cfg.Database), nil
}
