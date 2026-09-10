package models

import (
	"testing"
)

// TestParseWeightPhotoAngle_DefaultsToFront verifies the empty angle
// (sent by older clients that never knew about slots) means front,
// while unknown strings are rejected.
func TestParseWeightPhotoAngle_DefaultsToFront(t *testing.T) {
	angle, ok := ParseWeightPhotoAngle("")
	if !ok || angle != WeightPhotoFront {
		t.Errorf("empty = (%q, %v), want (front, true)", string(angle), ok)
	}
	for _, raw := range []string{"front", "side", "back"} {
		if _, ok := ParseWeightPhotoAngle(raw); !ok {
			t.Errorf("angle %q should parse", raw)
		}
	}
	if _, ok := ParseWeightPhotoAngle("diagonal"); ok {
		t.Error("angle diagonal should not parse")
	}
}

// TestSharedWeightPhotoAngles verifies the helper returns only the
// angles both entries hold, in front/side/back preference order.
func TestSharedWeightPhotoAngles(t *testing.T) {
	a := &WeightEntry{FrontPhotoKey: "f", SidePhotoKey: "s"}
	b := &WeightEntry{FrontPhotoKey: "f", BackPhotoKey: "b"}
	shared := SharedWeightPhotoAngles(a, b)
	if len(shared) != 1 || shared[0] != WeightPhotoFront {
		t.Errorf("shared = %v, want [front]", shared)
	}

	c := &WeightEntry{FrontPhotoKey: "f", SidePhotoKey: "s", BackPhotoKey: "b"}
	shared = SharedWeightPhotoAngles(a, c)
	if len(shared) != 2 || shared[0] != WeightPhotoFront || shared[1] != WeightPhotoSide {
		t.Errorf("shared = %v, want [front side]", shared)
	}

	d := &WeightEntry{SidePhotoKey: "s"}
	if shared := SharedWeightPhotoAngles(a, d); len(shared) != 1 || shared[0] != WeightPhotoSide {
		t.Errorf("shared = %v, want [side]", shared)
	}
	if SharedWeightPhotoAngles(a, nil) != nil {
		t.Error("nil entry should yield no shared angles")
	}
}

// TestWeightEntryPhotoCount verifies the aggregate helpers count
// filled slots and report per-angle presence.
func TestWeightEntryPhotoCount(t *testing.T) {
	e := &WeightEntry{FrontPhotoKey: "f", BackPhotoKey: "b"}
	if e.PhotoCount() != 2 {
		t.Errorf("PhotoCount = %d, want 2", e.PhotoCount())
	}
	if !e.HasPhoto() {
		t.Error("HasPhoto should be true")
	}
	if e.HasPhotoForAngle(WeightPhotoSide) {
		t.Error("side slot should be empty")
	}
	if (&WeightEntry{}).HasPhoto() {
		t.Error("empty entry should have no photo")
	}
}
