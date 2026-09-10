// Package trainingstats pre-computes deterministic training summaries
// from raw workout data. The weekly Coach report is hybrid by design:
// this package owns every number ("what happened"), and the LLM only
// narrates ("what it means", "what to do"). That keeps reports
// testable, cheap, and free of hallucinated figures.
//
// Grain: per-exercise aggregation over trailing windows from
// []models.ExerciseEntry (the stable grain — a future Workouts layer
// links entries via an optional workout_id, which arrives here as the
// opaque WorkoutID passthrough and is currently ignored).
package trainingstats

import (
	"math"
	"sort"
	"time"

	"hylete/internal/models"
)

// Windows and thresholds. Hard-coded product decisions (same policy
// as weightReminderCronSpec): cadence/thresholds are not per-deploy knobs.
const (
	// RecentWindow is the "this week" slice the report covers.
	RecentWindow = 7 * 24 * time.Hour
	// BaselineWindow is the trailing history the week is compared against.
	BaselineWindow = 4 * 7 * 24 * time.Hour
	// MinSessionsForReport gates the LLM call: fewer sessions in the
	// recent window yields a thin-data template with no LLM spend.
	MinSessionsForReport = 3
	// PlateauWeeks flags stalling when no e1RM or volume PR appears
	// in this many trailing weeks.
	PlateauWeeks = 4
	// PlateauWindow bounds the history scanned for the last PR. It
	// must exceed PlateauWeeks, otherwise the 4-week baseline
	// window could never evidence a 4-week stall.
	PlateauWindow = 12 * 7 * 24 * time.Hour
)

// Epley1RM estimates a one-rep max from reps × weight via the Epley
// formula. Returns 0 for non-positive input. Single reps return the
// weight itself.
func Epley1RM(reps int, weight float64) float64 {
	if reps <= 0 || weight <= 0 {
		return 0
	}
	if reps == 1 {
		return weight
	}
	return weight * (1 + float64(reps)/30.0)
}

// Volume is the mechanical work of one strength set: sets(1) × reps × load.
func Volume(reps int, weight float64) float64 {
	if reps <= 0 || weight <= 0 {
		return 0
	}
	return float64(reps) * weight
}

// ExerciseStats is the per-exercise rollup for one window.
type ExerciseStats struct {
	// ExerciseID and ExerciseName identify the movement.
	ExerciseID   string `json:"exercise_id"`
	ExerciseName string `json:"exercise_name"`
	// IsCardio mirrors the entry type split: cardio rows aggregate
	// duration/distance, strength rows aggregate volume/e1RM.
	IsCardio bool `json:"is_cardio"`
	// Sessions counts distinct training days with entries.
	Sessions int `json:"sessions"`
	// Sets counts exercise entries (one entry == one set/session).
	Sets int `json:"sets"`
	// TotalVolume is Σ sets×reps×weight (strength only).
	TotalVolume float64 `json:"total_volume"`
	// BestE1RM is the max Epley estimate in the window (strength only).
	BestE1RM float64 `json:"best_e1rm"`
	// MaxWeight is the heaviest single set (strength only).
	MaxWeight float64 `json:"max_weight"`
	// TotalDistanceM and TotalDurationS cover cardio only.
	TotalDistanceM  float64 `json:"total_distance_m"`
	TotalDurationS  int     `json:"total_duration_s"`
	BestPaceSecPerKm float64 `json:"best_pace_sec_per_km"`
}

// ExerciseDelta compares recent vs baseline for one exercise.
type ExerciseDelta struct {
	ExerciseStats
	// BaselineSessions is the per-week average over the baseline window.
	BaselineSessions float64 `json:"baseline_sessions_per_week"`
	// VolumeChangePct is (recent − baselineAvg)/baselineAvg*100, 0 when
	// the baseline is empty so new movements read as "new", not "+Inf%".
	VolumeChangePct float64 `json:"volume_change_pct"`
	// E1RMChangePct is the same ratio for best-e1RM (strength only).
	E1RMChangePct float64 `json:"e1rm_change_pct"`
	// IsNew is true when the exercise has no baseline history.
	IsNew bool `json:"is_new"`
	// Plateaued is true for strength exercises with ≥PlateauWeeks of
	// history and no e1RM or volume PR in that span.
	Plateaued bool `json:"plateaued"`
	// WeeksSincePR counts weeks since the last e1RM/volume best.
	WeeksSincePR int `json:"weeks_since_pr"`
}

