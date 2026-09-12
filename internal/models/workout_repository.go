package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// WorkoutBlockInput describes one block (with its prescribed items)
// for a workout tree write. Positions are assigned server-side by
// index, so callers send blocks and items in order.
type WorkoutBlockInput struct {
	Type                     WorkoutBlockType
	Rounds                   int
	RestBetweenRoundsSeconds int
	IntervalSeconds          int
	TimeCapSeconds           int
	Items                    []WorkoutItemInput
}

// WorkoutItemInput describes one prescribed exercise inside a block
// write. ExerciseID must reference the global exercise catalogue;
// TargetSets is how many sets/sessions are prescribed and Targets
// carries the metric goals (validated and normalised against the
// linked exercise's type by the caller).
type WorkoutItemInput struct {
	ExerciseID string
	TargetSets int
	Targets    WorkoutItemTargets
}

// WorkoutRepository provides CRUD operations for dated workouts (plus
// their block/item trees) using sqlc-generated queries. Every method
// that takes a userID scopes to that user.
type WorkoutRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewWorkoutRepository creates a new workout repository backed by sqlc.
func NewWorkoutRepository(dbConn *db.DB) *WorkoutRepository {
	return &WorkoutRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// CreateWorkoutWithTree persists a new workout with its block/item
// tree in one transaction and returns the header. SourceWorkoutID
// records provenance (nil for workouts authored from scratch). Status
// defaults to planned when empty.
func (r *WorkoutRepository) CreateWorkoutWithTree(userID string, sourceWorkoutID *string, name, notes string, status WorkoutStatus, start, end *time.Time, blocks []WorkoutBlockInput) (*Workout, error) {
	ctx := context.Background()
	id := uuid.New().String()
	now := time.Now()
	if status == "" {
		status = WorkoutStatusPlanned
	}
	var sourceRef sql.NullString
	if sourceWorkoutID != nil && *sourceWorkoutID != "" {
		sourceRef = sql.NullString{String: *sourceWorkoutID, Valid: true}
	}
	var created Workout
	err := r.db.Transaction(func(tx *sql.Tx) error {
		qtx := r.queries.WithTx(tx)
		row, err := qtx.CreateWorkout(ctx, db.CreateWorkoutParams{
			ID:               id,
			UserID:           userID,
			SourceWorkoutID:  sourceRef,
			Name:             name,
			Notes:            stringToNullString(notes),
			Status:           string(status),
			ScheduledStart:   timePtrToNullTime(start),
			ScheduledEnd:     timePtrToNullTime(end),
			CompletedAt:      sql.NullTime{},
			CreatedAt:        now,
			UpdatedAt:        now,
		})
		if err != nil {
			return fmt.Errorf("failed to create workout: %w", err)
		}
		if err := insertWorkoutTree(ctx, qtx, id, blocks); err != nil {
			return err
		}
		created = mapWorkoutRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// GetWorkoutTree returns a workout header with its full block/item
// tree in position order (exercise names included), or nil when not
// found. Logged exercise entries are NOT included — the caller
// (controller) fetches those via the exercise-entry repository and
// assembles the WorkoutDetail. Scoped to the user.
func (r *WorkoutRepository) GetWorkoutTree(workoutID, userID string) (*Workout, []WorkoutBlockWithItems, error) {
	ctx := context.Background()
	row, err := r.queries.GetWorkout(ctx, db.GetWorkoutParams{
		ID:     workoutID,
		UserID: userID,
	})
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get workout: %w", err)
	}
	blocks, err := r.queries.ListWorkoutBlocks(ctx, workoutID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list workout blocks: %w", err)
	}
	items, err := r.queries.ListWorkoutItemsByWorkoutWithExercises(ctx, workoutID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list workout items: %w", err)
	}
	workout := mapWorkoutRow(row)
	tree := make([]WorkoutBlockWithItems, 0, len(blocks))
	byBlock := make(map[string][]WorkoutItem, len(blocks))
	for _, item := range items {
		byBlock[item.BlockID] = append(byBlock[item.BlockID], mapWorkoutItemWithExerciseRow(item))
	}
	for _, b := range blocks {
		tree = append(tree, WorkoutBlockWithItems{
			Block: mapWorkoutBlockRow(b),
			Items: byBlock[b.ID],
		})
	}
	return &workout, tree, nil
}

// WorkoutBuild is one workout to materialise inside BulkCreate.
// Blocks carry the already-validated, snapshot tree (copied from a
// source workout or authored inline); SourceWorkoutID records
// provenance.
type WorkoutBuild struct {
	SourceWorkoutID *string
	Name       string
	Notes      string
	Status     WorkoutStatus
	Start      *time.Time
	End        *time.Time
	Blocks     []WorkoutBlockInput
}

// BulkCreate persists every build in a single transaction (all or
// nothing) and returns the created headers in request order. Used by
// plan-ahead flows ("every Monday x N") so a mid-list failure cannot
// leave a partial set of dated copies.
func (r *WorkoutRepository) BulkCreate(userID string, builds []WorkoutBuild) ([]Workout, error) {
	ctx := context.Background()
	now := time.Now()
	created := make([]Workout, 0, len(builds))
	err := r.db.Transaction(func(tx *sql.Tx) error {
		qtx := r.queries.WithTx(tx)
		for _, b := range builds {
			status := b.Status
			if status == "" {
				status = WorkoutStatusPlanned
			}
			var sourceRef sql.NullString
			if b.SourceWorkoutID != nil && *b.SourceWorkoutID != "" {
				sourceRef = sql.NullString{String: *b.SourceWorkoutID, Valid: true}
			}
			row, err := qtx.CreateWorkout(ctx, db.CreateWorkoutParams{
				ID:              uuid.New().String(),
				UserID:          userID,
				SourceWorkoutID: sourceRef,
				Name:           b.Name,
				Notes:          stringToNullString(b.Notes),
				Status:         string(status),
				ScheduledStart: timePtrToNullTime(b.Start),
				ScheduledEnd:   timePtrToNullTime(b.End),
				CompletedAt:    sql.NullTime{},
				CreatedAt:      now,
				UpdatedAt:      now,
			})
			if err != nil {
				return fmt.Errorf("failed to bulk create workout: %w", err)
			}
			if err := insertWorkoutTree(ctx, qtx, row.ID, b.Blocks); err != nil {
				return err
			}
			created = append(created, mapWorkoutRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetWorkout returns a workout header, or nil when not found. Scoped
// to the user. Lighter than GetWorkoutTree for callers that only need
// existence and ownership (e.g. exercise-entry linkage validation).
func (r *WorkoutRepository) GetWorkout(workoutID, userID string) (*Workout, error) {
	ctx := context.Background()
	row, err := r.queries.GetWorkout(ctx, db.GetWorkoutParams{
		ID:     workoutID,
		UserID: userID,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workout: %w", err)
	}
	workout := mapWorkoutRow(row)
	return &workout, nil
}

// ListWorkouts returns every workout belonging to the user, planned
// first (scheduled ascending, unscheduled last) then by recency.
func (r *WorkoutRepository) ListWorkouts(userID string) ([]Workout, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkouts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workouts: %w", err)
	}
	out := make([]Workout, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapWorkoutRow(row))
	}
	return out, nil
}

// ListWorkoutsByRange returns workouts whose scheduled window overlaps
// the inclusive [start, end] range, ordered by scheduled start.
// Unscheduled workouts are excluded. Scoped to the user.
func (r *WorkoutRepository) ListWorkoutsByRange(userID string, start, end time.Time) ([]Workout, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkoutsByRange(ctx, db.ListWorkoutsByRangeParams{
		UserID:         userID,
		ScheduledStart: timeToNullTime(end),
		ScheduledEnd:   timeToNullTime(start),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workouts by range: %w", err)
	}
	out := make([]Workout, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapWorkoutRow(row))
	}
	return out, nil
}

// UpdateWorkoutHeader updates a workout's editable fields (name,
// notes, scheduled window) without touching its tree or status.
// Scoped to the user.
func (r *WorkoutRepository) UpdateWorkoutHeader(workoutID, userID, name, notes string, start, end *time.Time) error {
	ctx := context.Background()
	return r.queries.UpdateWorkout(ctx, db.UpdateWorkoutParams{
		Name:           name,
		Notes:          stringToNullString(notes),
		ScheduledStart: timePtrToNullTime(start),
		ScheduledEnd:   timePtrToNullTime(end),
		ID:             workoutID,
		UserID:         userID,
	})
}

// SetWorkoutStatus transitions a workout's status and manages
// completed_at: set on complete, cleared on reopen/cancel. A nil
// completedAt clears the column. Scoped to the user.
func (r *WorkoutRepository) SetWorkoutStatus(workoutID, userID string, status WorkoutStatus, completedAt *time.Time) error {
	ctx := context.Background()
	return r.queries.SetWorkoutStatus(ctx, db.SetWorkoutStatusParams{
		Status:      string(status),
		CompletedAt: timePtrToNullTime(completedAt),
		ID:          workoutID,
		UserID:      userID,
	})
}

// DeleteWorkout removes a workout by ID scoped to the user. Its
// blocks and items cascade; logged exercise entries survive with
// their workout links cleared (ON DELETE SET NULL).
func (r *WorkoutRepository) DeleteWorkout(workoutID, userID string) error {
	ctx := context.Background()
	return r.queries.DeleteWorkout(ctx, db.DeleteWorkoutParams{
		ID:     workoutID,
		UserID: userID,
	})
}

// GetWorkoutItemContext returns the parentage of one workout item
// (workout ID and block ID) scoped to the user. Returns sql.ErrNoRows
// when the item is missing or owned by another user — the controller
// maps that to a not-found error.
func (r *WorkoutRepository) GetWorkoutItemContext(workoutItemID, userID string) (workoutID, blockID string, err error) {
	ctx := context.Background()
	row, err := r.queries.GetWorkoutItemContext(ctx, db.GetWorkoutItemContextParams{
		ID:     workoutItemID,
		UserID: userID,
	})
	if err != nil {
		return "", "", err
	}
	return row.WorkoutID, row.BlockID, nil
}

// insertWorkoutTree writes blocks and items for a workout inside the
// caller's transaction. Positions are assigned by index.
func insertWorkoutTree(ctx context.Context, qtx *db.Queries, workoutID string, blocks []WorkoutBlockInput) error {
	for i, b := range blocks {
		blockID := uuid.New().String()
		if _, err := qtx.CreateWorkoutBlock(ctx, db.CreateWorkoutBlockParams{
			ID:                       blockID,
			WorkoutID:                workoutID,
			Type:                     string(b.Type),
			Position:                 int64(i),
			Rounds:                   int64(b.Rounds),
			RestBetweenRoundsSeconds: int64(b.RestBetweenRoundsSeconds),
			IntervalSeconds:          int64(b.IntervalSeconds),
			TimeCapSeconds:           int64(b.TimeCapSeconds),
		}); err != nil {
			return fmt.Errorf("failed to create workout block: %w", err)
		}
		for j, item := range b.Items {
			if _, err := qtx.CreateWorkoutItem(ctx, db.CreateWorkoutItemParams{
				ID:                    uuid.New().String(),
				BlockID:               blockID,
				ExerciseID:            item.ExerciseID,
				Position:              int64(j),
				TargetSets:            int64(item.TargetSets),
				TargetReps:            int64(item.Targets.TargetReps),
				TargetWeight:          item.Targets.TargetWeight,
				TargetRestSeconds:     int64(item.Targets.TargetRestSeconds),
				TargetDurationSeconds: int64(item.Targets.TargetDurationSeconds),
				TargetDistanceMeters:  item.Targets.TargetDistanceMeters,
				TargetAvgHeartRate:    int64(item.Targets.TargetAvgHeartRate),
				TargetCalories:        item.Targets.TargetCalories,
			}); err != nil {
				return fmt.Errorf("failed to create workout item: %w", err)
			}
		}
	}
	return nil
}

// --- Mapping helpers ---

func mapWorkoutRow(row db.Workout) Workout {
	var sourceWorkoutID *string
	if row.SourceWorkoutID.Valid && row.SourceWorkoutID.String != "" {
		id := row.SourceWorkoutID.String
		sourceWorkoutID = &id
	}
	return Workout{
		ID:              row.ID,
		UserID:          row.UserID,
		SourceWorkoutID: sourceWorkoutID,
		Name:           row.Name,
		Notes:          nullStringToString(row.Notes),
		Status:         WorkoutStatus(row.Status),
		ScheduledStart: nullTimeToTimePtr(row.ScheduledStart),
		ScheduledEnd:   nullTimeToTimePtr(row.ScheduledEnd),
		CompletedAt:    nullTimeToTimePtr(row.CompletedAt),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func mapWorkoutBlockRow(row db.WorkoutBlock) WorkoutBlock {
	return WorkoutBlock{
		ID:                       row.ID,
		WorkoutID:                row.WorkoutID,
		Type:                     WorkoutBlockType(row.Type),
		Position:                 int(row.Position),
		Rounds:                   int(row.Rounds),
		RestBetweenRoundsSeconds: int(row.RestBetweenRoundsSeconds),
		IntervalSeconds:          int(row.IntervalSeconds),
		TimeCapSeconds:           int(row.TimeCapSeconds),
	}
}

func mapWorkoutItemWithExerciseRow(row db.ListWorkoutItemsByWorkoutWithExercisesRow) WorkoutItem {
	return WorkoutItem{
		ID:                    row.ID,
		BlockID:               row.BlockID,
		ExerciseID:            row.ExerciseID,
		ExerciseName:          row.ExerciseName,
		ExerciseType:          ExerciseType(row.ExerciseType),
		Position:              int(row.Position),
		TargetSets:            int(row.TargetSets),
		TargetReps:            int(row.TargetReps),
		TargetWeight:          row.TargetWeight,
		TargetRestSeconds:     int(row.TargetRestSeconds),
		TargetDurationSeconds: int(row.TargetDurationSeconds),
		TargetDistanceMeters:  row.TargetDistanceMeters,
		TargetAvgHeartRate:    int(row.TargetAvgHeartRate),
		TargetCalories:        row.TargetCalories,
	}
}
