package controllers

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"hylete/internal/models"
)

// Sentinel errors for the workout controllers. Not-found sentinels map
// to 404 in the workout routes; validation sentinels map to 400.
var (
	// ErrWorkoutNotFound is returned when a workout ID does not resolve to a row owned by the caller.
	ErrWorkoutNotFound = errors.New("workout not found")
	// ErrWorkoutNameRequired is returned when a workout name is blank.
	ErrWorkoutNameRequired = errors.New("name is required")
	// ErrWorkoutNameTooLong is returned when a workout name exceeds 200 characters.
	ErrWorkoutNameTooLong = errors.New("name must be 200 characters or less")
	// ErrUnknownWorkoutBlockType is returned when a block type is not a known value.
	ErrUnknownWorkoutBlockType = errors.New("unknown workout block type")
	// ErrWorkoutExerciseNotFound is returned when a block item references an exercise that does not exist.
	ErrWorkoutExerciseNotFound = errors.New("exercise not found")
	// ErrWorkoutItemNeedsWorkout is returned when an exercise entry links to a workout item without a workout.
	ErrWorkoutItemNeedsWorkout = errors.New("workout item link requires a workout")
	// ErrWorkoutItemMismatch is returned when an exercise entry's workout item belongs to a different workout.
	ErrWorkoutItemMismatch = errors.New("workout item does not belong to the workout")
	// ErrWorkoutRoundNeedsWorkout is returned when an exercise entry carries a round number without a workout.
	ErrWorkoutRoundNeedsWorkout = errors.New("round number requires a workout")
	// ErrWorkoutRoundInvalid is returned when an exercise entry carries a negative round number.
	ErrWorkoutRoundInvalid = errors.New("round number must not be negative")
)

// maxWorkoutNameLen mirrors the goal title limit (title 1-200) so
// workout names reject the same inputs.
const maxWorkoutNameLen = 200

// validateWorkoutName checks a workout name: required, max 200
// characters (mirrors the goal title contract).
func validateWorkoutName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrWorkoutNameRequired
	}
	if len([]rune(name)) > maxWorkoutNameLen {
		return ErrWorkoutNameTooLong
	}
	return nil
}

// validateBlockTree checks every block and item in a tree write:
// block types must be known with their required params, rounds are
// normalised (straight forces 1), item exercises must exist, and item
// targets are normalised against the linked exercise's type (zeroing
// the non-applicable pair). Targets are optional — an item may be
// just a linked exercise with all-zero targets. Returns the
// normalised tree for persistence.
func validateBlockTree(exercises models.Repository, blocks []models.WorkoutBlockInput) ([]models.WorkoutBlockInput, error) {
	out := make([]models.WorkoutBlockInput, 0, len(blocks))
	for _, b := range blocks {
		if !b.Type.IsValid() {
			return nil, ErrUnknownWorkoutBlockType
		}
		if err := models.ValidateWorkoutBlockParams(b.Type, b.IntervalSeconds, b.TimeCapSeconds); err != nil {
			return nil, err
		}
		nb := b
		nb.Rounds = models.NormalizeWorkoutBlockRounds(b.Type, b.Rounds)
		nb.Items = make([]models.WorkoutItemInput, 0, len(b.Items))
		for _, item := range b.Items {
			exercise, err := exercises.GetExerciseByID(item.ExerciseID, "")
			if err != nil {
				return nil, err
			}
			if exercise == nil {
				return nil, ErrWorkoutExerciseNotFound
			}
			item.Targets = models.NormalizeWorkoutItemTargets(exercise.Type, item.Targets)
			nb.Items = append(nb.Items, item)
		}
		out = append(out, nb)
	}
	return out, nil
}

// WorkoutController orchestrates dated workout CRUD plus the
// exercise-entry linkage validation. Repository dependencies are
// interfaces from models/ so route tests can substitute fakes.
type WorkoutController struct {
	workouts  models.WorkoutRepo
	exercises models.Repository
	entries   models.Repository
}

// NewWorkoutController constructs a WorkoutController.
func NewWorkoutController(workouts models.WorkoutRepo, exercises models.Repository, entries models.Repository) *WorkoutController {
	return &WorkoutController{workouts: workouts, exercises: exercises, entries: entries}
}

