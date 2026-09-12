package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/db"
)

var ErrNotFound = errors.New("user: not found")
var ErrEmailTaken = errors.New("user: email already registered")

// Repository is hand-written sqlx today; see docs/adr/0001-orm-choice.md for
// why this will become a thin wrapper over generated sqlc code once a
// working cgo toolchain is available to run `make sqlc`. The queries here
// mirror internal/db/queries/user.sql exactly.
type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) Create(ctx context.Context, u *User) (int64, error) {
	const q = `INSERT INTO users (email, password_hash, password_algo, display_name, role)
	           VALUES (?, ?, ?, ?, ?)`
	res, err := r.db.Primary.ExecContext(ctx, q, u.Email, u.PasswordHash, u.PasswordAlgo, u.DisplayName, u.Role)
	if err != nil {
		if isDuplicateKey(err) {
			return 0, ErrEmailTaken
		}
		return 0, fmt.Errorf("user: create: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	return r.get(ctx, "email", email)
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*User, error) {
	return r.get(ctx, "id", id)
}

func (r *Repository) get(ctx context.Context, column string, value interface{}) (*User, error) {
	q := fmt.Sprintf(`SELECT id, email, password_hash, password_algo, display_name, role, created_at, updated_at
	                   FROM users WHERE %s = ?`, column)
	var u User
	err := r.db.Reader().GetContext(ctx, &u, q, value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user: get by %s: %w", column, err)
	}
	return &u, nil
}

// UpsertOAuthIdentity links a Google OIDC subject to a local user row,
// creating the user first if this is their first login via OAuth.
func (r *Repository) UpsertOAuthIdentity(ctx context.Context, tx *sqlx.Tx, provider, providerUserID string, newUser *User) (int64, error) {
	var userID int64
	err := tx.GetContext(ctx, &userID,
		`SELECT user_id FROM oauth_identities WHERE provider = ? AND provider_user_id = ?`,
		provider, providerUserID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("user: lookup oauth identity: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, password_algo, display_name, role) VALUES (?, '', 'bcrypt', ?, ?)`,
		newUser.Email, newUser.DisplayName, RoleRenter)
	if err != nil {
		return 0, fmt.Errorf("user: create oauth user: %w", err)
	}
	userID, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO oauth_identities (user_id, provider, provider_user_id) VALUES (?, ?, ?)`,
		userID, provider, providerUserID); err != nil {
		return 0, fmt.Errorf("user: link oauth identity: %w", err)
	}
	return userID, nil
}

// GetByIDs is the batching primitive internal/listing/loader.go uses instead
// of one GetByID call per listing owner.
func (r *Repository) GetByIDs(ctx context.Context, ids []int64) (map[int64]User, error) {
	result := make(map[int64]User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(
		`SELECT id, email, password_hash, password_algo, display_name, role, created_at, updated_at
		 FROM users WHERE id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("user: build in-query: %w", err)
	}
	query = r.db.Reader().Rebind(query)

	var users []User
	if err := r.db.Reader().SelectContext(ctx, &users, query, args...); err != nil {
		return nil, fmt.Errorf("user: get by ids: %w", err)
	}
	for _, u := range users {
		result[u.ID] = u
	}
	return result, nil
}

func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}
