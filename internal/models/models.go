package models

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// ExerciseType represents the type of exercise.
type ExerciseType string

const (
	ExerciseTypeStrength ExerciseType = "strength"
	ExerciseTypeCardio   ExerciseType = "cardio"
	ExerciseTypeOther    ExerciseType = "other"
)

// IsValid checks if the exercise type is a valid value.
func (et ExerciseType) IsValid() bool {
	return slices.Contains([]ExerciseType{ExerciseTypeStrength, ExerciseTypeCardio, ExerciseTypeOther}, et)
}

// Exercise represents a normalized exercise name with metadata.
// Aliases holds a comma-separated list of alternate names
// (e.g. "seated row,cable row"). It is never rendered visibly;
// it exists only so exercise lists can match on alternate names
// the same way they match on Name.
type Exercise struct {
	ID             string
	Name           string
	Aliases        string
	Description    string
	VideoURL       string
	ImgURL         string
	ImgURLOriginal string
	Type           ExerciseType
}

// ExerciseEntry represents a single set of an exercise. The metric pair that
// carries meaning depends on the linked exercise's type (never on client
// input): strength entries use Reps/Weight/RestTime, cardio entries use
// DurationSeconds/DistanceMeters with optional AvgHeartRate/CaloriesBurned.
// The server zeroes the pair that does not apply, so exactly one pair is
// ever non-zero. Distance is stored canonically in metres; pace is never
// stored, only derived via PaceSecPerKm.
type ExerciseEntry struct {
	ID              string
	ExerciseID      string
	UserID          string
	ExerciseName    string
	Reps            int
	Weight          float64
	Notes           string
	RestTime        int
	DurationSeconds int
	DistanceMeters  float64
	AvgHeartRate    int
	CaloriesBurned  float64
	ExerciseType    ExerciseType
	// WorkoutID optionally groups the exercise entry into a dated
	// workout. Empty means the exercise entry is standalone.
	WorkoutID string
	// WorkoutItemID is the most precise workout link: which planned
	// workout item the exercise entry logged against. Empty means the
	// exercise entry is ad-hoc within the workout (or standalone).
	WorkoutItemID string
	// RoundNumber records which round of a multi-round workout block
	// the exercise entry belongs to. 0 means unrounded.
	RoundNumber int
	CreatedAt   time.Time
}

// IsCardio reports whether this exercise entry belongs to a cardio exercise.
// Entries with an unknown/empty type (legacy rows) render as strength.
func (e *ExerciseEntry) IsCardio() bool {
	return e.ExerciseType == ExerciseTypeCardio
}

// HistoryStats summarises an exercise's training history for the header stat cards.
// LastSet is a zero-value ExerciseEntry when the user has no exercise entries for the exercise.
// Strength exercises surface MaxWeight and BestSetVolume (best single-set reps * weight,
// 0 when no exercise entries exist); cardio exercises surface BestPaceSecPerKm and
// LongestDistanceMeters (both 0 when no qualifying exercise entries exist). Callers pick
// the pair that matches the exercise's type — see IsCardio on ExerciseEntry.
type HistoryStats struct {
	MaxWeight             float64
	BestSetVolume         float64
	LastSet               ExerciseEntry
	BestPaceSecPerKm      float64
	LongestDistanceMeters float64
}

// ExerciseHistoryPage bundles a single page of history exercise entries with the stats
// needed to render the page header and the pagination state.
type ExerciseHistoryPage struct {
	ExerciseEntries []ExerciseEntry
	Stats           HistoryStats
	Page            int
	HasPrev         bool
	HasNext         bool
}

// FormattedWeight returns the weight labelled with the given unit.
// No conversion happens — the value is rendered as "%.1f <unit>" so
// the number is displayed using whatever unit the user prefers.
func (e *ExerciseEntry) FormattedWeight(unit string) string {
	return FormatWeight(e.Weight, unit)
}

// FormatWeight returns a human-readable weight string using the
// given unit. No conversion happens — the value is labelled with
// whatever unit the caller passes. Use everywhere a weight is
// shown to a user (display, chart labels, form labels) or written
// to a CSV.
func FormatWeight(value float64, unit string) string {
	return fmt.Sprintf("%.1f %s", value, unit)
}

