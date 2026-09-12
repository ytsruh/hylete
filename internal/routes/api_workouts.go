package routes

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// Workout handlers live here (rather than api_v1.go) so the
// workout surface stays in one file. Same conventions as the
// blocks handlers: thin bind -> validate -> controller -> DTO,
// sentinel errors become 400s, ErrWorkoutNotFound (and
// ErrWorkoutAssignmentNotFound) become 404s.

// SetWorkoutsController attaches the Workouts orchestrator. Kept
// as a setter (rather than NewHandler params) so existing
// construction sites and tests are untouched - same pattern as
// SetBlocksController.
func (h *Handler) SetWorkoutsController(ctrl *controllers.WorkoutsController) {
	h.workoutsCtrl = ctrl
}

// workoutValidationError maps workout controller sentinels to a
// client message. The second return is false for non-validation
// errors (caller responds 500).
func workoutValidationError(err error) (string, bool) {
	switch err {
	case controllers.ErrWorkoutTitleRequired:
		return "workout title is required", true
	case controllers.ErrWorkoutTitleTooLong:
		return "workout title must be 100 characters or less", true
	case controllers.ErrWorkoutDescriptionLong:
		return "workout description must be 1000 characters or less", true
	case controllers.ErrWorkoutBlocksRequired:
		return "a workout needs at least 1 block", true
	case controllers.ErrWorkoutBlocksTooMany:
		return "a workout can hold at most 20 blocks", true
	case controllers.ErrWorkoutBlockRequired:
		return "workout block is required", true
	case controllers.ErrWorkoutBlockNotFound:
		return "block not found", true
	case controllers.ErrWorkoutDateRequired:
		return "at least 1 date is required", true
	case controllers.ErrWorkoutDatesTooMany:
		return "at most 100 dates per request", true
	case controllers.ErrWorkoutDateInvalid:
		return "dates must be YYYY-MM-DD", true
	case controllers.ErrWorkoutScheduleRangeNeeded:
		return "both from and to are required", true
	case controllers.ErrWorkoutScheduleRangeOrder:
		return "from must not be after to", true
	case controllers.ErrWorkoutScheduleRangeSpan:
		return "range must not exceed 62 days", true
	}
	return "", false
}

// workoutDetail loads the workout's assignments and renders the
// full DTO. Assignments from the per-workout listing carry no
// title (the join only resolves titles on the range query), so
// the parent title is filled in for a stable contract.
func (h *Handler) workoutDetail(userID string, w *models.Workout) (WorkoutDTO, error) {
	assignments, err := h.workoutsCtrl.GetWorkoutSchedule(w.ID, userID)
	if err != nil {
		return WorkoutDTO{}, err
	}
	for i := range assignments {
		assignments[i].WorkoutTitle = w.Title
	}
	return WorkoutFromModel(*w, assignments), nil
}

// APIListWorkouts handles GET /api/v1/workouts. Returns the
// authenticated user's workout summaries (newest first).
func (h *Handler) APIListWorkouts(c echo.Context) error {
	claims := GetClaims(c)
	workouts, err := h.workoutsCtrl.ListWorkouts(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workouts"})
	}
	return c.JSON(http.StatusOK, map[string]any{"workouts": WorkoutSummariesFromModels(workouts)})
}

// APICreateWorkout handles POST /api/v1/workouts. Validates the
// body, resolves each block, and returns the created workout.
// The workout starts unscheduled - days are added via the
// assignments endpoint.
func (h *Handler) APICreateWorkout(c echo.Context) error {
	var in CreateWorkoutRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	created, err := h.workoutsCtrl.CreateWorkout(claims.UserID, controllers.CreateWorkoutInput{
		Title:       in.Title,
		Description: in.Description,
		BlockIDs:    in.BlockIDs,
	})
	if err != nil {
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create workout"})
	}
	return c.JSON(http.StatusCreated, WorkoutFromModel(*created, nil))
}

// APIGetWorkout handles GET /api/v1/workouts/:id. Returns the
// workout with its blocks and planned days. Returns 404 when the
// workout is missing or owned by another user.
func (h *Handler) APIGetWorkout(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	w, err := h.workoutsCtrl.GetWorkout(id, claims.UserID)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	dto, err := h.workoutDetail(claims.UserID, w)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	return c.JSON(http.StatusOK, dto)
}

