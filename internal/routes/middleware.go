// Package routes provides HTTP route handlers for the strength tracker application.
// This file contains Echo middleware for authentication.
package routes

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"hylete/internal/models"
	"hylete/internal/utils"
)

// authContextKey is used to store auth claims in the Echo context.
const authContextKey = "auth_claims"

// userContextKey is used to cache the authenticated user on the Echo
// context so multiple handlers in the same request only hit the DB once.
const userContextKey = "auth_user"

// apiV1Prefix is the URL prefix for JSON API routes. The auth
// middleware uses it to decide between an HTML redirect (for the
// web app) and a JSON 401 (for API clients) when a request lacks
// a valid token.
const apiV1Prefix = "/api/v1/"

// bearerPrefix is the scheme prefix for the Authorization header
// value the API clients send. The standard HTTP form is
// "Authorization: Bearer <token>".
const bearerPrefix = "Bearer "

// AuthMiddleware returns an Echo middleware function that verifies
// the auth token and injects the user claims into the request
// context. The token is read from the "Authorization: Bearer ..."
// header (preferred, used by the iOS client and any other API
// consumer) and falls back to the auth cookie (used by the web
// app). If the token is missing or invalid the response is either
// a redirect to /login (web app) or a JSON 401 (API clients), so
// the iOS app can surface a clean error instead of chasing a 302.
func AuthMiddleware(jwtService *utils.JWTService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip auth for public routes
			path := c.Request().URL.Path
			if isPublicRoute(path) {
				return next(c)
			}

			token, err := authTokenFromRequest(c)
			if err != nil || token == "" {
				return unauthorized(c)
			}

			claims, err := jwtService.VerifyToken(token)
			if err != nil {
				return unauthorized(c)
			}

			c.Set(authContextKey, claims)
			return next(c)
		}
	}
}

// authTokenFromRequest returns the JWT for the request, preferring
// the "Authorization: Bearer <token>" header set by API clients and
// falling back to the auth cookie used by the web app. Returns an
// empty string (no error) when no token is present, so callers can
// treat the missing- and invalid-token cases the same way.
func authTokenFromRequest(c echo.Context) (string, error) {
	if h := c.Request().Header.Get("Authorization"); strings.HasPrefix(h, bearerPrefix) {
		return strings.TrimSpace(h[len(bearerPrefix):]), nil
	}
	cookie, err := c.Cookie(utils.CookieName)
	if err != nil {
		return "", nil
	}
	return cookie.Value, nil
}

// unauthorized returns the right 401 shape for the request. API
// clients get a JSON body so the iOS app can surface a clean
// error; the web app gets a redirect to /login (or an htmx
// redirect for in-page htmx calls) so the user lands on the
// login form.
func unauthorized(c echo.Context) error {
	if isAPIPath(c.Request().URL.Path) {
		return c.JSON(http.StatusUnauthorized, APIError{Error: "unauthorized"})
	}
	return redirectToLogin(c)
}

// isAPIPath reports whether the given request path targets the
// JSON API namespace. The auth middleware uses this to decide
// between a JSON 401 and a redirect-to-login response.
func isAPIPath(path string) bool {
	return strings.HasPrefix(path, apiV1Prefix)
}

// publicFS is the filesystem backing the catch-all static handler
// (e.Static("/", "public") in RegisterRoutes). isPublicRoute uses it
// to recognise asset requests — anything that resolves to a real file
// inside the directory is treated as public.
var publicFS = http.Dir("public")

// isPublicRoute returns true for routes that don't require authentication.
//
// Two kinds of paths are public:
//
//   - Named routes: "/" (the marketing landing page; the Home handler
//     redirects anyone carrying a valid session cookie to the dashboard),
//     the auth pages (/login, /register, /forgot, /reset) and their
//     JSON API mirrors. /forgot and /reset must be reachable logged-out
//     so a user can still request a reset link and follow it; the
//     /reset POST handler additionally checks the token before mutating
//     anything, so this is not a security hole. The /api/v1/auth/logout
//     endpoint is stateless (the client just discards its token).
//   - Static assets: any path resolving to a file inside public/ (css,
//     icons, img, js, manifest.json, favicon, …). These are loaded by
//     pages anonymous visitors see — e.g. the auth pages' full-bleed
//     photos live under /img/ and /js/basecoat.js backs every layout
//     page — and exempting the whole directory means new assets work
//     without middleware changes. Directory paths themselves don't
//     count, so this never opens up an HTML route.
func isPublicRoute(path string) bool {
	// Exact match only: "/" is also the prefix of every path, so it
	// must not go through the prefix list below — that would make
	// every route on the site public.
	if path == "/" {
		return true
	}
	public := []string{
		"/login",
		"/register",
		"/forgot",
		"/reset",
		"/showcase",
		"/api/v1/auth/login",
		"/api/v1/auth/register",
		"/api/v1/auth/password-reset/request",
		"/api/v1/auth/logout",
	}
	for _, p := range public {
		if len(path) >= len(p) && path[:len(p)] == p {
			return true
		}
	}
	return isPublicAsset(path)
}

// isPublicAsset reports whether the URL path maps to a regular file
// inside the public/ static directory. http.Dir sanitises the path
// before opening, so traversal attempts ("../../secrets") resolve
// outside the directory and fail the lookup. Only files count —
// directories are rejected so "/" and folder listings stay gated.
func isPublicAsset(path string) bool {
	f, err := publicFS.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return !stat.IsDir()
}

func redirectToLogin(c echo.Context) error {
	// For htmx requests, return a client-side redirect instruction
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.NoContent(http.StatusUnauthorized)
	}
	return c.Redirect(http.StatusSeeOther, "/login")
}

// GetClaims retrieves the authenticated user's claims from the Echo context.
// Returns nil if no claims are present (should not happen on protected routes).
func GetClaims(c echo.Context) *utils.Claims {
	claims, ok := c.Get(authContextKey).(*utils.Claims)
	if !ok {
		return nil
	}
	return claims
}

// GetUser returns the authenticated user for the current request, loading
// it from the user repo on first access and caching the result on the
// Echo context. Subsequent calls in the same request are an in-memory
// lookup, so multiple handlers that need the user (or different fields
// of the user) only trigger one DB read.
//
// Returns nil when there are no claims (e.g. on a public route) or the
// DB read fails. Callers are expected to handle a nil user — typically
// by falling back to safe defaults (e.g. "kg" for the weight unit) or
// hiding optional widgets (e.g. the target-weight progress card).
func (h *Handler) GetUser(c echo.Context) *models.User {
	if cached, ok := c.Get(userContextKey).(*models.User); ok {
		return cached
	}
	claims := GetClaims(c)
	if claims == nil {
		return nil
	}
	user, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		c.Logger().Errorf("GetUser: %v", err)
		return nil
	}
	if user != nil {
		c.Set(userContextKey, user)
	}
	return user
}

// AdminMiddleware returns an Echo middleware function that restricts access
// to admin users only. It must be used after AuthMiddleware.
func AdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := GetClaims(c)
			if claims == nil || !claims.IsAdmin {
				return echo.NewHTTPError(http.StatusForbidden, "Admin access required")
			}
			return next(c)
		}
	}
}
