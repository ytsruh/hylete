package routes

import (
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	viewsexport "hylete/internal/views/export"
)

// DataExportPage renders the Data Export page, the single home for
// the web app's bulk data-portability downloads (weight entries ZIP,
// exercise entries ZIP). The workout-logging surfaces live in the
// iOS client; this page is read-only.
func (h *Handler) DataExportPage(c echo.Context) error {
	claims := GetClaims(c)
	return render(c, viewsexport.DataExportPage(claims.Name, claims.IsAdmin))
}

// ExportExerciseEntriesZip streams a zip archive of the authenticated
// user's exercise entries (one CSV row per set) as the response body.
// Content-Disposition forces a download with a date-stamped filename.
// The zip is built in a background goroutine (see
// controllers.ExerciseEntryController.ExportExerciseEntriesZip) so the
// response streams end-to-end.
//
// Mirrors ExportWeightZip; exercise entries carry no photos so the
// archive is just exercise_entries.csv + manifest.json.
func (h *Handler) ExportExerciseEntriesZip(c echo.Context) error {
	claims := GetClaims(c)

	weightUnit, distanceUnit := h.displayUnits(c)

	reader, filename, err := h.exerciseEntryCtrl.ExportExerciseEntriesZip(c.Request().Context(), claims.UserID, weightUnit, distanceUnit)
	if err != nil {
		return err
	}

	c.Response().Header().Set(echo.HeaderContentType, "application/zip")
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Response().WriteHeader(http.StatusOK)
	if _, err := io.Copy(c.Response().Writer, reader); err != nil {
		// We've already sent headers, so we can't return a
		// proper error response. Log it and abort the
		// connection so the client doesn't see a truncated
		// zip labelled "OK".
		c.Logger().Errorf("exercise entries export stream failed: %v", err)
	}
	return nil
}
