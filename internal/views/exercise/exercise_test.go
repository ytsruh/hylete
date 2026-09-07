package exercise

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"

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

func TestChartDataForExercise(t *testing.T) {
	// Two sets on the same day, plus one set a few days later. The chart
	// should collapse to 2 points, take the max weight of the multi-set day,
	// and sort ascending by date.
	day1Morning := time.Date(2024, 6, 10, 8, 0, 0, 0, time.UTC)
	day1Evening := time.Date(2024, 6, 10, 18, 0, 0, 0, time.UTC)
	day3 := time.Date(2024, 6, 12, 9, 0, 0, 0, time.UTC)
	entries := []models.ExerciseEntry{
		{Weight: 80, CreatedAt: day1Morning},
		{Weight: 85, CreatedAt: day1Evening},
		{Weight: 90, CreatedAt: day3},
	}
	props := chartDataForExercise(entries, "Squat", models.ExerciseTypeStrength, "kg", "km")
	if props.ID != "exercise-history-chart" {
		t.Errorf("ID = %q, want %q", props.ID, "exercise-history-chart")
	}
	if props.Title != "Last 3 Sets" {
		t.Errorf("Title = %q, want %q", props.Title, "Last 3 Sets")
	}
	if len(props.Labels) != 2 {
		t.Fatalf("expected 2 labels (one per unique day), got %d (%v)", len(props.Labels), props.Labels)
	}
	if props.Labels[0] != "10 Jun" || props.Labels[1] != "12 Jun" {
		t.Errorf("labels = %v, want [10 Jun 12 Jun]", props.Labels)
	}
	if len(props.Datasets) != 1 {
		t.Fatalf("expected 1 dataset, got %d", len(props.Datasets))
	}
	values := props.Datasets[0].Values
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != 85 {
		t.Errorf("day-1 value = %v, want 85 (max of 80, 85)", values[0])
	}
	if values[1] != 90 {
		t.Errorf("day-3 value = %v, want 90", values[1])
	}
	if props.Datasets[0].Label != "Squat (kg)" {
		t.Errorf("dataset label = %q, want %q", props.Datasets[0].Label, "Squat (kg)")
	}
}

func TestChartDataForExercise_Empty(t *testing.T) {
	props := chartDataForExercise(nil, "Squat", models.ExerciseTypeStrength, "kg", "km")
	if len(props.Labels) != 0 {
		t.Errorf("expected no labels for empty input, got %v", props.Labels)
	}
	if props.Title != "Last 0 Sets" {
		t.Errorf("Title = %q, want %q", props.Title, "Last 0 Sets")
	}
}

func TestExerciseHistory_WithEntries(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
		},
		Stats: models.HistoryStats{
			MaxWeight: 100,
			LastSet:   models.ExerciseEntry{ID: "entry-1", Reps: 5, Weight: 100},
		},
		Page: 1,
	}
	html := renderToString(t, ExerciseHistory(exercise, page, nil, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, "Squat") {
		t.Error("expected exercise name heading")
	}
	if strings.Contains(html, "No sets yet") {
		t.Error("did not expect empty state when entries exist")
	}
	if !strings.Contains(html, "100.0 kg") {
		t.Error("expected personal best value to appear")
	}
}

func TestExerciseHistory_Empty(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{Page: 1}
	html := renderToString(t, ExerciseHistory(exercise, page, nil, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, "No sets yet") {
		t.Error("expected empty state")
	}
}

func TestExerciseHistory_RendersChart(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
		},
		Stats: models.HistoryStats{MaxWeight: 100},
		Page:  1,
	}
	chartExerciseEntries := []models.ExerciseEntry{
		{Weight: 80, CreatedAt: time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC)},
		{Weight: 85, CreatedAt: time.Date(2024, 6, 12, 9, 0, 0, 0, time.UTC)},
		{Weight: 90, CreatedAt: time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)},
	}
	html := renderToString(t, ExerciseHistory(exercise, page, chartExerciseEntries, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, `id="exercise-history-chart"`) {
		t.Error("expected chart canvas with id exercise-history-chart")
	}
	if !strings.Contains(html, "Last 3 Sets") {
		t.Error("expected chart title to report the entry count")
	}
	// The exercise history chart is rendered in a narrow grid cell, so the
	// view should request the axis-tick-hiding option. Lock the wiring in
	// so a future refactor of chartDataForExercise can't silently regress it.
	if !strings.Contains(html, `"hideAxes":true`) {
		t.Error("expected chart payload to carry hideAxes=true (ticks hidden in small grid cell)")
	}
	// The chart now fills its parent; the view is responsible for sizing the
	// inner wrapper inside the card. Lock in the responsive height class so
	// a future refactor of the view can't silently drop sizing.
	if !strings.Contains(html, `h-28 sm:h-32 md:h-48`) {
		t.Error("expected inner chart wrapper to carry the responsive height class")
	}
}

