// Package routes provides HTTP route handlers for the strength tracker application.
// This package wraps the Echo framework and contains only HTTP-level concerns:
// request parsing, response rendering, redirects, and middleware.
// All business logic lives in the controllers package.
package routes

import (
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/utils"
)

// Handler holds dependencies for HTTP route handlers.
type Handler struct {
	authCtrl          *controllers.AuthController
	authRecoveryCtrl  *controllers.AuthRecoveryController
	exerciseEntryCtrl *controllers.ExerciseEntryController
	adminCtrl         *controllers.AdminController
	adminUserCtrl     *controllers.AdminUserController
	feedbackCtrl      *controllers.FeedbackController
	weightCtrl        *controllers.WeightController
	goalsCtrl         *controllers.GoalsController
	userRepo          models.UserRepo
	jwtService        *utils.JWTService
	validator         utils.Validator
	// clock is the time source the profile route uses
	// when computing the next reminder fire time on form
	// save. Tests substitute a fixed clock to assert on
	// the exact value written to next_fire_at; production
	// uses a wall clock.
	clock Clock
	// imageProcessor and imageUploader are the dependencies for
	// the admin exercise image upload route. Both are interfaces
	// (defined in admin_images.go) so tests can substitute fakes
	// without touching the real S3 client or imaging package.
	imageProcessor exerciseImageProcessor
	imageUploader  exerciseImageUploader
	// imageConfig controls the two variants produced per upload.
	imageConfig ExerciseImageConfig
}

// Clock is the time source the profile route uses to stamp
// the next reminder fire. RealClock is the production default;
// tests inject a fixed clock to keep assertions deterministic.
type Clock interface {
	Now() time.Time
}

// RealClock returns the wall clock.
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// NewHandler creates a new route handler instance.
func NewHandler(authCtrl *controllers.AuthController, authRecoveryCtrl *controllers.AuthRecoveryController, exerciseEntryCtrl *controllers.ExerciseEntryController, adminCtrl *controllers.AdminController, adminUserCtrl *controllers.AdminUserController, feedbackCtrl *controllers.FeedbackController, weightCtrl *controllers.WeightController, goalsCtrl *controllers.GoalsController, userRepo models.UserRepo, jwtService *utils.JWTService, validator utils.Validator, imageProcessor exerciseImageProcessor, imageUploader exerciseImageUploader, imageConfig ExerciseImageConfig) *Handler {
	return &Handler{
		authCtrl:          authCtrl,
		authRecoveryCtrl:  authRecoveryCtrl,
		exerciseEntryCtrl: exerciseEntryCtrl,
		adminCtrl:         adminCtrl,
		adminUserCtrl:     adminUserCtrl,
		feedbackCtrl:      feedbackCtrl,
		weightCtrl:        weightCtrl,
		goalsCtrl:         goalsCtrl,
		userRepo:          userRepo,
		jwtService:        jwtService,
		validator:         validator,
		clock:             RealClock{},
		imageProcessor:    imageProcessor,
		imageUploader:     imageUploader,
		imageConfig:       imageConfig,
	}
}

// dashboardPath is the URL of the signed-in web app's landing
// page (the dashboard). The root "/" serves the public marketing
// page instead, so every "go to your app" redirect — login,
// registration, feedback submissions — points here.
const dashboardPath = "/dashboard"

