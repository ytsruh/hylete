package routes

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/utils"
	"hylete/internal/views/admin"
)

// --- Admin User Handlers ---

// AdminListUsers renders the admin page listing all users.
func (h *Handler) AdminListUsers(c echo.Context) error {
	claims := GetClaims(c)
	users, err := h.adminUserCtrl.ListUsers(c.Request().Context())
	if err != nil {
		return err
	}

	return render(c, admin.AdminUserList(users, claims.UserID, claims.Name, true, claims.IsAdmin))
}

// AdminSetUserAdmin handles POST /admin/users/:id/admin. The form
// value is_admin ("true"/"false") carries the target state, so the
// same endpoint serves both grant and revoke. The controller rejects
// self-demotion (controllers.ErrCannotDemoteSelf). htmx requests get
// a toast fragment plus a triggerRedirect back to the list so the
// row re-renders with the new badge; plain POSTs fall back to a
// redirect.
func (h *Handler) AdminSetUserAdmin(c echo.Context) error {
	id := c.Param("id")
	isAdmin := c.FormValue("is_admin") == "true"
	claims := GetClaims(c)

	if err := h.adminUserCtrl.SetAdmin(c.Request().Context(), claims.UserID, id, isAdmin); err != nil {
		switch {
		case errors.Is(err, controllers.ErrUserNotFound):
			return render(c, admin.AdminUserActionError("User not found"))
		case errors.Is(err, controllers.ErrCannotDemoteSelf):
			return render(c, admin.AdminUserActionError("You can't remove your own admin access"))
		default:
			c.Logger().Errorf("admin set user admin failed: %v", err)
			return render(c, admin.AdminUserActionError("Failed to update user. Please try again."))
		}
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", `{"triggerRedirect": "/admin/users"}`)
		if isAdmin {
			return render(c, admin.AdminUserActionSuccess("Admin access granted"))
		}
		return render(c, admin.AdminUserActionSuccess("Admin access removed"))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/users")
}

// AdminSendUserPasswordReset handles POST /admin/users/:id/send-password-reset.
// Sends the same one-hour password-reset email the forgot-password
// flow sends — the admin action is only a different trigger, the
// reset link and /reset-password page are shared. htmx requests get
// a toast fragment; plain POSTs fall back to a redirect.
func (h *Handler) AdminSendUserPasswordReset(c echo.Context) error {
	id := c.Param("id")

	if err := h.adminUserCtrl.SendPasswordReset(c.Request().Context(), id); err != nil {
		if errors.Is(err, controllers.ErrUserNotFound) {
			return render(c, admin.AdminUserActionError("User not found"))
		}
		c.Logger().Errorf("admin send password reset failed: %v", err)
		return render(c, admin.AdminUserActionError("Failed to send password reset email. Please try again."))
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		return render(c, admin.AdminUserActionSuccess("Password reset email sent"))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/users")
}

// --- Admin Exercise Handlers ---

// AdminListExercises renders the admin page listing all exercises.
func (h *Handler) AdminListExercises(c echo.Context) error {
	claims := GetClaims(c)
	exercises, err := h.adminCtrl.List()
	if err != nil {
		return err
	}

	return render(c, admin.AdminExerciseList(exercises, claims.Name, true, claims.IsAdmin))
}

// AdminNewExerciseForm renders the form for creating a new exercise.
func (h *Handler) AdminNewExerciseForm(c echo.Context) error {
	claims := GetClaims(c)
	return render(c, admin.AdminExerciseForm(admin.AdminExerciseFormData{IsEdit: false}, claims.Name, true, claims.IsAdmin))
}

// AdminCreateExercise handles creating a new exercise.
func (h *Handler) AdminCreateExercise(c echo.Context) error {
	name := c.FormValue("name")
	if name == "" {
		return render(c, admin.AdminExerciseFormError("Exercise name is required"))
	}

	params := models.CreateExerciseParams{
		Name:           name,
		Description:    c.FormValue("description"),
		VideoURL:       c.FormValue("video_url"),
		ImgURL:         c.FormValue("img_key"),
		ImgURLOriginal: c.FormValue("img_key_original"),
		Type:           models.ExerciseType(c.FormValue("type")),
	}

	_, err := h.adminCtrl.Create(params)
	if err != nil {
		if errors.Is(err, controllers.ErrExerciseNameExists) {
			return render(c, admin.AdminExerciseFormError("An exercise with this name already exists"))
		}
		c.Logger().Errorf("admin create exercise failed: %v", err)
		return render(c, admin.AdminExerciseFormError("Failed to save exercise. Please try again."))
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", `{"triggerRedirect": "/admin/exercises"}`)
		return render(c, admin.AdminExerciseSuccessToast("Exercise created!"))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/exercises")
}

// AdminEditExerciseForm renders the form for editing an existing exercise.
func (h *Handler) AdminEditExerciseForm(c echo.Context) error {
	id := c.Param("id")

	exercise, err := h.adminCtrl.Get(id)
	if err != nil {
		if errors.Is(err, controllers.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "Exercise not found")
		}
		return err
	}

	claims := GetClaims(c)
	return render(c, admin.AdminExerciseForm(admin.AdminExerciseFormData{Exercise: exercise, IsEdit: true}, claims.Name, true, claims.IsAdmin))
}

// AdminUpdateExercise handles updating an existing exercise.
func (h *Handler) AdminUpdateExercise(c echo.Context) error {
	id := c.Param("id")

	name := c.FormValue("name")
	if name == "" {
		return render(c, admin.AdminExerciseFormError("Exercise name is required"))
	}

	// Load the current exercise so we can decide whether to delete
	// the previous R2 objects. Doing this before the update means
	// the new keys (if any) are still safe in the form payload.
	existing, err := h.adminCtrl.Get(id)
	if err != nil {
		if errors.Is(err, controllers.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "Exercise not found")
		}
		c.Logger().Errorf("admin load exercise for update failed: %v", err)
		return render(c, admin.AdminExerciseFormError("Failed to save exercise. Please try again."))
	}

	// Resolve the final img_url and img_url_original values:
	//  - if "clear_image" is checked, drop both to "" (and best-effort
	//    delete the old R2 objects)
	//  - else if a new key was uploaded, use it (and best-effort
	//    delete the old R2 objects)
	//  - else keep the existing values
	clearImage := c.FormValue("clear_image") == "true"
	newDisplayKey := c.FormValue("img_key")
	newOriginalKey := c.FormValue("img_key_original")

	finalDisplay := existing.ImgURL
	finalOriginal := existing.ImgURLOriginal

	if clearImage {
		finalDisplay = ""
		finalOriginal = ""
	} else if newDisplayKey != "" {
		// Only switch to the new key when the upload route actually
		// returned one. An empty input + an unchanged hidden value
		// means the user didn't change the image.
		finalDisplay = newDisplayKey
		finalOriginal = newOriginalKey
	}

	// Delete the old R2 objects if either:
	//  - the user cleared the image
	//  - the user replaced the image with a new upload
	// Best-effort — log and continue so a missing/old object doesn't
	// block the DB write.
	if clearImage || newDisplayKey != "" {
		if existing.ImgURL != "" {
			if delErr := utils.DeleteObject(existing.ImgURL); delErr != nil {
				c.Logger().Warnf("failed to delete exercise image %q from R2: %v", existing.ImgURL, delErr)
			}
		}
		if existing.ImgURLOriginal != "" {
			if delErr := utils.DeleteObject(existing.ImgURLOriginal); delErr != nil {
				c.Logger().Warnf("failed to delete exercise original image %q from R2: %v", existing.ImgURLOriginal, delErr)
			}
		}
	}

	params := models.UpdateExerciseParams{
		Name:           name,
		Description:    c.FormValue("description"),
		VideoURL:       c.FormValue("video_url"),
		ImgURL:         finalDisplay,
		ImgURLOriginal: finalOriginal,
		Type:           models.ExerciseType(c.FormValue("type")),
	}

	_, err = h.adminCtrl.Update(id, params)
	if err != nil {
		if errors.Is(err, controllers.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "Exercise not found")
		}
		if errors.Is(err, controllers.ErrExerciseNameExists) {
			return render(c, admin.AdminExerciseFormError("An exercise with this name already exists"))
		}
		c.Logger().Errorf("admin update exercise failed: %v", err)
		return render(c, admin.AdminExerciseFormError("Failed to save exercise. Please try again."))
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", `{"triggerRedirect": "/admin/exercises"}`)
		return render(c, admin.AdminExerciseSuccessToast("Exercise updated!"))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/exercises")
}