func TestExerciseHistory_OmitsChartWhenSingleDay(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
		},
		Stats: models.HistoryStats{MaxWeight: 100},
		Page:  1,
	}
	// Two entries on the same day should collapse to a single chart point,
	// so the chart (which needs >= 2 labels) should not render.
	sameDay := time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC)
	chartExerciseEntries := []models.ExerciseEntry{
		{Weight: 80, CreatedAt: sameDay},
		{Weight: 85, CreatedAt: sameDay.Add(time.Hour)},
	}
	html := renderToString(t, ExerciseHistory(exercise, page, chartExerciseEntries, "Test User", true, false, "kg", "km"))
	if strings.Contains(html, `id="exercise-history-chart"`) {
		t.Error("expected chart to be omitted when only one unique day is present")
	}
}

func TestHistoryTable_NoPagerWhenSinglePage(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
		},
		Page: 1,
	}
	html := renderToString(t, HistoryTable(exercise, page, "kg", "km"))
	if !strings.Contains(html, historyTableWrapID) {
		t.Error("expected swappable wrap region to be present")
	}
	if strings.Contains(html, "Previous page") || strings.Contains(html, "Next page") {
		t.Error("did not expect pagination buttons on a single-page result")
	}
}

// TestExerciseHistory_RendersChartButton locks in the secondary "Chart"
// link that sits next to the Add button in the history view header. The
// link is the entry point to the new chart sub-views, so it must remain
// visible regardless of whether the user has any entries.
func TestExerciseHistory_RendersChartButton(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{Page: 1}
	html := renderToString(t, ExerciseHistory(exercise, page, nil, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, `href="/exercises/ex-1/chart"`) {
		t.Error("expected Chart link pointing at /exercises/ex-1/chart")
	}
	if !strings.Contains(html, `class="btn-outline"`) {
		t.Error("expected the Chart link to use btn-outline styling")
	}
	// Populated state — make sure the button is also present when entries exist.
	populatedPage := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
		},
		Stats: models.HistoryStats{MaxWeight: 100},
		Page:  1,
	}
	populatedHTML := renderToString(t, ExerciseHistory(exercise, populatedPage, nil, "Test User", true, false, "kg", "km"))
	if !strings.Contains(populatedHTML, `href="/exercises/ex-1/chart"`) {
		t.Error("expected Chart link in populated history view as well")
	}
}