// CreateWorkoutInput bundles the fields for a workout create: the
// header plus the block/item tree in order (may be empty for a bare
// shell).
type CreateWorkoutInput struct {
	Name           string
	Notes          string
	ScheduledStart *time.Time
	ScheduledEnd   *time.Time
	Blocks         []models.WorkoutBlockInput
}

// CreateWorkout validates and persists a new workout, returning the
// full detail.
func (wc *WorkoutController) CreateWorkout(userID string, in CreateWorkoutInput) (*models.WorkoutDetail, error) {
	if err := validateWorkoutName(in.Name); err != nil {
		return nil, err
	}
	if err := models.ValidateWorkoutSchedule(in.ScheduledStart, in.ScheduledEnd); err != nil {
		return nil, err
	}
	blocks, err := validateBlockTree(wc.exercises, in.Blocks)
	if err != nil {
		return nil, err
	}
	created, err := wc.workouts.CreateWorkoutWithTree(userID, nil, in.Name, in.Notes, models.WorkoutStatusPlanned, in.ScheduledStart, in.ScheduledEnd, blocks)
	if err != nil {
		return nil, err
	}
	return wc.GetWorkoutDetail(created.ID, userID)
}

// BulkWorkoutInstance is one dated copy in a plan-ahead bulk create.
type BulkWorkoutInstance struct {
	Name           string
	Notes          string
	ScheduledStart *time.Time
	ScheduledEnd   *time.Time
}

// BulkCreateWorkouts snapshots a source workout into one dated copy
// per instance, atomically (all or nothing). Instance names default
// to the source name when blank. Returns ErrWorkoutNotFound when the
// source is missing.
func (wc *WorkoutController) BulkCreateWorkouts(userID, sourceWorkoutID string, instances []BulkWorkoutInstance) ([]models.Workout, error) {
	sourceID := sourceWorkoutID
	source, tree, err := wc.workouts.GetWorkoutTree(sourceID, userID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, ErrWorkoutNotFound
	}
	if len(instances) == 0 {
		return []models.Workout{}, nil
	}
	// Validate everything before writing so a bad instance fails
	// fast without touching the repository (the repo transaction
	// is the second, atomicity-enforcing layer).
	builds := make([]models.WorkoutBuild, 0, len(instances))
	for _, inst := range instances {
		name := inst.Name
		if strings.TrimSpace(name) == "" {
			name = source.Name
		}
		if err := validateWorkoutName(name); err != nil {
			return nil, err
		}
		if err := models.ValidateWorkoutSchedule(inst.ScheduledStart, inst.ScheduledEnd); err != nil {
			return nil, err
		}
		builds = append(builds, models.WorkoutBuild{
			SourceWorkoutID: &sourceID,
			Name:            name,
			Notes:           inst.Notes,
			Status:          models.WorkoutStatusPlanned,
			Start:           inst.ScheduledStart,
			End:             inst.ScheduledEnd,
			Blocks:          workoutTreeToInputs(tree),
		})
	}
	return wc.workouts.BulkCreate(userID, builds)
}

// DuplicateWorkoutInput overrides for a duplicate copy. A blank name
// becomes "Copy of <source>"; nil schedule bounds inherit the
// source's window.
type DuplicateWorkoutInput struct {
	Name           string
	ScheduledStart *time.Time
	ScheduledEnd   *time.Time
}

// DuplicateWorkout copies a workout (including its tree) under a new
// name and returns the full detail. Copies are snapshots: later edits
// to the source never affect the copy. Returns ErrWorkoutNotFound
// when the source is missing.
func (wc *WorkoutController) DuplicateWorkout(workoutID, userID string, in DuplicateWorkoutInput) (*models.WorkoutDetail, error) {
	source, tree, err := wc.workouts.GetWorkoutTree(workoutID, userID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, ErrWorkoutNotFound
	}
	name := in.Name
	if strings.TrimSpace(name) == "" {
		name = "Copy of " + source.Name
	}
	if err := validateWorkoutName(name); err != nil {
		return nil, err
	}
	start := in.ScheduledStart
	if start == nil {
		start = source.ScheduledStart
	}
	end := in.ScheduledEnd
	if end == nil {
		end = source.ScheduledEnd
	}
	if err := models.ValidateWorkoutSchedule(start, end); err != nil {
		return nil, err
	}
	created, err := wc.workouts.CreateWorkoutWithTree(userID, &workoutID, name, source.Notes, models.WorkoutStatusPlanned, start, end, workoutTreeToInputs(tree))
	if err != nil {
		return nil, err
	}
	return wc.GetWorkoutDetail(created.ID, userID)
}

