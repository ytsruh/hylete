// Package routes: api_workouts.go contains every JSON handler for the
// workout endpoints under /api/v1 — the contract the iOS client
// builds its beta Workouts surfaces against. Each handler is
// intentionally thin: it binds the request, validates it, calls the
// shared controller method, and translates the result into a DTO.
// Plan-ahead copies are snapshots of a source workout, never live
// references, and the web app has no workout UI in v1.
package routes

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// SetWorkoutServices attaches the workout orchestrator. Kept as a
// setter (rather than NewHandler params) so Handler construction
// sites stay stable — same pattern as SetCoachService.
func (h *Handler) SetWorkoutServices(workouts *controllers.WorkoutController) {
	h.workoutCtrl = workouts
}

// maxWorkoutRangeSpan caps the from/to window on GET /api/v1/workouts.
// Planning pulls months at a time, so this is generous headroom (over
// a year) rather than a feature limit — it exists to stop a caller
// requesting an effectively unbounded scan.
const maxWorkoutRangeSpan = 400 * 24 * time.Hour

// workoutValidationError reports whether err is one of the workout
// controllers' (or the workout models') validation sentinels and
// returns its human-readable message. These are client mistakes so
// they map to 400 rather than 500 — mirroring
// exerciseEntryValidationError. (Item targets are optional, so the
// exercise-entry sentinels can no longer surface here; logged sets
// keep their own strict mapping.)
func workoutValidationError(err error) (string, bool) {
	switch {
	case errors.Is(err, controllers.ErrWorkoutNameRequired),
		errors.Is(err, controllers.ErrWorkoutNameTooLong),
		errors.Is(err, controllers.ErrUnknownWorkoutBlockType),
		errors.Is(err, controllers.ErrWorkoutExerciseNotFound),
		errors.Is(err, controllers.ErrWorkoutItemNeedsWorkout),
		errors.Is(err, controllers.ErrWorkoutItemMismatch),
		errors.Is(err, controllers.ErrWorkoutRoundNeedsWorkout),
		errors.Is(err, controllers.ErrWorkoutRoundInvalid),
		errors.Is(err, models.ErrScheduleEndBeforeStart),
		errors.Is(err, models.ErrBlockIntervalRequired),
		errors.Is(err, models.ErrBlockTimeCapRequired):
		return err.Error(), true
	}
	return "", false
}

// workoutNotFound reports whether err is the workout not-found
// sentinel (missing / owned by another user — the controller returns
// nil rows for both cases so the iOS view treats them uniformly).
func workoutNotFound(err error) bool {
	return errors.Is(err, controllers.ErrWorkoutNotFound)
}

// --- Workouts ---

// APIListWorkouts handles GET /api/v1/workouts.
//
// Two mutually-exclusive ways to pick the window (mirroring the
// exercise-entries list):
//
//   - no params: every workout for the user, planned first.
//   - ?from=<RFC3339>&to=<RFC3339>: workouts whose scheduled window
//     overlaps the range, inclusive on both ends. Used by the iOS
//     dashboard calendar so day boundaries are computed client-side.
//
// When either from or to is present the range mode wins. Malformed
// ranges return 400s with an APIError body.
func (h *Handler) APIListWorkouts(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	claims := GetClaims(c)

	rawFrom, rawTo := c.QueryParam("from"), c.QueryParam("to")
	if rawFrom == "" && rawTo == "" {
		workouts, err := h.workoutCtrl.ListWorkouts(claims.UserID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workouts"})
		}
		return c.JSON(http.StatusOK, WorkoutsFromModels(workouts))
	}

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
	if to.Sub(from) > maxWorkoutRangeSpan {
		return c.JSON(http.StatusBadRequest, APIError{Error: "range must not exceed 400 days"})
	}
	workouts, err := h.workoutCtrl.ListWorkoutsByRange(claims.UserID, from, to)
	if err != nil {
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workouts"})
	}
	return c.JSON(http.StatusOK, WorkoutsFromModels(workouts))
}

