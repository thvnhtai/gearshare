package category

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/db"
)

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) List(ctx context.Context) ([]Category, error) {
	var cats []Category
	err := r.db.Reader().SelectContext(ctx, &cats, `SELECT id, name, slug FROM categories ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("category: list: %w", err)
	}
	return cats, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Category, error) {
	var c Category
	err := r.db.Reader().GetContext(ctx, &c, `SELECT id, name, slug FROM categories WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("category: get: %w", err)
	}
	return &c, nil
}

// GetByIDs is the batching primitive the listing dataloader (internal/listing/loader.go)
// uses instead of issuing one GetByID call per listing.
func (r *Repository) GetByIDs(ctx context.Context, ids []int64) (map[int64]Category, error) {
	if len(ids) == 0 {
		return map[int64]Category{}, nil
	}
	query, args, err := sqlx.In(`SELECT id, name, slug FROM categories WHERE id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("category: build in-query: %w", err)
	}
	query = r.db.Reader().Rebind(query)

	var cats []Category
	if err := r.db.Reader().SelectContext(ctx, &cats, query, args...); err != nil {
		return nil, fmt.Errorf("category: get by ids: %w", err)
	}

	byID := make(map[int64]Category, len(cats))
	for _, c := range cats {
		byID[c.ID] = c
	}
	return byID, nil
}