// workoutTreeToInputs converts a workout's persisted tree into block
// inputs for snapshot copies. Values are already validated and
// normalised (they were checked on workout write), so no
// re-validation is needed.
func workoutTreeToInputs(tree []models.WorkoutBlockWithItems) []models.WorkoutBlockInput {
	blocks := make([]models.WorkoutBlockInput, 0, len(tree))
	for _, b := range tree {
		in := models.WorkoutBlockInput{
			Type:                     b.Block.Type,
			Rounds:                   b.Block.Rounds,
			RestBetweenRoundsSeconds: b.Block.RestBetweenRoundsSeconds,
			IntervalSeconds:          b.Block.IntervalSeconds,
			TimeCapSeconds:           b.Block.TimeCapSeconds,
		}
		for _, item := range b.Items {
			in.Items = append(in.Items, models.WorkoutItemInput{
				ExerciseID: item.ExerciseID,
				TargetSets: item.TargetSets,
				Targets: models.WorkoutItemTargets{
					TargetReps:            item.TargetReps,
					TargetWeight:          item.TargetWeight,
					TargetRestSeconds:     item.TargetRestSeconds,
					TargetDurationSeconds: item.TargetDurationSeconds,
					TargetDistanceMeters:  item.TargetDistanceMeters,
					TargetAvgHeartRate:    item.TargetAvgHeartRate,
					TargetCalories:        item.TargetCalories,
				},
			})
		}
		blocks = append(blocks, in)
	}
	return blocks
}

// GetWorkoutDetail returns a workout with its tree and logged exercise
// entries (split into item-linked vs ad-hoc). Returns
// ErrWorkoutNotFound when missing.
func (wc *WorkoutController) GetWorkoutDetail(workoutID, userID string) (*models.WorkoutDetail, error) {
	workout, tree, err := wc.workouts.GetWorkoutTree(workoutID, userID)
	if err != nil {
		return nil, err
	}
	if workout == nil {
		return nil, ErrWorkoutNotFound
	}
	logged, err := wc.entries.GetExerciseEntriesByWorkout(workoutID, userID)
	if err != nil {
		return nil, err
	}
	detail := &models.WorkoutDetail{
		Workout: *workout,
		Blocks:  tree,
	}
	for _, e := range logged {
		if e.WorkoutItemID == "" {
			detail.AdHocExerciseEntries = append(detail.AdHocExerciseEntries, e)
		} else {
			detail.LoggedExerciseEntries = append(detail.LoggedExerciseEntries, e)
		}
	}
	if detail.LoggedExerciseEntries == nil {
		detail.LoggedExerciseEntries = []models.ExerciseEntry{}
	}
	if detail.AdHocExerciseEntries == nil {
		detail.AdHocExerciseEntries = []models.ExerciseEntry{}
	}
	return detail, nil
}

// ListWorkouts returns every workout for the user, planned first.
func (wc *WorkoutController) ListWorkouts(userID string) ([]models.Workout, error) {
	return wc.workouts.ListWorkouts(userID)
}

// ListWorkoutsByRange returns workouts overlapping the inclusive
// [start, end] range. Rejects an inverted range.
func (wc *WorkoutController) ListWorkoutsByRange(userID string, start, end time.Time) ([]models.Workout, error) {
	if end.Before(start) {
		return nil, models.ErrScheduleEndBeforeStart
	}
	return wc.workouts.ListWorkoutsByRange(userID, start, end)
}

// UpdateWorkoutInput replaces a workout's editable header fields. The
// tree and status are intentionally excluded (trees are fixed at
// creation in v1; status moves through Complete/Reopen/Cancel).
type UpdateWorkoutInput struct {
	Name           string
	Notes          string
	ScheduledStart *time.Time
	ScheduledEnd   *time.Time
}

// UpdateWorkout overwrites the header and returns the refreshed
// detail. Returns ErrWorkoutNotFound when missing.
func (wc *WorkoutController) UpdateWorkout(workoutID, userID string, in UpdateWorkoutInput) (*models.WorkoutDetail, error) {
	if err := validateWorkoutName(in.Name); err != nil {
		return nil, err
	}
	if err := models.ValidateWorkoutSchedule(in.ScheduledStart, in.ScheduledEnd); err != nil {
		return nil, err
	}
	existing, err := wc.workouts.GetWorkout(workoutID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrWorkoutNotFound
	}
	if err := wc.workouts.UpdateWorkoutHeader(workoutID, userID, in.Name, in.Notes, in.ScheduledStart, in.ScheduledEnd); err != nil {
		return nil, err
	}
	return wc.GetWorkoutDetail(workoutID, userID)
}

