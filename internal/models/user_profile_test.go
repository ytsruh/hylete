package models

import (
	"testing"
)

// TestNormalizeGender asserts the trust-boundary normaliser keeps
// the fixed list and maps everything else (including empty) to unset.
func TestNormalizeGender(t *testing.T) {
	for _, g := range ValidGenders {
		if got := NormalizeGender(g); got != g {
			t.Fatalf("NormalizeGender(%q) = %q, want same", g, got)
		}
	}
	for _, bad := range []string{"", "unknown", "MALE", "Male", "other"} {
		if got := NormalizeGender(bad); got != "" {
			t.Fatalf("NormalizeGender(%q) = %q, want empty", bad, got)
		}
	}
}

// TestGenderDisplay asserts nil and unknown stored values read as unset.
func TestGenderDisplay(t *testing.T) {
	var nilUser *User
	if got := nilUser.GenderDisplay(); got != "" {
		t.Fatalf("nil GenderDisplay = %q, want empty", got)
	}
	u := &User{Gender: "female"}
	if got := u.GenderDisplay(); got != "female" {
		t.Fatalf("GenderDisplay = %q, want female", got)
	}
	u.Gender = "bogus"
	if got := u.GenderDisplay(); got != "" {
		t.Fatalf("bogus GenderDisplay = %q, want empty", got)
	}
}

// TestFormatHeight asserts the cm label format used by the web
// profile, CSV-adjacent display and the AI USER_PROFILE block.
func TestFormatHeight(t *testing.T) {
	if got := FormatHeight(180); got != "180.0 cm" {
		t.Fatalf("FormatHeight = %q, want 180.0 cm", got)
	}
}
