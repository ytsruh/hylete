package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	aicoach "hylete/internal/ai"
	"hylete/internal/models"
)

// CoachReportDTO is the wire shape of one stored ai_reports row.
// Payload is the validated report JSON produced from the versioned
// prompt; the server returns it verbatim so prompt evolution cannot
// break old rows on the client.
type CoachReportDTO struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	PeriodStart   time.Time       `json:"period_start"`
	PeriodEnd     time.Time       `json:"period_end"`
	PromptVersion string          `json:"prompt_version"`
	Model         string          `json:"model"`
	Payload       json.RawMessage `json:"payload"`
	DismissedAt   *time.Time      `json:"dismissed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// CoachReportFromModel converts a domain AIReport to its DTO.
func CoachReportFromModel(r models.AIReport) CoachReportDTO {
	return CoachReportDTO{
		ID: r.ID, Type: r.Type,
		PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd,
		PromptVersion: r.PromptVersion, Model: r.Model,
		Payload:     json.RawMessage(r.PayloadJSON),
		DismissedAt: r.DismissedAt,
		CreatedAt:   r.CreatedAt,
	}
}

// CoachPreferencesDTO is the wire shape of the Coach consent state.
// It lives on the user row (ai_opt_in + ai_goal_text) and is the
// server-side gate: no workout data leaves the server unless
// opt_in is true.
type CoachPreferencesDTO struct {
	OptIn    bool   `json:"opt_in"`
	GoalText string `json:"goal_text"`
}

// UpdateCoachPreferencesRequest is the PUT body. GoalText is trimmed
// and capped at models.AIGoalTextMaxLength; longer bodies are a 400.
type UpdateCoachPreferencesRequest struct {
	OptIn    bool   `json:"opt_in"`
	GoalText string `json:"goal_text"`
}

// SetCoachService attaches the Coach orchestrator and report repo.
// Kept as a setter (rather than NewHandler params) so existing
// construction sites and tests are untouched; a nil service means
// "AI disabled" and every report endpoint answers 503.
func (h *Handler) SetCoachService(svc *aicoach.Service, reports models.AIReportRepo) {
	h.aiService = svc
	h.aiReports = reports
}

// coachGate loads the caller and enforces the server-side consent
// gate. requireEnabled additionally demands Cloudflare credentials:
// report endpoints need them, preference endpoints do not (a user
// may opt in before the deploy has keys).
func (h *Handler) coachGate(c echo.Context, requireEnabled bool) (*models.User, error) {
	claims := GetClaims(c)
	user, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		return nil, c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load user"})
	}
	if user == nil {
		return nil, c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
	}
	if !user.AIOptIn {
		return nil, c.JSON(http.StatusForbidden, APIError{Error: "coach is not enabled for this user"})
	}
	if requireEnabled && (h.aiService == nil || !h.aiService.Enabled()) {
		return nil, c.JSON(http.StatusServiceUnavailable, APIError{Error: "coach is unavailable"})
	}
	if h.aiReports == nil {
		return nil, c.JSON(http.StatusServiceUnavailable, APIError{Error: "coach is unavailable"})
	}
	return user, nil
}

// APICoachLatest handles GET /api/v1/coach/weekly:latest. Returns
// the newest stored weekly report, or 404 when the cron has not
// produced one yet (thin beta, new opt-in).
func (h *Handler) APICoachLatest(c echo.Context) error {
	user, errResp := h.coachGate(c, true)
	if errResp != nil {
		return errResp
	}
	report, err := h.aiReports.Latest(user.ID, models.AIReportTypeWeekly)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load report"})
	}
	if report == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "no report yet"})
	}
	return c.JSON(http.StatusOK, CoachReportFromModel(*report))
}

// APICoachHistory handles GET /api/v1/coach/weekly?limit=N (default
// 12, max 52). Newest first.
func (h *Handler) APICoachHistory(c echo.Context) error {
	user, errResp := h.coachGate(c, true)
	if errResp != nil {
		return errResp
	}
	limit := 12
	if raw := c.QueryParam("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	reports, err := h.aiReports.List(user.ID, models.AIReportTypeWeekly, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load reports"})
	}
	out := make([]CoachReportDTO, len(reports))
	for i, r := range reports {
		out[i] = CoachReportFromModel(r)
	}
	return c.JSON(http.StatusOK, map[string]any{"reports": out})
}

// APICoachDismiss handles POST /api/v1/coach/weekly/:id/dismiss.
func (h *Handler) APICoachDismiss(c echo.Context) error {
	user, errResp := h.coachGate(c, false)
	if errResp != nil {
		return errResp
	}
	id := c.Param("id")
	report, err := h.aiReports.GetByID(id, user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load report"})
	}
	if report == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "report not found"})
	}
	if err := h.aiReports.MarkDismissed(id, user.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to dismiss report"})
	}
	return c.NoContent(http.StatusNoContent)
}

// APICoachRestore handles POST /api/v1/coach/weekly/:id/restore.
// Clears dismissed_at so the report returns to the card list.
// Idempotent: restoring a live row is a no-op success.
func (h *Handler) APICoachRestore(c echo.Context) error {
	user, errResp := h.coachGate(c, false)
	if errResp != nil {
		return errResp
	}
	id := c.Param("id")
	report, err := h.aiReports.GetByID(id, user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load report"})
	}
	if report == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "report not found"})
	}
	if err := h.aiReports.Reopen(id, user.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to restore report"})
	}
	return c.NoContent(http.StatusNoContent)
}

// APICoachGetPreferences handles GET /api/v1/me/coach-preferences.
// Available even when the AI backend is unconfigured so the iOS
// Profile screen always renders.
func (h *Handler) APICoachGetPreferences(c echo.Context) error {
	claims := GetClaims(c)
	user, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load user"})
	}
	if user == nil {
		return c.JSON(http.StatusNotFound, APIError{Error: "user not found"})
	}
	return c.JSON(http.StatusOK, CoachPreferencesDTO{OptIn: user.AIOptIn, GoalText: user.AIGoalText})
}

// APICoachUpdatePreferences handles PUT /api/v1/me/coach-preferences.
// The single writer for AI consent state: opt-in toggle + free-text
// aim (trimmed, capped at models.AIGoalTextMaxLength).
func (h *Handler) APICoachUpdatePreferences(c echo.Context) error {
	var in UpdateCoachPreferencesRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	goalText := strings.TrimSpace(in.GoalText)
	if len(goalText) > models.AIGoalTextMaxLength {
		return c.JSON(http.StatusBadRequest, APIError{Error: "goal text must be 1000 characters or less"})
	}
	claims := GetClaims(c)
	if err := h.userRepo.UpdateUserAIPreferences(claims.UserID, in.OptIn, goalText); err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to save preferences"})
	}
	return c.JSON(http.StatusOK, CoachPreferencesDTO{OptIn: in.OptIn, GoalText: goalText})
}