// TestExerciseChart verifies the dedicated chart view renders the
// centered button group with all three sub-view links and marks the
// Chart link as active (aria-current="page"). The body is exercised in
// the populated / empty-state / aggregation tests below — this one
// focuses on the shared chrome that should be present in every variant.
//
// Two days of data are passed so the chart (and its subtitle) is
// rendered. The empty-state case is covered by TestExerciseChart_EmptyState.
func TestExerciseChart(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	day1 := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 17, 9, 0, 0, 0, time.UTC)
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "e1", Reps: 5, Weight: 100, CreatedAt: day1},
		{ID: "e2", Reps: 5, Weight: 105, CreatedAt: day2},
	}
	html := renderToString(t, ExerciseChart(exercise, chartExerciseEntries, "Test User", true, false, "kg", "km"))

	// Button group container with the documented basecoat class.
	if !strings.Contains(html, `class="button-group"`) {
		t.Error("expected basecoat button-group container")
	}
	if !strings.Contains(html, `role="group"`) {
		t.Error("expected role=\"group\" on the button-group container")
	}

	// All three sub-view links are present.
	for _, want := range []string{
		`href="/exercises/ex-1"`,
		`href="/exercises/ex-1/chart"`,
		`href="/exercises/ex-1/chart/advanced"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected link %s in chart view", want)
		}
	}

	// The Chart link is the active one — primary btn class plus
	// aria-current="page". The other two should still be outline.
	if !strings.Contains(html, `href="/exercises/ex-1/chart" class="btn" aria-current="page"`) {
		t.Error("expected Chart link to carry btn + aria-current=page")
	}
	if strings.Contains(html, `href="/exercises/ex-1" class="btn"`) {
		t.Error("did not expect History link to be marked as the active button")
	}
	if strings.Contains(html, `href="/exercises/ex-1/chart/advanced" class="btn"`) {
		t.Error("did not expect Advanced link to be marked as the active button")
	}

	// Page header is rendered for the dedicated chart view.
	if !strings.Contains(html, "Squat Progression") {
		t.Error("expected page header containing the exercise name + Progression")
	}
	if !strings.Contains(html, "Max weight per training day") {
		t.Error("expected subtitle describing the chart aggregation")
	}
}

// TestExerciseChart_RendersFullWidthChartCard locks in the chart card
// chrome: a full-width wrapper with the tall h-[60vh] min-h-96
// height (24rem floor), the dedicated "exercise-chart" canvas id
// (distinct from the small chart on the History view), and the JSON
// payload flagging hideAxes=false so axis tick labels are visible.
func TestExerciseChart_RendersFullWidthChartCard(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	now := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "e1", Reps: 5, Weight: 100, CreatedAt: now},
		{ID: "e2", Reps: 5, Weight: 105, CreatedAt: now.AddDate(0, 0, 7)},
		{ID: "e3", Reps: 3, Weight: 110, CreatedAt: now.AddDate(0, 0, 14)},
	}
	html := renderToString(t, ExerciseChart(exercise, chartExerciseEntries, "Test User", true, false, "kg", "km"))

	// Card container wraps the chart.
	if !strings.Contains(html, `<div class="card p-4">`) {
		t.Error("expected full-width card container with p-4 padding")
	}
	// Tall wrapper drives the chart's height.
	if !strings.Contains(html, `class="h-[60vh] min-h-96"`) {
		t.Error("expected tall fixed-height wrapper (h-[60vh] min-h-96)")
	}
	// Distinct canvas id so the History view's smaller chart and the
	// full-width chart can never collide.
	if !strings.Contains(html, `<canvas id="exercise-chart">`) {
		t.Error("expected canvas with id exercise-chart")
	}
	// JSON payload must NOT be hiding the axis tick labels at full width.
	re := regexp.MustCompile(`<script id="exercise-chart-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		t.Fatal("could not find exercise-chart-data script block")
	}
	var parsed struct {
		Labels   []string `json:"labels"`
		Datasets []struct {
			Label  string    `json:"label"`
			Values []float64 `json:"values"`
		} `json:"datasets"`
		HideAxes bool `json:"hideAxes"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("data block is not valid JSON: %v\ncontent: %s", err, m[1])
	}
	if parsed.HideAxes {
		t.Error("expected hideAxes=false at full width so axis tick labels are visible")
	}
	if len(parsed.Datasets) != 1 || parsed.Datasets[0].Label != "Squat (kg)" {
		t.Errorf("expected one dataset labelled 'Squat (kg)', got %+v", parsed.Datasets)
	}
	if len(parsed.Labels) != 3 || len(parsed.Datasets[0].Values) != 3 {
		t.Errorf("expected 3 day-bucketed points, got labels=%v values=%v", parsed.Labels, parsed.Datasets[0].Values)
	}
}

// TestExerciseChart_EmptyState asserts the friendly empty-state
// message is rendered in place of the chart when there is fewer than
// one unique day of data. The chart canvas and its JSON payload must
// not be rendered.
func TestExerciseChart_EmptyState(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	cases := map[string][]models.ExerciseEntry{
		"no entries": nil,
		"one entry on a single day": {
			{ID: "e1", Reps: 5, Weight: 100, CreatedAt: time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)},
		},
		"two entries on the same day (only one unique day)": {
			{ID: "e1", Reps: 5, Weight: 100, CreatedAt: time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)},
			{ID: "e2", Reps: 3, Weight: 105, CreatedAt: time.Date(2025, 6, 10, 17, 0, 0, 0, time.UTC)},
		},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			html := renderToString(t, ExerciseChart(exercise, entries, "Test User", true, false, "kg", "km"))
			if !strings.Contains(html, "Log at least 2 sessions to see your progression.") {
				t.Errorf("expected friendly empty-state message for %q", name)
			}
			if strings.Contains(html, `<canvas id="exercise-chart">`) {
				t.Errorf("did not expect chart canvas in empty state for %q", name)
			}
			if strings.Contains(html, `id="exercise-chart-data"`) {
				t.Errorf("did not expect chart JSON payload in empty state for %q", name)
			}
		})
	}
}

// TestExerciseChart_AggregatesMaxWeightPerDay asserts the chart reduces
// multiple sets on the same calendar day to a single point showing the
// heaviest weight of that day. Two days, four entries -> 2 chart points.
func TestExerciseChart_AggregatesMaxWeightPerDay(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	day1morning := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day1evening := time.Date(2025, 6, 10, 19, 30, 0, 0, time.UTC)
	day2morning := time.Date(2025, 6, 11, 9, 0, 0, 0, time.UTC)
	day2evening := time.Date(2025, 6, 11, 19, 30, 0, 0, time.UTC)
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "e1", Reps: 5, Weight: 100, CreatedAt: day1morning}, // day1: 100
		{ID: "e2", Reps: 3, Weight: 110, CreatedAt: day1evening}, // day1: 110 (max)
		{ID: "e3", Reps: 5, Weight: 112, CreatedAt: day2morning}, // day2: 112 (max)
		{ID: "e4", Reps: 5, Weight: 108, CreatedAt: day2evening}, // day2: 108 (not max)
	}
	html := renderToString(t, ExerciseChart(exercise, chartExerciseEntries, "Test User", true, false, "kg", "km"))

	re := regexp.MustCompile(`<script id="exercise-chart-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		t.Fatal("could not find exercise-chart-data script block")
	}
	var parsed struct {
		Labels   []string `json:"labels"`
		Datasets []struct {
			Label  string    `json:"label"`
			Values []float64 `json:"values"`
		} `json:"datasets"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("data block is not valid JSON: %v\ncontent: %s", err, m[1])
	}
	if len(parsed.Labels) != 2 {
		t.Fatalf("expected 2 day-bucketed labels, got %d (%v)", len(parsed.Labels), parsed.Labels)
	}
	if len(parsed.Datasets) != 1 {
		t.Fatalf("expected 1 dataset, got %d", len(parsed.Datasets))
	}
	wantValues := []float64{110, 112} // day1 max 110, day2 max 112
	if !floatSliceEqual(parsed.Datasets[0].Values, wantValues) {
		t.Errorf("expected max-weight-per-day values %v, got %v", wantValues, parsed.Datasets[0].Values)
	}
	// Labels should be the two day labels in ascending date order.
	if parsed.Labels[0] != "10 Jun" || parsed.Labels[1] != "11 Jun" {
		t.Errorf("expected labels [10 Jun, 11 Jun] in ascending order, got %v", parsed.Labels)
	}
}

// TestExerciseChartAdvanced mirrors TestExerciseChart but with the
// Advanced link marked as active. The body is exercised in the
// populated / empty-state / per-set-plotting tests below — this one
// focuses on the shared chrome that should be present in every variant.
//
// Two sets of data are passed so the chart (and its subtitle) is
// rendered. The empty-state case is covered by
// TestExerciseChartAdvanced_EmptyState.
func TestExerciseChartAdvanced(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "e1", Reps: 5, Weight: 100, CreatedAt: time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)},
		{ID: "e2", Reps: 5, Weight: 105, CreatedAt: time.Date(2025, 6, 17, 9, 0, 0, 0, time.UTC)},
	}
	html := renderToString(t, ExerciseChartAdvanced(exercise, chartExerciseEntries, "Test User", true, false, "kg"))

	if !strings.Contains(html, `class="button-group"`) {
		t.Error("expected basecoat button-group container")
	}
	if !strings.Contains(html, `role="group"`) {
		t.Error("expected role=\"group\" on the button-group container")
	}
	for _, want := range []string{
		`href="/exercises/ex-1"`,
		`href="/exercises/ex-1/chart"`,
		`href="/exercises/ex-1/chart/advanced"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected link %s in advanced chart view", want)
		}
	}

	if !strings.Contains(html, `href="/exercises/ex-1/chart/advanced" class="btn" aria-current="page"`) {
		t.Error("expected Advanced link to carry btn + aria-current=page")
	}
	if strings.Contains(html, `href="/exercises/ex-1/chart" class="btn"`) {
		t.Error("did not expect Chart link to be marked as the active button")
	}
	if strings.Contains(html, `href="/exercises/ex-1" class="btn"`) {
		t.Error("did not expect History link to be marked as the active button")
	}

	// Page header is rendered for the advanced chart view.
	if !strings.Contains(html, "Squat Volume") {
		t.Error("expected page header containing the exercise name + Volume")
	}
	if !strings.Contains(html, "Every set plotted by reps and weight") {
		t.Error("expected subtitle describing the scatter plot")
	}
}

