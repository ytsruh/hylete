package dashboard

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"
	"hylete/internal/views/components"

	"github.com/a-h/templ"
)

// renderToString renders a templ component to a string for assertions.
func renderToString(t *testing.T, component templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("failed to render component: %v", err)
	}
	return buf.String()
}

// === Helper function tests ===

func TestPopularExercisesChartProps_Empty(t *testing.T) {
	// Empty input should produce zero-value labels/values so the
	// DonutChart component renders nothing — call sites gate on
	// len(entries) > 0, but the helper itself must also be safe.
	props := popularExercisesChartProps(nil)
	if len(props.Labels) != 0 {
		t.Errorf("expected no labels for empty input, got %v", props.Labels)
	}
	if len(props.Values) != 0 {
		t.Errorf("expected no values for empty input, got %v", props.Values)
	}
	// ID and Title are still populated so a caller that forgets to
	// gate would at least see meaningful names.
	if props.ID != popularExercisesChartID {
		t.Errorf("ID = %q, want %q", props.ID, popularExercisesChartID)
	}
	if props.Title != popularExercisesChartTitle {
		t.Errorf("Title = %q, want %q", props.Title, popularExercisesChartTitle)
	}
}

func TestPopularExercisesChartProps_SingleExercise(t *testing.T) {
	now := time.Now()
	entries := []models.ExerciseEntry{
		{ExerciseName: "Squat", CreatedAt: now},
		{ExerciseName: "Squat", CreatedAt: now.Add(-time.Hour)},
		{ExerciseName: "Squat", CreatedAt: now.Add(-2 * time.Hour)},
	}
	props := popularExercisesChartProps(entries)
	if len(props.Labels) != 1 || props.Labels[0] != "Squat" {
		t.Errorf("expected one slice labelled Squat, got labels=%v", props.Labels)
	}
	if len(props.Values) != 1 || props.Values[0] != 3 {
		t.Errorf("expected one value of 3, got values=%v", props.Values)
	}
}

func TestPopularExercisesChartProps_SortedByCountDesc(t *testing.T) {
	now := time.Now()
	mk := func(name string, n int) []models.ExerciseEntry {
		out := make([]models.ExerciseEntry, n)
		for i := range out {
			out[i] = models.ExerciseEntry{ExerciseName: name, CreatedAt: now.Add(time.Duration(i) * time.Hour)}
		}
		return out
	}
	entries := []models.ExerciseEntry{}
	entries = append(entries, mk("Bench Press", 2)...)
	entries = append(entries, mk("Squat", 5)...)
	entries = append(entries, mk("Deadlift", 1)...)
	props := popularExercisesChartProps(entries)
	if len(props.Labels) != 3 {
		t.Fatalf("expected 3 slices, got %d (%v)", len(props.Labels), props.Labels)
	}
	if props.Labels[0] != "Squat" || props.Labels[1] != "Bench Press" || props.Labels[2] != "Deadlift" {
		t.Errorf("expected slices sorted by count desc, got %v", props.Labels)
	}
	if !floatSliceEqual(props.Values, []float64{5, 2, 1}) {
		t.Errorf("expected values [5 2 1] in slice order, got %v", props.Values)
	}
}

func TestPopularExercisesChartProps_TopNExactNoOther(t *testing.T) {
	// Exactly popularExercisesTopN exercises — no Other bucket and
	// no Colors override (the chart fills from its default brand
	// palette).
	now := time.Now()
	entries := []models.ExerciseEntry{
		{ExerciseName: "A", CreatedAt: now},
		{ExerciseName: "B", CreatedAt: now},
		{ExerciseName: "C", CreatedAt: now},
		{ExerciseName: "D", CreatedAt: now},
		{ExerciseName: "E", CreatedAt: now},
	}
	props := popularExercisesChartProps(entries)
	if len(props.Labels) != popularExercisesTopN {
		t.Fatalf("expected %d slices with no Other bucket, got %d (%v)", popularExercisesTopN, len(props.Labels), props.Labels)
	}
	for _, l := range props.Labels {
		if l == popularExercisesOtherLabel {
			t.Errorf("did not expect Other bucket when slice count equals top-N, got labels=%v", props.Labels)
		}
	}
	if len(props.Colors) != 0 {
		t.Errorf("expected no Colors override when there is no Other bucket, got %v", props.Colors)
	}
}

