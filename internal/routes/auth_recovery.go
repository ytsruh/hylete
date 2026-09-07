package routes

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/views/auth"
)

// ForgotPasswordForm renders the GET /forgot page. Always
// renders the empty form; the POST handler re-renders the same
// page with the "submitted" state.
func (h *Handler) ForgotPasswordForm(c echo.Context) error {
	return render(c, auth.ForgotPasswordPage("", false))
}

// RequestPasswordReset handles POST /forgot. Validates the
// email field, calls the controller, and renders the same
// page with a "we sent you a link" message on success or an
// error toast on validation failure.
//
// Note: the controller returns nil for both "user exists" and
// "user does not exist" so an attacker cannot enumerate
// accounts from the response. The only paths that return an
// error to the user are (a) SMTP outage and (b) a bad email
// format.
func (h *Handler) RequestPasswordReset(c echo.Context) error {
	email := c.FormValue("email")
	if email == "" {
		return render(c, auth.ForgotPasswordPage("Email is required", false))
	}

	err := h.authRecoveryCtrl.RequestPasswordReset(c.Request().Context(), email)
	if err != nil {
		return render(c, auth.ForgotPasswordPage(err.Error(), false))
	}

	// HTMX response: trigger a full re-render of the form
	// page (in "submitted" state) so the user sees the
	// confirmation copy. The trick is that the hx-post
	// response is swapped into #toaster; we instead want to
	// replace the form. Use HX-Trigger or a redirect for
	// non-HTMX clients. For simplicity, just return the
	// "submitted" page as the entire body when not an HTMX
	// request, and a HX-Redirect when HTMX.
	if c.Request().Header.Get("HX-Request") == "true" {
		// HTMX path: trigger a client-side redirect to
		// the same /forgot URL so the full page re-renders
		// in "submitted" state.
		c.Response().Header().Set("HX-Redirect", "/forgot?submitted=1")
		return c.NoContent(http.StatusNoContent)
	}
	return render(c, auth.ForgotPasswordPage("", true))
}

// ResetPasswordForm handles GET /reset?token=... . The token
// is the raw token from the email link; we do not validate it
// here (a user with a stale link clicking the button will get
// the same "invalid" error from the POST path).
func (h *Handler) ResetPasswordForm(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		// No token in the URL means the user navigated to
		// /reset directly. Redirect to the forgot page so
		// they can request a link.
		return c.Redirect(http.StatusSeeOther, "/forgot")
	}
	return render(c, auth.ResetPasswordPage(token, ""))
}

// ResetPassword handles POST /reset. Validates the form, calls
// the controller, and on success emits a toast + client-side
// redirect to /login (via HX-Redirect for HTMX, full redirect
// for non-HTMX).
func (h *Handler) ResetPassword(c echo.Context) error {
	token := c.FormValue("token")
	password := c.FormValue("password")
	confirm := c.FormValue("confirm")

	if token == "" {
		return c.Redirect(http.StatusSeeOther, "/forgot")
	}
	if password == "" {
		return render(c, auth.ResetPasswordPage(token, "Password is required"))
	}
	if password != confirm {
		return render(c, auth.ResetPasswordPage(token, "Passwords do not match"))
	}

	err := h.authRecoveryCtrl.ResetPassword(c.Request().Context(), token, password)
	switch {
	case err == nil:
		if c.Request().Header.Get("HX-Request") == "true" {
			c.Response().Header().Set("HX-Redirect", "/login")
			return c.NoContent(http.StatusNoContent)
		}
		return c.Redirect(http.StatusSeeOther, "/login")
	case errors.Is(err, models.ErrAuthTokenInvalid), errors.Is(err, controllers.ErrAuthTokenInvalid):
		// The token is missing, used, or expired. The user
		// can request a new one from /forgot. Render the
		// page with a generic error so the form remains
		// accessible (with an empty token hidden field,
		// the next submit will redirect them via the empty
		// token branch above).
		return render(c, auth.ResetPasswordPage("", "This reset link is invalid or has expired. Please request a new one."))
	case errors.Is(err, controllers.ErrInvalidPassword):
		return render(c, auth.ResetPasswordPage(token, err.Error()))
	default:
		return render(c, auth.ResetPasswordPage(token, "Could not reset password. Please try again."))
	}
}
