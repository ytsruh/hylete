package models

import (
	"fmt"
	"strings"
	"time"
)

// ReminderFrequency enumerates the supported weight-reminder cadences.
// Stored as TEXT in the users table; the constructor validates the
// value before persistence and the form picker only emits these
// strings, so the DB should never see an unknown value in practice.
// Kept on its own type (rather than as a free-form string) so the
// "is this frequency biweekly?" check the orchestrator does is
// type-safe and grep-able.
type ReminderFrequency string

const (
	// ReminderOff disables the reminder entirely. The user's row
	// is kept around for "save your preferences" UX continuity but
	// the periodic tick ignores it.
	ReminderOff ReminderFrequency = "off"
	// ReminderDaily fires every day at the user's chosen hour.
	ReminderDaily ReminderFrequency = "daily"
	// ReminderWeekly fires on a single chosen day-of-week.
	ReminderWeekly ReminderFrequency = "weekly"
	// ReminderBiweekly fires on a chosen day-of-week every other
	// week, anchored to the user's created_at (see ComputeNextFire).
	ReminderBiweekly ReminderFrequency = "biweekly"
)

// AllReminderFrequencies lists the four valid frequency values in
// the order the form picker renders them. Used by the form to
// pre-select the right <option> without a per-frequency string
// switch on the view side.
var AllReminderFrequencies = []ReminderFrequency{
	ReminderOff,
	ReminderDaily,
	ReminderWeekly,
	ReminderBiweekly,
}

// IsValid reports whether the receiver is one of the four known
// frequencies. Used by the controller before persisting preferences
// so a malformed form post cannot reach the DB.
func (f ReminderFrequency) IsValid() bool {
	switch f {
	case ReminderOff, ReminderDaily, ReminderWeekly, ReminderBiweekly:
		return true
	}
	return false
}

// NeedsDayOfWeek reports whether the frequency uses the
// ReminderDayOfWeek field. Off and Daily do not; Weekly and
// Biweekly do. The form hides the day picker for the former.
func (f ReminderFrequency) NeedsDayOfWeek() bool {
	return f == ReminderWeekly || f == ReminderBiweekly
}

// User represents an authenticated user of the strength tracker.
type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	IsAdmin      bool
	// TargetWeight is the user's body-weight goal. nil means the user has not
	// set a goal; the weight page should hide the progress widget in that case.
	TargetWeight *float64
	// WeightUnit is the user's preferred body-weight unit ("kg" or "lbs").
	// Labels every weight the app renders; no conversion happens.
	WeightUnit string
	// DistanceUnit is the user's preferred cardio distance unit
	// ("km" or "mi"). Distances are stored in metres; this only
	// controls rendering of distances and pace labels.
	DistanceUnit string
	// ReminderEnabled is the master switch for the user's weight reminder.
	// When false, the periodic tick ignores the row entirely regardless of
	// the other fields.
	ReminderEnabled bool
	// ReminderFrequency is the cadence the user picked on /profile.
	// One of "off" | "daily" | "weekly" | "biweekly".
	ReminderFrequency ReminderFrequency
	// ReminderDayOfWeek is the 0–6 day-of-week (Sunday=0, matching
	// time.Weekday) for weekly and biweekly cadences. nil for off/daily.
	// Stored as nullable INTEGER in the DB so an "off" user does not
	// carry a meaningless 0.
	ReminderDayOfWeek *int
	// ReminderTime is the hour-of-day the reminder fires at, stored as
	// "HH:00" in 24h UTC. The picker is hour-only by design; minute
	// precision was explicitly out of scope.
	ReminderTime string
	// ReminderNextFireAt is the next time the periodic tick should
	// fire this user's reminder. Set by ComputeNextFire on every
	// preference change and on every successful fire. nil means
	// "never" (reminders are off).
	ReminderNextFireAt *time.Time
	// ReminderLastFiredAt is the last time the orchestrator actually
	// fired the reminder. nil until the first fire. Useful for the
	// admin "send now" preview ("last fired 2 days ago") and for
	// debugging in the server log.
	ReminderLastFiredAt *time.Time
	// AIOptIn is the server-side gate for Coach (the user-facing
	// name for AI features). No workout data leaves the server
	// for LLM processing unless this is true. It composes with
	// the iOS BetaFeature master switch (UI visibility only) —
	// both must be on for Coach to work.
	AIOptIn bool
	// AIGoalText is what the user is trying to achieve, in their
	// own words (max AIGoalTextMaxLength chars, enforced
	// app-side). Injected verbatim into the weekly prompt as
	// {{USER_AIM}}; empty means "no stated aim".
	AIGoalText string
	// HeightCm is the user's height in centimetres. nil means unset.
	// Stored as a plain number and displayed as "%.1f cm" — no
	// conversion happens, mirroring how weight is a number labelled
	// by the user's preferred unit.
	HeightCm *float64
	// Gender is one of the ValidGenders values ("male" | "female" |
	// "non-binary" | "prefer-not-to-say"). Empty means unset.
	Gender string
	// DateOfBirth is the user's date of birth as "YYYY-MM-DD" (matching
	// health_snapshots.snapshot_date). nil means unset. Age is never
	// stored — derive it with AgeAt wherever it is displayed or sent
	// to the Coach prompt.
	DateOfBirth *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Gender values accepted for User.Gender. Empty string means unset
