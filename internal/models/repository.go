package models

import (
	"context"
	"database/sql"
	"time"
)
// Repository defines the interface for exercise data access.
// This abstraction allows handlers to be tested with mock implementations
// without requiring a real database connection.
type Repository interface {
	// Create creates a new exercise or returns the existing ID.
	// If tx is provided, the operation runs within the transaction.
	Create(tx *sql.Tx, name string) (string, error)

	// GetByName retrieves an exercise by its normalized name.
	// Returns nil if not found.
	GetByName(name string) (*Exercise, error)

	// GetExerciseByID retrieves an exercise by its UUID.
	// Returns nil if not found. Scopes to the given user ID.
	GetExerciseByID(id string, userID string) (*Exercise, error)

	// List returns all exercises ordered by name.
	List() ([]Exercise, error)

	// CreateExerciseEntry persists a new exercise entry and links it to its exercise.
	CreateExerciseEntry(exerciseEntry *ExerciseEntry) error

	// GetExerciseEntry retrieves a single exercise entry by ID with its exercise name.
	// Returns nil if not found. Scopes to the given user ID.
	GetExerciseEntry(id string, userID string) (*ExerciseEntry, error)

	// UpdateExerciseEntry updates an existing exercise entry without changing its created_at date.
	// Scopes to the given user ID.
	UpdateExerciseEntry(exerciseEntry *ExerciseEntry, userID string) error

	// UpdateExerciseEntryWithDate updates an existing exercise entry including its created_at date.
	// Scopes to the given user ID.
	UpdateExerciseEntryWithDate(exerciseEntry *ExerciseEntry, userID string) error

	// DeleteExerciseEntry removes an exercise entry by ID. Scopes to the given user ID.
	DeleteExerciseEntry(id string, userID string) error

	// ListExerciseEntries returns exercise entries ordered by created_at descending,
	// breaking ties by insertion order (rowid descending). Scopes to the given user ID.
	// If limit > 0, results are capped at that count.
	ListExerciseEntries(userID string, limit int) ([]ExerciseEntry, error)

	// GetExerciseEntriesByExercisePaginated returns a page of exercise entries for a specific
	// exercise ID, ordered by created_at descending with insertion order (rowid
	// descending) as the tie-breaker. Scopes to the given user ID.
	GetExerciseEntriesByExercisePaginated(exerciseID string, userID string, limit, offset int) ([]ExerciseEntry, error)

	// GetMaxWeightByExercise returns the heaviest weight logged for the given exercise by
	// the given user. Returns 0 when no exercise entries exist. Scopes to the given user ID.
	GetMaxWeightByExercise(exerciseID string, userID string) (float64, error)

	// GetMaxSetVolumeByExercise returns the best single-set volume (reps * weight)
	// logged for the given exercise by the given user. Returns 0 when no exercise
	// entries exist. Scopes to the given user ID.
	GetMaxSetVolumeByExercise(exerciseID string, userID string) (float64, error)

	// GetBestPaceByExercise returns the fastest pace (seconds per kilometre) across the
	// given exercise's exercise entries for the given user. Entries without a positive
	// duration and distance are excluded. Returns 0 when no qualifying exercise entries
	// exist. Scopes to the given user ID.
	GetBestPaceByExercise(exerciseID string, userID string) (float64, error)

	// GetLongestDistanceByExercise returns the longest distance (metres) logged for the
	// given exercise by the given user. Returns 0 when no exercise entries exist.
	// Scopes to the given user ID.
	GetLongestDistanceByExercise(exerciseID string, userID string) (float64, error)

	// GetLastSetByExercise returns the most recent exercise entry for the given exercise by
	// the given user, or sql.ErrNoRows when no exercise entries exist. Ties on
	// created_at are broken by insertion order (rowid descending). Scopes to the given user ID.
	GetLastSetByExercise(exerciseID string, userID string) (*ExerciseEntry, error)

	// GetExerciseEntriesByDateRange returns exercise entries within an inclusive date range.
	// Scopes to the given user ID.
	GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]ExerciseEntry, error)

	// ListExerciseEntriesLast7Days returns exercise entries from the last 7 days ordered by
	// created_at descending with insertion order (rowid descending) as the tie-breaker.
	// Scopes to the given user ID.
	ListExerciseEntriesLast7Days(userID string) ([]ExerciseEntry, error)

	// GetExerciseEntriesByWorkout returns every exercise entry logged
	// into the given workout in logging order (oldest first). Scopes
	// to the given user ID.
	GetExerciseEntriesByWorkout(workoutID string, userID string) ([]ExerciseEntry, error)
}