// APICreateWorkout handles POST /api/v1/workouts. Persists the
// header plus the block/item tree in order (may be empty for a bare
// shell) and returns the full detail (tree plus empty entry lists)
// so the client can open it without a follow-up GET.
func (h *Handler) APICreateWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	var in CreateWorkoutRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	created, err := h.workoutCtrl.CreateWorkout(claims.UserID, controllers.CreateWorkoutInput{
		Name:           in.Name,
		Notes:          in.Notes,
		ScheduledStart: in.ScheduledStart,
		ScheduledEnd:   in.ScheduledEnd,
		Blocks:         WorkoutBlockInputsFromDTOs(in.Blocks),
	})
	if err != nil {
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create workout"})
	}
	return c.JSON(http.StatusCreated, WorkoutDetailFromModel(created))
}

// APIBulkCreateWorkouts handles POST /api/v1/workouts/bulk. Snapshots
// one source workout into one dated copy per instance, atomically
// (all or nothing — a mid-list failure creates none). Powers
// plan-ahead flows ("every Monday x N"). Returns the created headers
// in request order so the client can splice them into its cache.
func (h *Handler) APIBulkCreateWorkouts(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	var in BulkCreateWorkoutsRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	instances := make([]controllers.BulkWorkoutInstance, 0, len(in.Instances))
	for _, inst := range in.Instances {
		instances = append(instances, controllers.BulkWorkoutInstance{
			Name:           inst.Name,
			Notes:          inst.Notes,
			ScheduledStart: inst.ScheduledStart,
			ScheduledEnd:   inst.ScheduledEnd,
		})
	}
	created, err := h.workoutCtrl.BulkCreateWorkouts(claims.UserID, in.WorkoutID, instances)
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create workouts"})
	}
	return c.JSON(http.StatusCreated, WorkoutsFromModels(created))
}

// APIDuplicateWorkout handles POST /api/v1/workouts/:id/duplicate.
// Copies the workout (including its tree) — a blank name becomes
// "Copy of <source>", nil schedule bounds inherit the source's
// window — and returns the new detail.
func (h *Handler) APIDuplicateWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	var in DuplicateWorkoutRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	duplicated, err := h.workoutCtrl.DuplicateWorkout(c.Param("id"), claims.UserID, controllers.DuplicateWorkoutInput{
		Name:           in.Name,
		ScheduledStart: in.ScheduledStart,
		ScheduledEnd:   in.ScheduledEnd,
	})
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to duplicate workout"})
	}
	return c.JSON(http.StatusCreated, WorkoutDetailFromModel(duplicated))
}

// APIGetWorkout handles GET /api/v1/workouts/:id. Returns the workout
// with its tree and logged exercise entries (split into item-linked
// vs ad-hoc), or 404 when missing or owned by another user.
func (h *Handler) APIGetWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	claims := GetClaims(c)
	detail, err := h.workoutCtrl.GetWorkoutDetail(c.Param("id"), claims.UserID)
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	return c.JSON(http.StatusOK, WorkoutDetailFromModel(detail))
}

// APIUpdateWorkout handles PUT /api/v1/workouts/:id. Header fields
// only — the tree is fixed at creation in v1 and status moves through
// the dedicated complete/reopen/cancel routes.
func (h *Handler) APIUpdateWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	var in UpdateWorkoutRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	updated, err := h.workoutCtrl.UpdateWorkout(c.Param("id"), claims.UserID, controllers.UpdateWorkoutInput{
		Name:           in.Name,
		Notes:          in.Notes,
		ScheduledStart: in.ScheduledStart,
		ScheduledEnd:   in.ScheduledEnd,
	})
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update workout"})
	}
	return c.JSON(http.StatusOK, WorkoutDetailFromModel(updated))
}

