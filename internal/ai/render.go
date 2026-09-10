package ai_coach

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"hylete/internal/models"
)

//go:embed prompts/weekly_review.md
var weeklyReviewPrompt string

// PromptVersion identifies the weekly prompt. Bump together with the
// <!-- prompt_version: --> header in prompts/weekly_review.md so
// every ai_reports row records which prompt produced it. Stored on
// the row, never sent to the model.
const PromptVersion = "v2"

// RenderWeekly substitutes the weekly prompt placeholders. statsJSON
// is the marshalled trainingstats.WeeklyStats; aim is the user's
// free-text goal (empty → "No stated aim."); goalsList is pre-joined
// "title (status)" lines; prefs carries display units; userProfile
// carries the optional height/age/gender context (empty →
// "No profile details provided.").
func RenderWeekly(statsJSON, aim, goalsList, prefs, userProfile string) string {
	aim = strings.TrimSpace(aim)
	if aim == "" {
		aim = "No stated aim."
	}
	goalsList = strings.TrimSpace(goalsList)
	if goalsList == "" {
		goalsList = "(no goals recorded)"
	}
	userProfile = strings.TrimSpace(userProfile)
	if userProfile == "" {
		userProfile = "No profile details provided."
	}
	out := weeklyReviewPrompt
	out = strings.ReplaceAll(out, "{{STATS_JSON}}", statsJSON)
	out = strings.ReplaceAll(out, "{{USER_AIM}}", aim)
	out = strings.ReplaceAll(out, "{{GOALS_LIST}}", goalsList)
	out = strings.ReplaceAll(out, "{{PREFS}}", prefs)
	out = strings.ReplaceAll(out, "{{USER_PROFILE}}", userProfile)
	return out
}

// RenderUserProfile builds the USER_PROFILE prompt block from the
// user's optional profile fields. Each set field renders as
// "key=value" joined by " "; every field unset yields "" so the
// caller falls back to "No profile details provided.". Height
// renders with one decimal in cm (e.g. "height=180.0 cm"); the birth
// date is never sent — only the whole-years age derived via
// models.AgeAt at now (e.g. "age=30"), so the LLM sees no raw DOB.
func RenderUserProfile(heightCm *float64, gender string, dob *string, now time.Time) string {
	parts := make([]string, 0, 3)
	if heightCm != nil {
		parts = append(parts, "height="+models.FormatHeight(*heightCm))
	}
	if g := models.NormalizeGender(gender); g != "" {
		parts = append(parts, "gender="+g)
	}
	if dob != nil {
		if age := models.AgeAt(*dob, now); age >= 0 {
			parts = append(parts, fmt.Sprintf("age=%d", age))
		}
	}
	return strings.Join(parts, " ")
}

// ValidatePromptVersion fails startup wiring loudly if the file
// header and the const drift apart. Called once by the service
// constructor.
func ValidatePromptVersion() error {
	if !strings.Contains(weeklyReviewPrompt, "prompt_version: "+PromptVersion) {
		return fmt.Errorf("ai: prompt file header does not contain prompt_version: %s", PromptVersion)
	}
	return nil
}