// TestExerciseChartAdvanced_RendersScatterCard locks in the scatter
// card chrome: full-width card, tall wrapper, the dedicated
// "exercise-chart-advanced" canvas id (distinct from the line chart's
// "exercise-chart" id), and a JSON payload with xLabel/yLabel/points
// and hideAxes=false so axis tick labels are visible.
func TestExerciseChartAdvanced_RendersScatterCard(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	now := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "e1", Reps: 5, Weight: 100, CreatedAt: now},
		{ID: "e2", Reps: 3, Weight: 110, CreatedAt: now.AddDate(0, 0, 1)},
		{ID: "e3", Reps: 8, Weight: 90, CreatedAt: now.AddDate(0, 0, 2)},
	}
	html := renderToString(t, ExerciseChartAdvanced(exercise, chartExerciseEntries, "Test User", true, false, "kg"))

	if !strings.Contains(html, `<div class="card p-4">`) {
		t.Error("expected full-width card container with p-4 padding")
	}
	if !strings.Contains(html, `class="h-[60vh] min-h-96"`) {
		t.Error("expected tall fixed-height wrapper (h-[60vh] min-h-96)")
	}
	// Distinct canvas id from the line chart on /chart.
	if !strings.Contains(html, `<canvas id="exercise-chart-advanced">`) {
		t.Error("expected canvas with id exercise-chart-advanced")
	}
	if strings.Contains(html, `<canvas id="exercise-chart">`) {
		t.Error("did not expect the line-chart canvas id on the advanced view")
	}

	re := regexp.MustCompile(`<script id="exercise-chart-advanced-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		t.Fatal("could not find exercise-chart-advanced-data script block")
	}
	var parsed struct {
		XLabel string `json:"xLabel"`
		YLabel string `json:"yLabel"`
		Points []struct {
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			Date string  `json:"date"`
		} `json:"points"`
		HideAxes bool `json:"hideAxes"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("data block is not valid JSON: %v\ncontent: %s", err, m[1])
	}
	if parsed.XLabel != "Reps" {
		t.Errorf("expected xLabel 'Reps', got %q", parsed.XLabel)
	}
	if parsed.YLabel != "Squat (kg)" {
		t.Errorf("expected yLabel 'Squat (kg)', got %q", parsed.YLabel)
	}
	if parsed.HideAxes {
		t.Error("expected hideAxes=false at full width so axis tick labels are visible")
	}
	if len(parsed.Points) != 3 {
		t.Errorf("expected 3 scatter points (one per set), got %d", len(parsed.Points))
	}
}