// WorkoutRepo defines the interface for dated workout data access.
// The controller depends on this so route tests can substitute an
// in-memory fake without touching the real sqlc repository.
type WorkoutRepo interface {
	// CreateWorkoutWithTree persists a new workout with its block/item
	// tree and returns the header. SourceWorkoutID records provenance
	// (nil for workouts authored from scratch); status defaults to
	// planned when empty.
	CreateWorkoutWithTree(userID string, sourceWorkoutID *string, name, notes string, status WorkoutStatus, start, end *time.Time, blocks []WorkoutBlockInput) (*Workout, error)
	// GetWorkout returns a workout header, or nil when not found.
	// Scoped to the user.
	GetWorkout(workoutID, userID string) (*Workout, error)
	// BulkCreate persists every build in a single transaction (all or
	// nothing) and returns the created headers in request order.
	BulkCreate(userID string, builds []WorkoutBuild) ([]Workout, error)
	// GetWorkoutTree returns a workout header with its full tree (nil
	// when not found). Logged exercise entries are assembled by the
	// controller, not the repository.
	GetWorkoutTree(workoutID, userID string) (*Workout, []WorkoutBlockWithItems, error)
	// ListWorkouts returns every workout for the user, planned first.
	ListWorkouts(userID string) ([]Workout, error)
	// ListWorkoutsByRange returns workouts whose scheduled window
	// overlaps the inclusive [start, end] range. Cancelled workouts
	// are excluded so cancelling clears the calendar.
	ListWorkoutsByRange(userID string, start, end time.Time) ([]Workout, error)
	// UpdateWorkoutHeader updates a workout's name, notes, and
	// scheduled window without touching its tree or status.
	UpdateWorkoutHeader(workoutID, userID, name, notes string, start, end *time.Time) error
	// SetWorkoutStatus transitions a workout's status; completedAt is
	// set on complete and cleared on reopen/cancel.
	SetWorkoutStatus(workoutID, userID string, status WorkoutStatus, completedAt *time.Time) error
	// DeleteWorkout removes a workout. Its tree cascades; logged
	// exercise entries survive with their links cleared.
	DeleteWorkout(workoutID, userID string) error
	// GetWorkoutItemContext returns one workout item's parentage for
	// linkage validation. Returns sql.ErrNoRows when missing.
	GetWorkoutItemContext(workoutItemID, userID string) (workoutID, blockID string, err error)
}

// Compile-time check to ensure WorkoutRepository implements WorkoutRepo.
var _ WorkoutRepo = (*WorkoutRepository)(nil)

// UserRepo defines the interface for user data access.
type UserRepo interface {
	CreateUser(user *User) error
	GetUserByEmail(email string) (*User, error)
	GetUserByID(id string) (*User, error)
	UpdateUser(user *User) error
	// UpdateUserPassword replaces a user's password hash. Used by
	// the password-reset flow. Kept separate from UpdateUser so a
	// profile form cannot be tricked into clearing the password.
	UpdateUserPassword(userID, passwordHash string) error
	// UpdateUserReminder writes the user's reminder
	// preferences and the next fire time computed by the
	// route. Kept separate from UpdateUser for the same
	// reason as UpdateUserPassword: a narrow,
	// single-purpose method prevents the wrong form from
	// clobbering reminder state and keeps the SQL UPDATE
	// focused on the columns it actually owns.
	UpdateUserReminder(userID string, prefs ReminderPreferences) error
	// UpdateUserAIPreferences writes the Coach opt-in toggle
	// and free-text aim. Narrow so no other form can clobber
	// AI consent state.
	UpdateUserAIPreferences(userID string, optIn bool, goalText string) error
	// ListAIOptedInUsers returns every user with ai_opt_in =
	// 1. The weekly Coach cron iterates this list.
	ListAIOptedInUsers(ctx context.Context) ([]User, error)
}

