package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"hylete/internal/db"
)

// UserRepository provides CRUD operations for users.
type UserRepository struct {
	queries *db.Queries
}

// NewUserRepository creates a new user repository.
func NewUserRepository(dbConn *db.DB) *UserRepository {
	return &UserRepository{
		queries: db.New(dbConn.Conn()),
	}
}

// UpdateUserPassword replaces the bcrypt hash for a user. Used by the
// password-reset flow after a reset token has been successfully
// consumed. Kept separate from UpdateUser so the profile-editing
// form cannot be tricked into clearing the password by omitting
// fields. Returns the same wrapped error shape as the other repo
// methods so the controller can compare with errors.Is if needed.
func (r *UserRepository) UpdateUserPassword(userID, passwordHash string) error {
	if userID == "" {
		return fmt.Errorf("failed to update password: user id is empty")
	}
	if passwordHash == "" {
		return fmt.Errorf("failed to update password: hash is empty")
	}
	ctx := context.Background()
	if err := r.queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		PasswordHash: passwordHash,
		UpdatedAt:    sql.NullTime{Time: time.Now(), Valid: true},
		ID:           userID,
	}); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return nil
}

// CreateUser inserts a new user into the database.
func (r *UserRepository) CreateUser(user *User) error {
	ctx := context.Background()
	isAdmin := int64(0)
	if user.IsAdmin {
		isAdmin = 1
	}
	id, err := r.queries.CreateUser(ctx, db.CreateUserParams{
		ID:           uuid.New().String(),
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		IsAdmin:      isAdmin,
	})
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	user.ID = id
	return nil
}

// GetUserByEmail retrieves a user by their email address.
// Returns nil if not found.
func (r *UserRepository) GetUserByEmail(email string) (*User, error) {
	ctx := context.Background()
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return mapUser(row), nil
}

// GetUserByID retrieves a user by their ID.
// Returns nil if not found.
func (r *UserRepository) GetUserByID(id string) (*User, error) {
	ctx := context.Background()
	row, err := r.queries.GetUserByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	return mapUser(row), nil
}