// TestExerciseChartAdvanced_EmptyState asserts the friendly empty-state
// message is rendered in place of the chart when there are fewer than
// 2 sets to plot. The chart canvas and its JSON payload must not be
// rendered.
func TestExerciseChartAdvanced_EmptyState(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	cases := map[string][]models.ExerciseEntry{
		"no entries": nil,
		"one entry": {
			{ID: "e1", Reps: 5, Weight: 100, CreatedAt: time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)},
		},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			html := renderToString(t, ExerciseChartAdvanced(exercise, entries, "Test User", true, false, "kg"))
			if !strings.Contains(html, "Log at least 2 sets to see your volume profile.") {
				t.Errorf("expected friendly empty-state message for %q", name)
			}
			if strings.Contains(html, `<canvas id="exercise-chart-advanced">`) {
				t.Errorf("did not expect scatter canvas in empty state for %q", name)
			}
			if strings.Contains(html, `id="exercise-chart-advanced-data"`) {
				t.Errorf("did not expect scatter JSON payload in empty state for %q", name)
			}
		})
	}
}

// TestExerciseChartAdvanced_PlotsEverySet asserts the scatter view does
// NOT collapse by day — every set is its own (reps, weight) point so
// the user can see set-by-set volume patterns. Two days, three entries
// -> 3 scatter points.
func TestExerciseChartAdvanced_PlotsEverySet(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	day1morning := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day1evening := time.Date(2025, 6, 10, 19, 30, 0, 0, time.UTC)
	day2morning := time.Date(2025, 6, 11, 9, 0, 0, 0, time.UTC)
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "e1", Reps: 5, Weight: 100, CreatedAt: day1morning},
		{ID: "e2", Reps: 3, Weight: 110, CreatedAt: day1evening},
		{ID: "e3", Reps: 5, Weight: 112, CreatedAt: day2morning},
	}
	html := renderToString(t, ExerciseChartAdvanced(exercise, chartExerciseEntries, "Test User", true, false, "kg"))

	re := regexp.MustCompile(`<script id="exercise-chart-advanced-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		t.Fatal("could not find exercise-chart-advanced-data script block")
	}
	var parsed struct {
		Points []struct {
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			Date string  `json:"date"`
		} `json:"points"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("data block is not valid JSON: %v", err)
	}
	if len(parsed.Points) != 3 {
		t.Fatalf("expected 3 per-set points (no per-day collapse), got %d", len(parsed.Points))
	}
	// Spot-check the (reps, weight) pairs flow through untouched.
	wantXY := []struct {
		X, Y float64
	}{
		{5, 100},
		{3, 110},
		{5, 112},
	}
	for i, want := range wantXY {
		if parsed.Points[i].X != want.X || parsed.Points[i].Y != want.Y {
			t.Errorf("point %d: want (%.0f, %.0f), got (%.0f, %.0f)", i, want.X, want.Y, parsed.Points[i].X, parsed.Points[i].Y)
		}
	}
	// Each point should carry the formatted date for the tooltip.
	for i, p := range parsed.Points {
		if p.Date == "" {
			t.Errorf("point %d missing Date field for tooltip", i)
		}
	}
}