// AdminUserRepo defines the interface for admin user operations.
type AdminUserRepo interface {
	ListUsers(ctx context.Context) ([]User, error)
	// GetUserByID retrieves a single user by ID, or nil when the
	// user does not exist. Used by the admin actions (admin toggle,
	// password reset email) to validate the target user before
	// acting, so a stale row from the list page surfaces as a clean
	// not-found instead of a silent no-op.
	GetUserByID(ctx context.Context, id string) (*User, error)
	// SetUserAdmin grants or revokes a user's admin status. Kept
	// separate from the user-facing UpdateUser (which never touches
	// is_admin) so the profile form cannot grant itself admin.
	SetUserAdmin(ctx context.Context, userID string, isAdmin bool) error
}

// Compile-time check to ensure AdminUserRepository implements AdminUserRepo.
var _ AdminUserRepo = (*UserAdminRepository)(nil)

// Compile-time check to ensure ExerciseRepository implements Repository.
var _ Repository = (*ExerciseRepository)(nil)

// FeedbackRepoInterface defines the interface for feedback data access (used by controllers).
type FeedbackRepoInterface interface {
	Create(feedback *Feedback) error
	GetAll(filter string) ([]*Feedback, error)
	GetByID(id string) (*Feedback, error)
	UpdateStatus(id string, isClosed bool) error
}

// WeightRepo defines the interface for weight entry data access.
type WeightRepo interface {
	Create(entry *WeightEntry) error
	GetByID(id string, userID string) (*WeightEntry, error)
	List(userID string) ([]WeightEntry, error)
	Update(entry *WeightEntry, userID string) error
	Delete(id string, userID string) error
	GetByIDs(idA, idB, userID string) ([]WeightEntry, error)
}

// Compile-time check to ensure WeightRepository implements WeightRepo.
var _ WeightRepo = (*WeightRepository)(nil)

// GoalRepo defines the interface for goal data access. The controller
// depends on this so the route tests can substitute an in-memory fake
// without touching the real sqlc repository.
type GoalRepo interface {
	// Create persists a new goal and assigns the generated ID back
	// onto the supplied value.
	Create(g *Goal) error
	// GetByID returns the goal or nil when not found. Scoped to
	// the user.
	GetByID(id, userID string) (*Goal, error)
	// List returns every goal for the user, active first then
	// completed.
	List(userID string) ([]Goal, error)
	// Update overwrites the editable fields (title, description,
	// dates). completed_at is managed by MarkComplete / Reopen.
	Update(g *Goal, userID string) error
	// MarkComplete sets completed_at to the supplied time. No-op
	// when the goal is already complete.
	MarkComplete(id, userID string, completedAt time.Time) error
	// Reopen clears completed_at. No-op when the goal is already active.
	Reopen(id, userID string) error
	// Delete removes a goal. Scoped to the user.
	Delete(id, userID string) error
}

// Compile-time check to ensure GoalRepository implements GoalRepo.
var _ GoalRepo = (*GoalRepository)(nil)

// HealthSnapshotRepo defines the interface for health snapshot
// data access. The controller depends on this so route tests can
// substitute an in-memory fake without touching the real sqlc
// repository.
type HealthSnapshotRepo interface {
	// Upsert inserts a snapshot or replaces the row for the same
	// (user_id, snapshot_date). The generated ID is assigned
	// back onto the supplied value.
	Upsert(entry *HealthSnapshot) error
	// GetByDate returns the snapshot for a device-local calendar
	// date (YYYY-MM-DD), or nil when none exists. Scoped to the
	// user.
	GetByDate(userID, snapshotDate string) (*HealthSnapshot, error)
	// ListRange returns snapshots within an inclusive date
	// range, newest first. Scoped to the user.
	ListRange(userID, startDate, endDate string) ([]HealthSnapshot, error)
}

// Compile-time check to ensure HealthSnapshotRepository implements HealthSnapshotRepo.
var _ HealthSnapshotRepo = (*HealthSnapshotRepository)(nil)
