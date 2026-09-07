// Package routes: api_v1.go contains every JSON handler for the
// /api/v1/* namespace — the contract the iOS client (client/)
// is built against. Each handler is intentionally thin: it binds
// the request, validates it, calls the shared controller method,
// and translates the result into a DTO.
package routes

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// --- Auth ---

// APILogin handles POST /api/v1/auth/login. Returns the JWT in
// the response body (not a cookie) so native clients can store
// it in the Keychain. Validation errors return a 400 with the
// same APIError shape the rest of the namespace uses.
func (h *Handler) APILogin(c echo.Context) error {
	var in LoginRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	user, token, err := h.authCtrl.Login(in.Email, in.Password)
	if err != nil {
		if err == controllers.ErrInvalidCredentials {
			return c.JSON(http.StatusUnauthorized, APIError{Error: "invalid email or password"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create session"})
	}
	return c.JSON(http.StatusOK, AuthResponse{Token: token, User: UserFromModel(user)})
}

// APIRegister handles POST /api/v1/auth/register. Same
// behavior as APILogin for the response shape; the controller
// fires the welcome email on a goroutine and we don't wait for
// it before responding.
func (h *Handler) APIRegister(c echo.Context) error {
	var in RegisterRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	user, token, err := h.authCtrl.Register(in.Name, in.Email, in.Password)
	if err != nil {
		if err == controllers.ErrEmailExists {
			return c.JSON(http.StatusConflict, APIError{Error: "an account with that email already exists"})
		}
		return c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, AuthResponse{Token: token, User: UserFromModel(user)})
}

// APIRequestPasswordReset handles password-reset requests from native
// clients. The reset link still points to the web reset form, so the
// client does not need to handle reset tokens or duplicate that form.
// Unknown emails receive the same successful response as known emails.
func (h *Handler) APIRequestPasswordReset(c echo.Context) error {
	var in PasswordResetRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	if err := h.authRecoveryCtrl.RequestPasswordReset(c.Request().Context(), in.Email); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "could not send password reset email"})
	}
	return c.JSON(http.StatusOK, PasswordResetResponse{
		Message: "If an account exists for that email, a reset link has been sent.",
	})
}

// APILogout handles POST /api/v1/auth/logout. Stateless: the
// server has nothing to do because the JWT is self-contained.
// The client discards its token. Returns 204 No Content so the
// iOS app can simply ignore the response.
func (h *Handler) APILogout(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

// --- Current user ---

// APIMe handles GET /api/v1/me. Returns the safe public view
// of the authenticated user, read from the user repository.
// Useful as a "is my token still valid?" check on app launch.
func (h *Handler) APIMe(c echo.Context) error {
	claims := GetClaims(c)
	user, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load user"})
	}
	if user == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
	}
	return c.JSON(http.StatusOK, UserFromModel(user))
}

// APIUpdateMe handles PUT /api/v1/me. Updates the same
// user-editable fields the HTML profile form exposes (name,
// target weight, weight unit) and returns the updated
// UserDTO. Reminder preferences, push subscriptions, and
// notification channels are intentionally NOT updated here
// — the iOS app surfaces no UI for them yet, and the web
// keeps the form-only ownership of those fields for now.
//
// The JWT is not regenerated. iOS reads the token directly
// from the Keychain and the next /me round-trip on launch
// will surface the new name; the server's cookie path is
// web-only and the iOS client never sees it.
func (h *Handler) APIUpdateMe(c echo.Context) error {
	var in UpdateMeRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}
	if in.WeightUnit == "" {
		// The validator's `oneof` only fires on a non-empty
		// value, so an omitted field sails through. Default
		// to "kg" to match the SQL column default and the
		// HTML form's empty-input behavior.
		in.WeightUnit = "kg"
	}
	if in.DistanceUnit == "" {
		// Same empty-input rule as WeightUnit: the column
		// default is "km", so an omitted field means "km".
		in.DistanceUnit = models.DistanceUnitKm
	}

	claims := GetClaims(c)
	user := &models.User{
		ID:           claims.UserID,
		Name:         in.Name,
		Email:        claims.Email,
		IsAdmin:      claims.IsAdmin,
		TargetWeight: in.TargetWeight,
		WeightUnit:   in.WeightUnit,
		DistanceUnit: in.DistanceUnit,
	}
	if err := h.userRepo.UpdateUser(user); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update profile"})
	}

	// Re-read so the response reflects any server-side
	// normalisation (e.g. WeightUnitDisplay falling back to
	// "kg" for unknown values) and the post-update timestamp.
	updated, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load user"})
	}
	if updated == nil {
		// Should not happen — the JWT referenced a user
		// that existed when we just updated it. Treat as a
		// 500 so the client retries rather than caching a
		// stale view.
		return c.JSON(http.StatusInternalServerError, APIError{Error: "user not found after update"})
	}
	return c.JSON(http.StatusOK, UserFromModel(updated))
}