func TestHistoryTable_ShowsNextAndPrev(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
		},
		Page:    2,
		HasPrev: true,
		HasNext: true,
	}
	html := renderToString(t, HistoryTable(exercise, page, "kg", "km"))
	if !strings.Contains(html, `aria-label="Previous page"`) {
		t.Error("expected Previous button on a middle page")
	}
	if !strings.Contains(html, `aria-label="Next page"`) {
		t.Error("expected Next button on a middle page")
	}
	if !strings.Contains(html, `hx-get="/exercises/ex-1?page=1"`) {
		t.Error("expected Previous button to point at page 1")
	}
	if !strings.Contains(html, `hx-get="/exercises/ex-1?page=3"`) {
		t.Error("expected Next button to point at page 3")
	}
	if !strings.Contains(html, `hx-push-url="true"`) {
		t.Error("expected pagination links to push URL for shareable history")
	}
	if !strings.Contains(html, `hx-target="#`+historyTableWrapID+`"`) {
		t.Error("expected pagination to target the swappable wrap region")
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

// --- Cardio rendering ---

// TestExerciseHistory_Cardio verifies a cardio exercise's history page
// swaps the weight stat cards for session-based ones (Last Session /
// Best Pace) and never renders reps × weight values.
func TestExerciseHistory_Cardio(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-run", Name: "Run", Type: models.ExerciseTypeCardio}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: time.Now()},
		},
		Stats: models.HistoryStats{
			BestPaceSecPerKm:      300,
			LongestDistanceMeters: 5000,
			LastSet:               models.ExerciseEntry{ID: "entry-1", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000},
		},
		Page: 1,
	}
	html := renderToString(t, ExerciseHistory(exercise, page, nil, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, "Best Pace") {
		t.Error("expected Best Pace stat card on cardio history")
	}
	if !strings.Contains(html, "Last Session") {
		t.Error("expected Last Session stat card on cardio history")
	}
	if !strings.Contains(html, "5:00 /km") {
		t.Error("expected formatted best pace value")
	}
	if strings.Contains(html, "Personal Best") || strings.Contains(html, "Last Set") {
		t.Error("cardio history should not render strength stat cards")
	}
}

