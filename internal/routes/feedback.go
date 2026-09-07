package routes

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/views/admin"
	"hylete/internal/views/feedback"
)

// FeedbackForm renders the user-facing GET /feedback page. The
// dashboard's iOS banner links here so web users can get in
// touch (e.g. to request iOS access). Authentication is enforced
// by the global auth middleware — /feedback is not in the public
// route list.
func (h *Handler) FeedbackForm(c echo.Context) error {
	claims := GetClaims(c)
	return render(c, feedback.FeedbackPage(claims.Name, true, claims.IsAdmin))
}

// SubmitFeedback handles POST /feedback. Validation and storage
// are delegated to FeedbackController.Submit — the same entry
// point the iOS client uses via POST /api/v1/feedback — so both
// surfaces enforce identical title/message rules. htmx requests
// get toast fragments (error for validation failures, success +
// a form-reset trigger on success); plain POSTs fall back to a
// redirect to the dashboard.
func (h *Handler) SubmitFeedback(c echo.Context) error {
	title := c.FormValue("title")
	message := c.FormValue("message")

	claims := GetClaims(c)
	if err := h.feedbackCtrl.Submit(title, message, claims.UserID); err != nil {
		return render(c, feedback.FeedbackFormError(feedbackValidationError(err)))
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		// The success page listens for this event and resets the
		// form fields (see internal/views/feedback/feedback.templ).
		c.Response().Header().Set("HX-Trigger", "feedbackSubmitted")
		return render(c, feedback.FeedbackFormSuccess("Thanks for your feedback"))
	}

	return c.Redirect(http.StatusSeeOther, dashboardPath)
}

// feedbackValidationError maps FeedbackController validation
// errors to user-friendly toast messages.
func feedbackValidationError(err error) string {
	switch {
	case errors.Is(err, controllers.ErrTitleTooShort):
		return "Title must be at least 5 characters"
	case errors.Is(err, controllers.ErrTitleTooLong):
		return "Title must be at most 100 characters"
	case errors.Is(err, controllers.ErrMessageTooShort):
		return "Message must be at least 10 characters"
	case errors.Is(err, controllers.ErrMessageTooLong):
		return "Message must be at most 1000 characters"
	default:
		return "Failed to submit feedback: " + err.Error()
	}
}

// AdminListFeedback renders the admin feedback inbox with an
// optional open/closed filter. The default view (no filter query
// param) shows only open feedback; "all" must be requested
// explicitly via ?filter=all.
func (h *Handler) AdminListFeedback(c echo.Context) error {
	filter := c.QueryParam("filter")
	if filter == "" {
		filter = "open"
	}

	claims := GetClaims(c)
	feedback, err := h.feedbackCtrl.AdminList(filter)
	if err != nil {
		return err
	}

	return render(c, admin.AdminFeedbackList(feedback, filter, claims.Name, true, claims.IsAdmin))
}

func (h *Handler) AdminFeedbackDetail(c echo.Context) error {
	id := c.Param("id")

	claims := GetClaims(c)
	feedback, err := h.feedbackCtrl.AdminDetail(id)
	if err != nil {
		return err
	}

	return render(c, admin.AdminFeedbackDetail(feedback, claims.Name, true, claims.IsAdmin))
}

func (h *Handler) AdminCloseFeedback(c echo.Context) error {
	id := c.Param("id")

	if err := h.feedbackCtrl.Close(id); err != nil {
		return err
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		// The detail page listens for this event and redirects back
		// to the feedback list once the confirmation toast has been
		// visible for a moment (see internal/views/admin/feedback.templ).
		c.Response().Header().Set("HX-Trigger", "feedbackStatusUpdated")
		return render(c, admin.AdminFeedbackClosedSuccess())
	}

	return c.Redirect(http.StatusSeeOther, "/admin/feedback")
}