// RegisterRoutes registers all routes with the Echo instance.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.Gzip())
	e.Use(AuthMiddleware(h.jwtService))

	// Serve manifest with correct MIME type for PWA compatibility
	e.GET("/manifest.json", h.ServeManifest)

	// Auth routes (public, handled by auth middleware skip list)
	e.GET("/login", h.LoginForm)
	e.POST("/login", h.Login)
	e.GET("/register", h.RegisterForm)
	e.POST("/register", h.Register)
	e.POST("/logout", h.Logout)

	// Password recovery
	e.GET("/forgot", h.ForgotPasswordForm)
	e.POST("/forgot", h.RequestPasswordReset)
	e.GET("/reset", h.ResetPasswordForm)
	e.POST("/reset", h.ResetPassword)

	// Routes
	//
	// The root serves the public marketing landing page; the
	// signed-in web app lives under /dashboard. dashboardPath is
	// the single source of truth for that URL across handlers.
	e.GET("/", h.Home)
	e.GET("/showcase", h.Showcase)
	e.GET("/dashboard", h.Dashboard)

	// User profile
	e.GET("/profile", h.Profile)
	e.POST("/profile", h.UpdateProfile)

	// Feedback (user-facing form; submissions land in the admin
	// feedback inbox). The dashboard's iOS banner links here.
	e.GET("/feedback", h.FeedbackForm)
	e.POST("/feedback", h.SubmitFeedback)

	// Data Export page and its bulk data-portability downloads
	// (weight ZIP incl. photos; exercise entries ZIP), all grouped
	// under the /export prefix.
	e.GET("/export", h.DataExportPage)
	e.GET("/export/weight", h.ExportWeightZip)
	e.GET("/export/exercises", h.ExportExerciseEntriesZip)

	// Exercise history (read-only on the web — set logging lives in
	// the iOS client via /api/v1/exercise-entries)
	e.GET("/exercises", h.ListExercisesUI)
	e.GET("/exercises/:id", h.ExerciseHistory)

	// Exercise chart views
	e.GET("/exercises/:id/chart", h.ExerciseChart)
	e.GET("/exercises/:id/chart/advanced", h.ExerciseChartAdvanced)

	// Admin routes
	admin := e.Group("/admin", AdminMiddleware())
	admin.GET("/exercises", h.AdminListExercises)
	admin.GET("/exercises/new", h.AdminNewExerciseForm)
	admin.POST("/exercises", h.AdminCreateExercise)
	admin.GET("/exercises/:id/edit", h.AdminEditExerciseForm)
	admin.POST("/exercises/:id", h.AdminUpdateExercise)
	// Admin image upload (multipart, 10 MB cap applied inside the
	// handler via http.MaxBytesReader so a hostile client can't
	// exhaust memory before the multipart parser runs).
	admin.POST("/exercises/image-upload", h.AdminExerciseImageUpload)
	admin.GET("/feedback", h.AdminListFeedback)
	admin.GET("/feedback/:id", h.AdminFeedbackDetail)
	admin.POST("/feedback/:id/close", h.AdminCloseFeedback)
	admin.GET("/users", h.AdminListUsers)
	admin.POST("/users/:id/admin", h.AdminSetUserAdmin)
	admin.POST("/users/:id/send-password-reset", h.AdminSendUserPasswordReset)

	// JSON API namespace for the iOS client and any other
	// native client. The auth middleware (registered above
	// via e.Use) already accepts the "Authorization: Bearer
	// <token>" header and returns a JSON 401 for /api/v1
	// paths, so no group-level middleware is needed here.
	registerAPIRoutes(e, h)

	// Static files (PWA assets, icons, etc.) - registered last as catch-all
	e.Static("/", "public")
}