// NormalizeWeightUnit returns a clean "kg" or "lbs", or "kg" if
// the input is empty or unrecognised. Use at trust boundaries
// (e.g. reading a value from a form or the DB) so downstream code
// can rely on a normalised value.
func NormalizeWeightUnit(unit string) string {
	switch unit {
	case "kg", "lbs":
		return unit
	default:
		return "kg"
	}
}

// Distance display units. Distances are stored in metres; these units
// control rendering only, mirroring how weight_unit labels weights.
const (
	DistanceUnitKm = "km"
	DistanceUnitMi = "mi"
)

// MetersPerMile converts stored metres into miles for "mi" display.
const MetersPerMile = 1609.344

// NormalizeDistanceUnit returns a clean "km" or "mi", defaulting to
// "km" for empty or unrecognised input. Use at trust boundaries so
// downstream code can rely on a normalised value.
func NormalizeDistanceUnit(unit string) string {
	switch unit {
	case DistanceUnitMi:
		return DistanceUnitMi
	default:
		return DistanceUnitKm
	}
}

// FormatDuration renders a duration in seconds as M:SS, switching to
// H:MM:SS once an hour is involved ("25:00", "1:05:30"). Negative and
// zero values render as "00:00". Use everywhere a duration is shown.
func FormatDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// FormatDistance renders a distance given in metres using the given
// unit ("km" or "mi"), e.g. "5.20 km" / "3.11 mi". No conversion of the
// user's preference happens here — pass a normalised unit (see
// NormalizeDistanceUnit) or accept the km default.
func FormatDistance(meters float64, unit string) string {
	switch NormalizeDistanceUnit(unit) {
	case DistanceUnitMi:
		return fmt.Sprintf("%.2f mi", meters/MetersPerMile)
	default:
		return fmt.Sprintf("%.2f km", meters/1000.0)
	}
}

// FormattedDistance returns the entry's distance rendered with the
// given unit.
func (e *ExerciseEntry) FormattedDistance(unit string) string {
	return FormatDistance(e.DistanceMeters, unit)
}

// PaceSecPerKm derives the entry's pace in seconds per kilometre from
// its duration and distance. Returns 0 when either is missing — there
// is no meaningful pace without both.
func (e *ExerciseEntry) PaceSecPerKm() float64 {
	if e.DistanceMeters <= 0 || e.DurationSeconds <= 0 {
		return 0
	}
	return float64(e.DurationSeconds) / (e.DistanceMeters / 1000.0)
}

// FormatPace renders a pace expressed in seconds per kilometre as
// minutes:seconds per the given unit, e.g. "4:58 /km" or "8:01 /mi".
// Returns an empty string for non-positive paces so callers can omit
// the label entirely.
func FormatPace(secPerKm float64, unit string) string {
	if secPerKm <= 0 {
		return ""
	}
	sec := secPerKm
	if NormalizeDistanceUnit(unit) == DistanceUnitMi {
		sec = secPerKm * MetersPerMile / 1000.0
	}
	return fmt.Sprintf("%d:%02d /%s", int(sec)/60, int(sec)%60, NormalizeDistanceUnit(unit))
}

// FormattedDate returns a human-readable date in UK short format
func (e *ExerciseEntry) FormattedDate() string {
	return e.CreatedAt.Format("02/01/06")
}

// Summary returns a compact one-line description of the exercise entry's
// metrics for timeline tables and stat cards: "5 × 100.0 kg" for strength,
// "25:00 · 5.20 km" for cardio. Use wherever rows for mixed exercise types
// share a single column so the reader never sees a meaningless zero.
func (e *ExerciseEntry) Summary(weightUnit, distanceUnit string) string {
	if e.IsCardio() {
		return fmt.Sprintf("%s · %s", FormatDuration(e.DurationSeconds), FormatDistance(e.DistanceMeters, distanceUnit))
	}
	return fmt.Sprintf("%d × %s", e.Reps, FormatWeight(e.Weight, weightUnit))
}

// ValidateURL checks if a string is a valid URL.
func ValidateURL(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// NormalizeAliases cleans a raw comma-separated alias string: splits on
// commas, trims whitespace, drops empties, and de-dupes
// case-insensitively (keeping the first casing). The result is joined
// with ",". An alias cannot contain a comma — commas are separators.
func NormalizeAliases(raw string) string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return strings.Join(out, ",")
}
