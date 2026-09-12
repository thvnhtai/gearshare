package availability

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/db"
)

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// CountOverlapping is called from within internal/booking/tx.go's
// transaction, immediately before InsertBlock, so the check-then-insert
// pair is atomic under the transaction's row locks — this is what actually
// prevents a double-booking race, not the CHECK constraint alone.
func (r *Repository) CountOverlapping(ctx context.Context, tx *sqlx.Tx, listingID int64, start, end time.Time) (int, error) {
	var count int
	const q = `SELECT COUNT(*) FROM availability_blocks
	           WHERE listing_id = ? AND start_date <= ? AND end_date >= ?
	           FOR UPDATE`
	if err := tx.GetContext(ctx, &count, q, listingID, end, start); err != nil {
		return 0, fmt.Errorf("availability: count overlapping: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertBlock(ctx context.Context, tx *sqlx.Tx, listingID int64, bookingID int64, start, end time.Time, reason Reason) (int64, error) {
	const q = `INSERT INTO availability_blocks (listing_id, booking_id, start_date, end_date, reason)
	           VALUES (?, ?, ?, ?, ?)`
	res, err := tx.ExecContext(ctx, q, listingID, bookingID, start, end, reason)
	if err != nil {
		return 0, fmt.Errorf("availability: insert block: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) ListForListing(ctx context.Context, listingID int64) ([]Block, error) {
	var blocks []Block
	const q = `SELECT id, listing_id, booking_id, start_date, end_date, reason, created_at
	           FROM availability_blocks WHERE listing_id = ? ORDER BY start_date`
	if err := r.db.Reader().SelectContext(ctx, &blocks, q, listingID); err != nil {
		return nil, fmt.Errorf("availability: list for listing: %w", err)
	}
	return blocks, nil
}
