package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/db"
)

var ErrAPIKeyNotFound = errors.New("auth: api key not found or revoked")

// APIKeyRepository is the persistence side of apikey.go's hashing logic —
// kept in the same package since api_keys is purely an auth concern, not a
// domain entity any other package needs to know about.
type APIKeyRepository struct {
	db *db.DB
}

func NewAPIKeyRepository(database *db.DB) *APIKeyRepository {
	return &APIKeyRepository{db: database}
}

func (r *APIKeyRepository) Create(ctx context.Context, ownerLabel, keyHash string) (int64, error) {
	res, err := r.db.Primary.ExecContext(ctx,
		`INSERT INTO api_keys (owner_label, key_hash) VALUES (?, ?)`, ownerLabel, keyHash)
	if err != nil {
		return 0, fmt.Errorf("auth: create api key: %w", err)
	}
	return res.LastInsertId()
}

// FindActiveByHash returns the owner label for a non-revoked key hash — the
// only thing RequireAPIKey needs to know to authenticate a partner request.
func (r *APIKeyRepository) FindActiveByHash(ctx context.Context, hash string) (string, error) {
	var ownerLabel string
	err := r.db.Reader().GetContext(ctx, &ownerLabel,
		`SELECT owner_label FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAPIKeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("auth: find api key: %w", err)
	}
	return ownerLabel, nil
}
