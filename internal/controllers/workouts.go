package controllers

import (
	"errors"
	"strings"
	"time"

	"hylete/internal/models"
)

// Workout validation sentinels. Routes map these to 400s (and
// ErrWorkoutNotFound / ErrWorkoutAssignmentNotFound to 404s);
// anything else is a 500.
var (
	ErrWorkoutNotFound            = errors.New("workout not found")
	ErrWorkoutTitleRequired       = errors.New("workout title is required")
	ErrWorkoutTitleTooLong        = errors.New("workout title must be 100 characters or less")
	ErrWorkoutDescriptionLong     = errors.New("workout description must be 1000 characters or less")
	ErrWorkoutBlocksRequired      = errors.New("a workout needs at least 1 block")
	ErrWorkoutBlocksTooMany       = errors.New("a workout can hold at most 20 blocks")
	ErrWorkoutBlockRequired       = errors.New("workout block is required")
	ErrWorkoutBlockNotFound       = errors.New("block not found")
	ErrWorkoutAssignmentNotFound  = errors.New("workout assignment not found")
	ErrWorkoutDateRequired        = errors.New("at least 1 date is required")
	ErrWorkoutDatesTooMany        = errors.New("at most 100 dates per request")
	ErrWorkoutDateInvalid         = errors.New("dates must be YYYY-MM-DD")
	ErrWorkoutScheduleRangeNeeded = errors.New("both from and to are required")
	ErrWorkoutScheduleRangeOrder  = errors.New("from must not be after to")
	ErrWorkoutScheduleRangeSpan   = errors.New("range must not exceed 62 days")
)

// Workout limits.
const (
	WorkoutTitleMaxLen       = 100
	WorkoutDescriptionMaxLen = 1000
	WorkoutBlocksMin         = 1
	WorkoutBlocksMax         = 20
	WorkoutAssignDatesMin    = 1
	WorkoutAssignDatesMax    = 100
	// WorkoutScheduleRangeMaxDays caps the calendar from/to
	// window, mirroring maxExerciseEntryRangeSpan (62 days) on
	// the exercise-entries range endpoint.
	WorkoutScheduleRangeMaxDays = 62
)

// workoutDateLayout is the date-only format for assignments.
// Day boundaries are computed client-side in the user's local
// timezone; the server stores the calendar date as opaque text
// and compares lexicographically (zero-padded, so string order
// matches chronological order).
const workoutDateLayout = "2006-01-02"

// WorkoutBlockLookup resolves a block ID for validation. The
// models BlockRepository implements this; tests substitute a fake.
type WorkoutBlockLookup interface {
	GetByID(id string, userID string) (*models.Block, error)
}

// WorkoutsController orchestrates workout CRUD plus scheduling.
// Depends on the WorkoutRepo interface (models/) and a narrow
// block lookup so route tests can substitute fakes without a
// database.
type WorkoutsController struct {
	repo   models.WorkoutRepo
	blocks WorkoutBlockLookup
}

// NewWorkoutsController constructs a WorkoutsController backed by
// the supplied repositories.
func NewWorkoutsController(repo models.WorkoutRepo, blocks WorkoutBlockLookup) *WorkoutsController {
	return &WorkoutsController{repo: repo, blocks: blocks}
}

// CreateWorkoutInput bundles the editable workout fields for
// create. BlockIDs are ordered (position = slice order); the same
// block ID may repeat.
type CreateWorkoutInput struct {
	Title       string
	Description string
	BlockIDs    []string
}

// UpdateWorkoutInput is the same shape as CreateWorkoutInput.
// Block links are fully replaced on update (simplest correct V1
// semantic, mirroring block items).
type UpdateWorkoutInput struct {
	Title       string
	Description string
	BlockIDs    []string
}

// ListWorkouts returns every workout summary for the user.
func (wc *WorkoutsController) ListWorkouts(userID string) ([]models.WorkoutSummary, error) {
	return wc.repo.List(userID)
}