func TestPopularExercisesChartProps_TopNCollapsesToOther(t *testing.T) {
	// popularExercisesTopN + 2 distinct exercises; the bottom two
	// should collapse into the "Other" bucket with summed counts.
	now := time.Now()
	entries := []models.ExerciseEntry{
		// top 5
		{ExerciseName: "A", CreatedAt: now},
		{ExerciseName: "A", CreatedAt: now},
		{ExerciseName: "B", CreatedAt: now},
		{ExerciseName: "B", CreatedAt: now},
		{ExerciseName: "B", CreatedAt: now},
		{ExerciseName: "C", CreatedAt: now},
		{ExerciseName: "D", CreatedAt: now},
		{ExerciseName: "E", CreatedAt: now},
		// tail (should be lumped into Other)
		{ExerciseName: "F", CreatedAt: now},
		{ExerciseName: "G", CreatedAt: now},
		{ExerciseName: "G", CreatedAt: now},
	}
	props := popularExercisesChartProps(entries)
	if len(props.Labels) != popularExercisesTopN+1 {
		t.Fatalf("expected %d slices (top-N + Other), got %d (%v)", popularExercisesTopN+1, len(props.Labels), props.Labels)
	}
	// Counts: A=2, B=3, C=1, D=1, E=1, F=1, G=2. Sort by count desc,
	// then by name asc: B(3), A(2), G(2), C(1), D(1), E(1), F(1).
	// Top 5: B, A, G, C, D. Other bucket sums E(1) + F(1) = 2.
	wantLabels := []string{"B", "A", "G", "C", "D", popularExercisesOtherLabel}
	for i, want := range wantLabels {
		if props.Labels[i] != want {
			t.Errorf("label %d: want %q, got %q", i, want, props.Labels[i])
		}
	}
	wantValues := []float64{3, 2, 2, 1, 1, 2}
	if !floatSliceEqual(props.Values, wantValues) {
		t.Errorf("expected values %v, got %v", wantValues, props.Values)
	}
	// The "Other" slice must be wired to the neutral gray so it
	// visually de-emphasises itself against the brand palette. The
	// top-N slots are "" — DonutChart fills those from its default
	// brand palette.
	if len(props.Colors) != popularExercisesTopN+1 {
		t.Fatalf("expected Colors override of length %d, got %d (%v)", popularExercisesTopN+1, len(props.Colors), props.Colors)
	}
	for i := 0; i < popularExercisesTopN; i++ {
		if props.Colors[i] != "" {
			t.Errorf("expected top-N color slot %d to be empty (delegate to default palette), got %q", i, props.Colors[i])
		}
	}
	if props.Colors[popularExercisesTopN] != popularExercisesOtherColor {
		t.Errorf("expected Other slot to use %q, got %q", popularExercisesOtherColor, props.Colors[popularExercisesTopN])
	}
}

func TestPopularExercisesChartProps_TieBreakDeterministic(t *testing.T) {
	// Two exercises with the same count should appear in
	// alphabetical order, never Go map-iteration order.
	now := time.Now()
	entries := []models.ExerciseEntry{
		{ExerciseName: "Zebra", CreatedAt: now},
		{ExerciseName: "Apple", CreatedAt: now},
	}
	props := popularExercisesChartProps(entries)
	if len(props.Labels) != 2 {
		t.Fatalf("expected 2 slices, got %d", len(props.Labels))
	}
	if props.Labels[0] != "Apple" || props.Labels[1] != "Zebra" {
		t.Errorf("expected alphabetical tie-break [Apple, Zebra], got %v", props.Labels)
	}
}

// === Component rendering tests ===

