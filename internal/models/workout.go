package models

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// WorkoutStatus is the overall state of a scheduled workout. It is
// set explicitly by the client and never derived server-side in V1 —
// a completed workout may still hold pending blocks (partial
// completion is allowed).
type WorkoutStatus string

const (
	WorkoutStatusPlanned    WorkoutStatus = "planned"
	WorkoutStatusInProgress WorkoutStatus = "in_progress"
	WorkoutStatusCompleted  WorkoutStatus = "completed"
	WorkoutStatusSkipped    WorkoutStatus = "skipped"
)

// IsValid reports whether the workout status is a known value.
func (s WorkoutStatus) IsValid() bool {
	return slices.Contains([]WorkoutStatus{
		WorkoutStatusPlanned,
		WorkoutStatusInProgress,
		WorkoutStatusCompleted,
		WorkoutStatusSkipped,
	}, s)
}

// WorkoutBlockStatus is the per-block completion state inside a
// workout. Each join row is checked off independently.
type WorkoutBlockStatus string

const (
	WorkoutBlockPending WorkoutBlockStatus = "pending"
	WorkoutBlockDone    WorkoutBlockStatus = "done"
	WorkoutBlockSkipped WorkoutBlockStatus = "skipped"
)

// IsValid reports whether the workout block status is a known value.
func (s WorkoutBlockStatus) IsValid() bool {
	return slices.Contains([]WorkoutBlockStatus{
		WorkoutBlockPending,
		WorkoutBlockDone,
		WorkoutBlockSkipped,
	}, s)
}

// DefaultHealthActivityType is the blanket Apple Health activity
// type for workouts (HKWorkoutActivityType case-name key). New
// feature, accepted risk: every workout records as Strength until
// re-picked in the editor. The server treats the value as opaque
// (length-checked only); the iOS allowlist owns validity.
const DefaultHealthActivityType = "traditionalStrengthTraining"

// HealthActivityTypeMaxLen caps the opaque activity-type key.
const HealthActivityTypeMaxLen = 64

// NormalizeHealthActivityType trims the key and falls back to the
// blanket default when empty, so old clients (which send no key)
// and duplicates of unset rows keep the default instead of
// persisting "".
func NormalizeHealthActivityType(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return DefaultHealthActivityType
	}
	return s
}

