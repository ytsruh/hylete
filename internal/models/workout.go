package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// Workout is a user-owned planned session composed of blocks. The
// Blocks slice holds links to the blocks catalog (see WorkoutBlock)
// with an explicit order - not logged exercise entries. Scheduling
// is separate: Assignments holds the calendar days the workout is
// planned for (see WorkoutAssignment). Logging stays manual in V1:
// a workout is purely a plan shown on the calendar.
type Workout struct {
	ID          string
	UserID      string
	Title       string
	Description string
	Blocks      []WorkoutBlock
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkoutBlock is a single block linked into a Workout. BlockName
// and BlockType are resolved at read time for display; they are
// never written by the client. The same block may appear more than
// once in a workout (e.g. a warm-up repeated), so identity is the
// link row ID, not the block ID. Position is the zero-based order
// in the workout.
type WorkoutBlock struct {
	ID        string
	WorkoutID string
	BlockID   string
	BlockName string
	BlockType BlockType
	Position  int
	CreatedAt time.Time
}

// WorkoutSummary is the list-view shape: the workout without its
// blocks plus the block count so the UI can render "3 blocks"
// without N+1 queries.
type WorkoutSummary struct {
	Workout
	BlockCount int
}

// WorkoutAssignment is one planned calendar day for a workout.
// ScheduledDate is date-only text (YYYY-MM-DD), not an instant:
// day boundaries are computed client-side in the user's local
// timezone, mirroring the exercise-entries range endpoint contract.
// WorkoutTitle is resolved at read time for the calendar cell.
type WorkoutAssignment struct {
	ID            string
	WorkoutID     string
	WorkoutTitle  string
	ScheduledDate string
	CreatedAt     time.Time
}

// WorkoutRepository provides CRUD for workouts using sqlc queries.
// All workout reads/writes scope to the owning user; block-link
// reads are gated by a prior scoped GetWorkout so a guessed
// workout ID cannot leak another user's plan. Assignment deletes
// scope to the user via the denormalized user_id column.
type WorkoutRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewWorkoutRepository creates a workout repository backed by sqlc.
func NewWorkoutRepository(dbConn *db.DB) *WorkoutRepository {
	return &WorkoutRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Create persists a new workout with its block links. Assigns
// generated IDs back onto the supplied value. Links are stored in
// slice order (position = index). Owns created_at/updated_at via
// time.Now(). Assignments are managed separately (see
// AddAssignments) so composing a workout never implies scheduling
// it.
func (r *WorkoutRepository) Create(w *Workout) error {
	ctx := context.Background()
	id := uuid.New().String()
	now := time.Now()
	row, err := r.queries.CreateWorkout(ctx, db.CreateWorkoutParams{
		ID:          id,
		UserID:      w.UserID,
		Title:       w.Title,
		Description: w.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return fmt.Errorf("failed to create workout: %w", err)
	}
	w.ID = row.ID
	w.CreatedAt = row.CreatedAt
	w.UpdatedAt = row.UpdatedAt
	if len(w.Blocks) == 0 {
		return nil
	}
	return r.replaceBlocks(ctx, w.ID, w.Blocks, &w.Blocks)
}

// GetByID returns a workout with its blocks (ordered by position)
// and each link's block name/type resolved. Links whose block was
// deleted are gone (CASCADE removed the link row) - the workout
// itself survives. Assignments are not loaded; use
// ListAssignmentsForWorkout for the schedule. Returns nil when not
// found. Scoped to the user.
func (r *WorkoutRepository) GetByID(id, userID string) (*Workout, error) {
	ctx := context.Background()
	row, err := r.queries.GetWorkout(ctx, db.GetWorkoutParams{ID: id, UserID: userID})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workout: %w", err)
	}
	w := mapWorkoutRow(row)
	links, err := r.queries.ListWorkoutBlocksWithBlock(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list workout blocks: %w", err)
	}
	w.Blocks = make([]WorkoutBlock, 0, len(links))
	for _, l := range links {
		w.Blocks = append(w.Blocks, WorkoutBlock{
			ID:        l.ID,
			WorkoutID: l.WorkoutID,
			BlockID:   l.BlockID,
			BlockName: l.BlockName,
			BlockType: BlockType(l.BlockType),
			Position:  int(l.Position),
			CreatedAt: l.CreatedAt,
		})
	}
	return &w, nil
}

// List returns every workout for the user (newest first) with
// block counts. Blocks themselves are not loaded - use GetByID
// for detail.
func (r *WorkoutRepository) List(userID string) ([]WorkoutSummary, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkoutsWithBlockCount(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workouts: %w", err)
	}
	out := make([]WorkoutSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, WorkoutSummary{
			Workout: Workout{
				ID:          row.ID,
				UserID:      row.UserID,
				Title:       row.Title,
				Description: row.Description,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			},
			BlockCount: int(row.BlockCount),
		})
	}
	return out, nil
}

// Update overwrites a workout's fields and fully replaces its
// block links (delete-all + re-insert in one transaction).
// Assigns fresh link IDs back onto w.Blocks. Assignments are
// untouched - rescheduling goes through AddAssignments /
// DeleteAssignment. Scoped to the user.
func (r *WorkoutRepository) Update(w *Workout, userID string) error {
	ctx := context.Background()
	if err := r.queries.UpdateWorkout(ctx, db.UpdateWorkoutParams{
		Title:       w.Title,
		Description: w.Description,
		ID:          w.ID,
		UserID:      userID,
	}); err != nil {
		return fmt.Errorf("failed to update workout: %w", err)
	}
	return r.replaceBlocks(ctx, w.ID, w.Blocks, &w.Blocks)
}

// Delete removes a workout, its block links, and its assignments
// (explicit deletes first so databases ignoring ON DELETE CASCADE
// stay correct). Scoped to the user.
func (r *WorkoutRepository) Delete(id, userID string) error {
	ctx := context.Background()
	if err := r.queries.DeleteWorkoutBlocks(ctx, id); err != nil {
		return fmt.Errorf("failed to delete workout blocks: %w", err)
	}
	if err := r.queries.DeleteWorkoutAssignmentsForWorkout(ctx, id); err != nil {
		return fmt.Errorf("failed to delete workout assignments: %w", err)
	}
	return r.queries.DeleteWorkout(ctx, db.DeleteWorkoutParams{ID: id, UserID: userID})
}

// Duplicate copies a workout's fields and block links into a new
// workout titled "<title> copy". When copySchedule is true the
// assignments are copied too; otherwise the copy starts
// unscheduled. Returns the new workout (with blocks resolved).
// Scoped to the user; returns nil when the source is missing.
func (r *WorkoutRepository) Duplicate(id, userID string, copySchedule bool) (*Workout, error) {
	src, err := r.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, nil
	}
	var dates []string
	if copySchedule {
		ctx := context.Background()
		rows, err := r.queries.ListWorkoutAssignmentsForWorkout(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to list workout assignments: %w", err)
		}
		for _, a := range rows {
			// Only copy this user's own rows; the join is
			// workout-scoped and the workout is already gated
			// to the user, but belt-and-braces costs nothing.
			if a.UserID == userID {
				dates = append(dates, a.ScheduledDate)
			}
		}
	}
	dup := &Workout{
		UserID:      userID,
		Title:       src.Title + " copy",
		Description: src.Description,
		Blocks:      src.Blocks,
	}
	if err := r.Create(dup); err != nil {
		return nil, err
	}
	if len(dates) > 0 {
		if _, err := r.AddAssignments(dup.ID, userID, dates); err != nil {
			return nil, err
		}
	}
	return dup, nil
}

