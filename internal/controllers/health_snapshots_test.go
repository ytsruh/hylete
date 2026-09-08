package controllers

import (
	"errors"
	"sync"
	"testing"
	"time"

	"hylete/internal/models"
)

// fakeHealthSnapshotRepo is an in-memory models.HealthSnapshotRepo.
// Upsert overwrites the same (user_id, snapshot_date) key, mirroring
// the SQL ON CONFLICT contract so overwrite semantics are covered
// without a live DB.
type fakeHealthSnapshotRepo struct {
	mu   sync.Mutex
	rows map[string]models.HealthSnapshot

	errUpsert error
	errGet    error
	errList   error
}

func newFakeHealthSnapshotRepo() *fakeHealthSnapshotRepo {
	return &fakeHealthSnapshotRepo{rows: map[string]models.HealthSnapshot{}}
}

func healthSnapshotKey(userID, date string) string { return userID + "\x00" + date }

func (f *fakeHealthSnapshotRepo) Upsert(entry *models.HealthSnapshot) error {
	if f.errUpsert != nil {
		return f.errUpsert
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry.ID == "" {
		entry.ID = "hs-" + entry.SnapshotDate
	}
	cp := *entry
	f.rows[healthSnapshotKey(entry.UserID, entry.SnapshotDate)] = cp
	return nil
}

func (f *fakeHealthSnapshotRepo) GetByDate(userID, snapshotDate string) (*models.HealthSnapshot, error) {
	if f.errGet != nil {
		return nil, f.errGet
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[healthSnapshotKey(userID, snapshotDate)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (f *fakeHealthSnapshotRepo) ListRange(userID, startDate, endDate string) ([]models.HealthSnapshot, error) {
	if f.errList != nil {
		return nil, f.errList
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []models.HealthSnapshot
	for _, row := range f.rows {
		if row.UserID != userID {
			continue
		}
		if row.SnapshotDate >= startDate && row.SnapshotDate <= endDate {
			out = append(out, row)
		}
	}
	return out, nil
}

func TestValidateSnapshotDate(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 3).Format("2006-01-02")

	if err := validateSnapshotDate(yesterday); err != nil {
		t.Errorf("yesterday: got %v, want nil", err)
	}
	if err := validateSnapshotDate("2026-09-01"); err != nil {
		t.Errorf("fixed past date: got %v, want nil", err)
	}
	if err := validateSnapshotDate("09/01/2026"); !errors.Is(err, ErrHealthSnapshotDateInvalid) {
		t.Errorf("bad shape: got %v, want ErrHealthSnapshotDateInvalid", err)
	}
	if err := validateSnapshotDate(""); !errors.Is(err, ErrHealthSnapshotDateInvalid) {
		t.Errorf("empty: got %v, want ErrHealthSnapshotDateInvalid", err)
	}
	if err := validateSnapshotDate(future); !errors.Is(err, ErrHealthSnapshotDateFuture) {
		t.Errorf("future: got %v, want ErrHealthSnapshotDateFuture", err)
	}
}

func TestUpsertSnapshots_PersistsBatch(t *testing.T) {
	repo := newFakeHealthSnapshotRepo()
	ctrl := NewHealthSnapshotController(repo)
	when := time.Date(2026, 9, 6, 18, 30, 0, 0, time.UTC)

	out, err := ctrl.UpsertSnapshots("u1", []HealthSnapshotInput{
		{SnapshotDate: "2026-09-06", Tz: "Europe/London", Steps: 8432, Weight: 80.2, WeightMeasuredAt: &when},
		{SnapshotDate: "2026-09-05", Steps: 1200},
	})
	if err != nil {
		t.Fatalf("UpsertSnapshots: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2 (request order preserved)", len(out))
	}
	if out[0].Steps != 8432 || out[0].Tz != "Europe/London" {
		t.Errorf("first row = %+v, want 8432 steps / Europe/London", out[0])
	}
	if out[0].WeightMeasuredAt == nil || !out[0].WeightMeasuredAt.Equal(when) {
		t.Errorf("weight measured_at not round-tripped: %+v", out[0].WeightMeasuredAt)
	}
	if out[1].WeightMeasuredAt != nil {
		t.Errorf("absent measured_at should stay nil, got %v", out[1].WeightMeasuredAt)
	}
	if len(repo.rows) != 2 {
		t.Fatalf("repo rows = %d, want 2", len(repo.rows))
	}
}

func TestUpsertSnapshots_OverwriteSameDate(t *testing.T) {
	repo := newFakeHealthSnapshotRepo()
	ctrl := NewHealthSnapshotController(repo)

	if _, err := ctrl.UpsertSnapshots("u1", []HealthSnapshotInput{{SnapshotDate: "2026-09-06", Steps: 100}}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	out, err := ctrl.UpsertSnapshots("u1", []HealthSnapshotInput{{SnapshotDate: "2026-09-06", Steps: 9000}})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("repo rows = %d, want 1 (upsert, not duplicate)", len(repo.rows))
	}
	if out[0].Steps != 9000 {
		t.Errorf("steps = %d, want 9000 (second write wins)", out[0].Steps)
	}
}

func TestUpsertSnapshots_BadDateFailsWholeBatch(t *testing.T) {
	repo := newFakeHealthSnapshotRepo()
	ctrl := NewHealthSnapshotController(repo)

	_, err := ctrl.UpsertSnapshots("u1", []HealthSnapshotInput{
		{SnapshotDate: "2026-09-06", Steps: 100},
		{SnapshotDate: "not-a-date", Steps: 200},
	})
	if !errors.Is(err, ErrHealthSnapshotDateInvalid) {
		t.Fatalf("got %v, want ErrHealthSnapshotDateInvalid", err)
	}
	if len(repo.rows) != 0 {
		t.Errorf("repo rows = %d, want 0 (batch fails before any write)", len(repo.rows))
	}
}

func TestUpsertSnapshots_BatchTooLarge(t *testing.T) {
	repo := newFakeHealthSnapshotRepo()
	ctrl := NewHealthSnapshotController(repo)

	inputs := make([]HealthSnapshotInput, maxHealthSnapshotBatch+1)
	for i := range inputs {
		inputs[i].SnapshotDate = "2026-09-06"
	}
	if _, err := ctrl.UpsertSnapshots("u1", inputs); !errors.Is(err, ErrHealthSnapshotBatchTooLarge) {
		t.Errorf("got %v, want ErrHealthSnapshotBatchTooLarge", err)
	}
}

func TestNormalizeMeasuredAt_ZeroBecomesNil(t *testing.T) {
	if got := normalizeMeasuredAt(nil); got != nil {
		t.Errorf("nil: got %v, want nil", got)
	}
	zero := time.Time{}
	if got := normalizeMeasuredAt(&zero); got != nil {
		t.Errorf("zero: got %v, want nil", got)
	}
	when := time.Now()
	if got := normalizeMeasuredAt(&when); got == nil || !got.Equal(when) {
		t.Errorf("non-zero: got %v, want %v", got, when)
	}
}

func TestListSnapshots_DelegatesRange(t *testing.T) {
	repo := newFakeHealthSnapshotRepo()
	ctrl := NewHealthSnapshotController(repo)
	repo.rows[healthSnapshotKey("u1", "2026-09-06")] = models.HealthSnapshot{UserID: "u1", SnapshotDate: "2026-09-06"}
	repo.rows[healthSnapshotKey("u2", "2026-09-06")] = models.HealthSnapshot{UserID: "u2", SnapshotDate: "2026-09-06"}

	out, err := ctrl.ListSnapshots("u1", "2026-09-01", "2026-09-07")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(out) != 1 || out[0].UserID != "u1" {
		t.Errorf("got %+v, want only u1's row", out)
	}
}
