package models

import (
	"context"
	"errors"
	"testing"
	"time"

	"hylete/internal/db"
)

// authTokenTestHarness returns a connected in-memory DB plus a
// pre-seeded user and an AuthTokenRepository. Closing the returned
// database is the caller's responsibility. The seeded user exists
// because auth_tokens.user_id has a NOT NULL FK to users(id); every
// test that needs to issue a token can reuse it.
func authTokenTestHarness(t *testing.T) (*AuthTokenRepository, *UserRepository, *User, *db.DB) {
	t.Helper()

	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("in-memory db: %v", err)
	}

	userRepo := NewUserRepository(database)
	user := &User{
		Name:         "Test User",
		Email:        "user@example.com",
		PasswordHash: "hash",
	}
	if err := userRepo.CreateUser(user); err != nil {
		database.Close()
		t.Fatalf("seed user: %v", err)
	}

	return NewAuthTokenRepository(database), userRepo, user, database
}

func TestAuthTokenRepository_CreateAndConsume_HappyPath(t *testing.T) {
	repo, _, user, database := authTokenTestHarness(t)
	defer database.Close()

	ctx := context.Background()
	const raw = "the-quick-brown-fox"

	id, err := repo.CreatePasswordResetToken(ctx, user.ID, raw, time.Hour)
	if err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty row ID")
	}

	got, err := repo.ConsumePasswordResetToken(ctx, raw)
	if err != nil {
		t.Fatalf("ConsumePasswordResetToken: %v", err)
	}
	if got != user.ID {
		t.Fatalf("userID = %q, want %q", got, user.ID)
	}
}

