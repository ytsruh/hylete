package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// Goal represents a single user-owned todo-style record with optional
// start, target, and end dates, and an optional completed_at timestamp.
type Goal struct {
	ID          string
	UserID      string
	Title       string
	Description string
	StartDate   *time.Time
	TargetDate  *time.Time
	EndDate     *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsComplete returns true when the goal has a non-nil CompletedAt.
// Equivalent to checking g.CompletedAt != nil but reads better at the
// call site ("if goal.IsComplete() { ... }").
func (g *Goal) IsComplete() bool {
	return g.CompletedAt != nil
}

// FormattedDate returns the goal's created_at in UK short format
// ("DD/MM/YY"). Mirrors WeightEntry.FormattedDate so date chips
// across the app look consistent.
func (g *Goal) FormattedDate() string {
	return g.CreatedAt.Format("02/01/06")
}

// FormattedCompletedDate returns the completed_at in long UK format
// ("02 Jan 2026"). Used on completed goal cards where there is more
// space than the table-style short date.
func (g *Goal) FormattedCompletedDate() string {
	if g.CompletedAt == nil {
		return ""
	}
	return g.CompletedAt.Format("02 Jan 2006")
}

// DaysUntilTarget returns the number of whole calendar days between
// the given reference time and the goal's TargetDate. Returns nil
// when there is no target date, or when the target is at or before
// the reference (so callers can render "due in N days" only when the
// goal is genuinely in the future). The pointer return lets the
// template use {{ if days }} to suppress the chip entirely.
func (g *Goal) DaysUntilTarget(now time.Time) *int {
	if g.TargetDate == nil {
		return nil
	}
	// Truncate both sides to date boundaries so the difference is in
	// whole calendar days, not 24h periods (which can read as 0 the
	// same morning the goal is due).
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := time.Date(g.TargetDate.Year(), g.TargetDate.Month(), g.TargetDate.Day(), 0, 0, 0, 0, g.TargetDate.Location())
	days := int(end.Sub(start).Hours() / 24)
	if days < 0 {
		return nil
	}
	return &days
}

// GoalRepository provides CRUD operations for goals using sqlc-generated
// queries. All "Get/List/Update/Delete" methods that take a userID
// scope the query to that user so a request cannot read or mutate
// another user's data.
type GoalRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewGoalRepository creates a new goal repository backed by sqlc.
func NewGoalRepository(dbConn *db.DB) *GoalRepository {
	return &GoalRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Create persists a new goal. Sets g.ID to the generated UUID on
// success. Uses time.Now() for created_at and updated_at so the
// repository owns the timestamp (matches WeightRepository.Create).
func (r *GoalRepository) Create(g *Goal) error {
	ctx := context.Background()
	id := uuid.New().String()
	now := time.Now()
	row, err := r.queries.CreateGoal(ctx, db.CreateGoalParams{
		ID:          id,
		UserID:      g.UserID,
		Title:       g.Title,
		Description: stringToNullString(g.Description),
		StartDate:   timePtrToNullTime(g.StartDate),
		TargetDate:  timePtrToNullTime(g.TargetDate),
		EndDate:     timePtrToNullTime(g.EndDate),
		CompletedAt: timePtrToNullTime(g.CompletedAt),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return fmt.Errorf("failed to create goal: %w", err)
	}
	g.ID = row.ID
	g.CreatedAt = row.CreatedAt
	g.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID retrieves a goal by ID scoped to the user. Returns nil if
// not found.
func (r *GoalRepository) GetByID(id, userID string) (*Goal, error) {
	ctx := context.Background()
	row, err := r.queries.GetGoal(ctx, db.GetGoalParams{
		ID:     id,
		UserID: userID,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get goal: %w", err)
	}
	goal := mapGoalRow(row)
	return &goal, nil
}

// List returns every goal belonging to the user, grouped: active
// goals first (ordered by target_date asc with nulls last, then
// created_at asc) followed by completed goals (most recently
// completed first). The grouping is done in Go to keep the SQL
// queries simple and to match the view layer's "active then
// completed" layout.
func (r *GoalRepository) List(userID string) ([]Goal, error) {
	ctx := context.Background()
	active, err := r.queries.ListActiveGoals(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list active goals: %w", err)
	}
	completed, err := r.queries.ListCompletedGoals(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list completed goals: %w", err)
	}
	out := make([]Goal, 0, len(active)+len(completed))
	for _, row := range active {
		out = append(out, mapGoalRow(row))
	}
	for _, row := range completed {
		out = append(out, mapGoalRow(row))
	}
	return out, nil
}

// Update updates a goal's editable fields (title, description, and
// the three optional dates). completed_at is intentionally NOT in
// this signature so the edit form cannot accidentally clear or set
// it; status changes go through MarkComplete / Reopen.
func (r *GoalRepository) Update(g *Goal, userID string) error {
	ctx := context.Background()
	return r.queries.UpdateGoal(ctx, db.UpdateGoalParams{
		Title:       g.Title,
		Description: stringToNullString(g.Description),
		StartDate:   timePtrToNullTime(g.StartDate),
		TargetDate:  timePtrToNullTime(g.TargetDate),
		EndDate:     timePtrToNullTime(g.EndDate),
		ID:          g.ID,
		UserID:      userID,
	})
}

// MarkComplete sets completed_at to the supplied time. No-op if the
// goal is already complete (the SQL UPDATE matches only rows where
// completed_at IS NULL). Scoped to userID so a request cannot
// complete someone else's goal.
func (r *GoalRepository) MarkComplete(id, userID string, completedAt time.Time) error {
	ctx := context.Background()
	return r.queries.MarkGoalComplete(ctx, db.MarkGoalCompleteParams{
		CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
		ID:          id,
		UserID:      userID,
	})
}

// Reopen clears completed_at. No-op if the goal is already active,
// so the route can call it without first checking the current state.
// Scoped to userID for the same reason as MarkComplete.
func (r *GoalRepository) Reopen(id, userID string) error {
	ctx := context.Background()
	return r.queries.ReopenGoal(ctx, db.ReopenGoalParams{
		ID:     id,
		UserID: userID,
	})
}

// Delete removes a goal by ID scoped to the user.
func (r *GoalRepository) Delete(id, userID string) error {
	ctx := context.Background()
	return r.queries.DeleteGoal(ctx, db.DeleteGoalParams{
		ID:     id,
		UserID: userID,
	})
}

// --- Mapping helpers ---

// mapGoalRow converts a sqlc Goal row into a domain Goal with the
// nullable fields unwrapped into pointers. Centralised so every read
// path uses the same mapping.
func mapGoalRow(row db.Goal) Goal {
	return Goal{
		ID:          row.ID,
		UserID:      row.UserID,
		Title:       row.Title,
		Description: nullStringToString(row.Description),
		StartDate:   nullTimeToTimePtr(row.StartDate),
		TargetDate:  nullTimeToTimePtr(row.TargetDate),
		EndDate:     nullTimeToTimePtr(row.EndDate),
		CompletedAt: nullTimeToTimePtr(row.CompletedAt),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

// timePtrToNullTime turns an optional time into a sql.NullTime. A
// nil pointer (or a zero-value time) yields an invalid (NULL) value.
func timePtrToNullTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullTimeToTimePtr is the inverse of timePtrToNullTime: invalid or
// zero values become nil so the rest of the codebase can use a
// straight `if g.TargetDate != nil` check.
func nullTimeToTimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid || nt.Time.IsZero() {
		return nil
	}
	return &nt.Time
}
