// Package routes provides HTTP route handlers for the strength tracker application.
package routes

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/views/auth"
)

// --- Auth Handlers ---

// LoginForm renders the login page.
func (h *Handler) LoginForm(c echo.Context) error {
	return render(c, auth.LoginPage(""))
}

// loginInput represents the validated login form data.
type loginInput struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required"`
}

// Login handles login form submission.
func (h *Handler) Login(c echo.Context) error {
	input := loginInput{
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
	}

	if err := h.validator.ValidateStruct(&input); err != nil {
		return render(c, auth.LoginPageError(err.Error()))
	}

	_, token, err := h.authCtrl.Login(input.Email, input.Password)
	if err != nil {
		if errors.Is(err, controllers.ErrInvalidCredentials) {
			return render(c, auth.LoginPageError("Invalid email or password"))
		}
		return render(c, auth.LoginPageError("Failed to create session"))
	}

	setAuthCookie(c, token)

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", `{"triggerRedirect": "`+dashboardPath+`"}`)
		return render(c, auth.LoginSuccessToast())
	}
	return c.Redirect(http.StatusSeeOther, dashboardPath)
}

// RegisterForm renders the registration page.
func (h *Handler) RegisterForm(c echo.Context) error {
	return render(c, auth.RegisterPage(""))
}

// registerInput represents the validated registration form data.
type registerInput struct {
	Name     string `validate:"required,min=1,max=100"`
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=6"`
}

// Register handles registration form submission.
func (h *Handler) Register(c echo.Context) error {
	input := registerInput{
		Name:     c.FormValue("name"),
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
	}

	if err := h.validator.ValidateStruct(&input); err != nil {
		return render(c, auth.RegisterPageError(err.Error()))
	}

	_, token, err := h.authCtrl.Register(input.Name, input.Email, input.Password)
	if err != nil {
		return render(c, auth.RegisterPageError(err.Error()))
	}

	setAuthCookie(c, token)

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", `{"triggerRedirect": "`+dashboardPath+`"}`)
		return render(c, auth.RegisterSuccessToast())
	}
	return c.Redirect(http.StatusSeeOther, dashboardPath)
}

// Logout clears the auth cookie and redirects to login.
func (h *Handler) Logout(c echo.Context) error {
	clearAuthCookie(c)

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", `{"triggerRedirect": "/login"}`)
		return render(c, auth.LogoutSuccessToast())
	}
	return c.Redirect(http.StatusSeeOther, "/login")
}
