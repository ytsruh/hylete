package ai_coach

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed prompts/weekly_review.md
var weeklyReviewPrompt string

// PromptVersion identifies the weekly prompt. Bump together with the
// <!-- prompt_version: --> header in prompts/weekly_review.md so
// every ai_reports row records which prompt produced it. Stored on
// the row, never sent to the model.
const PromptVersion = "v1"

// RenderWeekly substitutes the weekly prompt placeholders. statsJSON
// is the marshalled trainingstats.WeeklyStats; aim is the user's
// free-text goal (empty → "No stated aim."); goalsList is pre-joined
// "title (status)" lines; prefs carries display units.
func RenderWeekly(statsJSON, aim, goalsList, prefs string) string {
	aim = strings.TrimSpace(aim)
	if aim == "" {
		aim = "No stated aim."
	}
	goalsList = strings.TrimSpace(goalsList)
	if goalsList == "" {
		goalsList = "(no goals recorded)"
	}
	out := weeklyReviewPrompt
	out = strings.ReplaceAll(out, "{{STATS_JSON}}", statsJSON)
	out = strings.ReplaceAll(out, "{{USER_AIM}}", aim)
	out = strings.ReplaceAll(out, "{{GOALS_LIST}}", goalsList)
	out = strings.ReplaceAll(out, "{{PREFS}}", prefs)
	return out
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
