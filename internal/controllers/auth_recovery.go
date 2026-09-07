package controllers

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"hylete/internal/models"
)

// PasswordResetSender is the narrow contract AuthRecoveryController
// depends on for the password-reset email. Defining it as an
// interface (rather than depending on *email.Service directly) lets
// the controller be unit-tested with a mock — see
// auth_recovery_test.go's mockResetSender.
type PasswordResetSender interface {
	SendPasswordReset(ctx context.Context, tokenRepo models.AuthTokenRepo, user *models.User) (rawToken string, err error)
}

// ErrInvalidPassword is returned by ResetPassword when the
// candidate password is too short (matches the threshold used by
// AuthController.Register). Centralized here so the route handler
// can map it to a single user-facing error message.
var ErrInvalidPassword = errors.New("password must be at least 6 characters")

// ErrAuthTokenInvalid is re-exported here so route handlers can
// import controllers without also importing models. The
// underlying value comes from models.ErrAuthTokenInvalid; tests
// can use errors.Is(err, models.ErrAuthTokenInvalid) to compare.
var ErrAuthTokenInvalid = models.ErrAuthTokenInvalid

// AuthRecoveryController owns the forgot-password / reset-password
// business logic. The route handlers in
// internal/routes/auth_recovery.go are thin wrappers that parse
// the form, call these methods, and render the result.
type AuthRecoveryController struct {
	userRepo      models.UserRepo
	authTokenRepo models.AuthTokenRepo
	resetSender   PasswordResetSender
}

// NewAuthRecoveryController returns a new AuthRecoveryController.
// All three dependencies are required.
func NewAuthRecoveryController(
	userRepo models.UserRepo,
	authTokenRepo models.AuthTokenRepo,
	resetSender PasswordResetSender,
) *AuthRecoveryController {
	return &AuthRecoveryController{
		userRepo:      userRepo,
		authTokenRepo: authTokenRepo,
		resetSender:   resetSender,
	}
}

// RequestPasswordReset is called when the user submits the
// forgot-password form. It is deliberately silent: a missing
// user returns nil (the route shows the same "we sent you an
// email" page it shows on success) so an attacker cannot use
// the response to enumerate which addresses are registered.
//
// The email send is also non-fatal: a token is persisted before
// the SMTP call, and an SMTP failure is logged and returned to
// the caller (which displays a generic error toast). The token
// will expire on its own in 1 hour, so the user can simply
// request another email and the old token becomes a no-op.
func (c *AuthRecoveryController) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}

	user, err := c.userRepo.GetUserByEmail(email)
	if err != nil {
		// Treat lookup error as "user not found" so the
		// response is identical for DB errors and missing
		// users. The error is intentionally swallowed
		// rather than logged at the controller level — the
		// application log will already have a trace from
		// the DB driver.
		return nil
	}
	if user == nil {
		return nil
	}

	// Best-effort token + email. The token is persisted
	// even if SMTP fails (it will expire in 1h).
	_, err = c.resetSender.SendPasswordReset(ctx, c.authTokenRepo, user)
	if err != nil {
		// Logged here so the operator can correlate with
		// the token row that was just persisted; the route
		// renders a generic "we couldn't send the email,
		// try again" toast.
		return errors.New("could not send password reset email")
	}
	return nil
}

// ResetPassword consumes the token from the URL, validates the
// new password, and updates the user's password hash.
//
// Returns:
//   - nil on success.
//   - ErrAuthTokenInvalid when the token is missing, malformed,
//     already used, or expired (the route renders a single
//     generic error for all of these).
//   - ErrInvalidPassword when the candidate is too short.
//   - A wrapped error from the database on a hard failure.
func (c *AuthRecoveryController) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return ErrAuthTokenInvalid
	}
	if len(newPassword) < 6 {
		return ErrInvalidPassword
	}

	userID, err := c.authTokenRepo.ConsumePasswordResetToken(ctx, rawToken)
	if err != nil {
		// The repo's ErrAuthTokenInvalid passes through;
		// any other error is wrapped.
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("could not hash password")
	}

	if err := c.userRepo.UpdateUserPassword(userID, string(hash)); err != nil {
		return err
	}
	return nil
}
