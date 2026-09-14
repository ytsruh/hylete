package routes

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// Workout handlers live here (rather than api_v1.go) so the workout
// surface stays in one file. Same conventions as the blocks
// handlers: thin bind → validate → controller → DTO, sentinel
// errors become 400s, ErrWorkoutNotFound becomes a 404.

// SetWorkoutsController attaches the Workouts orchestrator. Kept as a
// setter (rather than NewHandler params) so existing construction
// sites and tests are untouched — same pattern as
// SetBlocksController.
func (h *Handler) SetWorkoutsController(ctrl *controllers.WorkoutsController) {
	h.workoutsCtrl = ctrl
}

// workoutValidationError maps workout controller sentinels to a
// client message. The second return is false for non-validation
// errors (caller responds 500).
func workoutValidationError(err error) (string, bool) {
	switch err {
	case controllers.ErrWorkoutNameRequired:
		return "workout name is required", true
	case controllers.ErrWorkoutNameTooLong:
		return "workout name must be 100 characters or less", true
	case controllers.ErrWorkoutDescriptionLong:
		return "workout description must be 1000 characters or less", true
	case controllers.ErrWorkoutDateRequired:
		return "scheduled date is required", true
	case controllers.ErrWorkoutDateInvalid:
		return "scheduled date must be YYYY-MM-DD", true
	case controllers.ErrWorkoutStatusInvalid:
		return "workout status must be planned, in_progress, completed or skipped", true
	case controllers.ErrWorkoutBlocksRequired:
		return "a workout needs at least 1 block", true
	case controllers.ErrWorkoutBlocksTooMany:
		return "a workout can hold at most 20 blocks", true
	case controllers.ErrWorkoutBlockRequired:
		return "workout block is required", true
	case controllers.ErrWorkoutBlockNotFound:
		return "block not found", true
	case controllers.ErrWorkoutBlockStatusInvalid:
		return "block status must be pending, done or skipped", true
	case controllers.ErrWorkoutRangeInvalid:
		return "from date must not be after to date", true
	case controllers.ErrWorkoutBulkRequired:
		return "at least 1 date is required", true
	case controllers.ErrWorkoutBulkTooMany:
		return "at most 50 workouts can be created per batch", true
	case controllers.ErrWorkoutBulkDuplicateDate:
		return "duplicate dates are not allowed in a batch", true
	}
	return "", false
}

// workoutBlocksToInputs converts request block DTOs into controller
// inputs. Position is implicit (slice order).
func workoutBlocksToInputs(ins []CreateWorkoutBlockRequest) []controllers.WorkoutBlockInput {
	out := make([]controllers.WorkoutBlockInput, 0, len(ins))
	for _, in := range ins {
		out = append(out, controllers.WorkoutBlockInput{BlockID: in.BlockID})
	}
	return out
}

// APIListWorkouts handles GET /api/v1/workouts. Returns the
// authenticated user's workout summaries (newest scheduled date
// first), or the inclusive ?from=&to= (YYYY-MM-DD) range oldest
// first for schedule views like the dashboard calendar.
func (h *Handler) APIListWorkouts(c echo.Context) error {
	claims := GetClaims(c)
	from := c.QueryParam("from")
	to := c.QueryParam("to")
	workouts, err := h.workoutsCtrl.ListWorkouts(claims.UserID, from, to)
	if err != nil {
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workouts"})
	}
	return c.JSON(http.StatusOK, map[string]any{"workouts": WorkoutSummariesFromModels(workouts)})
}

// APICreateWorkout handles POST /api/v1/workouts. Validates the
// body, resolves each block, and returns the created workout with
// blocks.
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
		Name:          in.Name,
		Description:   in.Description,
		ScheduledDate: in.ScheduledDate,
		Status:        models.WorkoutStatus(in.Status),
		Blocks:        workoutBlocksToInputs(in.Blocks),
	})
	if err != nil {
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create workout"})
	}
	return c.JSON(http.StatusCreated, WorkoutFromModel(*created))
}

// APIGetWorkout handles GET /api/v1/workouts/:id. Returns 404 when
// the workout is missing or owned by another user.
//
// ?include=items embeds every block's planned exercises (the Workout
// Player's single-call fetch). Without it the response is the plain
// workout with blocks only — the shape the list, calendar, and detail
// views already consume.
func (h *Handler) APIGetWorkout(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if c.QueryParam("include") == "items" {
		w, err := h.workoutsCtrl.GetWorkoutWithItems(id, claims.UserID)
		if err != nil {
			if errors.Is(err, controllers.ErrWorkoutNotFound) {
				return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
			}
			return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
		}
		return c.JSON(http.StatusOK, WorkoutWithItemsFromModel(*w))
	}

	w, err := h.workoutsCtrl.GetWorkout(id, claims.UserID)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	return c.JSON(http.StatusOK, WorkoutFromModel(*w))
}

