package db

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// MySQL error 1213 = ER_LOCK_DEADLOCK, 1205 = ER_LOCK_WAIT_TIMEOUT.
// Both are transient and safe to retry with a fresh transaction — see
// docs/adr/0002-cap-tradeoffs.md "failure modes" for why booking creation
// retries here instead of surfacing a 500 on the first conflict.
const (
	mysqlErrLockDeadlock    = 1213
	mysqlErrLockWaitTimeout = 1205
)

const maxTxRetries = 3

// WithinTransaction runs fn inside a *sqlx.Tx, committing on success and
// rolling back on error. Deadlocks/lock-wait-timeouts are retried with jitter
// up to maxTxRetries times before the error is returned to the caller — this
// is the concrete "failure mode" handling required by the database-depth
// requirement, exercised by internal/booking/repository_integration_test.go.
func (d *DB) WithinTransaction(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	var lastErr error
	for attempt := 0; attempt < maxTxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 20 * time.Millisecond
			jitter := time.Duration(rand.Intn(20)) * time.Millisecond
			time.Sleep(backoff + jitter)
		}

		err := d.attemptTx(ctx, fn)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
	}
	return fmt.Errorf("db: transaction failed after %d attempts: %w", maxTxRetries, lastErr)
}

func (d *DB) attemptTx(ctx context.Context, fn func(tx *sqlx.Tx) error) (err error) {
	tx, err := d.Primary.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	err = fn(tx)
	return err
}

func isRetryable(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == mysqlErrLockDeadlock || mysqlErr.Number == mysqlErrLockWaitTimeout
	}
	return false
}