// WeeklyStats is the full deterministic input rendered into the prompt.
type WeeklyStats struct {
	// WeekStart/WeekEnd bound the recent window (UTC dates).
	WeekStart time.Time `json:"week_start"`
	WeekEnd   time.Time `json:"week_end"`
	// Sessions counts distinct training days in the recent window.
	Sessions int `json:"sessions"`
	// BaselineSessionsPerWeek is the trailing-4wk weekly average.
	BaselineSessionsPerWeek float64 `json:"baseline_sessions_per_week"`
	// AdherencePct is sessions/baselineAvg*100 (100 when both are 0).
	AdherencePct float64 `json:"adherence_pct"`
	// Exercises holds per-exercise recent-vs-baseline deltas,
	// sorted by recent volume descending (strength) then name.
	Exercises []ExerciseDelta `json:"exercises"`
	// BodyWeightDelta is recent-median minus baseline-median (0 when
	// fewer than 2 weigh-ins exist across both windows).
	BodyWeightDelta float64 `json:"bodyweight_delta"`
	// BodyWeightRecent is the recent-window median (0 when none).
	BodyWeightRecent float64 `json:"bodyweight_recent"`
	// Recovery notes derived ONLY from HealthSnapshot measured_at
	// observations: each string is pre-phrased ("resting HR up 6bpm
	// vs 4-wk median") so the LLM relays rather than invents.
	RecoveryNotes []string `json:"recovery_notes"`
	// ThinData is true when Sessions < MinSessionsForReport: the
	// service stores a template payload and skips the LLM call.
	ThinData bool `json:"thin_data"`
	// WorkoutID is the forward-compat seam for the planned Workouts
	// layer (template → blocks → exercises → entries). Entries may
	// eventually carry an optional workout link; aggregation stays
	// per-exercise, so this is currently informational only.
	WorkoutID string `json:"workout_id,omitempty"`
}

// Input bundles the raw rows a weekly report is built from.
type Input struct {
	// Entries is the user's exercise entries (8+ weeks ideally).
	Entries []models.ExerciseEntry
	// Weights is the user's body-weight entries.
	Weights []models.WeightEntry
	// Health is the user's daily snapshots (for recovery notes).
	Health []models.HealthSnapshot
	// Now anchors the windows; WeekStart = start of the recent window.
	Now time.Time
	// WorkoutID passthrough for the future Workouts layer.
	WorkoutID string
}

// Build computes WeeklyStats for the trailing RecentWindow ending at Now.
func Build(in Input) WeeklyStats {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	weekStart := now.Add(-RecentWindow)
	baseStart := now.Add(-BaselineWindow)

	var recent, baseline, plateau []models.ExerciseEntry
	for _, e := range in.Entries {
		if e.CreatedAt.After(now) {
			continue
		}
		if !e.CreatedAt.Before(weekStart) {
			recent = append(recent, e)
		} else if !e.CreatedAt.Before(baseStart) {
			baseline = append(baseline, e)
		}
		if !e.CreatedAt.Before(now.Add(-PlateauWindow)) {
			plateau = append(plateau, e)
		}
	}

	stats := WeeklyStats{WeekStart: weekStart, WeekEnd: now, WorkoutID: in.WorkoutID}
	stats.Sessions = countSessions(recent)
	baseSessions := countSessions(baseline)
	stats.BaselineSessionsPerWeek = float64(baseSessions) / 4.0
	if stats.BaselineSessionsPerWeek > 0 {
		stats.AdherencePct = float64(stats.Sessions) / stats.BaselineSessionsPerWeek * 100
	} else if stats.Sessions > 0 {
		stats.AdherencePct = 100
	} else {
		stats.AdherencePct = 100
	}

	stats.Exercises = diffExercises(recent, baseline, plateau, now)
	recentW, baseW := splitWeights(in.Weights, weekStart, baseStart)
	stats.BodyWeightRecent = median(recentW)
	if len(recentW) >= 1 && len(baseW) >= 1 {
		stats.BodyWeightDelta = median(recentW) - median(baseW)
	}
	stats.RecoveryNotes = recoveryNotes(in.Health, weekStart, baseStart, now)
	stats.ThinData = stats.Sessions < MinSessionsForReport
	return stats
}