// Complete marks a workout completed (server owns completed_at) and
// returns the refreshed detail. Idempotent on already-completed
// workouts. Returns ErrWorkoutNotFound when missing.
func (wc *WorkoutController) Complete(workoutID, userID string, completedAt time.Time) (*models.WorkoutDetail, error) {
	if err := wc.requireWorkout(workoutID, userID); err != nil {
		return nil, err
	}
	if err := wc.workouts.SetWorkoutStatus(workoutID, userID, models.WorkoutStatusCompleted, &completedAt); err != nil {
		return nil, err
	}
	return wc.GetWorkoutDetail(workoutID, userID)
}

// Reopen moves a workout back to planned and clears completed_at.
// Idempotent on already-planned workouts. Returns ErrWorkoutNotFound
// when missing.
func (wc *WorkoutController) Reopen(workoutID, userID string) (*models.WorkoutDetail, error) {
	if err := wc.requireWorkout(workoutID, userID); err != nil {
		return nil, err
	}
	if err := wc.workouts.SetWorkoutStatus(workoutID, userID, models.WorkoutStatusPlanned, nil); err != nil {
		return nil, err
	}
	return wc.GetWorkoutDetail(workoutID, userID)
}

// Cancel marks a workout cancelled and clears completed_at. Idempotent.
// Returns ErrWorkoutNotFound when missing.
func (wc *WorkoutController) Cancel(workoutID, userID string) (*models.WorkoutDetail, error) {
	if err := wc.requireWorkout(workoutID, userID); err != nil {
		return nil, err
	}
	if err := wc.workouts.SetWorkoutStatus(workoutID, userID, models.WorkoutStatusCancelled, nil); err != nil {
		return nil, err
	}
	return wc.GetWorkoutDetail(workoutID, userID)
}

// DeleteWorkout hard-deletes a workout scoped to the user. Its tree
// cascades; logged exercise entries survive with their links cleared.
// Returns ErrWorkoutNotFound when missing.
func (wc *WorkoutController) DeleteWorkout(workoutID, userID string) error {
	if err := wc.requireWorkout(workoutID, userID); err != nil {
		return err
	}
	return wc.workouts.DeleteWorkout(workoutID, userID)
}

// requireWorkout returns ErrWorkoutNotFound unless the workout exists
// and is owned by the caller.
func (wc *WorkoutController) requireWorkout(workoutID, userID string) error {
	existing, err := wc.workouts.GetWorkout(workoutID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWorkoutNotFound
	}
	return nil
}

// ValidateExerciseEntryLinkage checks an exercise-entry submission's
// optional workout links. Standalone submissions (all empty) pass.
// Callers (the JSON API handlers) must run this before persisting:
// the exercise-entry controller stores the linkage as given.
//
// Rules: an item link requires a workout and the item must belong to
// that workout; a positive round number requires a workout; round
// numbers must not be negative.
func (wc *WorkoutController) ValidateExerciseEntryLinkage(userID string, link models.WorkoutLinkage) error {
	if link.IsStandalone() {
		return nil
	}
	if link.RoundNumber < 0 {
		return ErrWorkoutRoundInvalid
	}
	if link.WorkoutID == "" {
		if link.WorkoutItemID != "" {
			return ErrWorkoutItemNeedsWorkout
		}
		return ErrWorkoutRoundNeedsWorkout
	}
	workout, err := wc.workouts.GetWorkout(link.WorkoutID, userID)
	if err != nil {
		return err
	}
	if workout == nil {
		return ErrWorkoutNotFound
	}
	if link.WorkoutItemID == "" {
		return nil
	}
	itemWorkoutID, _, err := wc.workouts.GetWorkoutItemContext(link.WorkoutItemID, userID)
	if err == sql.ErrNoRows {
		return ErrWorkoutItemMismatch
	}
	if err != nil {
		return err
	}
	if itemWorkoutID != link.WorkoutID {
		return ErrWorkoutItemMismatch
	}
	return nil
}
