package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// mockHealthSnapshotRepository satisfies models.HealthSnapshotRepo
// for the route tests. Upsert overwrites the same
// (user_id, snapshot_date) key like the SQL ON CONFLICT clause;
// ListRange filters by user and date window, newest first, like
// the real query. Per-method error injection covers the 500
// paths without a live DB.
type mockHealthSnapshotRepository struct {
	mu   sync.Mutex
	rows map[string]models.HealthSnapshot

	errUpsert error
	errGet    error
	errList   error
}

func newMockHealthSnapshotRepository() *mockHealthSnapshotRepository {
	return &mockHealthSnapshotRepository{rows: map[string]models.HealthSnapshot{}}
}

func mockHealthKey(userID, date string) string { return userID + "\x00" + date }

func (m *mockHealthSnapshotRepository) Upsert(entry *models.HealthSnapshot) error {
	if m.errUpsert != nil {
		return m.errUpsert
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry.ID == "" {
		entry.ID = "hs-" + entry.SnapshotDate
	}
	cp := *entry
	m.rows[mockHealthKey(entry.UserID, entry.SnapshotDate)] = cp
	return nil
}

func (m *mockHealthSnapshotRepository) GetByDate(userID, snapshotDate string) (*models.HealthSnapshot, error) {
	if m.errGet != nil {
		return nil, m.errGet
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[mockHealthKey(userID, snapshotDate)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (m *mockHealthSnapshotRepository) ListRange(userID, startDate, endDate string) ([]models.HealthSnapshot, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.HealthSnapshot
	for _, row := range m.rows {
		if row.UserID != userID {
			continue
		}
		if row.SnapshotDate >= startDate && row.SnapshotDate <= endDate {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SnapshotDate > out[j].SnapshotDate })
	return out, nil
}

// mustSwapHealthRepo replaces h.healthCtrl with a controller
// backed by the supplied mock, mirroring mustSwapWeightRepo so
// tests can seed and assert on the repository directly.
func mustSwapHealthRepo(t *testing.T, h *Handler, repo *mockHealthSnapshotRepository) *mockHealthSnapshotRepository {
	t.Helper()
	h.healthCtrl = controllers.NewHealthSnapshotController(repo)
	return repo
}

func healthSnapshotItem(date string, steps int64) HealthSnapshotItem {
	return HealthSnapshotItem{SnapshotDate: date, Tz: "Europe/London", Steps: steps}
}

func TestAPIUpsertHealthSnapshots_Batch(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hs@example.com", "HS")
	mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())
	when := time.Date(2026, 9, 6, 18, 30, 0, 0, time.UTC)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	dayBefore := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	item := healthSnapshotItem(yesterday, 8432)
	item.Weight = 80.2
	item.WeightMeasuredAt = &when
	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{
		Snapshots: []HealthSnapshotItem{item, healthSnapshotItem(dayBefore, 1200)},
	})
	resp := decodeAPI[HealthSnapshotsResponse](t, rec, http.StatusOK)
	if len(resp.Snapshots) != 2 {
		t.Fatalf("snapshots len = %d, want 2", len(resp.Snapshots))
	}
	first := resp.Snapshots[0]
	if first.SnapshotDate != yesterday || first.Steps != 8432 || first.Tz != "Europe/London" {
		t.Errorf("first = %+v, want yesterday / 8432 / Europe/London", first)
	}
	if first.WeightMeasuredAt == nil || !first.WeightMeasuredAt.Equal(when) {
		t.Errorf("weight_measured_at not round-tripped: %+v", first.WeightMeasuredAt)
	}
	if resp.Snapshots[1].WeightMeasuredAt != nil {
		t.Errorf("absent measured_at should be nil, got %v", resp.Snapshots[1].WeightMeasuredAt)
	}
}

func TestAPIUpsertHealthSnapshots_Overwrite(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hso@example.com", "HSO")
	repo := mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	post := func(steps int64) {
		apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{
			Snapshots: []HealthSnapshotItem{healthSnapshotItem(yesterday, steps)},
		})
	}
	post(100)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{
		Snapshots: []HealthSnapshotItem{healthSnapshotItem(yesterday, 9000)},
	})
	resp := decodeAPI[HealthSnapshotsResponse](t, rec, http.StatusOK)
	if resp.Snapshots[0].Steps != 9000 {
		t.Fatalf("steps = %d, want 9000 (second write wins)", resp.Snapshots[0].Steps)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("repo rows = %d, want 1 (upsert, not duplicate)", len(repo.rows))
	}
}

