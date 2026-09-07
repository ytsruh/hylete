package controllers

import (
	"context"
	"errors"
	"fmt"

	"hylete/internal/models"
)

// ErrUserNotFound is returned by the admin user actions when the
// target user does not exist — e.g. the row was deleted between the
// admin list page rendering and the action request landing.
var ErrUserNotFound = errors.New("user not found")

// ErrCannotDemoteSelf is returned by SetAdmin when an admin tries to
// revoke their own admin status. is_admin is the only gate to the
// /admin namespace, so a single click could otherwise lock every
// admin out of user management.
var ErrCannotDemoteSelf = errors.New("you cannot remove your own admin access")

// AdminUserController handles admin-only user operations.
type AdminUserController struct {
	repo          models.AdminUserRepo
	authTokenRepo models.AuthTokenRepo
	resetSender   PasswordResetSender
}

// NewAdminUserController creates a new AdminUserController instance.
// All three dependencies are required: repo covers the admin-scope
// user reads and the admin-flag write, authTokenRepo persists the
// password-reset token, and resetSender delivers the reset email
// (email.Service satisfies the PasswordResetSender contract).
func NewAdminUserController(repo models.AdminUserRepo, authTokenRepo models.AuthTokenRepo, resetSender PasswordResetSender) *AdminUserController {
	return &AdminUserController{
		repo:          repo,
		authTokenRepo: authTokenRepo,
		resetSender:   resetSender,
	}
}

// ListUsers returns all users ordered by creation date (newest first).
func (uc *AdminUserController) ListUsers(ctx context.Context) ([]models.User, error) {
	return uc.repo.ListUsers(ctx)
}

// SetAdmin grants or revokes a user's admin status. actingUserID is
// the admin performing the action and targetUserID the user being
// changed; when they match and the target state is "not admin" the
// call is rejected with ErrCannotDemoteSelf so the acting admin
// cannot lock themselves out of /admin. Returns ErrUserNotFound when
// the target user does not exist.
func (uc *AdminUserController) SetAdmin(ctx context.Context, actingUserID, targetUserID string, isAdmin bool) error {
	if actingUserID == targetUserID && !isAdmin {
		return ErrCannotDemoteSelf
	}

	user, err := uc.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	return uc.repo.SetUserAdmin(ctx, targetUserID, isAdmin)
}

// SendPasswordReset triggers the same password-reset email the
// user-facing forgot-password flow sends: a one-hour token is
// persisted (sha256 hash only) and the reset link is emailed to the
// user's address. Returns ErrUserNotFound when the target user does
// not exist. The raw token is discarded here — like the self-service
// flow, it only ever travels over SMTP.
func (uc *AdminUserController) SendPasswordReset(ctx context.Context, userID string) error {
	user, err := uc.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	if _, err := uc.resetSender.SendPasswordReset(ctx, uc.authTokenRepo, user); err != nil {
		return fmt.Errorf("could not send password reset email: %w", err)
	}
	return nil
}
