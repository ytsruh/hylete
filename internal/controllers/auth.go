// Package controllers provides business logic for the strength tracker application.
// Controllers orchestrate between the HTTP routes layer and the data models,
// containing all domain logic that is independent of the web framework.
package controllers

import (
	"context"
	"errors"
	"log"

	"golang.org/x/crypto/bcrypt"

	"hylete/internal/models"
	"hylete/internal/utils"
)

// WelcomeSender is the narrow contract AuthController depends on
// for the fire-and-forget welcome email sent right after a user
// registers. Defining it as an interface (rather than depending
// on *email.Service directly) lets the controller be unit-tested
// with a mock — see auth_test.go's mockWelcomeSender.
type WelcomeSender interface {
	SendWelcome(ctx context.Context, user *models.User) error
}

// AuthController handles user authentication business logic.
type AuthController struct {
	userRepo      models.UserRepo
	jwtService    *utils.JWTService
	welcomeSender WelcomeSender
}

// NewAuthController creates a new AuthController instance. The
// welcomeSender is the email service used to send the
// "welcome to Hylete" message after a successful Register. The
// controller is designed to be tolerant of a nil welcomeSender
// (so existing tests that pre-date the email feature keep
// working) — Register skips the send when the sender is nil.
func NewAuthController(userRepo models.UserRepo, jwtService *utils.JWTService, welcomeSender WelcomeSender) *AuthController {
	return &AuthController{
		userRepo:      userRepo,
		jwtService:    jwtService,
		welcomeSender: welcomeSender,
	}
}

// Login validates credentials and returns the authenticated user and JWT token.
// Returns ErrInvalidCredentials if the email or password is incorrect.
func (ac *AuthController) Login(email, password string) (*models.User, string, error) {
	if email == "" || password == "" {
		return nil, "", ErrInvalidCredentials
	}

	user, err := ac.userRepo.GetUserByEmail(email)
	if err != nil {
		return nil, "", ErrInvalidCredentials
	}
	if user == nil {
		return nil, "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := ac.jwtService.GenerateToken(user.ID, user.Email, user.Name, user.IsAdmin)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

// Register creates a new user account and returns the user with a JWT token.
// Returns ErrEmailExists if the email is already registered.
func (ac *AuthController) Register(name, email, password string) (*models.User, string, error) {
	if name == "" || email == "" || password == "" {
		return nil, "", errors.New("all fields are required")
	}

	if len(password) < 6 {
		return nil, "", errors.New("password must be at least 6 characters")
	}

	existing, err := ac.userRepo.GetUserByEmail(email)
	if err != nil {
		return nil, "", errors.New("something went wrong")
	}
	if existing != nil {
		return nil, "", ErrEmailExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", errors.New("something went wrong")
	}

	user := &models.User{
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
	}
	if err := ac.userRepo.CreateUser(user); err != nil {
		return nil, "", errors.New("failed to create account")
	}

	token, err := ac.jwtService.GenerateToken(user.ID, user.Email, user.Name, user.IsAdmin)
	if err != nil {
		return nil, "", errors.New("failed to create session")
	}

	// Fire-and-forget welcome email. A failure must NOT fail
	// the registration: the user has an account, a session,
	// and is already authenticated. The goroutine has a
	// recover so a panic in the email path (e.g. nil user
	// dereference after a future refactor) does not crash
	// the process. The context is a fresh background one
	// because the request context will be cancelled the
	// moment the HTTP response is written, well before the
	// SMTP conversation completes.
	if ac.welcomeSender != nil {
		go func(u *models.User) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("auth: welcome email panicked: %v", r)
				}
			}()
			if err := ac.welcomeSender.SendWelcome(context.Background(), u); err != nil {
				log.Printf("auth: welcome email failed for %s: %v", u.Email, err)
			}
		}(user)
	}

	return user, token, nil
}