// --- Exercises ---

// APIListExercises handles GET /api/v1/exercises. Returns
// every exercise in the catalogue. The iOS app uses this to
// populate the picker on the new-set screen.
func (h *Handler) APIListExercises(c echo.Context) error {
	exercises, err := h.exerciseEntryCtrl.List()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercises"})
	}
	return c.JSON(http.StatusOK, ExercisesFromModels(exercises))
}

// --- Exercise entries (sets) ---

// maxExerciseEntryRangeSpan caps the from/to window on
// GET /api/v1/exercise-entries. The calendar fetches one week at a
// time (plus buffer), so this is generous headroom rather than a
// feature limit — it exists to stop a caller requesting an
// effectively unbounded scan.
const maxExerciseEntryRangeSpan = 62 * 24 * time.Hour

// APIListExerciseEntries handles GET /api/v1/exercise-entries.
//
// Two mutually-exclusive ways to pick the window:
//
//   - ?days=N (default 7, clamped to [1, 365]): the user's sets
//     from the last N days relative to now, newest first. Same
//     data the web dashboard shows.
//   - ?from=<RFC3339>&to=<RFC3339>: an explicit instant range,
//     inclusive on both ends. Used by the iOS week calendar so
//     day boundaries are computed client-side in the user's
//     local timezone and sent as absolute instants.
//
// When either from or to is present the range mode wins; days is
// ignored. Malformed ranges return 400s with an APIError body.
func (h *Handler) APIListExerciseEntries(c echo.Context) error {
	claims := GetClaims(c)

	rawFrom, rawTo := c.QueryParam("from"), c.QueryParam("to")

	var (
		entries []models.ExerciseEntry
		err     error
	)
	switch {
	case rawFrom != "" || rawTo != "":
		// Explicit range mode. Both bounds are required — a
		// half-specified range is almost certainly a client bug,
		// so fail loudly instead of guessing the missing edge.
		if rawFrom == "" || rawTo == "" {
			return c.JSON(http.StatusBadRequest, APIError{Error: "both from and to are required"})
		}
		from, parseErr := time.Parse(time.RFC3339, rawFrom)
		if parseErr != nil {
			return c.JSON(http.StatusBadRequest, APIError{Error: "from must be an RFC3339 timestamp"})
		}
		to, parseErr := time.Parse(time.RFC3339, rawTo)
		if parseErr != nil {
			return c.JSON(http.StatusBadRequest, APIError{Error: "to must be an RFC3339 timestamp"})
		}
		if from.After(to) {
			return c.JSON(http.StatusBadRequest, APIError{Error: "from must not be after to"})
		}
		if to.Sub(from) > maxExerciseEntryRangeSpan {
			return c.JSON(http.StatusBadRequest, APIError{Error: "range must not exceed 62 days"})
		}
		entries, err = h.exerciseEntryCtrl.GetExerciseEntriesByDateRange(from, to, claims.UserID)
	default:
		days := 7
		if raw := c.QueryParam("days"); raw != "" {
			n, atoiErr := strconv.Atoi(raw)
			if atoiErr != nil || n < 1 {
				return c.JSON(http.StatusBadRequest, APIError{Error: "days must be a positive integer"})
			}
			if n > 365 {
				n = 365
			}
			days = n
		}

		if days == 7 {
			// Fast path: reuses the existing "last 7 days" query,
			// which is the dashboard's hot path.
			entries, err = h.exerciseEntryCtrl.ListExerciseEntriesLast7Days(claims.UserID)
		} else {
			end := time.Now()
			start := end.AddDate(0, 0, -days)
			entries, err = h.exerciseEntryCtrl.GetExerciseEntriesByDateRange(start, end, claims.UserID)
		}
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise entries"})
	}
	return c.JSON(http.StatusOK, ExerciseEntriesFromModels(entries))
}

