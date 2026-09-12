package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/db"
)

var ErrNotFound = errors.New("booking: not found")
var ErrInvalidTransition = errors.New("booking: invalid status transition")

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// CreateInTx inserts the booking row as part of an already-open transaction
// (see tx.go) — the atomicity guarantee is the transaction boundary, not
// this method in isolation.
func (r *Repository) CreateInTx(ctx context.Context, tx *sqlx.Tx, b *Booking) (int64, error) {
	const q = `INSERT INTO bookings (listing_id, renter_id, start_date, end_date, status, total_price_cents)
	           VALUES (?, ?, ?, ?, ?, ?)`
	res, err := tx.ExecContext(ctx, q, b.ListingID, b.RenterID, b.StartDate, b.EndDate, b.Status, b.TotalPriceCents)
	if err != nil {
		return 0, fmt.Errorf("booking: create in tx: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Booking, error) {
	var b Booking
	const q = `SELECT id, listing_id, renter_id, start_date, end_date, status, total_price_cents, created_at, updated_at
	           FROM bookings WHERE id = ?`
	err := r.db.Reader().GetContext(ctx, &b, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("booking: get: %w", err)
	}
	return &b, nil
}

// UpdateStatus enforces the lifecycle state machine (model.go's
// CanTransition) before writing, so an invalid transition never reaches
// the database.
func (r *Repository) UpdateStatus(ctx context.Context, id int64, to Status) error {
	current, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !CanTransition(current.Status, to) {
		return ErrInvalidTransition
	}

	const q = `UPDATE bookings SET status = ? WHERE id = ?`
	if _, err := r.db.Primary.ExecContext(ctx, q, to, id); err != nil {
		return fmt.Errorf("booking: update status: %w", err)
	}
	return nil
}

// ListForOwner backs the SSE owner-dashboard feed (internal/realtime/sse.go).
func (r *Repository) ListForOwner(ctx context.Context, ownerID int64) ([]Booking, error) {
	var bookings []Booking
	const q = `SELECT b.id, b.listing_id, b.renter_id, b.start_date, b.end_date, b.status, b.total_price_cents, b.created_at, b.updated_at
	           FROM bookings b
	           JOIN gear_listings gl ON gl.id = b.listing_id
	           WHERE gl.owner_id = ?
	           ORDER BY b.updated_at DESC`
	if err := r.db.Reader().SelectContext(ctx, &bookings, q, ownerID); err != nil {
		return nil, fmt.Errorf("booking: list for owner: %w", err)
	}
	return bookings, nil
}
