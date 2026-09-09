// Package routes: api_reminders.go contains the JSON handlers for the
// user's weight-reminder schedule — the contract the iOS Profile tab's
// ReminderEditView is built against.
//
// Kept separate from api_v1.go (and from PUT /api/v1/me) for the same
// reason UpdateUserReminder is separate from UpdateUser: a narrow,
// single-purpose endpoint prevents a client PUTting /me without
// reminder fields from clobbering the schedule, and keeps the
// validation + next-fire computation next to each other.
package routes

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/models"
)

// APIGetReminderPreferences handles GET /api/v1/me/reminders.
// Returns the authenticated user's weight-reminder schedule. A row
// that has never been touched reads as reminders-off with the
// 09:00 default time (see ReminderPreferencesFromModel), so the iOS
// editor always has a usable first-paint state.
func (h *Handler) APIGetReminderPreferences(c echo.Context) error {
	claims := GetClaims(c)
	user, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load reminder preferences"})
	}
	if user == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
	}
	return c.JSON(http.StatusOK, ReminderPreferencesFromModel(user))
}

// APIUpdateReminderPreferences handles PUT /api/v1/me/reminders.
// Validates the body with the same rules as the web profile form,
// nils the day-of-week for frequencies that don't need it (off /
// daily) so the row never carries a meaningless 0, computes the next
// fire time via User.ComputeNextFire, and persists via
// UpdateUserReminder. Returns the stored preferences so the iOS view
// can render the exact server state without a follow-up GET.
func (h *Handler) APIUpdateReminderPreferences(c echo.Context) error {
	var in UpdateReminderPreferencesRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}
	// The validator's tags do not parse "HH:MM" out of the box, so
	// the hour-only check lives here — the same rule the web form
	// enforces (see UpdateProfile). A hand-rolled PUT with "09:30"
	// is rejected with a friendly error the client can surface.
	if _, _, err := models.ParseReminderTimeForRoute(in.Time); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "Reminder time must be on the hour (HH:00) in 24h format"})
	}

	freq := models.ReminderFrequency(in.Frequency)
	if !freq.IsValid() {
		return c.JSON(http.StatusBadRequest, APIError{Error: "Reminder frequency must be one of off, daily, weekly, biweekly"})
	}
	// Day of week is nil for off / daily so the form does not carry
	// a meaningless 0 forward (mirrors UpdateProfile).
	var dayOfWeek *int
	if freq.NeedsDayOfWeek() {
		dayOfWeek = in.DayOfWeek
	}
	prefs := models.ReminderPreferences{
		Enabled:   in.Enabled,
		Frequency: freq,
		DayOfWeek: dayOfWeek,
		Time:      in.Time,
	}

	// Compute the next fire time so the hourly tick picks it up
	// without re-deriving. ComputeNextFire needs the user's
	// CreatedAt anchor, so build a transient User carrying both the
	// stored CreatedAt and the freshly-validated prefs (the same
	// function the model tests exercise is the one called here).
	now := h.clock.Now()
	user, err := h.userRepo.GetUserByID(GetClaims(c).UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load user"})
	}
	if user == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
	}
	transient := *user
	transient.ReminderEnabled = prefs.Enabled
	transient.ReminderFrequency = prefs.Frequency
	transient.ReminderDayOfWeek = prefs.DayOfWeek
	transient.ReminderTime = prefs.Time
	if t, ok := transient.ComputeNextFire(now); ok {
		nextFire := t.Truncate(time.Second).UTC()
		prefs.NextFireAt = &nextFire
	} else {
		prefs.NextFireAt = nil
	}

	if err := h.userRepo.UpdateUserReminder(GetClaims(c).UserID, prefs); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update reminder preferences"})
	}

	updated, err := h.userRepo.GetUserByID(GetClaims(c).UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load reminder preferences"})
	}
	if updated == nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "user not found after update"})
	}
	return c.JSON(http.StatusOK, ReminderPreferencesFromModel(updated))
}