// countSessions counts distinct UTC calendar days with entries.
func countSessions(entries []models.ExerciseEntry) int {
	days := map[string]struct{}{}
	for _, e := range entries {
		days[e.CreatedAt.UTC().Format("2006-01-02")] = struct{}{}
	}
	return len(days)
}

// aggregate collapses entries into per-exercise stats for one window.
func aggregate(entries []models.ExerciseEntry) map[string]*ExerciseStats {
	out := map[string]*ExerciseStats{}
	for _, e := range entries {
		s, ok := out[e.ExerciseID]
		if !ok {
			s = &ExerciseStats{
				ExerciseID:   e.ExerciseID,
				ExerciseName: e.ExerciseName,
				IsCardio:     e.IsCardio(),
			}
			out[e.ExerciseID] = s
		}
		s.Sets++
		if e.IsCardio() {
			s.TotalDistanceM += e.DistanceMeters
			s.TotalDurationS += e.DurationSeconds
			if p := e.PaceSecPerKm(); p > 0 && (s.BestPaceSecPerKm == 0 || p < s.BestPaceSecPerKm) {
				s.BestPaceSecPerKm = p
			}
			continue
		}
		s.TotalVolume += Volume(e.Reps, e.Weight)
		if est := Epley1RM(e.Reps, e.Weight); est > s.BestE1RM {
			s.BestE1RM = est
		}
		if e.Weight > s.MaxWeight {
			s.MaxWeight = e.Weight
		}
	}
	// Sessions per exercise: distinct days.
	byEx := map[string]map[string]struct{}{}
	for _, e := range entries {
		if byEx[e.ExerciseID] == nil {
			byEx[e.ExerciseID] = map[string]struct{}{}
		}
		byEx[e.ExerciseID][e.CreatedAt.UTC().Format("2006-01-02")] = struct{}{}
	}
	for id, days := range byEx {
		out[id].Sessions = len(days)
	}
	return out
}

// diffExercises joins recent and baseline aggregates into deltas.
// plateauEntries carries the longer PlateauWindow history used only
// for stall detection (never for volume ratios).
func diffExercises(recent, baseline, plateauEntries []models.ExerciseEntry, now time.Time) []ExerciseDelta {
	rAgg := aggregate(recent)
	bAgg := aggregate(baseline)
	weeks := 4.0

	var out []ExerciseDelta
	for id, r := range rAgg {
		d := ExerciseDelta{ExerciseStats: *r}
		if b, ok := bAgg[id]; ok {
			d.BaselineSessions = float64(b.Sessions) / weeks
			if b.TotalVolume > 0 {
				d.VolumeChangePct = (r.TotalVolume - b.TotalVolume/weeks) / (b.TotalVolume / weeks) * 100
			}
			if b.BestE1RM > 0 {
				d.E1RMChangePct = (r.BestE1RM - b.BestE1RM) / b.BestE1RM * 100
			}
		} else {
			d.IsNew = true
		}
		// Plateau: only for strength with baseline presence — needs
		// history, so brand-new movements never read as stalling.
		if !r.IsCardio && !d.IsNew {
			d.WeeksSincePR, d.Plateaued = plateauInfo(id, plateauEntries, r.BestE1RM, now)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalVolume != out[j].TotalVolume {
			return out[i].TotalVolume > out[j].TotalVolume
		}
		return out[i].ExerciseName < out[j].ExerciseName
	})
	return out
}

// plateauInfo walks weekly best-e1RM/volume history to find the most
// recent PR week. Returns weeks-since-PR and whether it exceeds PlateauWeeks.
func plateauInfo(exerciseID string, entries []models.ExerciseEntry, recentBest float64, now time.Time) (int, bool) {
	// Weekly bests, oldest → newest, over the trailing 8 weeks.
	type wk struct{ e1rm, vol float64 }
	buckets := map[int]*wk{}
	var maxWeek int
	for _, e := range entries {
		if e.ExerciseID != exerciseID || e.IsCardio() {
			continue
		}
		w := int(now.Sub(e.CreatedAt).Hours() / (24 * 7))
		if w < 0 || w > 12 {
			continue
		}
		if w > maxWeek {
			maxWeek = w
		}
		b := buckets[w]
		if b == nil {
			b = &wk{}
			buckets[w] = b
		}
		if est := Epley1RM(e.Reps, e.Weight); est > b.e1rm {
			b.e1rm = est
		}
		b.vol += Volume(e.Reps, e.Weight)
	}
	if len(buckets) < 2 {
		return 0, false
	}
	// Running all-time best from oldest to newest; the last week that
	// sets a new best is the PR week.
	bestE, bestV := 0.0, 0.0
	prWeekAgo := 0
	for w := maxWeek; w >= 0; w-- {
		b := buckets[w]
		if b == nil {
			continue
		}
		if b.e1rm > bestE || b.vol > bestV {
			if b.e1rm > bestE {
				bestE = b.e1rm
			}
			if b.vol > bestV {
				bestV = b.vol
			}
			prWeekAgo = w
		}
	}
	_ = recentBest
	return prWeekAgo, prWeekAgo >= PlateauWeeks
}