// TestExerciseHistory_CardioRendersPaceChart locks in that a cardio
// exercise's chart is fed the fastest-pace-per-day series (labelled
// min/km) rather than max weight.
func TestExerciseHistory_CardioRendersPaceChart(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-run", Name: "Run", Type: models.ExerciseTypeCardio}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: time.Now()},
		},
		Stats: models.HistoryStats{BestPaceSecPerKm: 300},
		Page:  1,
	}
	chartExerciseEntries := []models.ExerciseEntry{
		{ID: "c1", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC)}, // 300 s/km
		{ID: "c2", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1560, DistanceMeters: 5000, CreatedAt: time.Date(2024, 6, 12, 9, 0, 0, 0, time.UTC)}, // 312 s/km
		{ID: "c3", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1470, DistanceMeters: 5000, CreatedAt: time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)}, // 294 s/km (best)
	}
	html := renderToString(t, ExerciseHistory(exercise, page, chartExerciseEntries, "Test User", true, false, "kg", "km"))
	if !strings.Contains(html, `id="exercise-history-chart"`) {
		t.Fatal("expected chart canvas to render")
	}
	if !strings.Contains(html, "Last 3 Sessions") {
		t.Error("expected cardio chart title to count sessions")
	}
	re := regexp.MustCompile(`<script id="exercise-history-chart-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		t.Fatal("could not find chart data block")
	}
	var parsed struct {
		Datasets []struct {
			Label  string    `json:"label"`
			Values []float64 `json:"values"`
		} `json:"datasets"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("chart data not valid JSON: %v", err)
	}
	if len(parsed.Datasets) != 1 || parsed.Datasets[0].Label != "Run (min/km)" {
		t.Fatalf("expected dataset labelled 'Run (min/km)', got %+v", parsed.Datasets)
	}
	want := []float64{300, 312, 294} // fastest pace per day
	if !floatSliceEqual(parsed.Datasets[0].Values, want) {
		t.Errorf("expected fastest-pace-per-day values %v, got %v", want, parsed.Datasets[0].Values)
	}
}

// TestHistoryTable_CardioColumns verifies the paginated table renders
// Duration / Distance headers and values for cardio exercises.
func TestHistoryTable_CardioColumns(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-run", Name: "Run", Type: models.ExerciseTypeCardio}
	page := &models.ExerciseHistoryPage{
		ExerciseEntries: []models.ExerciseEntry{
			{ID: "entry-1", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 3900, DistanceMeters: 10000, CreatedAt: time.Now()},
		},
		Page: 1,
	}
	html := renderToString(t, HistoryTable(exercise, page, "kg", "mi"))
	for _, want := range []string{"<th>Duration</th>", "<th>Distance</th>", "1:05:00", "6.21 mi"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected cardio table to contain %q", want)
		}
	}
	if strings.Contains(html, "<th>Reps</th>") || strings.Contains(html, "<th>Weight</th>") {
		t.Error("cardio table should not render strength column headers")
	}
}

// TestExerciseChart_CardioHidesAdvancedNav verifies cardio exercises do
// not get an Advanced nav link (the scatter's axes are strength-only).
func TestExerciseChart_CardioHidesAdvancedNav(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-run", Name: "Run", Type: models.ExerciseTypeCardio}
	day1 := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 17, 9, 0, 0, 0, time.UTC)
	entries := []models.ExerciseEntry{
		{ID: "c1", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: day1},
		{ID: "c2", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1440, DistanceMeters: 5000, CreatedAt: day2},
	}
	html := renderToString(t, ExerciseChart(exercise, entries, "Test User", true, false, "kg", "km"))
	if strings.Contains(html, `/chart/advanced`) {
		t.Error("expected Advanced link to be hidden for cardio exercises")
	}
	if !strings.Contains(html, "Fastest pace per training day") {
		t.Error("expected cardio chart subtitle describing the pace aggregation")
	}
	if !strings.Contains(html, "Run Pace Progression") {
		t.Error("expected cardio-specific page header")
	}
}

// TestExerciseChartAdvanced_CardioNotice verifies direct navigation to
// the advanced view for a cardio exercise renders the explanation
// instead of the reps-vs-weight scatter.
func TestExerciseChartAdvanced_CardioNotice(t *testing.T) {
	exercise := &models.Exercise{ID: "ex-run", Name: "Run", Type: models.ExerciseTypeCardio}
	html := renderToString(t, ExerciseChartAdvanced(exercise, nil, "Test User", true, false, "kg"))
	if !strings.Contains(html, "strength exercises only") {
		t.Error("expected strength-only notice for cardio advanced view")
	}
	if strings.Contains(html, `<canvas id="exercise-chart-advanced">`) {
		t.Error("did not expect the scatter canvas for a cardio exercise")
	}
}
