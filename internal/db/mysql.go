// Package db owns the MySQL connection pool(s) and the transaction boundary
// helper shared by every internal/<capability> repository.
package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/config"
)

// DB wraps a primary (read/write) and a replica (read-only) *sqlx.DB.
// In local dev both point at the same instance; docker-compose.yml wires a
// real binlog-replicated mysql-replica for the primary/replica scaling demo
// (docs/adr/0002-cap-tradeoffs.md).
type DB struct {
	Primary *sqlx.DB
	Replica *sqlx.DB
}

// Reader returns the connection pool that should serve read-only queries —
// the replica when one is configured, the primary otherwise. Callers that
// must read their own recent write (read-after-write consistency) should
// use Primary directly instead.
func (d *DB) Reader() *sqlx.DB {
	if d.Replica != nil {
		return d.Replica
	}
	return d.Primary
}

func Connect(ctx context.Context, cfg config.MySQLConfig) (*DB, error) {
	primary, err := connect(ctx, cfg.PrimaryDSN, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect primary: %w", err)
	}

	replicaDSN := cfg.ReplicaDSN
	if replicaDSN == "" {
		replicaDSN = cfg.PrimaryDSN
	}
	replica, err := connect(ctx, replicaDSN, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect replica: %w", err)
	}

	return &DB{Primary: primary, Replica: replica}, nil
}

func connect(ctx context.Context, dsn string, cfg config.MySQLConfig) (*sqlx.DB, error) {
	conn, err := sqlx.ConnectContext(ctx, "mysql", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	return conn, nil
}

func (d *DB) Close() error {
	var errs []error
	if err := d.Primary.Close(); err != nil {
		errs = append(errs, err)
	}
	if d.Replica != d.Primary {
		if err := d.Replica.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("db: close errors: %v", errs)
	}
	return nil
}

// TxOptions is re-exported so callers don't need to import database/sql
// just to pass sql.LevelSerializable, etc.
type TxOptions = sql.TxOptions