// APIGetExerciseEntry handles GET /api/v1/exercise-entries/:id.
// Returns a single set scoped to the authenticated user.
func (h *Handler) APIGetExerciseEntry(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	entry, err := h.exerciseEntryCtrl.GetExerciseEntry(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise entry"})
	}
	if entry == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "exercise entry not found"})
	}
	return c.JSON(http.StatusOK, ExerciseEntryFromModel(*entry))
}

// exerciseEntryValidationError reports whether err is one of the
// controller's per-type validation sentinels and returns its
// human-readable message. These are client mistakes (e.g. a cardio
// set without a duration) so they map to 400 rather than 500.
func exerciseEntryValidationError(err error) (string, bool) {
	switch {
	case errors.Is(err, controllers.ErrRepsRequired),
		errors.Is(err, controllers.ErrDurationRequired),
		errors.Is(err, controllers.ErrDistanceRequired):
		return err.Error(), true
	}
	return "", false
}

// APICreateExerciseEntries handles POST /api/v1/exercise-entries.
// Mirrors the web form's "create one or more sets at once"
// behavior: the body contains an array of sets, and a single
// exercise entry is created per set, all sharing the same
// exercise, notes, and createdAt timestamp. Validation is keyed off
// the linked exercise's type: strength sets need reps >= 1, cardio
// sets need duration_seconds > 0 and distance_meters > 0; the server
// zeroes whichever metric pair does not apply.
func (h *Handler) APICreateExerciseEntries(c echo.Context) error {
	var in CreateExerciseEntriesRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)

	// Confirm the exercise actually exists. Without this check
	// the controller would happily create a set linked to a
	// non-existent exercise ID — fine for a typo on the iOS
	// app's side, but the user would never see the set in the
	// history view (which filters by exercise ID).
	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(in.ExerciseID, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise"})
	}
	if exercise == nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "exercise not found"})
	}

	createdAt := time.Now()
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}

	sets := make([]controllers.ExerciseSetInput, 0, len(in.Sets))
	for _, s := range in.Sets {
		sets = append(sets, controllers.ExerciseSetInput{
			Reps:            s.Reps,
			Weight:          s.Weight,
			RestTime:        s.RestTime,
			DurationSeconds: s.DurationSeconds,
			DistanceMeters:  s.DistanceMeters,
			AvgHeartRate:    s.AvgHeartRate,
			CaloriesBurned:  s.CaloriesBurned,
		})
	}

	created, err := h.exerciseEntryCtrl.CreateExerciseEntries(claims.UserID, in.ExerciseID, exercise.Type, in.Notes, createdAt, sets)
	if err != nil {
		if msg, ok := exerciseEntryValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to save exercise entry"})
	}
	return c.JSON(http.StatusCreated, ExerciseEntriesFromModels(created))
}

