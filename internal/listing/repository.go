package listing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/db"
)

var ErrNotFound = errors.New("listing: not found")

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) Create(ctx context.Context, l *Listing) (int64, error) {
	const q = `INSERT INTO gear_listings (owner_id, category_id, title, description, price_per_day_cents, deposit_cents, status)
	           VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.Primary.ExecContext(ctx, q, l.OwnerID, l.CategoryID, l.Title, l.Description, l.PricePerDayCents, l.DepositCents, l.Status)
	if err != nil {
		return 0, fmt.Errorf("listing: create: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Listing, error) {
	var l Listing
	err := r.db.Reader().GetContext(ctx, &l,
		`SELECT id, owner_id, category_id, title, description, price_per_day_cents, deposit_cents, status, created_at, updated_at
		 FROM gear_listings WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("listing: get: %w", err)
	}
	return &l, nil
}

// ListActiveIDs is step 1 of the naive feed path: fetch just the page of
// active listings, nothing else. See query_naive.go for what happens next.
func (r *Repository) ListActivePage(ctx context.Context, limit, offset int) ([]Listing, error) {
	var listings []Listing
	err := r.db.Reader().SelectContext(ctx, &listings,
		`SELECT id, owner_id, category_id, title, description, price_per_day_cents, deposit_cents, status, created_at, updated_at
		 FROM gear_listings WHERE status = 'active' ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing: list active page: %w", err)
	}
	return listings, nil
}