// GetWorkout fetches a single workout with blocks. Returns
// ErrWorkoutNotFound when missing or owned by another user.
func (wc *WorkoutsController) GetWorkout(id, userID string) (*models.Workout, error) {
	w, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrWorkoutNotFound
	}
	return w, nil
}

// GetWorkoutSchedule returns the planned days for a workout
// (ascending by date). Returns ErrWorkoutNotFound when the
// workout is missing or owned by another user.
func (wc *WorkoutsController) GetWorkoutSchedule(id, userID string) ([]models.WorkoutAssignment, error) {
	if _, err := wc.GetWorkout(id, userID); err != nil {
		return nil, err
	}
	return wc.repo.ListAssignmentsForWorkout(id)
}

// CreateWorkout validates the input, resolves each block (so
// unknown IDs fail before anything is stored), and persists. The
// workout starts unscheduled; days are added via AddAssignments.
func (wc *WorkoutsController) CreateWorkout(userID string, in CreateWorkoutInput) (*models.Workout, error) {
	blocks, err := wc.validateAndResolveBlocks(userID, in.BlockIDs)
	if err != nil {
		return nil, err
	}
	w := &models.Workout{
		UserID:      userID,
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		Blocks:      blocks,
	}
	if err := validateWorkoutFields(w); err != nil {
		return nil, err
	}
	if err := wc.repo.Create(w); err != nil {
		return nil, err
	}
	return w, nil
}

// UpdateWorkout validates, resolves blocks, and overwrites the
// workout plus its block links. Assignments are untouched.
// Returns ErrWorkoutNotFound when missing.
func (wc *WorkoutsController) UpdateWorkout(id, userID string, in UpdateWorkoutInput) (*models.Workout, error) {
	existing, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrWorkoutNotFound
	}
	blocks, err := wc.validateAndResolveBlocks(userID, in.BlockIDs)
	if err != nil {
		return nil, err
	}
	existing.Title = strings.TrimSpace(in.Title)
	existing.Description = strings.TrimSpace(in.Description)
	existing.Blocks = blocks
	if err := validateWorkoutFields(existing); err != nil {
		return nil, err
	}
	if err := wc.repo.Update(existing, userID); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteWorkout hard-deletes a workout, its block links, and its
// assignments, scoped to the user. Returns ErrWorkoutNotFound
// when missing.
func (wc *WorkoutsController) DeleteWorkout(id, userID string) error {
	existing, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWorkoutNotFound
	}
	return wc.repo.Delete(id, userID)
}

// DuplicateWorkout copies a workout (title + " copy") with its
// block links. When copySchedule is true the planned days are
// copied too; otherwise the copy starts unscheduled. Returns
// ErrWorkoutNotFound when the source is missing.
func (wc *WorkoutsController) DuplicateWorkout(id, userID string, copySchedule bool) (*models.Workout, error) {
	dup, err := wc.repo.Duplicate(id, userID, copySchedule)
	if err != nil {
		return nil, err
	}
	if dup == nil {
		return nil, ErrWorkoutNotFound
	}
	return dup, nil
}

// AddAssignments plans the workout on each of the given dates
// (YYYY-MM-DD). Duplicate days are skipped idempotently.
// Returns ErrWorkoutNotFound when the workout is missing.
func (wc *WorkoutsController) AddAssignments(workoutID, userID string, dates []string) ([]models.WorkoutAssignment, error) {
	w, err := wc.GetWorkout(workoutID, userID)
	if err != nil {
		return nil, err
	}
	clean, err := validateWorkoutDates(dates)
	if err != nil {
		return nil, err
	}
	created, err := wc.repo.AddAssignments(workoutID, userID, clean)
	if err != nil {
		return nil, err
	}
	// The insert path carries no title; the workout is known
	// (just gated above), so fill it in for a stable contract
	// with the range endpoint.
	for i := range created {
		created[i].WorkoutTitle = w.Title
	}
	return created, nil
}