// APIUpdateExerciseEntry handles PUT /api/v1/exercise-entries/:id.
// Single-set update. The exercise is loaded from the request's
// exercise_id so validation uses the linked exercise's type (and a
// non-existent exercise id is rejected instead of silently orphaning
// the row), then the controller applies the same type rules as create.
func (h *Handler) APIUpdateExerciseEntry(c echo.Context) error {
	var in UpdateExerciseEntryRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	id := c.Param("id")

	existing, err := h.exerciseEntryCtrl.GetExerciseEntry(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise entry"})
	}
	if existing == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "exercise entry not found"})
	}

	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(in.ExerciseID, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise"})
	}
	if exercise == nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "exercise not found"})
	}

	createdAt := existing.CreatedAt
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}

	updated, err := h.exerciseEntryCtrl.UpdateExerciseEntry(id, claims.UserID, in.ExerciseID, exercise.Type, in.Notes, controllers.ExerciseSetInput{
		Reps:            in.Reps,
		Weight:          in.Weight,
		RestTime:        in.RestTime,
		DurationSeconds: in.DurationSeconds,
		DistanceMeters:  in.DistanceMeters,
		AvgHeartRate:    in.AvgHeartRate,
		CaloriesBurned:  in.CaloriesBurned,
	}, createdAt)
	if err != nil {
		if msg, ok := exerciseEntryValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update exercise entry"})
	}
	return c.JSON(http.StatusOK, ExerciseEntryFromModel(*updated))
}

// APIDeleteExerciseEntry handles DELETE /api/v1/exercise-entries/:id.
// Returns 204 on success so the iOS app can simply call it and
// refresh the list.
func (h *Handler) APIDeleteExerciseEntry(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if err := h.exerciseEntryCtrl.DeleteExerciseEntry(id, claims.UserID); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete exercise entry"})
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Exercise history & chart ---

// APIGetExerciseHistory handles GET /api/v1/exercises/:id/history.
// Accepts an optional ?page=N (1-indexed, clamped to 1) and
// returns a page of sets for the given exercise together with
// the lifetime stats (max weight, last set).
func (h *Handler) APIGetExerciseHistory(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise"})
	}
	if exercise == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "exercise not found"})
	}

	page := parsePage(c.QueryParam("page"))
	history, err := h.exerciseEntryCtrl.GetExerciseEntriesByExercise(exercise.ID, claims.UserID, page)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load history"})
	}
	return c.JSON(http.StatusOK, HistoryPageFromModel(history))
}

// APIGetExerciseChartData handles GET /api/v1/exercises/:id/chart.
// Returns every exercise entry for the given exercise so the
// iOS app can plot the line chart locally with Swift Charts.
// No pagination — the iOS chart needs the full series.
func (h *Handler) APIGetExerciseChartData(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load exercise"})
	}
	if exercise == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "exercise not found"})
	}

	entries, err := h.exerciseEntryCtrl.GetAllExerciseEntriesForChart(exercise.ID, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load chart data"})
	}
	return c.JSON(http.StatusOK, ExerciseEntriesFromModels(entries))
}

// --- Goals ---

// APIListGoals handles GET /api/v1/goals. Returns every goal
// for the authenticated user, active first then completed
// (the same ordering the web view renders — the repository's
// List method concatenates ListActiveGoals + ListCompletedGoals).
func (h *Handler) APIListGoals(c echo.Context) error {
	claims := GetClaims(c)
	goals, err := h.goalsCtrl.ListGoals(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load goals"})
	}
	return c.JSON(http.StatusOK, map[string]any{"goals": GoalsFromModels(goals)})
}