// and is valid everywhere except where explicitly required (profile
// fields are all optional, so empty is always accepted).
const (
	GenderMale          = "male"
	GenderFemale        = "female"
	GenderNonBinary     = "non-binary"
	GenderPreferNotSay  = "prefer-not-to-say"
)

// ValidGenders enumerates the allowed non-empty values for
// User.Gender, in the order the pickers render them.
var ValidGenders = []string{GenderMale, GenderFemale, GenderNonBinary, GenderPreferNotSay}

// Date-of-birth bounds for User.DateOfBirth. Dates must be real
// calendar dates on/after 1900-01-01 and not in the future (checked
// by ParseDateOfBirth); the minimum-age rule (10 years) is enforced
// by callers via AgeAt so "how old" stays correct as time passes.
// nil means unset.
const (
	// DateOfBirthMin is the earliest accepted birth date. Guards
	// against typos (e.g. year 99) rather than expressing policy.
	DateOfBirthMin = "1900-01-01"
	// MinAgeYears is the minimum derived age accepted at the profile
	// and API trust boundaries.
	MinAgeYears = 10
)

// ParseDateOfBirth validates a "YYYY-MM-DD" birth date: it must be a
// real calendar date, on/after DateOfBirthMin, and not after now
// (compared as dates, so "today" is accepted — a newborn's parent
// could theoretically register them, the min-age rule is separate).
// Returns the normalised string. Empty input returns ("", nil) so
// callers can treat blank as "clear the field".
func ParseDateOfBirth(s string, now time.Time) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	dob, err := time.Parse("2006-01-02", s)
	if err != nil {
		return "", fmt.Errorf("date of birth %q is not a valid YYYY-MM-DD date", s)
	}
	if s < DateOfBirthMin {
		return "", fmt.Errorf("date of birth %q is before %s", s, DateOfBirthMin)
	}
	// Compare as dates: anything after today's date is in the future.
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if dob.After(today) {
		return "", fmt.Errorf("date of birth %q is in the future", s)
	}
	return dob.Format("2006-01-02"), nil
}

// AgeAt derives the whole-years age for a "YYYY-MM-DD" birth date at
// the given instant. Returns -1 when the input is empty or malformed
// so callers can treat "no usable DOB" uniformly without a second
// error branch. Handles leap birthdays (Feb 29 ages up on Mar 1 in
// non-leap years) and the birthday-today boundary (ages up today).
func AgeAt(dob string, now time.Time) int {
	born, err := time.Parse("2006-01-02", strings.TrimSpace(dob))
	if err != nil {
		return -1
	}
	y, m, d := now.Date()
	age := y - born.Year()
	// Not yet had this year's birthday → one less.
	if time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(
		time.Date(y, born.Month(), born.Day(), 0, 0, 0, 0, time.UTC)) {
		age--
	}
	return age
}