// APIUpdateWorkout handles PUT /api/v1/workouts/:id. Block links
// are fully replaced (same shape as create); assignments are
// untouched.
func (h *Handler) APIUpdateWorkout(c echo.Context) error {
	var in UpdateWorkoutRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	id := c.Param("id")
	updated, err := h.workoutsCtrl.UpdateWorkout(id, claims.UserID, controllers.UpdateWorkoutInput{
		Title:       in.Title,
		Description: in.Description,
		BlockIDs:    in.BlockIDs,
	})
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update workout"})
	}
	dto, err := h.workoutDetail(claims.UserID, updated)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	return c.JSON(http.StatusOK, dto)
}

// APIDeleteWorkout handles DELETE /api/v1/workouts/:id. Hard
// delete scoped to the authenticated user, including block links
// and assignments. Returns 204.
func (h *Handler) APIDeleteWorkout(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if err := h.workoutsCtrl.DeleteWorkout(id, claims.UserID); err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete workout"})
	}
	return c.NoContent(http.StatusNoContent)
}

// APIDuplicateWorkout handles POST
// /api/v1/workouts/:id/duplicate. Copies the workout as
// "<title> copy" with its block links; the schedule is copied
// only when copy_schedule is true. Returns 201.
func (h *Handler) APIDuplicateWorkout(c echo.Context) error {
	var in DuplicateWorkoutRequest
	// The body is optional (empty means schedule is not
	// copied); a bind failure on an empty body is not fatal.
	_ = c.Bind(&in)

	claims := GetClaims(c)
	id := c.Param("id")
	dup, err := h.workoutsCtrl.DuplicateWorkout(id, claims.UserID, in.CopySchedule)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to duplicate workout"})
	}
	dto, err := h.workoutDetail(claims.UserID, dup)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	return c.JSON(http.StatusCreated, dto)
}

// APIAddWorkoutAssignments handles POST
// /api/v1/workouts/:id/assignments. Plans the workout on each
// YYYY-MM-DD date (repeats are expanded client-side into
// individual days). Duplicate days are skipped idempotently.
func (h *Handler) APIAddWorkoutAssignments(c echo.Context) error {
	var in AddWorkoutAssignmentsRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	id := c.Param("id")
	created, err := h.workoutsCtrl.AddAssignments(id, claims.UserID, in.Dates)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to schedule workout"})
	}
	return c.JSON(http.StatusCreated, map[string]any{"assignments": WorkoutAssignmentsFromModels(created)})
}

// APIDeleteWorkoutAssignment handles DELETE
// /api/v1/workout-assignments/:id. Removes a single planned day.
// Returns 204, or 404 when the assignment is missing or owned by
// another user.
func (h *Handler) APIDeleteWorkoutAssignment(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if err := h.workoutsCtrl.DeleteAssignment(id, claims.UserID); err != nil {
		if errors.Is(err, controllers.ErrWorkoutAssignmentNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout assignment not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete workout assignment"})
	}
	return c.NoContent(http.StatusNoContent)
}

// APIGetWorkoutSchedule handles GET /api/v1/workouts/schedule.
// Query params from/to are inclusive YYYY-MM-DD dates (both
// required, at most 62 days apart). Used by the iOS week calendar
// so day boundaries stay computed client-side in the user's local
// timezone.
func (h *Handler) APIGetWorkoutSchedule(c echo.Context) error {
	claims := GetClaims(c)
	rawFrom, rawTo := c.QueryParam("from"), c.QueryParam("to")
	if rawFrom == "" || rawTo == "" {
		return c.JSON(http.StatusBadRequest, APIError{Error: "both from and to are required"})
	}
	assignments, err := h.workoutsCtrl.GetSchedule(claims.UserID, rawFrom, rawTo)
	if err != nil {
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout schedule"})
	}
	return c.JSON(http.StatusOK, map[string]any{"assignments": WorkoutAssignmentsFromModels(assignments)})
}