// ListAssignmentsForWorkout returns every planned day for a
// workout (ascending by date). The caller must have gated the
// workout to the user via GetWorkout first.
func (r *WorkoutRepository) ListAssignmentsForWorkout(workoutID string) ([]WorkoutAssignment, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkoutAssignmentsForWorkout(ctx, workoutID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workout assignments: %w", err)
	}
	out := make([]WorkoutAssignment, 0, len(rows))
	for _, a := range rows {
		out = append(out, WorkoutAssignment{
			ID:            a.ID,
			WorkoutID:     a.WorkoutID,
			ScheduledDate: a.ScheduledDate,
			CreatedAt:     a.CreatedAt,
		})
	}
	return out, nil
}

// ListAssignmentsByDateRange returns every planned day for the
// user on [start, end] (inclusive YYYY-MM-DD) with the workout
// title resolved, ordered by date. Powers the calendar range
// query. Scoped to the user.
func (r *WorkoutRepository) ListAssignmentsByDateRange(userID, start, end string) ([]WorkoutAssignment, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkoutAssignmentsByDateRange(ctx, db.ListWorkoutAssignmentsByDateRangeParams{
		UserID:          userID,
		ScheduledDate:   start,
		ScheduledDate_2: end,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workout schedule: %w", err)
	}
	out := make([]WorkoutAssignment, 0, len(rows))
	for _, a := range rows {
		out = append(out, WorkoutAssignment{
			ID:            a.ID,
			WorkoutID:     a.WorkoutID,
			WorkoutTitle:  a.WorkoutTitle,
			ScheduledDate: a.ScheduledDate,
			CreatedAt:     a.CreatedAt,
		})
	}
	return out, nil
}

// AddAssignments plans the workout on each of the given dates
// (YYYY-MM-DD). Duplicate (workout, date) pairs are skipped via
// the UNIQUE guard so repeat-expansion retries stay idempotent.
// Returns the assignments created (skips excluded). The caller
// must have gated the workout to the user first.
func (r *WorkoutRepository) AddAssignments(workoutID, userID string, dates []string) ([]WorkoutAssignment, error) {
	ctx := context.Background()
	now := time.Now()
	out := make([]WorkoutAssignment, 0, len(dates))
	for _, d := range dates {
		row, err := r.queries.CreateWorkoutAssignment(ctx, db.CreateWorkoutAssignmentParams{
			ID:            uuid.New().String(),
			WorkoutID:     workoutID,
			UserID:        userID,
			ScheduledDate: d,
			CreatedAt:     now,
		})
		if err != nil {
			// UNIQUE(workout_id, scheduled_date): a retry or
			// an overlapping repeat expansion re-sending a
			// day is a skip, not a failure.
			if isUniqueViolation(err) {
				continue
			}
			return nil, fmt.Errorf("failed to add workout assignment: %w", err)
		}
		out = append(out, WorkoutAssignment{
			ID:            row.ID,
			WorkoutID:     row.WorkoutID,
			ScheduledDate: row.ScheduledDate,
			CreatedAt:     row.CreatedAt,
		})
	}
	return out, nil
}

// GetAssignment returns a single planned day with its workout
// title resolved. Returns nil when missing. Scoped to the user
// via the denormalized user_id column.
func (r *WorkoutRepository) GetAssignment(id, userID string) (*WorkoutAssignment, error) {
	ctx := context.Background()
	row, err := r.queries.GetWorkoutAssignment(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workout assignment: %w", err)
	}
	if row.UserID != userID {
		return nil, nil
	}
	return &WorkoutAssignment{
		ID:            row.ID,
		WorkoutID:     row.WorkoutID,
		ScheduledDate: row.ScheduledDate,
		CreatedAt:     row.CreatedAt,
	}, nil
}

// DeleteAssignment removes a single planned day. Scoped to the
// user via the denormalized user_id column.
func (r *WorkoutRepository) DeleteAssignment(id, userID string) error {
	ctx := context.Background()
	return r.queries.DeleteWorkoutAssignment(ctx, db.DeleteWorkoutAssignmentParams{ID: id, UserID: userID})
}

// replaceBlocks deletes every link on the workout then inserts
// the supplied blocks in slice order. Link IDs are regenerated.
// The same block ID may appear multiple times (positions
// disambiguate).
func (r *WorkoutRepository) replaceBlocks(ctx context.Context, workoutID string, blocks []WorkoutBlock, out *[]WorkoutBlock) error {
	return r.db.Transaction(func(tx *sql.Tx) error {
		q := r.queries.WithTx(tx)
		if err := q.DeleteWorkoutBlocks(ctx, workoutID); err != nil {
			return err
		}
		now := time.Now()
		fresh := make([]WorkoutBlock, 0, len(blocks))
		for i, b := range blocks {
			row, err := q.CreateWorkoutBlock(ctx, db.CreateWorkoutBlockParams{
				ID:        uuid.New().String(),
				WorkoutID: workoutID,
				BlockID:   b.BlockID,
				Position:  int64(i),
				CreatedAt: now,
			})
			if err != nil {
				return err
			}
			fresh = append(fresh, WorkoutBlock{
				ID:        row.ID,
				WorkoutID: row.WorkoutID,
				BlockID:   row.BlockID,
				BlockName: b.BlockName,
				BlockType: b.BlockType,
				Position:  int(row.Position),
				CreatedAt: row.CreatedAt,
			})
		}
		*out = fresh
		return nil
	})
}

// isUniqueViolation reports whether err is a SQLite UNIQUE
// constraint failure (surfaced through the turso/libsql driver
// with the stock "UNIQUE constraint failed" message). Used to
// turn duplicate (workout, date) assignments into skips.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// mapWorkoutRow converts a sqlc Workout row into a domain Workout
// without blocks (callers load blocks separately).
func mapWorkoutRow(row db.Workout) Workout {
	return Workout{
		ID:          row.ID,
		UserID:      row.UserID,
		Title:       row.Title,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