func TestAuthTokenRepository_ConsumeIsSingleUse(t *testing.T) {
	// A second consume of the same raw token must fail with
	// ErrAuthTokenInvalid even though the row still exists. This is
	// the property that prevents a leaked reset link from being
	// used twice (e.g. by an attacker who races the victim).
	repo, _, user, database := authTokenTestHarness(t)
	defer database.Close()

	ctx := context.Background()
	if _, err := repo.CreatePasswordResetToken(ctx, user.ID, "raw-token", time.Hour); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	// First consume succeeds.
	if _, err := repo.ConsumePasswordResetToken(ctx, "raw-token"); err != nil {
		t.Fatalf("first ConsumePasswordResetToken: %v", err)
	}
	// Second consume fails identically to "unknown token" so a
	// caller cannot distinguish used vs not-yet-issued.
	_, err := repo.ConsumePasswordResetToken(ctx, "raw-token")
	if !errors.Is(err, ErrAuthTokenInvalid) {
		t.Fatalf("second consume err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthTokenRepository_ConsumeUnknownToken(t *testing.T) {
	// No row exists for this hash. The repo must return
	// ErrAuthTokenInvalid, not a wrapped sql.ErrNoRows, so the
	// controller can rely on errors.Is.
	repo, _, _, database := authTokenTestHarness(t)
	defer database.Close()

	_, err := repo.ConsumePasswordResetToken(context.Background(), "never-issued")
	if !errors.Is(err, ErrAuthTokenInvalid) {
		t.Fatalf("err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthTokenRepository_ConsumeExpiredToken(t *testing.T) {
	// A token whose expires_at is in the past must consume-fail
	// with ErrAuthTokenInvalid, exactly like a never-issued token.
	// We bypass the repo's "ttl must be positive" guard by
	// inserting directly via sqlc.
	repo, _, user, database := authTokenTestHarness(t)
	defer database.Close()

	// Bypass the positive-ttl guard: use the negative-ttl path on
	// the repo, which currently rejects. To exercise the
	// database-clock-based expiry check (the one used in
	// production) we need a row in the past. The simplest way is
	// to construct one through CreatePasswordResetToken with a
	// negative ttl, but the repo refuses. Use sqlc directly to
	// model the real production row shape.
	queries := db.New(database.Conn())
	_, err := queries.CreateAuthToken(context.Background(), db.CreateAuthTokenParams{
		ID:        "row-1",
		UserID:    user.ID,
		Purpose:   string(AuthTokenPurposePasswordReset),
		TokenHash: hashToken("expired-raw"),
		// Yesterday — well past any sane TTL.
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed expired row: %v", err)
	}

	_, err = repo.ConsumePasswordResetToken(context.Background(), "expired-raw")
	if !errors.Is(err, ErrAuthTokenInvalid) {
		t.Fatalf("err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthTokenRepository_CreateRejectsEmptyInputs(t *testing.T) {
	// All three inputs are programming errors; they get specific
	// messages so a misconfigured caller can be told which knob
	// they got wrong. We only assert "non-nil" because the exact
	// message is implementation detail and is documented in the
	// method's docstring.
	repo, _, user, database := authTokenTestHarness(t)
	defer database.Close()

	ctx := context.Background()

	if _, err := repo.CreatePasswordResetToken(ctx, "", "raw", time.Hour); err == nil {
		t.Error("expected error for empty userID, got nil")
	}
	if _, err := repo.CreatePasswordResetToken(ctx, user.ID, "", time.Hour); err == nil {
		t.Error("expected error for empty rawToken, got nil")
	}
	if _, err := repo.CreatePasswordResetToken(ctx, user.ID, "raw", 0); err == nil {
		t.Error("expected error for zero ttl, got nil")
	}
	if _, err := repo.CreatePasswordResetToken(ctx, user.ID, "raw", -time.Second); err == nil {
		t.Error("expected error for negative ttl, got nil")
	}
}

func TestAuthTokenRepository_ConsumeEmptyRawToken(t *testing.T) {
	// An empty raw token is the caller's way of saying "I have no
	// token" — never a real reset attempt. Return
	// ErrAuthTokenInvalid instead of a database error so the
	// controller's path is the same as for any other invalid
	// token.
	repo, _, _, database := authTokenTestHarness(t)
	defer database.Close()

	_, err := repo.ConsumePasswordResetToken(context.Background(), "")
	if !errors.Is(err, ErrAuthTokenInvalid) {
		t.Fatalf("err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthTokenRepository_DistinctRawTokensDoNotCollide(t *testing.T) {
	// Two distinct raw tokens must produce two distinct hashes and
	// consume independently. Guards against a future refactor that
	// accidentally short-circuits the hash step.
	repo, _, user, database := authTokenTestHarness(t)
	defer database.Close()

	ctx := context.Background()
	if _, err := repo.CreatePasswordResetToken(ctx, user.ID, "alpha", time.Hour); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := repo.CreatePasswordResetToken(ctx, user.ID, "bravo", time.Hour); err != nil {
		t.Fatalf("Create bravo: %v", err)
	}

	if got, err := repo.ConsumePasswordResetToken(ctx, "alpha"); err != nil {
		t.Fatalf("consume alpha: %v", err)
	} else if got != user.ID {
		t.Fatalf("alpha userID = %q, want %q", got, user.ID)
	}
	if got, err := repo.ConsumePasswordResetToken(ctx, "bravo"); err != nil {
		t.Fatalf("consume bravo: %v", err)
	} else if got != user.ID {
		t.Fatalf("bravo userID = %q, want %q", got, user.ID)
	}
}

func TestHashToken_DeterministicAndHex(t *testing.T) {
	// The hash must be deterministic (so a future migration can
	// safely rebuild the index) and hex-encoded (so the existing
	// TEXT column is the right shape).
	if a, b := hashToken("x"), hashToken("x"); a != b {
		t.Errorf("hashToken not deterministic: %q vs %q", a, b)
	}
	if hashToken("x") == hashToken("y") {
		t.Error("distinct inputs produced the same hash")
	}
	if got := hashToken("anything"); len(got) != 64 {
		t.Errorf("len(hash) = %d, want 64 (sha256 hex)", len(got))
	}
}