// Workout is a user-owned scheduled collection of blocks. The Blocks
// slice holds live references to the block catalog (see
// WorkoutBlock) — editing a block propagates to every workout
// referencing it. ScheduledDate is a device-local calendar date
// (YYYY-MM-DD, same convention as HealthSnapshot.SnapshotDate).
// HealthActivityType is the Apple Health workout type key
// (HKWorkoutActivityType case name); empty normalizes to
// DefaultHealthActivityType on write.
type Workout struct {
	ID                 string
	UserID             string
	Name               string
	Description        string
	ScheduledDate      string
	Status             WorkoutStatus
	HealthActivityType string
	Blocks             []WorkoutBlock
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// WorkoutBlock is a single planned block inside a Workout, with its
// own completion status. BlockName/Type/Description/ItemCount are
// resolved at read time for display; they are never written by the
// client.
type WorkoutBlock struct {
	ID               string
	WorkoutID        string
	BlockID          string
	BlockName        string
	BlockDescription string
	BlockType        BlockType
	Position         int
	Status           WorkoutBlockStatus
	ItemCount        int
	CreatedAt        time.Time
}

// WorkoutSummary is the list-view shape: the workout plus its block
// and done counts so the UI can render "3/5 blocks" without N+1
// queries.
type WorkoutSummary struct {
	Workout
	BlockCount int
	DoneCount  int
}

// WorkoutBlockDetail is one planned block inside a workout with its
// planned exercises resolved. Used by GET /api/v1/workouts/:id
// ?include=items so the Workout Player can render every block and
// row in one call instead of N+1 BlockStore.detail fetches.
//
// Rounds/RestSeconds/TimeCapSeconds/IntervalSeconds mirror the
// catalogue block's time config so the player can show each type's
// time structure (circuit rounds/rest, AMRAP cap, EMOM interval).
// They stay zero when the catalogue block is gone (same tolerance
// as the missing-block branch in GetWorkoutWithItems).
type WorkoutBlockDetail struct {
	WorkoutBlock
	Items           []BlockItem
	Rounds          int
	RestSeconds     int
	TimeCapSeconds  int
	IntervalSeconds int
}

// WorkoutWithItems is a workout with every block's planned items
// resolved. The Blocks slice stays in position order.
type WorkoutWithItems struct {
	Workout
	Blocks []WorkoutBlockDetail
}

// AllBlocksDoneOrSkipped reports whether every block in the workout
// is done or skipped (no pending rows). Backs the auto-complete rule:
// the last block flip to done/skipped completes the workout. An empty
// workout never counts as complete.
func (w *Workout) AllBlocksDoneOrSkipped() bool {
	if len(w.Blocks) == 0 {
		return false
	}
	for _, b := range w.Blocks {
		if b.Status == WorkoutBlockPending {
			return false
		}
	}
	return true
}

// StatusDisplayName returns the user-facing label for the workout
// status.
func (w *Workout) StatusDisplayName() string {
	switch w.Status {
	case WorkoutStatusInProgress:
		return "In Progress"
	case WorkoutStatusCompleted:
		return "Completed"
	case WorkoutStatusSkipped:
		return "Skipped"
	default:
		return "Planned"
	}
}

// WorkoutRepository provides CRUD for workouts using sqlc queries.
// All workout reads/writes scope to the owning user; block reads are
// gated by a prior scoped GetWorkout so a guessed workout ID cannot
// leak another user's schedule.
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

// Create persists a new workout with its blocks. Assigns generated
// IDs back onto the supplied value. Blocks are stored in slice order
// (position = index) with fresh pending statuses. Owns
// created_at/updated_at via time.Now().
func (r *WorkoutRepository) Create(w *Workout) error {
	ctx := context.Background()
	if err := insertWorkout(ctx, r.queries, w); err != nil {
		return err
	}
	if len(w.Blocks) == 0 {
		return nil
	}
	return r.replaceBlocks(ctx, w.ID, w.Blocks, &w.Blocks)
}

// CreateBatch persists several workouts with their blocks in a
// single transaction: either every row lands or none does. Assigns
// generated IDs back onto each supplied value. Backs the recurring
// duplicate endpoint, where a mid-batch failure must not leave a
// partial schedule behind.
func (r *WorkoutRepository) CreateBatch(ws []*Workout) error {
	ctx := context.Background()
	return r.db.Transaction(func(tx *sql.Tx) error {
		q := r.queries.WithTx(tx)
		for _, w := range ws {
			if err := insertWorkout(ctx, q, w); err != nil {
				return err
			}
			if len(w.Blocks) == 0 {
				continue
			}
			fresh, err := insertBlocks(ctx, q, w.ID, w.Blocks)
			if err != nil {
				return err
			}
			w.Blocks = fresh
		}
		return nil
	})
}

// insertWorkout inserts one workout row (without blocks) using the
// supplied queries, assigning the generated ID and timestamps back
// onto w. Callers own the transaction when batching.
func insertWorkout(ctx context.Context, q *db.Queries, w *Workout) error {
	id := uuid.New().String()
	now := time.Now()
	row, err := q.CreateWorkout(ctx, db.CreateWorkoutParams{
		ID:                 id,
		UserID:             w.UserID,
		Name:               w.Name,
		Description:        w.Description,
		ScheduledDate:      w.ScheduledDate,
		Status:             string(w.Status),
		HealthActivityType: NormalizeHealthActivityType(w.HealthActivityType),
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		return fmt.Errorf("failed to create workout: %w", err)
	}
	w.ID = row.ID
	w.CreatedAt = row.CreatedAt
	w.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID returns a workout with its blocks (ordered by position)
// and each block's name/type/item count resolved. Returns nil when
// not found. Scoped to the user.
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
	blocks, err := r.queries.ListWorkoutBlocksWithBlock(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list workout blocks: %w", err)
	}
	w.Blocks = make([]WorkoutBlock, 0, len(blocks))
	for _, b := range blocks {
		w.Blocks = append(w.Blocks, WorkoutBlock{
			ID:               b.ID,
			WorkoutID:        b.WorkoutID,
			BlockID:          b.BlockID,
			BlockName:        b.BlockName,
			BlockDescription: b.BlockDescription,
			BlockType:        BlockType(b.BlockType),
			Position:         int(b.Position),
			Status:           WorkoutBlockStatus(b.Status),
			ItemCount:        int(b.ItemCount),
			CreatedAt:        b.CreatedAt,
		})
	}
	return &w, nil
}

// List returns every workout for the user (newest scheduled date
// first) with block/done counts. Blocks themselves are not loaded —
// use GetByID for detail.
func (r *WorkoutRepository) List(userID string) ([]WorkoutSummary, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkoutsWithBlockCounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workouts: %w", err)
	}
	return mapWorkoutSummaryRows(rows), nil
}

// ListRange returns workouts in an inclusive scheduled-date range
// (YYYY-MM-DD) with block/done counts, oldest first for schedule
// views. Empty bounds are the caller's concern — pass "" only via
// List.
func (r *WorkoutRepository) ListRange(userID, from, to string) ([]WorkoutSummary, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWorkoutsInRangeWithBlockCounts(ctx, db.ListWorkoutsInRangeWithBlockCountsParams{
		UserID:          userID,
		ScheduledDate:   from,
		ScheduledDate_2: to,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workouts in range: %w", err)
	}
	out := make([]WorkoutSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, WorkoutSummary{
			Workout: Workout{
				ID:                 row.ID,
				UserID:             row.UserID,
				Name:               row.Name,
				Description:        row.Description,
				ScheduledDate:      row.ScheduledDate,
				Status:             WorkoutStatus(row.Status),
				HealthActivityType: row.HealthActivityType,
				CreatedAt:          row.CreatedAt,
				UpdatedAt:          row.UpdatedAt,
			},
			BlockCount: int(row.BlockCount),
			DoneCount:  int(row.DoneCount),
		})
	}
	return out, nil
}

// Update overwrites a workout's fields and fully replaces its blocks
// (delete-all + re-insert in one transaction, statuses reset to the
// supplied values). Assigns fresh join IDs back onto w.Blocks.
// Scoped to the user.
func (r *WorkoutRepository) Update(w *Workout, userID string) error {
	ctx := context.Background()
	if err := r.queries.UpdateWorkout(ctx, db.UpdateWorkoutParams{
		Name:               w.Name,
		Description:        w.Description,
		ScheduledDate:      w.ScheduledDate,
		Status:             string(w.Status),
		HealthActivityType: NormalizeHealthActivityType(w.HealthActivityType),
		ID:                 w.ID,
		UserID:             userID,
	}); err != nil {
		return fmt.Errorf("failed to update workout: %w", err)
	}
	return r.replaceBlocks(ctx, w.ID, w.Blocks, &w.Blocks)
}

// SetBlockStatus updates one join row's completion status. The caller
// must have gated the workout via GetByID (scoped to the user).
func (r *WorkoutRepository) SetBlockStatus(workoutID, workoutBlockID string, status WorkoutBlockStatus) error {
	ctx := context.Background()
	return r.queries.UpdateWorkoutBlockStatus(ctx, db.UpdateWorkoutBlockStatusParams{
		Status:    string(status),
		ID:        workoutBlockID,
		WorkoutID: workoutID,
	})
}

// GetWorkoutBlock returns one join row scoped to its workout. The
// caller must have gated the workout via GetByID (scoped to the
// user). Returns nil when not found.
func (r *WorkoutRepository) GetWorkoutBlock(workoutID, workoutBlockID string) (*WorkoutBlock, error) {
	ctx := context.Background()
	row, err := r.queries.GetWorkoutBlock(ctx, db.GetWorkoutBlockParams{ID: workoutBlockID, WorkoutID: workoutID})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workout block: %w", err)
	}
	b := WorkoutBlock{
		ID:        row.ID,
		WorkoutID: row.WorkoutID,
		BlockID:   row.BlockID,
		Position:  int(row.Position),
		Status:    WorkoutBlockStatus(row.Status),
		CreatedAt: row.CreatedAt,
	}
	return &b, nil
}