// APICreateGoal handles POST /api/v1/goals. Validates the
// request body, then delegates to the controller (the same
// path the HTML form uses).
func (h *Handler) APICreateGoal(c echo.Context) error {
	var in CreateGoalRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	created, err := h.goalsCtrl.CreateGoal(claims.UserID, controllers.CreateGoalInput{
		Title:       in.Title,
		Description: in.Description,
		StartDate:   in.StartDate,
		TargetDate:  in.TargetDate,
		EndDate:     in.EndDate,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create goal"})
	}
	return c.JSON(http.StatusCreated, GoalFromModel(*created))
}

// APIGetGoal handles GET /api/v1/goals/:id. Returns 404 when
// the goal is missing or owned by another user — the
// controller's ErrGoalNotFound sentinel covers both cases.
func (h *Handler) APIGetGoal(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	g, err := h.goalsCtrl.GetGoal(id, claims.UserID)
	if err != nil {
		if err == controllers.ErrGoalNotFound {
			return c.JSON(http.StatusNotFound, APIError{Error: "goal not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load goal"})
	}
	return c.JSON(http.StatusOK, GoalFromModel(*g))
}

// APIUpdateGoal handles PUT /api/v1/goals/:id. The body has the
// same shape as create (minus the server-generated id). The
// completed_at field is intentionally not editable here —
// status changes go through the dedicated complete/reopen
// routes so the server owns the timestamp.
func (h *Handler) APIUpdateGoal(c echo.Context) error {
	var in UpdateGoalRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	id := c.Param("id")
	updated, err := h.goalsCtrl.UpdateGoal(id, claims.UserID, controllers.UpdateGoalInput{
		Title:       in.Title,
		Description: in.Description,
		StartDate:   in.StartDate,
		TargetDate:  in.TargetDate,
		EndDate:     in.EndDate,
	})
	if err != nil {
		if err == controllers.ErrGoalNotFound {
			return c.JSON(http.StatusNotFound, APIError{Error: "goal not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update goal"})
	}
	return c.JSON(http.StatusOK, GoalFromModel(*updated))
}

// APIMarkGoalComplete handles POST /api/v1/goals/:id/complete.
// The server sets completed_at to time.Now() — the client
// does not send a timestamp. Idempotent: completing an already
// complete goal is a no-op that still returns 200 with the
// current row (matches the web's behavior).
func (h *Handler) APIMarkGoalComplete(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	updated, err := h.goalsCtrl.MarkComplete(id, claims.UserID, time.Now())
	if err != nil {
		if err == controllers.ErrGoalNotFound {
			return c.JSON(http.StatusNotFound, APIError{Error: "goal not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to mark goal complete"})
	}
	return c.JSON(http.StatusOK, GoalFromModel(*updated))
}

// APIReopenGoal handles POST /api/v1/goals/:id/reopen. Clears
// completed_at; idempotent on an already-active goal. Mirrors
// the web's reopen button which fires a toast and lets the
// card "move" from the completed section back to active.
func (h *Handler) APIReopenGoal(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	updated, err := h.goalsCtrl.Reopen(id, claims.UserID)
	if err != nil {
		if err == controllers.ErrGoalNotFound {
			return c.JSON(http.StatusNotFound, APIError{Error: "goal not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to reopen goal"})
	}
	return c.JSON(http.StatusOK, GoalFromModel(*updated))
}

// APIDeleteGoal handles DELETE /api/v1/goals/:id. Hard delete,
// scoped to the authenticated user. Returns 204 so the iOS
// app can simply call it and refresh the list.
func (h *Handler) APIDeleteGoal(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if err := h.goalsCtrl.DeleteGoal(id, claims.UserID); err != nil {
		if err == controllers.ErrGoalNotFound {
			return c.JSON(http.StatusNotFound, APIError{Error: "goal not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete goal"})
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Feedback ---

// APISubmitFeedback handles POST /api/v1/feedback. Accepts a
// {title, message} body and delegates to FeedbackController.Submit,
// which applies the trim + length rules and persists the row scoped
// to the authenticated user. Returns 204 No Content on success
// so the iOS client can simply call it and dismiss its form;
// validation failures return the controller's human-readable
// message as the APIError body so the iOS view can surface it
// inline.
func (h *Handler) APISubmitFeedback(c echo.Context) error {
	var in SubmitFeedbackRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}

	claims := GetClaims(c)
	if err := h.feedbackCtrl.Submit(in.Title, in.Message, claims.UserID); err != nil {
		switch {
		case errors.Is(err, controllers.ErrTitleTooShort),
			errors.Is(err, controllers.ErrTitleTooLong),
			errors.Is(err, controllers.ErrMessageTooShort),
			errors.Is(err, controllers.ErrMessageTooLong):
			return c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to submit feedback"})
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Weight ---

// APIListWeightEntries handles GET /api/v1/weight. Returns
// every weight entry for the authenticated user, newest first
// (the same order the repository's List method returns). The
// response is wrapped in a named envelope (`entries`) so the
// server can add fields like overview stats without breaking
// the iOS contract.
func (h *Handler) APIListWeightEntries(c echo.Context) error {
	claims := GetClaims(c)
	entries, err := h.weightCtrl.ListWeightEntries(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load weight entries"})
	}
	return c.JSON(http.StatusOK, WeightEntriesResponse{Entries: WeightEntriesFromModels(entries)})
}

// APICreateWeightEntry handles POST /api/v1/weight. Validates
// the body, then delegates to the same controller method the
// HTML form uses. CreatedAt is optional — the server defaults
// to time.Now() when omitted so the iOS "log it now" button
// can send an empty body field. Returns 201 Created with the
// freshly persisted entry so the iOS view can splice it into
// its local cache without a follow-up GET.
func (h *Handler) APICreateWeightEntry(c echo.Context) error {
	var in CreateWeightEntryRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	createdAt := time.Now()
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}

	created, err := h.weightCtrl.CreateWeightEntry(claims.UserID, in.Weight, in.Notes, in.PhotoKey)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create weight entry"})
	}
	// The controller stamps CreatedAt internally; honour the
	// client-supplied timestamp when the request carried one
	// (e.g. backdating an entry). CreateWeightEntry doesn't
	// accept a createdAt argument, so we re-update in place
	// when it differs from the server default.
	if !createdAt.Equal(created.CreatedAt) {
		updated, err := h.weightCtrl.UpdateWeightEntry(created.ID, claims.UserID, in.Weight, in.Notes, in.PhotoKey, createdAt)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to backdate weight entry"})
		}
		created = updated
	}
	return c.JSON(http.StatusCreated, WeightEntryFromModel(*created))
}

// APIGetWeightEntry handles GET /api/v1/weight/:id. Returns
// 404 when the entry is missing or owned by another user — the
// controller's GetByID returns nil for both cases so the iOS
// view can treat the two outcomes uniformly.
func (h *Handler) APIGetWeightEntry(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	entry, err := h.weightCtrl.GetWeightEntry(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load weight entry"})
	}
	if entry == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "weight entry not found"})
	}
	return c.JSON(http.StatusOK, WeightEntryFromModel(*entry))
}

// APIUpdateWeightEntry handles PUT /api/v1/weight/:id. Mirrors
// the existing HTML PUT handler's photo-handling precedence:
//   - remove_photo=true clears the photo_key (and best-effort
//     deletes the underlying R2 object)
//   - non-empty photo_key replaces the existing key
//   - otherwise the existing key is preserved
//
// CreatedAt is optional; when omitted, the existing timestamp
// is kept so the "edit a recent entry" flow doesn't
// accidentally reset the entry to time.Now().
func (h *Handler) APIUpdateWeightEntry(c echo.Context) error {
	var in UpdateWeightEntryRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	id := c.Param("id")

	existing, err := h.weightCtrl.GetWeightEntry(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load weight entry"})
	}
	if existing == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "weight entry not found"})
	}

	// Resolve the final photo state following the same
	// precedence the HTML PUT handler applies (see
	// internal/routes/weight.go:157-169).
	photoKey := existing.PhotoKey
	if in.RemovePhoto {
		photoKey = ""
	} else if in.PhotoKey != "" {
		photoKey = in.PhotoKey
	}

	createdAt := existing.CreatedAt
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}

	updated, err := h.weightCtrl.UpdateWeightEntry(id, claims.UserID, in.Weight, in.Notes, photoKey, createdAt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update weight entry"})
	}
	return c.JSON(http.StatusOK, WeightEntryFromModel(*updated))
}

