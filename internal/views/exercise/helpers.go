package exercise

import (
	"fmt"
	"sort"
	"time"

	"hylete/internal/models"
)

// chartDay holds one aggregated value for a single calendar day, used by
// the chart builders to collapse multiple exercise entries into a single
// line point. The value's meaning depends on the series: heaviest weight
// (strength) or fastest pace in seconds per kilometre (cardio). Shared
// between the history and chart views.
type chartDay struct {
	date  time.Time
	value float64
}

// aggregateExerciseEntriesByDay groups a slice of ExerciseEntry by
// calendar day and reduces each day to the heaviest weight recorded. Days
// are returned in ascending order so the resulting line runs
// left-to-right. The two strength chart views (history and full-width)
// both wrap this helper to build their own ChartProps.
func aggregateExerciseEntriesByDay(exerciseEntries []models.ExerciseEntry) []chartDay {
	// Preserves the original strength semantics: every exercise entry
	// contributes its weight, including zero-weight (bodyweight) rows.
	return aggregateExerciseEntriesByDayWith(exerciseEntries, func(e models.ExerciseEntry) (float64, bool) {
		return e.Weight, true
	}, func(current, candidate float64) bool {
		return candidate > current
	})
}

// aggregateExerciseEntriesPaceByDay groups cardio exercise entries by
// calendar day and reduces each day to the fastest pace (lowest seconds
// per kilometre) recorded that day — the cardio mirror of "heaviest
// weight per day". Entries without a derivable pace (no duration or no
// distance) are skipped. Days are returned in ascending order.
func aggregateExerciseEntriesPaceByDay(exerciseEntries []models.ExerciseEntry) []chartDay {
	return aggregateExerciseEntriesByDayWith(exerciseEntries, func(e models.ExerciseEntry) (float64, bool) {
		pace := e.PaceSecPerKm()
		if pace <= 0 {
			return 0, false
		}
		return pace, true
	}, func(current, candidate float64) bool {
		// Faster (lower) pace wins.
		return candidate < current
	})
}

// aggregateExerciseEntriesByDayWith is the shared calendar-day reducer
// behind the weight and pace aggregations. metric extracts an entry's
// plot value (ok=false skips the entry entirely); better reports whether
// the candidate value beats the current day best so each aggregation can
// pick its own direction (heavier vs faster).
func aggregateExerciseEntriesByDayWith(exerciseEntries []models.ExerciseEntry, metric func(models.ExerciseEntry) (float64, bool), better func(current, candidate float64) bool) []chartDay {
	byDay := make(map[string]chartDay)
	for _, e := range exerciseEntries {
		value, ok := metric(e)
		if !ok {
			continue
		}
		key := e.CreatedAt.Format("2006-01-02")
		existing, exists := byDay[key]
		if !exists || better(existing.value, value) {
			byDay[key] = chartDay{date: e.CreatedAt, value: value}
		}
	}
	days := make([]chartDay, 0, len(byDay))
	for _, d := range byDay {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].date.Before(days[j].date)
	})
	return days
}

// dayLabelsAndValues turns a []chartDay into the parallel labels /
// values slices the chart props expect. format controls how the date is
// rendered (e.g. "02 Jan" for the history view).
func dayLabelsAndValues(days []chartDay, format string) ([]string, []float64) {
	labels := make([]string, len(days))
	values := make([]float64, len(days))
	for i, d := range days {
		labels[i] = d.date.Format(format)
		values[i] = d.value
	}
	return labels, values
}

// chartDatasetLabel returns the legend label for an exercise's trend
// chart: "<name> (<weight unit>)" for strength, "<name> (min/<distance
// unit>)" for cardio so the reader knows what the y axis means.
func chartDatasetLabel(exerciseName string, exerciseType models.ExerciseType, weightUnit string, distanceUnit string) string {
	if exerciseType == models.ExerciseTypeCardio {
		return fmt.Sprintf("%s (min/%s)", exerciseName, models.NormalizeDistanceUnit(distanceUnit))
	}
	return fmt.Sprintf("%s (%s)", exerciseName, weightUnit)
}