// CountBlockUsage returns how many of the user's workouts reference
// the given block. Backs the 409 guard on block deletion.
func (r *WorkoutRepository) CountBlockUsage(blockID, userID string) (int64, error) {
	ctx := context.Background()
	n, err := r.queries.CountWorkoutsUsingBlock(ctx, db.CountWorkoutsUsingBlockParams{
		BlockID: blockID,
		UserID:  userID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count block usage: %w", err)
	}
	return n, nil
}

// Delete removes a workout and its blocks (explicit block delete
// first so databases ignoring ON DELETE CASCADE stay correct).
// Logged exercise entries are detached first (workout_id and
// workout_block_id nulled) so history survives the delete.
// Scoped to the user.
func (r *WorkoutRepository) Delete(id, userID string) error {
	ctx := context.Background()
	return r.db.Transaction(func(tx *sql.Tx) error {
		q := r.queries.WithTx(tx)
		if err := q.NullExerciseEntryLinksForWorkout(ctx, db.NullExerciseEntryLinksForWorkoutParams{
			WorkoutID: sql.NullString{String: id, Valid: true},
			UserID:    sql.NullString{String: userID, Valid: true},
		}); err != nil {
			return fmt.Errorf("failed to detach workout exercise entries: %w", err)
		}
		if err := q.DeleteWorkoutBlocks(ctx, id); err != nil {
			return fmt.Errorf("failed to delete workout blocks: %w", err)
		}
		return q.DeleteWorkout(ctx, db.DeleteWorkoutParams{ID: id, UserID: userID})
	})
}

// replaceBlocks deletes every block on the workout then inserts the
// supplied blocks in slice order. Join IDs are regenerated, so linked
// exercise entries lose their precise workout_block_id pointer first
// (nulled explicitly — SQLite FKs default to OFF); their stable
// workout_id + block_id attribution is retained.
func (r *WorkoutRepository) replaceBlocks(ctx context.Context, workoutID string, blocks []WorkoutBlock, out *[]WorkoutBlock) error {
	return r.db.Transaction(func(tx *sql.Tx) error {
		q := r.queries.WithTx(tx)
		if err := q.NullWorkoutBlockLinksForWorkout(ctx, sql.NullString{String: workoutID, Valid: true}); err != nil {
			return err
		}
		if err := q.DeleteWorkoutBlocks(ctx, workoutID); err != nil {
			return err
		}
		fresh, err := insertBlocks(ctx, q, workoutID, blocks)
		if err != nil {
			return err
		}
		*out = fresh
		return nil
	})
}

// MarkCompletedIfBlocksDone sets the workout to completed when every
// block is done or skipped. No-op otherwise. Used by the
// auto-complete rule after a block status flip. Scoped to the user.
func (r *WorkoutRepository) MarkCompletedIfBlocksDone(workoutID, userID string) error {
	ctx := context.Background()
	w, err := r.GetByID(workoutID, userID)
	if err != nil {
		return err
	}
	if w == nil || !w.AllBlocksDoneOrSkipped() || w.Status == WorkoutStatusCompleted {
		return nil
	}
	return r.queries.UpdateWorkoutStatus(ctx, db.UpdateWorkoutStatusParams{
		Status: string(WorkoutStatusCompleted),
		ID:     workoutID,
	})
}

// SetWorkoutStatus overwrites only the workout status (bumps
// updated_at). Unlike Update, it never touches the blocks, so block
// check-offs survive explicit status changes from the player Finish
// button and the detail status picker. Scoped to the user.
func (r *WorkoutRepository) SetWorkoutStatus(workoutID, userID string, status WorkoutStatus) error {
	ctx := context.Background()
	return r.queries.UpdateWorkoutStatusScoped(ctx, db.UpdateWorkoutStatusScopedParams{
		Status: string(status),
		ID:     workoutID,
		UserID: userID,
	})
}

// SetHealthActivityType overwrites only the Apple Health activity
// type (bumps updated_at). Like SetWorkoutStatus it never touches
// the blocks, so the editor type row and detail type picker can
// save without resetting check-offs (which the full-replacement
// Update intentionally resets). Scoped to the user.
func (r *WorkoutRepository) SetHealthActivityType(workoutID, userID, activityType string) error {
	ctx := context.Background()
	return r.queries.UpdateWorkoutHealthActivityTypeScoped(ctx, db.UpdateWorkoutHealthActivityTypeScopedParams{
		HealthActivityType: NormalizeHealthActivityType(activityType),
		ID:                 workoutID,
		UserID:             userID,
	})
}

// SweepStalePlannedWorkouts auto-skips every still-planned workout
// scheduled before today (YYYY-MM-DD) that has zero linked exercise
// entries, and flips each one's still-pending blocks to skipped.
// done/skipped blocks are left untouched; in_progress, completed
// and skipped workouts are never matched (see
// ListStalePlannedWorkoutsWithoutEntries). Block-less workouts are
// included: no blocks plus no exercise entries means nothing was
// done.
//
// Each workout is swept in its own transaction (pending count read,
// workout flip, pending-block flip) so one bad row cannot abort the
// whole tick. today is compared lexically against scheduled_date,
// which is chronological for YYYY-MM-DD. Returns the number of
// workouts flipped and the number of blocks flipped.
func (r *WorkoutRepository) SweepStalePlannedWorkouts(today string) (workoutsSkipped int64, blocksSkipped int64, err error) {
	ctx := context.Background()
	ids, err := r.queries.ListStalePlannedWorkoutsWithoutEntries(ctx, today)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list stale planned workouts: %w", err)
	}
	for _, id := range ids {
		var pending int64
		txErr := r.db.Transaction(func(tx *sql.Tx) error {
			q := r.queries.WithTx(tx)
			n, err := q.CountPendingWorkoutBlocks(ctx, id)
			if err != nil {
				return fmt.Errorf("failed to count pending blocks: %w", err)
			}
			pending = n
			if err := q.UpdateWorkoutStatus(ctx, db.UpdateWorkoutStatusParams{
				Status: string(WorkoutStatusSkipped),
				ID:     id,
			}); err != nil {
				return fmt.Errorf("failed to mark workout skipped: %w", err)
			}
			if err := q.MarkPendingWorkoutBlocksSkipped(ctx, id); err != nil {
				return fmt.Errorf("failed to mark pending blocks skipped: %w", err)
			}
			return nil
		})
		if txErr != nil {
			return workoutsSkipped, blocksSkipped, txErr
		}
		workoutsSkipped++
		blocksSkipped += pending
	}
	return workoutsSkipped, blocksSkipped, nil
}

