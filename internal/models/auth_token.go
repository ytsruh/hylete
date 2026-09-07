package models

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hylete/internal/db"
)

// AuthTokenPurpose enumerates the kinds of one-time credential the
// auth_tokens table can hold. The column is TEXT in the schema so new
// purposes (magic link, email verify) can be added without a
// migration; just add a constant here and a sqlc query if it needs
// a different lookup path.
type AuthTokenPurpose string

const (
	// AuthTokenPurposePasswordReset is issued by the "forgot password"
	// flow and lets the holder choose a new password. Default TTL: 1h.
	AuthTokenPurposePasswordReset AuthTokenPurpose = "password_reset"
)

// ErrAuthTokenInvalid is returned by Consume*Token methods when the
// supplied raw token does not match any row, has already been used,
// or has expired. The three cases are deliberately collapsed into a
// single error so an attacker cannot probe the state of someone
// else's token by observing different error messages.
var ErrAuthTokenInvalid = errors.New("auth token is invalid, used, or expired")

// AuthToken is the application-level view of a row in auth_tokens.
// TokenHash is the sha256 hex digest of the raw token, never the raw
// token itself; the raw token only ever lives in the email link.
type AuthToken struct {
	ID        string
	UserID    string
	Purpose   AuthTokenPurpose
	TokenHash string
	ExpiresAt time.Time
	// UsedAt is the zero value when the token is still outstanding;
	// sql.NullTime so the "unused" case round-trips correctly.
	UsedAt    sql.NullTime
	CreatedAt time.Time
}

// AuthTokenRepo is the data-access surface the auth-recovery
// controller depends on. Defined as an interface (rather than a
// concrete repository type) so the controller can be unit-tested
// without touching the database, and so callers can declare only
// the methods they need.
//
// The interface accepts the *raw* token at every boundary; the
// repository is responsible for hashing. Keeping the hashing inside
// the repository ensures the raw-vs-hash split is enforced in one
// place: a caller that needs a token's user-id can never
// accidentally read the raw token off a row.
type AuthTokenRepo interface {
	// CreatePasswordResetToken stores a new password-reset row whose
	// hash is sha256(rawToken). Returns the row's ID (which the
	// caller does not need to use — it is exposed for tests and for
	// the future admin "revoke outstanding tokens" UI).
	CreatePasswordResetToken(ctx context.Context, userID, rawToken string, ttl time.Duration) (string, error)

	// ConsumePasswordResetToken atomically claims a token, marks it
	// used, and returns the owning userID. Returns
	// ErrAuthTokenInvalid if the token is unknown, already used, or
	// expired (see the error's docstring for the rationale).
	ConsumePasswordResetToken(ctx context.Context, rawToken string) (string, error)
}

// Compile-time check: AuthTokenRepository satisfies the model-level
// interface. Kept here (rather than next to the interface in
// repository.go) so the wiring for the auth-token feature is in a
// single file.
var _ AuthTokenRepo = (*AuthTokenRepository)(nil)

// AuthTokenRepository is the default implementation backed by the
// sqlc-generated queries. Same shape as the other repositories in
// the package: thin wrapper around *db.Queries, with the raw-token
// <-> hash conversion kept here so the rest of the codebase never
// has to think about it.
type AuthTokenRepository struct {
	queries *db.Queries
}

// NewAuthTokenRepository returns a repository bound to the given
// database connection. The connection is wrapped in a sqlc Queries
// object on every call to keep the constructor cheap and consistent
// with the other repositories in the package.
func NewAuthTokenRepository(database *db.DB) *AuthTokenRepository {
	return &AuthTokenRepository{queries: db.New(database.Conn())}
}

// CreatePasswordResetToken mints a new password-reset row. rawToken
// is hashed with sha256 before insertion so the database never
// contains a usable credential. ttl is added to time.Now() to set
// expires_at; passing a zero/negative value is a programming error
// and is returned as such.
func (r *AuthTokenRepository) CreatePasswordResetToken(ctx context.Context, userID, rawToken string, ttl time.Duration) (string, error) {
	if userID == "" {
		return "", errors.New("auth_tokens: userID is empty")
	}
	if rawToken == "" {
		return "", errors.New("auth_tokens: rawToken is empty")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("auth_tokens: ttl must be positive, got %s", ttl)
	}

	row, err := r.queries.CreateAuthToken(ctx, db.CreateAuthTokenParams{
		ID:        uuid.New().String(),
		UserID:    userID,
		Purpose:   string(AuthTokenPurposePasswordReset),
		TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().Add(ttl),
	})
	if err != nil {
		return "", fmt.Errorf("create auth token: %w", err)
	}
	return row.ID, nil
}

// ConsumePasswordResetToken looks up the token by hash, atomically
// marks it used, and returns the userID. The single-statement
// UPDATE...RETURNING in the underlying query is what makes
// consumption race-free across tabs/devices.
//
// All three "bad" outcomes (unknown hash, already used, expired)
// are reported as ErrAuthTokenInvalid so the controller can return a
// single generic error to the route handler.
func (r *AuthTokenRepository) ConsumePasswordResetToken(ctx context.Context, rawToken string) (string, error) {
	if rawToken == "" {
		return "", ErrAuthTokenInvalid
	}
	row, err := r.queries.ConsumeAuthToken(ctx, db.ConsumeAuthTokenParams{
		TokenHash: hashToken(rawToken),
		Purpose:   string(AuthTokenPurposePasswordReset),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAuthTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("consume auth token: %w", err)
	}
	return row.UserID, nil
}

// hashToken returns the lowercase hex sha256 of the raw token. The
// hex form is what gets stored in the database; the raw token only
// ever lives in the email link. Same digest on every call so the
// value is safe to use as a lookup-key index (see
// idx_auth_tokens_lookup in the migration).
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