// UpdateUser updates an existing user's name, target weight, weight unit, and
// distance unit.
// The target weight is written as NULL when the user has cleared their goal,
// so the form can be reset by submitting an empty input.
func (r *UserRepository) UpdateUser(user *User) error {
	ctx := context.Background()
	err := r.queries.UpdateUser(ctx, db.UpdateUserParams{
		Name:         user.Name,
		TargetWeight: ptrToNullFloat64(user.TargetWeight),
		WeightUnit:   user.WeightUnit,
		DistanceUnit: user.DistanceUnit,
		UpdatedAt:    sql.NullTime{Time: time.Now(), Valid: true},
		ID:           user.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// UpdateUserReminder writes the user's reminder preferences and the
// next_fire_at computed by the controller. Kept separate from
// UpdateUser for the same reason as UpdateUserPassword: a narrow,
// single-purpose method prevents the wrong form from clobbering
// reminder state and keeps the SQL UPDATE focused on the columns
// it actually owns.
//
// The reminder_day_of_week is written as NULL when the frequency
// does not need it (off / daily) so the row does not carry a
// meaningless 0 that the form would re-render as Sunday.
func (r *UserRepository) UpdateUserReminder(userID string, prefs ReminderPreferences) error {
	if userID == "" {
		return fmt.Errorf("failed to update reminder preferences: user id is empty")
	}
	if !prefs.Frequency.IsValid() {
		return fmt.Errorf("failed to update reminder preferences: frequency %q is invalid", prefs.Frequency)
	}
	ctx := context.Background()
	var nextFire sql.NullTime
	if prefs.NextFireAt != nil {
		nextFire = sql.NullTime{Time: *prefs.NextFireAt, Valid: true}
	}
	var dayOfWeek sql.NullInt64
	if prefs.DayOfWeek != nil {
		dayOfWeek = sql.NullInt64{Int64: int64(*prefs.DayOfWeek), Valid: true}
	}
	// The email channel mirrors the master switch: reminders are
	// email-only, so enabling reminders IS opting into the email.
	// Writing Enabled here also self-heals any legacy
	// reminder_email_enabled=false rows on the next save.
	err := r.queries.UpdateUserReminder(ctx, db.UpdateUserReminderParams{
		ReminderEnabled:      boolToInt(prefs.Enabled),
		ReminderFrequency:    string(prefs.Frequency),
		ReminderDayOfWeek:    dayOfWeek,
		ReminderTime:         prefs.Time,
		ReminderEmailEnabled: boolToInt(prefs.Enabled),
		ReminderNextFireAt:   nextFire,
		ID:                   userID,
	})
	if err != nil {
		return fmt.Errorf("failed to update reminder preferences: %w", err)
	}
	return nil
}

// ListUsersDueForReminder returns every enabled user whose
// next_fire_at is at or before now. The hourly tick calls this
// once per hour; the orchestrator decides who to fire and what
// to send. Defined on UserRepository (rather than a separate
// ReminderRepository) so the admin "list all users" path does
// not have to import two repos for the same table.
func (r *UserRepository) ListUsersDueForReminder(ctx context.Context, now time.Time) ([]User, error) {
	rows, err := r.queries.ListUsersDueForReminder(ctx, sql.NullTime{Time: now, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list users due for reminder: %w", err)
	}
	users := make([]User, len(rows))
	for i, row := range rows {
		users[i] = *mapUser(row)
	}
	return users, nil
}

// MarkUserReminderFired atomically advances the user's next_fire_at
// to the supplied value and stamps last_fired_at. Called by the
// orchestrator after a successful (or partially successful) fire
// for a single user, so a future tick will not pick the same row up
// again until the new next_fire_at passes.
//
// nextFire is the only nullable parameter; lastFired is set to
// time.Now() by the caller (and not by the DB) so the orchestrator
// can pass a clock-injected time in tests. The supplied context is
// used for the underlying sqlc call so caller-driven cancellation
// (e.g. an admin "stop the tick" signal) propagates to the DB.
func (r *UserRepository) MarkUserReminderFired(ctx context.Context, userID string, lastFired, nextFire time.Time) error {
	if userID == "" {
		return fmt.Errorf("failed to mark reminder fired: user id is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err := r.queries.MarkUserReminderFired(ctx, db.MarkUserReminderFiredParams{
		ReminderLastFiredAt: sql.NullTime{Time: lastFired, Valid: true},
		ReminderNextFireAt:  sql.NullTime{Time: nextFire, Valid: true},
		ID:                  userID,
	})
	if err != nil {
		return fmt.Errorf("failed to mark reminder fired: %w", err)
	}
	return nil
}

// ReminderPreferences is the controller-shaped struct the
// /profile form posts. Decoupled from models.User so the form
// layer never accidentally touches unrelated fields and the
// repo method's signature reads as "reminder preferences" at
// a glance.
type ReminderPreferences struct {
	// Enabled is the master switch. When false the orchestrator
	// ignores the user regardless of the other fields; the row
	// is kept in place so the user can flip it back on without
	// re-entering the rest of the form.
	Enabled bool
	// Frequency is one of off | daily | weekly | biweekly.
	Frequency ReminderFrequency
	// DayOfWeek is 0–6 (Sunday=0) for weekly / biweekly; nil
	// for off / daily. The form posts 0–6 as a string and the
	// controller turns it into a *int.
	DayOfWeek *int
	// Time is "HH:00" in 24h UTC. The picker is hour-only by
	// design.
	Time string
	// NextFireAt is the next fire time the controller computed
	// from the user's preferences via User.ComputeNextFire.
	// Written verbatim to the row so the tick picks it up
	// unchanged.
	NextFireAt *time.Time
}

func mapUser(row db.User) *User {
	isAdmin := row.IsAdmin == 1
	return &User{
		ID:                   row.ID,
		Name:                 row.Name,
		Email:                row.Email,
		PasswordHash:         row.PasswordHash,
		IsAdmin:              isAdmin,
		TargetWeight:         nullFloat64ToPtr(row.TargetWeight),
		WeightUnit:           row.WeightUnit,
		DistanceUnit:         row.DistanceUnit,
		ReminderEnabled:      row.ReminderEnabled == 1,
		ReminderFrequency:    ReminderFrequency(row.ReminderFrequency),
		ReminderDayOfWeek:    nullInt64ToIntPtr(row.ReminderDayOfWeek),
		ReminderTime:         row.ReminderTime,
		ReminderNextFireAt:   nullTimeToTimePtr(row.ReminderNextFireAt),
		ReminderLastFiredAt:  nullTimeToTimePtr(row.ReminderLastFiredAt),
		CreatedAt:            nullTimeToTime(row.CreatedAt),
		UpdatedAt:            nullTimeToTime(row.UpdatedAt),
	}
}

// nullFloat64ToPtr converts a sql.NullFloat64 to a *float64. nil for SQL NULL,
// &value otherwise. Lets the rest of the app distinguish "no goal" from "0".
func nullFloat64ToPtr(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	v := nf.Float64
	return &v
}

// ptrToNullFloat64 converts a *float64 to a sql.NullFloat64. nil becomes
// SQL NULL, &value becomes Valid: true.
func ptrToNullFloat64(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

// nullInt64ToIntPtr converts a sql.NullInt64 to a *int. nil for SQL
// NULL, &value otherwise. The mirror of ptrToNullFloat64 for the
// integer fields (currently reminder_day_of_week).
func nullInt64ToIntPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

// boolToInt converts a bool to the 0/1 the SQLite INTEGER columns
// expect. Used for the reminder enabled flags so the SQL stays
// schema-as-written.
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