// MarkInProgressIfPlanned flips a planned workout to in_progress on
// the first linked exercise entry. No-op for any other status.
// Backs the "Start is UI state until data is submitted" rule.
func (r *WorkoutRepository) MarkInProgressIfPlanned(workoutID, userID string) error {
	ctx := context.Background()
	w, err := r.GetByID(workoutID, userID)
	if err != nil {
		return err
	}
	if w == nil || w.Status != WorkoutStatusPlanned {
		return nil
	}
	return r.queries.UpdateWorkoutStatus(ctx, db.UpdateWorkoutStatusParams{
		Status: string(WorkoutStatusInProgress),
		ID:     workoutID,
	})
}

// insertBlocks inserts workout-block join rows in slice order
// (position = index) using the supplied queries, returning the rows
// with regenerated join IDs and resolved display fields carried over
// from the input. Callers own the transaction when batching.
func insertBlocks(ctx context.Context, q *db.Queries, workoutID string, blocks []WorkoutBlock) ([]WorkoutBlock, error) {
	now := time.Now()
	fresh := make([]WorkoutBlock, 0, len(blocks))
	for i, b := range blocks {
		status := string(b.Status)
		if status == "" {
			status = string(WorkoutBlockPending)
		}
		row, err := q.CreateWorkoutBlock(ctx, db.CreateWorkoutBlockParams{
			ID:        uuid.New().String(),
			WorkoutID: workoutID,
			BlockID:   b.BlockID,
			Position:  int64(i),
			Status:    status,
			CreatedAt: now,
		})
		if err != nil {
			return nil, err
		}
		fresh = append(fresh, WorkoutBlock{
			ID:               row.ID,
			WorkoutID:        row.WorkoutID,
			BlockID:          row.BlockID,
			BlockName:        b.BlockName,
			BlockDescription: b.BlockDescription,
			BlockType:        b.BlockType,
			Position:         int(row.Position),
			Status:           WorkoutBlockStatus(row.Status),
			ItemCount:        b.ItemCount,
			CreatedAt:        row.CreatedAt,
		})
	}
	return fresh, nil
}