// APIDeleteWeightEntry handles DELETE /api/v1/weight/:id.
// Returns 204 No Content so the iOS app can simply call it
// and refresh the view. Best-effort deletes the underlying
// R2 photo (the controller logs and continues on R2 failure
// so an orphaned bucket object doesn't block the deletion).
func (h *Handler) APIDeleteWeightEntry(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if err := h.weightCtrl.DeleteWeightEntry(id, claims.UserID); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete weight entry"})
	}
	return c.NoContent(http.StatusNoContent)
}

// APICompareWeight handles GET /api/v1/weight/compare?a=<id1>&b=<id2>.
// Validates the pair (both must exist, both must belong to the
// user, both must have a photo) and returns the sorted
// (before, after) pair plus the formatted delta string. The
// controller's `GetWeightEntriesForCompare` returns a
// human-readable error for the failure modes so the iOS view
// can surface it inline.
func (h *Handler) APICompareWeight(c echo.Context) error {
	idA := c.QueryParam("a")
	idB := c.QueryParam("b")

	claims := GetClaims(c)
	entries, err := h.weightCtrl.GetWeightEntriesForCompare(idA, idB, claims.UserID)
	if err != nil {
		// 400 — the request is well-formed but the pair is
		// not suitable for comparison (missing id, missing
		// photo, same id twice, etc.). 500 would be wrong
		// here because the client can fix the request by
		// choosing a different pair.
		return c.JSON(http.StatusBadRequest, APIError{Error: err.Error()})
	}

	before := entries[0]
	after := entries[1]
	delta := after.Weight - before.Weight

	// Pull the user's preferred unit from the cached user so
	// the delta line matches the entry labels the view
	// renders. The controller doesn't need the unit — that's
	// only used for display.
	unit := "kg"
	if user := h.GetUser(c); user != nil {
		unit = user.WeightUnitDisplay()
	}

	return c.JSON(http.StatusOK, WeightCompareResponse{
		Before:    WeightEntryFromModel(before),
		After:     WeightEntryFromModel(after),
		DeltaText: formatWeightDelta(delta, unit),
	})
}

