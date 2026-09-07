package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/db"
	"hylete/internal/email"
	"hylete/internal/export"
	"hylete/internal/imaging"
	"hylete/internal/models"
	"hylete/internal/reminders"
	"hylete/internal/routes"
	"hylete/internal/utils"
	"hylete/internal/views"
)

// weightReminderCronSpec is the cron expression for the
// hourly "who is due for a weight reminder?" tick.
// "0 * * * *" is minute=0, every hour, every day. Each
// tick finds every user whose next_fire_at is at or before
// now and fires their chosen channels. The per-user
// schedule (frequency, day-of-week, time) lives in the
// users table — this is just the heartbeat.
//
// Interpreted in UTC by the cron wrapper so the schedule
// is the same regardless of where the server happens to
// be deployed. Per project policy (AGENTS.md) the spec is
// hard-coded rather than driven by an env var: the tick
// cadence is a product decision, not a per-deployment knob.
const weightReminderCronSpec = "0 * * * *"

func main() {
	// Load and validate environment variables on startup
	cfg, err := utils.LoadAndValidateEnv()
	if err != nil {
		log.Fatalf("Failed to load environment variables: %v", err)
	}

	// Load and validate storage (R2) configuration
	if _, err := utils.LoadStorageConfig(); err != nil {
		log.Fatalf("Failed to load storage configuration: %v", err)
	}

	// Initialize database
	database, err := db.NewConnection(cfg.DB_PATH, cfg.TURSO_DATABASE_URL, cfg.TURSO_AUTH_TOKEN)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize repositories
	repo := models.NewExerciseRepository(database)
	userRepo := models.NewUserRepository(database)
	adminUserRepo := models.NewUserAdminRepository(database)
	weightRepo := models.NewWeightRepository(database)
	authTokenRepo := models.NewAuthTokenRepository(database)
	goalsRepo := models.NewGoalRepository(database)

	// Initialize auth service
	jwtService := utils.NewJWTService(cfg.JWT_SECRET)

	// Wire the email service. The SMTP client targets Cloudflare's
	// implicit-TLS endpoint (smtp.mx.cloudflare.net:465); the
	// default From address, port, and timeout are set by
	// email.NewClient from the APIToken alone. Email is required at
	// startup (the env var is in the required list), so a failure
	// to construct the client is fatal. The PUBLIC_URL env var is
	// the base URL threaded into every link the email contains
	// (dashboard button, password-reset URL, footer link).
	emailClient, err := email.NewClient(email.ClientConfig{
		APIToken: cfg.CLOUDFLARE_EMAIL_TOKEN,
	})
	if err != nil {
		log.Fatalf("Failed to initialize email client: %v", err)
	}
	emailService, err := email.NewService(emailClient, cfg.PUBLIC_URL)
	if err != nil {
		log.Fatalf("Failed to initialize email service: %v", err)
	}

	// Initialize controllers
	authCtrl := controllers.NewAuthController(userRepo, jwtService, emailService)
	exerciseEntryCtrl := controllers.NewExerciseEntryController(repo)
	adminCtrl := controllers.NewAdminController(repo)
	adminUserCtrl := controllers.NewAdminUserController(adminUserRepo, authTokenRepo, emailService)
	feedbackCtrl := controllers.NewFeedbackController(models.NewFeedbackRepository(database))
	weightCtrl := controllers.NewWeightController(weightRepo, r2PhotoGetter{})
	authRecoveryCtrl := controllers.NewAuthRecoveryController(userRepo, authTokenRepo, emailService)
	goalsCtrl := controllers.NewGoalsController(goalsRepo)

	// Initialize the per-user weight-reminder orchestrator here so
	// the hourly cron scheduler below can drive it. The orchestrator
	// fires each due user's email reminder on
	// every tick; there is no admin UI for it any more — the hourly
	// schedule is the only trigger.
	weightReminder, err := reminders.NewUserReminder(
		userRepo,
		emailService,
		reminders.UserReminderConfig{},
	)
	if err != nil {
		log.Fatalf("Failed to initialize weight reminder: %v", err)
	}

	// Initialize route handlers
	validator := utils.NewValidator()
	h := routes.NewHandler(
		authCtrl,
		authRecoveryCtrl,
		exerciseEntryCtrl,
		adminCtrl,
		adminUserCtrl,
		feedbackCtrl,
		weightCtrl,
		goalsCtrl,
		userRepo,
		jwtService,
		validator,
		imaging.NewStdProcessor(),
		utilsR2Uploader{},
		routes.DefaultExerciseImageConfig,
	)

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Custom HTTP error handler. HTML routes get a templ-rendered
	// error page so the user can read the message in the same
	// design as the rest of the app; API routes (anything under
	// /api/v1/) get a JSON body so native clients like the iOS
	// app can parse the error without trying to render HTML.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		message := "Something went wrong"

		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			if he.Message != nil {
				if msg, ok := he.Message.(string); ok {
					message = msg
				}
			}
		}

		if strings.HasPrefix(c.Request().URL.Path, "/api/v1/") {
			if !c.Response().Committed {
				_ = c.JSON(code, routes.APIError{Error: message})
			}
			return
		}

		data := views.NewErrorData(code, message)
		views.ErrorPage(data).Render(c.Request().Context(), c.Response())
	}

	// Register routes
	h.RegisterRoutes(e)

	// Start the hourly weight-reminder scheduler. The
	// orchestrator was constructed above; the cron wrapper is
	// the only place in the codebase that imports the
	// third-party scheduling library, so a future "swap for a
	// queue / system cron" change is a one-package diff. A bad
	// spec (e.g. a typo in "0 * * * *") fails startup rather
	// than silently never firing.
	scheduler, err := reminders.NewCronScheduler(
		weightReminderCronSpec,
		time.UTC,
		// The cron job discards the TickResult — every
		// useful field is already written to the server log.
		func(ctx context.Context) { _, _ = weightReminder.Run(ctx) },
	)
	if err != nil {
		log.Fatalf("Failed to initialize reminder scheduler: %v", err)
	}
	scheduler.Start()
	defer scheduler.Stop()

	// Start server
	localIP := getLocalIP()
	if localIP != "" {
		log.Printf("Server starting on http://localhost:%s and http://%s:%s", cfg.PORT, localIP, cfg.PORT)
	} else {
		log.Printf("Server starting on http://localhost:%s", cfg.PORT)
	}
	if err := e.Start(":" + cfg.PORT); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// getLocalIP returns the first non-loopback IPv4 address of the machine.
// Returns an empty string if no suitable interface is found.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

// r2PhotoGetter adapts utils.GetObject to the export.PhotoGetter
// interface. Kept as its own type so a test fake can be substituted in
// if/when the export package is exercised via the controller.
type r2PhotoGetter struct{}

func (r2PhotoGetter) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return utils.GetObject(ctx, key)
}

// Compile-time check: r2PhotoGetter satisfies export.PhotoGetter.
var _ export.PhotoGetter = r2PhotoGetter{}

// utilsR2Uploader adapts utils.PutObject to the
// routes.exerciseImageUploader interface. Same shape as
// r2PhotoGetter (the read-side adapter used by the export flow);
// keeping it as a named type makes it easy to swap a fake in tests
// when the admin image route eventually needs end-to-end coverage.
type utilsR2Uploader struct{}

func (utilsR2Uploader) PutObject(ctx context.Context, key, contentType string, body io.Reader) error {
	return utils.PutObject(ctx, key, contentType, body)
}

// Compile-time check: utilsR2Uploader satisfies the
// routes.exerciseImageUploader interface.
var _ interface {
	PutObject(ctx context.Context, key, contentType string, body io.Reader) error
} = utilsR2Uploader{}
