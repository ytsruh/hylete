// Package routes provides HTTP route handlers for the strength
// tracker application. This file contains the public marketing
// landing page served at the root path.
package routes

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/views"
)

// Home renders the public marketing landing page at "/".
//
// "/" is in the auth middleware's public-route list, so claims are
// never populated here — even when the visitor carries a valid
// session cookie. To keep signed-in users out of the sales pitch,
// the handler runs a best-effort token check itself and redirects
// anyone with a valid cookie straight to the dashboard; everyone
// else gets the landing page with Login/Register calls to action.
func (h *Handler) Home(c echo.Context) error {
	if token, err := authTokenFromRequest(c); err == nil && token != "" {
		if _, err := h.jwtService.VerifyToken(token); err == nil {
			return c.Redirect(http.StatusSeeOther, dashboardPath)
		}
	}
	return render(c, views.Landing())
}