// formatWeightDelta produces a short human-readable summary of the
// weight change between two entries. Positive deltas are prefixed with
// "+", negative with "−" (U+2212), and deltas within ±0.05 return an
// empty string so the caller can omit the change indicator entirely
// (avoids the awkward "· no change" label). unit is the user's
// preferred display unit ("kg" or "lbs") and is appended to the value
// so the delta is labelled consistently with the entries it compares.
func formatWeightDelta(delta float64, unit string) string {
	switch {
	case delta > 0.05:
		return fmt.Sprintf("+%.1f %s", delta, unit)
	case delta < -0.05:
		return fmt.Sprintf("−%.1f %s", -delta, unit)
	default:
		return ""
	}
}

// APIRequestPhotoUploadURL handles POST /api/v1/weight/photo-upload.
// Thin JWT-protected wrapper around the shared presigned-URL flow
// (see `internal/routes/photos.go:33`). The iOS client uses this
// JWT-protected variant so the native session never relies on
// browser cookie auth.
func (h *Handler) APIRequestPhotoUploadURL(c echo.Context) error {
	var in WeightPhotoUploadRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	// Delegate to the shared handler for the file-type /
	// extension / presigned-URL logic. The shared helper
	// enforces the image/* MIME check, so the iOS client
	// cannot bypass it by going via the JSON API.
	return h.PhotoUploadURL(c)
}