// DeleteAssignment removes a single planned day. Returns
// ErrWorkoutAssignmentNotFound when missing or owned by another
// user.
func (wc *WorkoutsController) DeleteAssignment(id, userID string) error {
	existing, err := wc.repo.GetAssignment(id, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWorkoutAssignmentNotFound
	}
	return wc.repo.DeleteAssignment(id, userID)
}

// GetSchedule returns the user's planned days on [from, to]
// (inclusive YYYY-MM-DD) with workout titles resolved, for the
// calendar range query.
func (wc *WorkoutsController) GetSchedule(userID, from, to string) ([]models.WorkoutAssignment, error) {
	start, err := time.Parse(workoutDateLayout, strings.TrimSpace(from))
	if err != nil {
		return nil, ErrWorkoutDateInvalid
	}
	end, err := time.Parse(workoutDateLayout, strings.TrimSpace(to))
	if err != nil {
		return nil, ErrWorkoutDateInvalid
	}
	if end.Before(start) {
		return nil, ErrWorkoutScheduleRangeOrder
	}
	// Day-granular span: end-start in hours can read 0 for two
	// adjacent dates, so count calendar days instead.
	days := int(end.Sub(start).Hours()/24) + 1
	if days > WorkoutScheduleRangeMaxDays {
		return nil, ErrWorkoutScheduleRangeSpan
	}
	return wc.repo.ListAssignmentsByDateRange(userID, start.Format(workoutDateLayout), end.Format(workoutDateLayout))
}

// validateAndResolveBlocks checks the link count, requires a
// block ID per link, and resolves each block so a typo'd ID
// surfaces as ErrWorkoutBlockNotFound instead of a foreign-key
// error. Position is implicit (slice order); repeats are allowed.
func (wc *WorkoutsController) validateAndResolveBlocks(userID string, ids []string) ([]models.WorkoutBlock, error) {
	if len(ids) < WorkoutBlocksMin {
		return nil, ErrWorkoutBlocksRequired
	}
	if len(ids) > WorkoutBlocksMax {
		return nil, ErrWorkoutBlocksTooMany
	}
	out := make([]models.WorkoutBlock, 0, len(ids))
	for i, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, ErrWorkoutBlockRequired
		}
		b, err := wc.blocks.GetByID(id, userID)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, ErrWorkoutBlockNotFound
		}
		out = append(out, models.WorkoutBlock{
			BlockID:   b.ID,
			BlockName: b.Name,
			BlockType: b.Type,
			Position:  i,
		})
	}
	return out, nil
}

// validateWorkoutFields checks the title/description lengths.
// Composition rules live in validateAndResolveBlocks.
func validateWorkoutFields(w *models.Workout) error {
	if w.Title == "" {
		return ErrWorkoutTitleRequired
	}
	if len(w.Title) > WorkoutTitleMaxLen {
		return ErrWorkoutTitleTooLong
	}
	if len(w.Description) > WorkoutDescriptionMaxLen {
		return ErrWorkoutDescriptionLong
	}
	return nil
}

// validateWorkoutDates trims, requires at least one date, caps
// the batch, and rejects anything that is not a real YYYY-MM-DD
// calendar date. Returns normalized dates in input order.
func validateWorkoutDates(dates []string) ([]string, error) {
	if len(dates) < WorkoutAssignDatesMin {
		return nil, ErrWorkoutDateRequired
	}
	if len(dates) > WorkoutAssignDatesMax {
		return nil, ErrWorkoutDatesTooMany
	}
	out := make([]string, 0, len(dates))
	for _, d := range dates {
		d = strings.TrimSpace(d)
		t, err := time.Parse(workoutDateLayout, d)
		if err != nil {
			return nil, ErrWorkoutDateInvalid
		}
		out = append(out, t.Format(workoutDateLayout))
	}
	return out, nil
}