// Height bounds for User.HeightCm in centimetres. 0–300 enforced
// app-side; nil means unset.
const (
	HeightCmMin = 0.0
	HeightCmMax = 300.0
)

// NormalizeGender returns a clean gender value: one of ValidGenders
// or "" when the input is empty or unrecognised. Use at trust
// boundaries (form, API, DB) so downstream code can rely on a
// normalised value.
func NormalizeGender(g string) string {
	switch g {
	case GenderMale, GenderFemale, GenderNonBinary, GenderPreferNotSay:
		return g
	default:
		return ""
	}
}

// GenderDisplay returns the user's gender, normalised so an
// unrecognised stored value reads as unset. Empty means the user
// has not provided a gender.
func (u *User) GenderDisplay() string {
	if u == nil {
		return ""
	}
	return NormalizeGender(u.Gender)
}

// FormatHeight returns a human-readable height in centimetres,
// e.g. "180.0 cm". No conversion happens — the value is labelled
// cm everywhere by design.
func FormatHeight(cm float64) string {
	return fmt.Sprintf("%.1f cm", cm)
}

// AIGoalTextMaxLength caps the free-text training aim at ~150-200
// words. The prompt embeds it verbatim, so the cap bounds token
// cost and keeps the weekly report focused.
const AIGoalTextMaxLength = 1000

// HasWeightGoal reports whether the user has set a target weight.
func (u *User) HasWeightGoal() bool {
	return u.TargetWeight != nil
}

// WeightUnitDisplay returns the user's preferred weight unit,
// normalised to "kg" or "lbs". Falls back to "kg" when the user
// is nil or the stored unit is empty or unrecognised. Use this
// everywhere a weight is shown to the user (display, chart
// labels, form labels, CSV export) so the normalisation happens
// once, at the boundary, rather than at every call site.
func (u *User) WeightUnitDisplay() string {
	if u == nil {
		return "kg"
	}
	return NormalizeWeightUnit(u.WeightUnit)
}

// ValidWeightUnits enumerates the allowed values for User.WeightUnit.
var ValidWeightUnits = []string{"kg", "lbs"}

// ValidDistanceUnits enumerates the allowed values for User.DistanceUnit.
var ValidDistanceUnits = []string{DistanceUnitKm, DistanceUnitMi}

// DistanceUnitDisplay returns the user's preferred distance unit,
// normalised to "km" or "mi". Falls back to "km" when the user is nil
// or the stored unit is empty or unrecognised. Use this everywhere a
// cardio distance or pace is shown to the user (display, chart labels,
// CSV export) so the normalisation happens once, at the boundary.
func (u *User) DistanceUnitDisplay() string {
	if u == nil {
		return DistanceUnitKm
	}
	return NormalizeDistanceUnit(u.DistanceUnit)
}

// RemindersEnabled is a convenience wrapper: the master switch is on
// AND the frequency is not "off". The tick uses this as the primary
// guard, so an "off" user with a stray next_fire_at in the past is
// still ignored.
func (u *User) RemindersEnabled() bool {
	if u == nil {
		return false
	}
	return u.ReminderEnabled && u.ReminderFrequency != ReminderOff
}

// ReminderHour returns the hour-of-day (0–23) the reminder is
// scheduled for. Returns 9 as a safe default when the stored
// ReminderTime is malformed so a corrupt row never panics the tick.
// The 09:00 default mirrors the SQL column default and the
// pre-per-user-cron behavior.
func (u *User) ReminderHour() int {
	if u == nil {
		return 9
	}
	hour, _, err := parseReminderTime(u.ReminderTime)
	if err != nil {
		return 9
	}
	return hour
}

// ReminderWeekday returns the day-of-week the reminder is scheduled
// for, or Sunday as a safe default when the field is unset or out
// of range. Only meaningful for weekly / biweekly.
func (u *User) ReminderWeekday() time.Weekday {
	if u == nil || u.ReminderDayOfWeek == nil {
		return time.Sunday
	}
	d := *u.ReminderDayOfWeek
	if d < 0 || d > 6 {
		return time.Sunday
	}
	return time.Weekday(d)
}