// APIListWorkoutExerciseEntries handles GET
// /api/v1/workouts/:id/exercise-entries. Returns every exercise entry
// the user logged against the workout, newest first. The workout must
// exist and belong to the user (404 otherwise) so a guessed ID cannot
// leak another user's sets. Backs player resume ("2 logged" counts).
func (h *Handler) APIListWorkoutExerciseEntries(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if _, err := h.workoutsCtrl.GetWorkout(id, claims.UserID); err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout"})
	}
	entries, err := h.exerciseEntryCtrl.ListExerciseEntriesByWorkout(id, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load workout exercise entries"})
	}
	return c.JSON(http.StatusOK, ExerciseEntriesFromModels(entries))
}

// APIUpdateWorkout handles PUT /api/v1/workouts/:id. Blocks are
// fully replaced with statuses reset to pending (same shape as
// create).
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
		Name:          in.Name,
		Description:   in.Description,
		ScheduledDate: in.ScheduledDate,
		Status:        models.WorkoutStatus(in.Status),
		Blocks:        workoutBlocksToInputs(in.Blocks),
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
	return c.JSON(http.StatusOK, WorkoutFromModel(*updated))
}

// APIDeleteWorkout handles DELETE /api/v1/workouts/:id. Hard delete
// scoped to the authenticated user. Returns 204.
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

// APIUpdateWorkoutBlockStatus handles PATCH
// /api/v1/workouts/:id/blocks/:blockId. Marks one block
// pending/done/skipped and returns the refreshed workout.
func (h *Handler) APIUpdateWorkoutBlockStatus(c echo.Context) error {
	var in UpdateWorkoutBlockStatusRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	updated, err := h.workoutsCtrl.SetWorkoutBlockStatus(
		c.Param("id"), c.Param("blockId"), claims.UserID,
		models.WorkoutBlockStatus(in.Status),
	)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update block status"})
	}
	return c.JSON(http.StatusOK, WorkoutFromModel(*updated))
}

// APIDuplicateWorkout handles POST
// /api/v1/workouts/:id/duplicate. Copies the workout onto a new
// scheduled date (exact name) with block statuses reset to pending.
// Returns 201.
func (h *Handler) APIDuplicateWorkout(c echo.Context) error {
	var in DuplicateWorkoutRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	cp, err := h.workoutsCtrl.DuplicateWorkout(c.Param("id"), claims.UserID, in.ScheduledDate)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to duplicate workout"})
	}
	return c.JSON(http.StatusCreated, WorkoutFromModel(*cp))
}

// blockInUseMessage builds the 409 body when a block cannot be
// deleted because workouts reference it. The count tells the client
// exactly what to show ("used by 3 workouts — remove it from those
// first") so the UI can give clear feedback instead of a generic
// failure.
func blockInUseMessage(useCount int64) string {
	if useCount == 1 {
		return "this block is used by 1 workout — remove it from that workout first"
	}
	return fmt.Sprintf("this block is used by %d workouts — remove it from those workouts first", useCount)
}

// APIDuplicateWorkoutBatch handles POST
// /api/v1/workouts/:id/duplicate-batch. Copies the workout onto
// every listed date (the client's expanded recurrence) in one
// atomic batch — all copies land or none do. Copies keep the
// source's exact name; block statuses reset to pending. Returns 201
// with the created workouts.
func (h *Handler) APIDuplicateWorkoutBatch(c echo.Context) error {
	var in DuplicateWorkoutBatchRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	copies, err := h.workoutsCtrl.DuplicateWorkoutBatch(c.Param("id"), claims.UserID, in.Dates)
	if err != nil {
		if errors.Is(err, controllers.ErrWorkoutNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "workout not found"})
		}
		if msg, ok := workoutValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to duplicate workout"})
	}
	out := make([]WorkoutDTO, 0, len(copies))
	for _, w := range copies {
		out = append(out, WorkoutFromModel(*w))
	}
	return c.JSON(http.StatusCreated, map[string]any{"workouts": out})
}
