// Package routes provides HTTP route handlers for the strength tracker application.
package routes

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/models"
	"hylete/internal/views/profile"
)

// profileInput represents the parsed and validated form data for profile updates.
// TargetWeight is a pointer so an empty form value can be distinguished from a
// real "0" entry and stored as SQL NULL (i.e. "no goal").
type profileInput struct {
	Name         string   `validate:"required,min=2,max=100"`
	TargetWeight *float64 `validate:"omitempty,gte=0,lte=1000"`
	WeightUnit   string   `validate:"omitempty,oneof=kg lbs"`
	DistanceUnit string   `validate:"omitempty,oneof=km mi"`
	// ReminderEnabled is the master switch. The form
	// posts the literal "1" or ""; "" reads as false.
	ReminderEnabled bool
	// ReminderFrequency is the cadence string from the
	// <select>. Validated against the four known values
	// before persistence; an unknown value returns a
	// friendly validation error.
	ReminderFrequency string `validate:"omitempty,oneof=off daily weekly biweekly"`
	// ReminderDayOfWeek is a pointer so the empty form
	// value (off / daily) reads as nil. 0..6 (Sunday=0).
	ReminderDayOfWeek *int `validate:"omitempty,gte=0,lte=6"`
	// ReminderTime is "HH:00" UTC. Validated by
	// parseReminderTime in models/user.go at the
	// repository level (the form posts what the user
	// typed; the controller normalises to "HH:00" before
	// persistence).
	ReminderTime string
}

// defaultWeightUnit is used when the form omits the weight_unit field entirely
// (e.g. on older forms). Matches the SQL DEFAULT in migration 00005.
const defaultWeightUnit = "kg"

// defaultDistanceUnit is used when the form omits the distance_unit field.
// Matches the SQL DEFAULT in migration 00010.
const defaultDistanceUnit = models.DistanceUnitKm

// defaultReminderTime is the hour-of-day used when the form
// omits reminder_time entirely. Matches the SQL DEFAULT in
// migration 00009.
const defaultReminderTime = "09:00"

// Profile renders the authenticated user's profile page.
func (h *Handler) Profile(c echo.Context) error {
	claims := GetClaims(c)

	// GetUser caches the user on the request context, so this is a
	// single DB read. A missing user (shouldn't happen — claims came
	// from a verified JWT) is treated as a user with no goal, the
	// default unit, and the default reminder state.
	user := h.GetUser(c)
	var target *float64
	unit := defaultWeightUnit
	distanceUnit := defaultDistanceUnit
	reminderState := profile.ReminderFormState{
		Frequency: models.ReminderWeekly,
		Time:      defaultReminderTime,
	}
	if user != nil {
		target = user.TargetWeight
		unit = user.WeightUnitDisplay()
		distanceUnit = user.DistanceUnitDisplay()
		// The user's stored preferences seed the form's
		// first-paint state. Frequency falls back to off
		// when the stored value is the empty string (a row
		// that has never been touched by the form) so the
		// picker is not blank.
		reminderState.Enabled = user.ReminderEnabled
		if user.ReminderFrequency != "" {
			reminderState.Frequency = user.ReminderFrequency
		}
		reminderState.DayOfWeek = user.ReminderDayOfWeek
		if user.ReminderTime != "" {
			reminderState.Time = user.ReminderTime
		}
	}

	return render(c, profile.ProfilePage(
		claims.Name,
		claims.Email,
		claims.IsAdmin,
		target,
		unit,
		distanceUnit,
		reminderState,
	))
}