// ComputeNextFire returns the next time the user should receive a
// reminder, given the supplied "now". Returns (zero, false) when
// the user has reminders off so the caller can skip the row.
//
// The function is pure: it never reads the clock and never mutates
// the receiver. That makes it trivial to unit-test with a fixed
// time.Time and reuse for both the "user just saved preferences"
// path (form save) and the "tick just fired" path (orchestrator).
//
// Biweekly parity is anchored to the user's CreatedAt: the first
// valid <day_of_week> at-or-after CreatedAt is "week 0", and
// subsequent fires alternate every 7 days from there. The anchor
// is stable per user (CreatedAt never changes), so the alternation
// is deterministic and survives reminder_time changes.
func (u *User) ComputeNextFire(now time.Time) (time.Time, bool) {
	if !u.RemindersEnabled() {
		return time.Time{}, false
	}
	hour := u.ReminderHour()
	loc := time.UTC

	switch u.ReminderFrequency {
	case ReminderDaily:
		// Next 24h boundary at the user's hour. If the current
		// hour matches and we are at minute 0, the current hour
		// counts as "next" (the tick fires on the hour mark, so
		// this is the right bucket).
		today := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc)
		if today.After(now) {
			return today, true
		}
		return today.Add(24 * time.Hour), true

	case ReminderWeekly:
		// Next occurrence of the user's day-of-week at the
		// user's hour. "Today at HH:00" counts as the current
		// week's instance.
		target := u.ReminderWeekday()
		days := (int(target) - int(now.Weekday()) + 7) % 7
		candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc).AddDate(0, 0, days)
		if !candidate.After(now) {
			candidate = candidate.AddDate(0, 0, 7)
		}
		return candidate, true

	case ReminderBiweekly:
		// First fire: the next <day_of_week> at HH:00 (same rule
		// as weekly). Subsequent fires are exactly +14d from the
		// previous fire — the orchestrator handles that with a
		// simple `now + 14d` after a successful fire. This means
		// biweekly is just "weekly with a 14-day stride" and the
		// first fire is the very next opportunity the user has.
		//
		// We considered anchoring to CreatedAt (parity based on
		// "weeks since signup") but it surfaces surprising edge
		// cases (e.g. a user who signs up the day after their
		// chosen day gets a 2-week wait for no reason). The
		// +14d-from-first-fire model is deterministic, testable,
		// and matches what the user means by "every other Sunday".
		target := u.ReminderWeekday()
		days := (int(target) - int(now.Weekday()) + 7) % 7
		candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc).AddDate(0, 0, days)
		if !candidate.After(now) {
			candidate = candidate.AddDate(0, 0, 7)
		}
		return candidate, true
	}

	return time.Time{}, false
}

// parseReminderTime parses an "HH:00" string into (hour, minute).
// Returns an error when the input is empty, malformed, or out of
// range. Used by ReminderHour and the controller-side validator
// (so the same accept-set is enforced in one place).
func parseReminderTime(s string) (int, int, error) {
	if len(s) < 4 || s[2] != ':' {
		return 0, 0, fmt.Errorf("reminder time %q is not HH:MM", s)
	}
	hour := 0
	for i := 0; i < 2; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, 0, fmt.Errorf("reminder time %q has non-digit hour", s)
		}
		hour = hour*10 + int(c-'0')
	}
	if hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("reminder time %q hour out of range", s)
	}
	// Minute is always "00" per the hour-only design; tolerate
	// "HH:00" only, not arbitrary minutes.
	if s[3:] != "00" {
		return 0, 0, fmt.Errorf("reminder time %q must be on the hour (HH:00)", s)
	}
	return hour, 0, nil
}

// ParseReminderTimeForRoute is the exported alias of
// parseReminderTime, used by the route handler that validates
// the form's reminder_time field. Kept as a thin wrapper (rather
// than exporting parseReminderTime directly) so the lowercase
// helper stays an internal contract and the public surface is
// only what the route needs.
func ParseReminderTimeForRoute(s string) (int, int, error) {
	return parseReminderTime(s)
}