func TestAPIUpsertHealthSnapshots_BadDate(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hsb@example.com", "HSB")
	mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())

	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{
		Snapshots: []HealthSnapshotItem{healthSnapshotItem("06/09/2026", 100)},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPIUpsertHealthSnapshots_BadMetric(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hsm@example.com", "HSM")
	mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())

	item := healthSnapshotItem(time.Now().AddDate(0, 0, -1).Format("2006-01-02"), -5)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{
		Snapshots: []HealthSnapshotItem{item},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPIUpsertHealthSnapshots_EmptyBatch(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hse@example.com", "HSE")
	mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())

	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPIUpsertHealthSnapshots_RepoError(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hsr@example.com", "HSR")
	repo := mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())
	repo.errUpsert = errors.New("boom")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, UpsertHealthSnapshotsRequest{
		Snapshots: []HealthSnapshotItem{healthSnapshotItem("2026-09-06", 100)},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPIListHealthSnapshots_RangeAndIsolation(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hsl@example.com", "HSL")
	userID := "user-hsl@example.com"
	repo := mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())
	repo.rows[mockHealthKey(userID, "2026-09-06")] = models.HealthSnapshot{ID: "a", UserID: userID, SnapshotDate: "2026-09-06", Steps: 1}
	repo.rows[mockHealthKey(userID, "2026-09-05")] = models.HealthSnapshot{ID: "b", UserID: userID, SnapshotDate: "2026-09-05", Steps: 2}
	repo.rows[mockHealthKey("other-user", "2026-09-06")] = models.HealthSnapshot{ID: "c", UserID: "other-user", SnapshotDate: "2026-09-06", Steps: 3}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/health-snapshots?from=2026-09-01&to=2026-09-07", token, nil)
	resp := decodeAPI[HealthSnapshotsResponse](t, rec, http.StatusOK)
	if len(resp.Snapshots) != 2 {
		t.Fatalf("snapshots len = %d, want 2 (other user's row excluded)", len(resp.Snapshots))
	}
	if resp.Snapshots[0].SnapshotDate != "2026-09-06" {
		t.Errorf("first = %s, want newest first (2026-09-06)", resp.Snapshots[0].SnapshotDate)
	}
}

func TestAPIListHealthSnapshots_BadRanges(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hslb@example.com", "HSLB")
	mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())

	for _, q := range []string{
		"?from=2026-09-01",               // half-specified
		"?from=09/01/2026&to=2026-09-07", // bad shape
		"?from=2026-09-07&to=2026-09-01", // reversed
		"?from=2020-01-01&to=2026-09-07", // over 400 days
		"?days=abc",                      // not an integer
	} {
		rec := apiDo(t, e, http.MethodGet, "/api/v1/health-snapshots"+q, token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("q=%s: status = %d, want 400, body = %s", q, rec.Code, rec.Body.String())
		}
	}
}

// TestAPIUpsertHealthSnapshots_SwiftWireShape posts the exact
// JSON the Swift client emits (ISO8601 "Z" measured_at, absent
// keys for nil timestamps — never explicit null) to lock the
// cross-language contract. If either side renames a key or
// changes date encoding, this fails instead of the sync
// silently dropping data in production.
func TestAPIUpsertHealthSnapshots_SwiftWireShape(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hsw@example.com", "HSW")
	mustSwapHealthRepo(t, h, newMockHealthSnapshotRepository())

	raw := json.RawMessage(`{"snapshots": [{
		"snapshot_date": "2026-09-06",
		"tz": "Europe/London",
		"steps": 8432,
		"distance_meters": 5123.5,
		"active_energy_kcal": 412,
		"basal_energy_kcal": 1500,
		"exercise_minutes": 32,
		"sleep_seconds": 25920,
		"weight": 80.2,
		"weight_measured_at": "2026-09-06T18:30:00Z",
		"bmi": 24.9,
		"body_fat_percentage": 18.2,
		"lean_body_mass": 65.1,
		"heart_rate": 72,
		"resting_heart_rate": 58,
		"resting_hr_measured_at": "2026-09-06T07:00:00Z",
		"walking_heart_rate_avg": 95,
		"hrv_ms": 42,
		"cardio_recovery_bpm": 22,
		"vo2_max": 45.5
	}]}`)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/health-snapshots", token, raw)
	resp := decodeAPI[HealthSnapshotsResponse](t, rec, http.StatusOK)
	if len(resp.Snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(resp.Snapshots))
	}
	got := resp.Snapshots[0]
	if got.Steps != 8432 || got.Weight != 80.2 || got.Tz != "Europe/London" {
		t.Errorf("scalar round-trip failed: %+v", got)
	}
	if got.WeightMeasuredAt == nil || got.RestingHRMeasuredAt == nil {
		t.Errorf("measured_at timestamps lost: %+v", got)
	}
	if got.BMIMeasuredAt != nil || got.HRVMeasuredAt != nil {
		t.Errorf("absent keys should decode nil, got bmi=%v hrv=%v", got.BMIMeasuredAt, got.HRVMeasuredAt)
	}
}