// registerAPIRoutes wires every /api/v1/* handler onto the
// given Echo instance. Kept in its own function (and file,
// in api_v1.go for the handler bodies) so the HTML route
// table above stays focused on the web app's surface.
func registerAPIRoutes(e *echo.Echo, h *Handler) {
	// Auth (login & register are public — see isPublicRoute)
	e.POST("/api/v1/auth/login", h.APILogin)
	e.POST("/api/v1/auth/register", h.APIRegister)
	e.POST("/api/v1/auth/password-reset/request", h.APIRequestPasswordReset)
	e.POST("/api/v1/auth/logout", h.APILogout)

	// Current user
	e.GET("/api/v1/me", h.APIMe)
	e.PUT("/api/v1/me", h.APIUpdateMe)

	// Exercises
	e.GET("/api/v1/exercises", h.APIListExercises)

	// Exercise entries (sets)
	e.GET("/api/v1/exercise-entries", h.APIListExerciseEntries)
	e.POST("/api/v1/exercise-entries", h.APICreateExerciseEntries)
	e.GET("/api/v1/exercise-entries/:id", h.APIGetExerciseEntry)
	e.PUT("/api/v1/exercise-entries/:id", h.APIUpdateExerciseEntry)
	e.DELETE("/api/v1/exercise-entries/:id", h.APIDeleteExerciseEntry)

	// Per-exercise history & chart data
	e.GET("/api/v1/exercises/:id/history", h.APIGetExerciseHistory)
	e.GET("/api/v1/exercises/:id/chart", h.APIGetExerciseChartData)

	// Goals (JSON mirror of the HTML goals routes — used by
	// the iOS client). All handlers are JWT-protected via the
	// /api/v1 prefix group in middleware.go.
	e.GET("/api/v1/goals", h.APIListGoals)
	e.POST("/api/v1/goals", h.APICreateGoal)
	e.GET("/api/v1/goals/:id", h.APIGetGoal)
	e.PUT("/api/v1/goals/:id", h.APIUpdateGoal)
	e.POST("/api/v1/goals/:id/complete", h.APIMarkGoalComplete)
	e.POST("/api/v1/goals/:id/reopen", h.APIReopenGoal)
	e.DELETE("/api/v1/goals/:id", h.APIDeleteGoal)

	// Feedback (JSON mirror of the HTML /feedback POST
	// handler — used by the iOS client). The same
	// FeedbackController.Submit validates the body, so the
	// iOS and web surfaces enforce the same length rules.
	e.POST("/api/v1/feedback", h.APISubmitFeedback)

	// Weight entries (JSON mirror of the HTML /weight/* routes
	// — used by the iOS client). The CRUD routes delegate to
	// the same controller methods the HTML handlers use, so
	// validation and the photo-removal semantics stay aligned
	// across both surfaces. The photo-upload endpoint is the
	// thin JWT-protected wrapper around the existing
	// /api/weight/photo-upload handler.
	e.GET("/api/v1/weight", h.APIListWeightEntries)
	e.POST("/api/v1/weight", h.APICreateWeightEntry)
	e.GET("/api/v1/weight/compare", h.APICompareWeight)
	e.GET("/api/v1/weight/:id", h.APIGetWeightEntry)
	e.PUT("/api/v1/weight/:id", h.APIUpdateWeightEntry)
	e.DELETE("/api/v1/weight/:id", h.APIDeleteWeightEntry)
	e.POST("/api/v1/weight/photo-upload", h.APIRequestPhotoUploadURL)
}

// ServeManifest serves the web app manifest with the correct MIME type.
// Browsers expect application/manifest+json for manifest files.
func (h *Handler) ServeManifest(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "application/manifest+json")
	return c.File("public/manifest.json")
}

// Helper functions

func render(c echo.Context, component templ.Component) error {
	return component.Render(c.Request().Context(), c.Response())
}

func setAuthCookie(c echo.Context, token string) {
	cookie := &http.Cookie{
		Name:     utils.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(utils.JWTTTL.Seconds()), // matches JWT exp claim
	}
	c.SetCookie(cookie)
}

func clearAuthCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     utils.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	c.SetCookie(cookie)
}

// friendlyValidationError converts validation error messages to user-friendly strings.
func friendlyValidationError(err error) string {
	return friendlyError(err)
}

// friendlyError extracts the message from Echo HTTP errors and makes validation errors user-friendly.
func friendlyError(err error) string {
	msg := err.Error()
	// Strip "code=XXX, message=" prefix if present
	if strings.HasPrefix(msg, "code=") {
		if idx := strings.Index(msg, ", message="); idx != -1 {
			msg = msg[idx+len(", message="):]
		}
	}
	if strings.Contains(msg, "Reps") && strings.Contains(msg, "lte") {
		return "Reps must be 1000 or less"
	}
	if strings.Contains(msg, "Weight") && strings.Contains(msg, "lte") {
		return "Weight must be 5000 or less"
	}
	if strings.Contains(msg, "ExerciseName") && strings.Contains(msg, "min") {
		return "Exercise name is required"
	}
	if strings.Contains(msg, "Notes") && strings.Contains(msg, "max") && strings.Contains(msg, "500") {
		return "Notes must be 500 characters or less"
	}
	if strings.Contains(msg, "Notes") && strings.Contains(msg, "max") && strings.Contains(msg, "1000") {
		return "Notes must be 1000 characters or less"
	}
	if strings.Contains(msg, "Weight") && strings.Contains(msg, "lte") {
		return "Weight must be 1000 or less"
	}
	return msg
}
