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

// TestWeightEntryFromModel_MapsAngles verifies each photo angle slot
// reaches the DTO with its own key/URL/flag triple, the aggregate
// photo count reflects the filled slots, and empty slots are omitted
// from the JSON so the iOS decoder can rely on the has_* flags.
func TestWeightEntryFromModel_MapsAngles(t *testing.T) {
	dto := WeightEntryFromModel(models.WeightEntry{
		ID:            "w1",
		Weight:        80,
		FrontPhotoKey: "weight/u/front.jpg",
		BackPhotoKey:  "weight/u/back.jpg",
	})
	if !dto.HasFrontPhoto || dto.HasSidePhoto || !dto.HasBackPhoto {
		t.Errorf("flags = front:%v side:%v back:%v, want true/false/true", dto.HasFrontPhoto, dto.HasSidePhoto, dto.HasBackPhoto)
	}
	if !dto.HasPhoto {
		t.Errorf("has_photo = false, want true (two slots filled)")
	}
	if dto.PhotoCount != 2 {
		t.Errorf("photo_count = %d, want 2", dto.PhotoCount)
	}
	if dto.FrontPhotoKey != "weight/u/front.jpg" || dto.BackPhotoKey != "weight/u/back.jpg" {
		t.Errorf("keys = %q/%q, want the per-angle keys", dto.FrontPhotoKey, dto.BackPhotoKey)
	}

	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"front_photo_key"`, `"back_photo_key"`, `"photo_count":2`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("expected %s in JSON, got %s", want, raw)
		}
	}
	// Empty slots must be omitted, not encoded as empty strings.
	for _, want := range []string{`side_photo_key`, `side_photo_url`} {
		if strings.Contains(string(raw), want) {
			t.Errorf("expected no %s in JSON, got %s", want, raw)
		}
	}
}

// TestWeightEntryFromModel_NoPhotos verifies a photo-less entry maps
// to all-false flags and a zero count.
func TestWeightEntryFromModel_NoPhotos(t *testing.T) {
	dto := WeightEntryFromModel(models.WeightEntry{ID: "w1", Weight: 80})
	if dto.HasPhoto || dto.PhotoCount != 0 {
		t.Errorf("has_photo = %v photo_count = %d, want false/0", dto.HasPhoto, dto.PhotoCount)
	}
	if dto.HasFrontPhoto || dto.HasSidePhoto || dto.HasBackPhoto {
		t.Errorf("angle flags should all be false: %+v", dto)
	}
}
