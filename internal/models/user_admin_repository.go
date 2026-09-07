package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"
)

// UserAdminRepository provides admin-only user operations.
type UserAdminRepository struct {
	queries *db.Queries
}

// NewUserAdminRepository creates a new admin user repository.
func NewUserAdminRepository(dbConn *db.DB) *UserAdminRepository {
	return &UserAdminRepository{
		queries: db.New(dbConn.Conn()),
	}
}

// ListUsers retrieves all users ordered by creation date (newest first).
// The context is passed through to the underlying sqlc query so a
// caller-driven cancellation (e.g. the weekly reminder's parent
// context) propagates to the database. Production code that does
// not have a request context to thread through can pass
// context.Background().
func (r *UserAdminRepository) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	users := make([]User, len(rows))
	for i, row := range rows {
		users[i] = *mapUser(row)
	}
	return users, nil
}

// GetUserByID retrieves a single user by their ID. Returns nil when
// the user does not exist. The context is passed through to the
// underlying sqlc query so a caller-driven cancellation (e.g. the
// request being aborted mid-action) propagates to the database.
func (r *UserAdminRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row, err := r.queries.GetUserByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	return mapUser(row), nil
}

// SetUserAdmin grants or revokes a user's admin status, bumping
// updated_at so the row's edit history stays accurate. The flag is
// written as the 0/1 INTEGER the users table stores. The SET query's
// RETURNING clause makes "user does not exist" surface as an error
// rather than a silent no-op — a guard against the admin acting on a
// row that was deleted after the list page rendered.
func (r *UserAdminRepository) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) error {
	if userID == "" {
		return fmt.Errorf("failed to set admin status: user id is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var isAdminInt int64
	if isAdmin {
		isAdminInt = 1
	}
	if _, err := r.queries.SetUserAdmin(ctx, db.SetUserAdminParams{
		IsAdmin:   isAdminInt,
		UpdatedAt: sql.NullTime{Time: time.Now(), Valid: true},
		ID:        userID,
	}); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("failed to set admin status: user %s not found", userID)
		}
		return fmt.Errorf("failed to set admin status: %w", err)
	}
	return nil
}
