// Package routes provides HTTP route handlers for the strength tracker application.
package routes

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"hylete/internal/models"
	"hylete/internal/views/dashboard"
	exerciseviews "hylete/internal/views/exercise"
)

// --- Exercise Entry Handlers ---
//
// The web app is a read-only surface for exercise entries: it renders
// the dashboard's recent-activity table and the per-exercise
// history/chart pages. Creating, editing, and deleting exercise
// entries happens in the iOS client via /api/v1/exercise-entries.

// Dashboard renders the main dashboard page.
func (h *Handler) Dashboard(c echo.Context) error {
	claims := GetClaims(c)
	exerciseEntries, err := h.exerciseEntryCtrl.ListExerciseEntriesLast7Days(claims.UserID)
	if err != nil {
		return err
	}

	weightUnit, distanceUnit := h.displayUnits(c)

	return render(c, dashboard.Dashboard(exerciseEntries, claims.Name, true, claims.IsAdmin, weightUnit, distanceUnit))
}

// ExerciseHistory shows all exercise entries for a specific exercise.
//
// Reads the optional ?page=N query parameter (1-indexed, clamped to 1) and
// returns a paginated page of exercise entries together with lifetime stats.
// When the request comes from htmx (HX-Request: true), the response is the
// table fragment only so it can swap into the existing #history-table-wrap
// region.
func (h *Handler) ExerciseHistory(c echo.Context) error {
	id := c.Param("id")

	claims := GetClaims(c)
	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(id, claims.UserID)
	if err != nil {
		return err
	}

	page := parsePage(c.QueryParam("page"))

	history, err := h.exerciseEntryCtrl.GetExerciseEntriesByExercise(exercise.ID, claims.UserID, page)
	if err != nil {
		return err
	}

	weightUnit, distanceUnit := h.displayUnits(c)

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("Vary", "HX-Request")
		return render(c, exerciseviews.HistoryTable(exercise, history, weightUnit, distanceUnit))
	}
	chartExerciseEntries, err := h.exerciseEntryCtrl.GetRecentExerciseEntriesForChart(exercise.ID, claims.UserID)
	if err != nil {
		return err
	}
	return render(c, exerciseviews.ExerciseHistory(exercise, history, chartExerciseEntries, claims.Name, true, claims.IsAdmin, weightUnit, distanceUnit))
}

// ExerciseChart renders the dedicated chart view for a specific exercise:
// a full-width line chart of the user's full workout history for that
// exercise, with the shared ExerciseNav button group at the top so the
// user can switch to the History or Advanced sub-views.
//
// All of the user's exercise entries for the exercise are fetched (uncapped)
// and handed to the view, which aggregates to "heaviest weight per calendar
// day" before plotting. With fewer than 2 unique days the view shows a
// short empty-state message instead of a chart.
func (h *Handler) ExerciseChart(c echo.Context) error {
	id := c.Param("id")

	claims := GetClaims(c)
	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(id, claims.UserID)
	if err != nil {
		return err
	}
	if exercise == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Exercise not found")
	}

	chartExerciseEntries, err := h.exerciseEntryCtrl.GetAllExerciseEntriesForChart(exercise.ID, claims.UserID)
	if err != nil {
		return err
	}

	weightUnit, distanceUnit := h.displayUnits(c)

	return render(c, exerciseviews.ExerciseChart(exercise, chartExerciseEntries, claims.Name, true, claims.IsAdmin, weightUnit, distanceUnit))
}

// ExerciseChartAdvanced renders the advanced chart view for a specific
// exercise: a full-width scatter plot of every set the user has logged,
// with reps on the x axis and weight on the y axis. All of the user's
// exercise entries for the exercise are fetched (uncapped) and handed to
// the view, which plots one translucent dot per set without per-day
// aggregation. With fewer than 2 exercise entries the view shows a short
// empty-state message instead of a chart. Cardio exercises render a short
// "strength only" notice instead of the scatter.
func (h *Handler) ExerciseChartAdvanced(c echo.Context) error {
	id := c.Param("id")

	claims := GetClaims(c)
	exercise, err := h.exerciseEntryCtrl.GetExerciseByID(id, claims.UserID)
	if err != nil {
		return err
	}
	if exercise == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Exercise not found")
	}

	chartExerciseEntries, err := h.exerciseEntryCtrl.GetAllExerciseEntriesForChart(exercise.ID, claims.UserID)
	if err != nil {
		return err
	}

	weightUnit, _ := h.displayUnits(c)

	return render(c, exerciseviews.ExerciseChartAdvanced(exercise, chartExerciseEntries, claims.Name, true, claims.IsAdmin, weightUnit))
}

// displayUnits returns the cached user's preferred weight and distance
// display units, falling back to "kg" / "km" when no user is attached to
// the request context. Use this at every render call site that needs both
// so the fallbacks live in one place.
func (h *Handler) displayUnits(c echo.Context) (weightUnit string, distanceUnit string) {
	weightUnit = models.NormalizeWeightUnit("")
	distanceUnit = models.NormalizeDistanceUnit("")
	if user := h.GetUser(c); user != nil {
		weightUnit = user.WeightUnitDisplay()
		distanceUnit = user.DistanceUnitDisplay()
	}
	return weightUnit, distanceUnit
}

// parsePage reads the ?page=N query param and returns a clamped 1-indexed page
// number. Defaults to 1 for empty or non-numeric values; values below 1 are
// clamped to 1.
func parsePage(raw string) int {
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 1
	}
	if n < 1 {
		return 1
	}
	return n
}

// ListExercisesUI renders the exercises list page.
func (h *Handler) ListExercisesUI(c echo.Context) error {
	claims := GetClaims(c)
	exercises, err := h.exerciseEntryCtrl.List()
	if err != nil {
		return err
	}

	return render(c, exerciseviews.ExercisesList(exercises, claims.Name, true, claims.IsAdmin))
}