// UpdateProfile handles profile update requests.
// It validates the input, updates the user's name, target weight, weight unit,
// and reminder preferences in a single pass, then regenerates the JWT so the
// new name is reflected in the cookie and re-issues the auth cookie.
//
// Reminder preferences are written via a separate repo call
// (UpdateUserReminder) so the existing UpdateUser method stays
// focused on the name / target / unit trio. The next reminder
// fire is computed from the freshly-validated prefs in this
// handler (rather than by the orchestrator) so the value the
// hourly tick picks up is exactly what the user just saved.
func (h *Handler) UpdateProfile(c echo.Context) error {
	claims := GetClaims(c)

	input := profileInput{
		Name:              c.FormValue("name"),
		WeightUnit:        c.FormValue("weight_unit"),
		DistanceUnit:      c.FormValue("distance_unit"),
		ReminderEnabled:   c.FormValue("reminder_enabled") == "1",
		ReminderFrequency: c.FormValue("reminder_frequency"),
		ReminderTime:      c.FormValue("reminder_time"),
	}
	if input.WeightUnit == "" {
		input.WeightUnit = defaultWeightUnit
	}
	if input.DistanceUnit == "" {
		input.DistanceUnit = defaultDistanceUnit
	}
	if input.ReminderFrequency == "" {
		// Empty form post keeps the existing frequency.
		// The /profile GET path renders the user's stored
		// value, so a fully-empty reminder section means
		// the form was rendered before the user had a
		// stored preference. Defaulting to "off" matches
		// the principle of least surprise.
		input.ReminderFrequency = string(models.ReminderOff)
	}
	if input.ReminderTime == "" {
		input.ReminderTime = defaultReminderTime
	}
	if raw := c.FormValue("reminder_day_of_week"); raw != "" {
		day, err := strconv.Atoi(raw)
		if err != nil {
			return render(c, profile.ProfileUpdateError("Day of week must be a number between 0 (Sunday) and 6 (Saturday)"))
		}
		input.ReminderDayOfWeek = &day
	}

	// target_weight is optional. An empty form value clears the goal.
	if raw := c.FormValue("target_weight"); raw != "" {
		target, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return render(c, profile.ProfileUpdateError("Target weight must be a valid positive number"))
		}
		input.TargetWeight = &target
	}

	if err := h.validator.ValidateStruct(&input); err != nil {
		return render(c, profile.ProfileUpdateError(friendlyValidationError(err)))
	}

	// The reminder_time field is hour-only by design
	// (the form's <input type="time" step="3600"> only
	// emits values on the hour). The validator's tags do
	// not parse "HH:MM" out of the box, so the check
	// lives next to the form parsing. A malformed value
	// (e.g. a hand-rolled POST with "09:30") is rejected
	// with the same friendly error the rest of the form
	// uses, so the user can correct and retry.
	if input.ReminderTime != "" {
		if _, _, err := models.ParseReminderTimeForRoute(input.ReminderTime); err != nil {
			return render(c, profile.ProfileUpdateError("Reminder time must be on the hour (HH:00) in 24h format"))
		}
	}

	user := &models.User{
		ID:           claims.UserID,
		Name:         input.Name,
		Email:        claims.Email,
		IsAdmin:      claims.IsAdmin,
		TargetWeight: input.TargetWeight,
		WeightUnit:   input.WeightUnit,
		DistanceUnit: input.DistanceUnit,
	}

	if err := h.userRepo.UpdateUser(user); err != nil {
		return render(c, profile.ProfileUpdateError("Failed to update profile"))
	}

	// Reminder preferences are written separately so a
	// (future) sub-form could submit just the reminder
	// section without touching the rest of the user row.
	// For now the form posts both, so both writes happen
	// in sequence.
	freq := models.ReminderFrequency(input.ReminderFrequency)
	// Day of week is nil for off / daily so the form
	// does not carry a meaningless 0 forward.
	var dayOfWeek *int
	if freq.NeedsDayOfWeek() {
		dayOfWeek = input.ReminderDayOfWeek
	}
	prefs := models.ReminderPreferences{
		Enabled:   input.ReminderEnabled,
		Frequency: freq,
		DayOfWeek: dayOfWeek,
		Time:      input.ReminderTime,
	}

	// Compute the next fire time so the hourly tick picks
	// it up without re-deriving. ComputeNextFire needs the
	// user's CreatedAt (for biweekly parity) and the
	// freshly-validated prefs. Build a transient User
	// that has both so the same function the model tests
	// exercise is the one we call here.
	now := h.clock.Now()
	var nextFire *time.Time
	if cached := h.GetUser(c); cached != nil {
		cached.ReminderEnabled = prefs.Enabled
		cached.ReminderFrequency = prefs.Frequency
		cached.ReminderDayOfWeek = prefs.DayOfWeek
		cached.ReminderTime = prefs.Time
		if t, ok := cached.ComputeNextFire(now); ok {
			nextFire = &t
		}
	}
	prefs.NextFireAt = nextFire

	if err := h.userRepo.UpdateUserReminder(claims.UserID, prefs); err != nil {
		return render(c, profile.ProfileUpdateError("Failed to update reminder preferences"))
	}

	token, err := h.jwtService.GenerateToken(user.ID, user.Email, user.Name, user.IsAdmin)
	if err != nil {
		return render(c, profile.ProfileUpdateError("Failed to generate token"))
	}
	setAuthCookie(c, token)

	return render(c, profile.ProfileUpdateSuccess())
}