// mapWorkoutSummaryRows converts ListWorkoutsWithBlockCounts rows
// into domain summaries.
func mapWorkoutSummaryRows(rows []db.ListWorkoutsWithBlockCountsRow) []WorkoutSummary {
	out := make([]WorkoutSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, WorkoutSummary{
			Workout: Workout{
				ID:                 row.ID,
				UserID:             row.UserID,
				Name:               row.Name,
				Description:        row.Description,
				ScheduledDate:      row.ScheduledDate,
				Status:             WorkoutStatus(row.Status),
				HealthActivityType: row.HealthActivityType,
				CreatedAt:          row.CreatedAt,
				UpdatedAt:          row.UpdatedAt,
			},
			BlockCount: int(row.BlockCount),
			DoneCount:  int(row.DoneCount),
		})
	}
	return out
}

// mapWorkoutRow converts a sqlc Workout row into a domain Workout
// without blocks (callers load blocks separately).
func mapWorkoutRow(row db.Workout) Workout {
	return Workout{
		ID:                 row.ID,
		UserID:             row.UserID,
		Name:               row.Name,
		Description:        row.Description,
		ScheduledDate:      row.ScheduledDate,
		Status:             WorkoutStatus(row.Status),
		HealthActivityType: row.HealthActivityType,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}