// APIDeleteWorkout handles DELETE /api/v1/workouts/:id. Hard delete,
// scoped to the authenticated user. The tree cascades; logged
// exercise entries survive with their links cleared. Returns 204 so
// the iOS app can simply call it and refresh the list.
func (h *Handler) APIDeleteWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	claims := GetClaims(c)
	if err := h.workoutCtrl.DeleteWorkout(c.Param("id"), claims.UserID); err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete workout"})
	}
	return c.NoContent(http.StatusNoContent)
}

// APICompleteWorkout handles POST /api/v1/workouts/:id/complete. The
// server sets completed_at to time.Now() — the client does not send a
// timestamp. Idempotent: completing an already-complete workout is a
// no-op that still returns 200 with the current detail (matches the
// goals surface).
func (h *Handler) APICompleteWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	claims := GetClaims(c)
	updated, err := h.workoutCtrl.Complete(c.Param("id"), claims.UserID, time.Now())
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to complete workout"})
	}
	return c.JSON(http.StatusOK, WorkoutDetailFromModel(updated))
}

// APIReopenWorkout handles POST /api/v1/workouts/:id/reopen. Moves
// the workout back to planned and clears completed_at; idempotent on
// an already-planned workout.
func (h *Handler) APIReopenWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	claims := GetClaims(c)
	updated, err := h.workoutCtrl.Reopen(c.Param("id"), claims.UserID)
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to reopen workout"})
	}
	return c.JSON(http.StatusOK, WorkoutDetailFromModel(updated))
}

// APICancelWorkout handles POST /api/v1/workouts/:id/cancel. Marks
// the workout cancelled and clears completed_at; idempotent.
func (h *Handler) APICancelWorkout(c echo.Context) error {
	if h.workoutCtrl == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "workouts unavailable"})
	}
	claims := GetClaims(c)
	updated, err := h.workoutCtrl.Cancel(c.Param("id"), claims.UserID)
	if err != nil {
		if workoutNotFound(err) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to cancel workout"})
	}
	return c.JSON(http.StatusOK, WorkoutDetailFromModel(updated))
}

// --- Exercise-entry workout linkage ---

// workoutLinkageFromRequest builds the submission-level linkage from
// the request fields. Empty IDs and a zero round mean standalone.
func workoutLinkageFromRequest(workoutID, workoutItemID string, roundNumber int) models.WorkoutLinkage {
	return models.WorkoutLinkage{
		WorkoutID:     workoutID,
		WorkoutItemID: workoutItemID,
		RoundNumber:   roundNumber,
	}
}

// workoutLinkageError maps an exercise-entry linkage validation
// failure to its human-readable message. Every linkage problem —
// including references to missing workouts or items — is a 400 here:
// the entry flow treats an unresolvable link as a malformed request,
// mirroring the existing "exercise not found" 400 (unlike the workout
// routes' own 404s for the same sentinels).
func workoutLinkageError(err error) (string, bool) {
	if msg, ok := workoutValidationError(err); ok {
		return msg, true
	}
	if errors.Is(err, controllers.ErrWorkoutNotFound) {
		return err.Error(), true
	}
	return "", false
}

// validateWorkoutLinkage runs the linkage rules when a workout
// controller is wired. A standalone linkage always passes so the
// pre-workout behaviour is unchanged when the feature is disabled.
// Linkage problems are client mistakes (400), including references to
// missing/third-party workouts — unlike the workout routes' own 404s,
// the entry flow treats an unresolvable link as a malformed request,
// mirroring the existing "exercise not found" 400.
func (h *Handler) validateWorkoutLinkage(userID string, link models.WorkoutLinkage) error {
	if link.IsStandalone() {
		return nil
	}
	if h.workoutCtrl == nil {
		return errors.New("workouts unavailable")
	}
	return h.workoutCtrl.ValidateExerciseEntryLinkage(userID, link)
}