// splitWeights partitions weight values into recent/baseline slices.
func splitWeights(weights []models.WeightEntry, weekStart, baseStart time.Time) ([]float64, []float64) {
	var recent, base []float64
	for _, w := range weights {
		if !w.CreatedAt.Before(weekStart) {
			recent = append(recent, w.Weight)
		} else if !w.CreatedAt.Before(baseStart) {
			base = append(base, w.Weight)
		}
	}
	return recent, base
}

// median returns the median of vals, or 0 when empty.
func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

// recoveryNotes derives relay-phrased signals from snapshots. Only
// values with a MeasuredAt inside their snapshot day count as
// observations; carried-forward values are ignored. Comparisons use
// recent-week median vs trailing baseline median with conservative
// thresholds so sparse data yields silence, not noise.
func recoveryNotes(health []models.HealthSnapshot, weekStart, baseStart, now time.Time) []string {
	var rRHR, bRHR, rHRV, bHRV, rSleep, bSleep []float64
	for _, h := range health {
		d, err := time.Parse("2006-01-02", h.SnapshotDate)
		if err != nil {
			continue
		}
		if d.After(now) {
			continue
		}
		recent := !d.Before(weekStart)
		base := !recent && !d.Before(baseStart)
		if !recent && !base {
			continue
		}
		// RHR/HRV/sleep need no measured_at gate: totals are
		// inherently measured that day; RHR/HRV are checked via
		// their timestamps to exclude carried-forward values.
		if h.RestingHeartRate > 0 && h.RestingHRMeasuredAt != nil {
			if recent {
				rRHR = append(rRHR, h.RestingHeartRate)
			} else {
				bRHR = append(bRHR, h.RestingHeartRate)
			}
		}
		if h.HRV > 0 && h.HRVMeasuredAt != nil {
			if recent {
				rHRV = append(rHRV, h.HRV)
			} else {
				bHRV = append(bHRV, h.HRV)
			}
		}
		if h.SleepSeconds > 0 {
			hrs := h.SleepSeconds / 3600
			if recent {
				rSleep = append(rSleep, hrs)
			} else {
				bSleep = append(bSleep, hrs)
			}
		}
	}
	var notes []string
	if mR, mB := median(rRHR), median(bRHR); len(rRHR) >= 2 && len(bRHR) >= 3 && mB > 0 {
		if d := mR - mB; d >= 5 {
			notes = append(notes, formatDelta("Resting heart rate up", d, "bpm"))
		} else if d <= -5 {
			notes = append(notes, formatDelta("Resting heart rate down", -d, "bpm"))
		}
	}
	if mR, mB := median(rHRV), median(bHRV); len(rHRV) >= 2 && len(bHRV) >= 3 && mB > 0 {
		if drop := (mB - mR) / mB; drop >= 0.15 {
			notes = append(notes, formatDelta("HRV down", drop*100, "%"))
		}
	}
	if mR, mB := median(rSleep), median(bSleep); len(rSleep) >= 2 && len(bSleep) >= 3 && mB > 0 {
		if d := mB - mR; d >= 1 {
			notes = append(notes, formatDelta("Sleep down", d, "h/night"))
		}
	}
	return notes
}

func formatDelta(label string, v float64, unit string) string {
	v = math.Round(v*10) / 10
	return label + " " + trimFloat(v) + unit + " vs 4-wk median"
}

func trimFloat(v float64) string {
	if v == math.Trunc(v) {
		return itoa(int(v))
	}
	return (itoa(int(v)) + "." + itoa(int(math.Abs(v-float64(int(v)))*10+0.5)))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
