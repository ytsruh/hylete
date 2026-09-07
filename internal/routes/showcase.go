// Package routes provides HTTP route handlers for the strength
// tracker application. This file contains the public bento-grid
// showcase page served at /showcase.
package routes

import (
	"github.com/labstack/echo/v4"

	"hylete/internal/views"
)

// Showcase renders the public bento-grid showcase page at "/showcase".
//
// "/showcase" is in the auth middleware's public-route list, so it is
// reachable without a session — and unlike "/" it does not redirect
// authenticated users away. Both anonymous and signed-in visitors get
// the same page, so no token check is needed here.
func (h *Handler) Showcase(c echo.Context) error {
	return render(c, views.Showcase())
}
