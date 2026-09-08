package routes

import (
	"encoding/json"
	"strings"
	"testing"

	"hylete/internal/models"
)

// TestExerciseFromModel_MapsAliases verifies the filter-only aliases
// string reaches the iOS-bound DTO so the picker can match on
// alternate names without displaying them.
func TestExerciseFromModel_MapsAliases(t *testing.T) {
	dto := ExerciseFromModel(models.Exercise{
		ID:      "ex-1",
		Name:    "Bent-Over Row",
		Aliases: "barbell row,seated row",
		Type:    models.ExerciseTypeStrength,
	})
	if dto.Aliases != "barbell row,seated row" {
		t.Errorf("Aliases = %q, want %q", dto.Aliases, "barbell row,seated row")
	}

	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"aliases":"barbell row,seated row"`) {
		t.Errorf("expected aliases key in JSON, got %s", raw)
	}
}

// TestExerciseFromModel_EmptyAliases verifies an exercise without
// aliases encodes as an empty string (not null), keeping the Swift
// Codable decoder simple.
func TestExerciseFromModel_EmptyAliases(t *testing.T) {
	dto := ExerciseFromModel(models.Exercise{ID: "ex-1", Name: "Squat"})
	if dto.Aliases != "" {
		t.Errorf("Aliases = %q, want empty", dto.Aliases)
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"aliases":""`) {
		t.Errorf("expected empty aliases key in JSON, got %s", raw)
	}
}