func TestDashboard_WithEntries(t *testing.T) {
	entries := []models.ExerciseEntry{
		{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
	}
	html := renderToString(t, Dashboard(entries, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, "7 Day History") {
		t.Error("expected page title")
	}
	if !strings.Contains(html, "Squat") {
		t.Error("expected exercise name in table")
	}
	if strings.Contains(html, "No workouts yet") {
		t.Error("did not expect empty state when entries exist")
	}
	// Popular-exercises donut sits between the action cards and the
	// 7-day history table; the canvas id is the contract surface.
	if !strings.Contains(html, `id="popular-exercises-chart"`) {
		t.Error("expected popular-exercises-chart canvas id when entries are present")
	}
	if !strings.Contains(html, "Most Popular Exercises (7d)") {
		t.Error("expected popular exercises donut title when entries are present")
	}
}

func TestDashboard_Empty(t *testing.T) {
	html := renderToString(t, Dashboard([]models.ExerciseEntry{}, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, "No workouts in the last 7 days") {
		t.Error("expected empty state when no entries")
	}
	// The donut should not render in the empty case (a zero-slice
	// donut is not meaningful and the EmptyState owns the screen).
	if strings.Contains(html, `id="popular-exercises-chart"`) {
		t.Error("did not expect popular-exercises-chart canvas id when no entries")
	}
}

func TestEntryList(t *testing.T) {
	entries := []models.ExerciseEntry{
		{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
		{ID: "entry-2", ExerciseName: "Bench Press", Reps: 8, Weight: 80},
	}
	html := renderToString(t, ExerciseEntryList(entries, "kg", "km"))
	if !strings.Contains(html, "Squat") {
		t.Error("expected Squat in output")
	}
	if !strings.Contains(html, "Bench Press") {
		t.Error("expected Bench Press in output")
	}
	if !strings.Contains(html, `<tbody id="exercise-entries-table">`) {
		t.Error("expected entries-table tbody")
	}
}

func TestEntryRow(t *testing.T) {
	entry := models.ExerciseEntry{
		ID:           "entry-1",
		ExerciseName: "Deadlift",
		Reps:         5,
		Weight:       180,
		Notes:        "PR",
		CreatedAt:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	html := renderToString(t, ExerciseEntryRow(entry, "kg", "km"))
	if !strings.Contains(html, "Deadlift") {
		t.Error("expected exercise name")
	}
	if !strings.Contains(html, "180.0 kg") {
		t.Error("expected formatted weight")
	}
	if !strings.Contains(html, "PR") {
		t.Error("expected notes")
	}
	if !strings.Contains(html, `id="exercise-entry-entry-1"`) {
		t.Error("expected entry ID attribute")
	}
	// The dashboard is read-only on the web (set editing happens in
	// the iOS client), so rows must not link to an edit form.
	if strings.Contains(html, "/exercise-entries/") {
		t.Error("row should not link to the removed edit form")
	}
}

func TestStatCard(t *testing.T) {
	html := renderToString(t, components.StatCard(components.StatCardProps{Label: "Total Sets", Value: "42"}))
	if !strings.Contains(html, "Total Sets") {
		t.Error("expected label")
	}
	if !strings.Contains(html, "42") {
		t.Error("expected value")
	}
}

// floatSliceEqual reports whether two float64 slices are element-wise
// equal. Used by the chart-aggregation tests so they can assert on
// max-weight-per-day reductions without coupling to the helper's
// internal sort order beyond what the chart's documented contract
// already promises.
func floatSliceEqual(a, b []float64) bool {
	return reflect.DeepEqual(a, b)
}

// TestDashboard_CardioRow verifies cardio entries render a
// duration · distance summary in the Details column instead of the
// strength "reps × weight" pair.
func TestDashboard_CardioRow(t *testing.T) {
	entries := []models.ExerciseEntry{
		{ID: "entry-1", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: time.Now()},
		{ID: "entry-2", ExerciseName: "Squat", ExerciseType: models.ExerciseTypeStrength, Reps: 5, Weight: 100, CreatedAt: time.Now()},
	}
	html := renderToString(t, Dashboard(entries, "Test User", true, false, "kg", "mi"))
	if !strings.Contains(html, "25:00 · 3.11 mi") {
		t.Error("expected cardio row summary '25:00 · 3.11 mi' (user prefers miles)")
	}
	if !strings.Contains(html, "5 × 100.0 kg") {
		t.Error("expected strength row summary '5 × 100.0 kg'")
	}
}
