package models

import (
	"testing"
	"time"
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

// TestParseDateOfBirth asserts the trust-boundary rules: real
// calendar date, on/after 1900-01-01, not in the future; blank
// reads as "clear the field".
func TestParseDateOfBirth(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"valid", "1996-03-04", "1996-03-04", false},
		{"blank clears", "", "", false},
		{"blank whitespace clears", "  ", "", false},
		{"leap day", "2000-02-29", "2000-02-29", false},
		{"today accepted", "2026-09-10", "2026-09-10", false},
		{"not a date", "old", "", true},
		{"wrong shape", "04/03/1996", "", true},
		{"impossible date", "1996-02-30", "", true},
		{"non-leap Feb 29", "1999-02-29", "", true},
		{"before minimum", "1899-12-31", "", true},
		{"minimum accepted", "1900-01-01", "1900-01-01", false},
		{"tomorrow is future", "2026-09-11", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDateOfBirth(tc.in, now)
			if tc.wantErr && err == nil {
				t.Fatalf("ParseDateOfBirth(%q) = %q, want error", tc.in, got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ParseDateOfBirth(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseDateOfBirth(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAgeAt asserts whole-years derivation including the
// day-before / day-of / day-after birthday boundaries and the
// Feb 29 rule (ages up on Mar 1 in non-leap years).
func TestAgeAt(t *testing.T) {
	for _, tc := range []struct {
		name string
		dob  string
		now  time.Time
		want int
	}{
		{"adult", "1996-03-04", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), 30},
		{"day before birthday", "1996-09-11", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), 29},
		{"birthday today", "1996-09-10", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), 30},
		{"day after birthday", "1996-09-09", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), 30},
		{"leap baby Feb 28 non-leap", "2000-02-29", time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), 25},
		{"leap baby Mar 1 non-leap", "2000-02-29", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 26},
		{"empty is unknown", "", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), -1},
		{"malformed is unknown", "not-a-date", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AgeAt(tc.dob, tc.now); got != tc.want {
				t.Fatalf("AgeAt(%q) = %d, want %d", tc.dob, got, tc.want)
			}
		})
	}
}
