package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/utils"
)

// --- Test helpers ---

// apiDo issues a request through the test Echo instance with
// an optional Bearer token and optional JSON body. Returns
// the recorder for assertion. The content-type is forced to
// application/json so the API handlers go down the JSON
// binding path.
func apiDo(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(raw)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// decodeAPI unmarshals a 2xx body into T. Fails the test on
// a non-matching status or a malformed body so callers only
// have to assert on the decoded shape.
func decodeAPI[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, wantStatus, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, rec.Body.String())
	}
	return out
}

// decodeAPIError unmarshals a 4xx/5xx body into an APIError.
func decodeAPIError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) APIError {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, wantStatus, rec.Body.String())
	}
	var e APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body: %v (body=%s)", err, rec.Body.String())
	}
	return e
}

// loginUser is a thin wrapper that seeds a user into the
// in-memory user repo and returns the JWT. The iOS app
// exercises the same "register then login" flow, so the
// tests use the real auth controller rather than hand-rolling
// tokens.
func loginUser(t *testing.T, h *Handler, mockUser *mockUserRepository, email, name string) (string, *models.User) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	mockUser.users = append(mockUser.users, models.User{
		ID:           "user-" + email,
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
		WeightUnit:   "kg",
	})
	user, token, err := h.authCtrl.Login(email, "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return token, user
}

// --- /auth/login ---

func TestAPILogin_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	loginUser(t, h, mockUser, "test@example.com", "Test")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/login", "", LoginRequest{
		Email:    "test@example.com",
		Password: "secret123",
	})
	resp := decodeAPI[AuthResponse](t, rec, http.StatusOK)
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if resp.User.Email != "test@example.com" {
		t.Fatalf("user email = %q, want test@example.com", resp.User.Email)
	}
	if resp.User.WeightUnit != "kg" {
		t.Fatalf("user weight_unit = %q, want kg", resp.User.WeightUnit)
	}
}

func TestAPILogin_BadCredentials(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/login", "", LoginRequest{
		Email:    "nope@example.com",
		Password: "wrong",
	})
	body := decodeAPIError(t, rec, http.StatusUnauthorized)
	if body.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPILogin_ValidationError(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email": "not-an-email",
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected validation error message")
	}
}

// --- /auth/register ---

func TestAPIRegister_Success(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/register", "", RegisterRequest{
		Name:     "Alice",
		Email:    "alice@example.com",
		Password: "secret123",
	})
	resp := decodeAPI[AuthResponse](t, rec, http.StatusOK)
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if resp.User.Name != "Alice" {
		t.Fatalf("user name = %q, want Alice", resp.User.Name)
	}
}

func TestAPIRegister_DuplicateEmail(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	// Seed an existing user with the same email the test
	// will try to register with. loginUser is the standard
	// seed helper; we discard the returned token because the
	// duplicate test doesn't need it.
	loginUser(t, h, mockUser, "dup@example.com", "Existing")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/register", "", RegisterRequest{
		Name:     "New",
		Email:    "dup@example.com",
		Password: "secret123",
	})
	apiErr := decodeAPIError(t, rec, http.StatusConflict)
	if apiErr.Error == "" {
		t.Fatal("expected conflict error message")
	}
}

// --- /auth/password-reset/request ---

func TestAPIRequestPasswordReset_UnknownEmailDoesNotEnumerate(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/password-reset/request", "", PasswordResetRequest{
		Email: "unknown@example.com",
	})
	resp := decodeAPI[PasswordResetResponse](t, rec, http.StatusOK)
	if resp.Message == "" {
		t.Fatal("expected a generic reset confirmation message")
	}
}

func TestAPIRequestPasswordReset_InvalidEmail(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/password-reset/request", "", PasswordResetRequest{
		Email: "not-an-email",
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

// --- /me ---

func TestAPIMe_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, user := loginUser(t, h, mockUser, "me@example.com", "Me")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/me", token, nil)
	dto := decodeAPI[UserDTO](t, rec, http.StatusOK)
	if dto.ID != user.ID {
		t.Fatalf("id = %q, want %q", dto.ID, user.ID)
	}
}

func TestAPIMe_Unauthorized(t *testing.T) {
	_, _, _, e := setupHandler(t)

	// No token — should return JSON 401, not a redirect to /login.
	rec := apiDo(t, e, http.MethodGet, "/api/v1/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	var body APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", body.Error)
	}
}

func TestAPIMe_BadToken(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodGet, "/api/v1/me", "this-is-not-a-jwt", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// --- /me PUT (update profile) ---

func TestAPIUpdateMe_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "upd@example.com", "Old Name")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name:         "New Name",
		TargetWeight: ptrFloat(85.5),
		WeightUnit:   "lbs",
	})
	dto := decodeAPI[UserDTO](t, rec, http.StatusOK)
	if dto.Name != "New Name" {
		t.Fatalf("name = %q, want New Name", dto.Name)
	}
	if dto.TargetWeight == nil || *dto.TargetWeight != 85.5 {
		t.Fatalf("target_weight = %v, want 85.5", dto.TargetWeight)
	}
	if dto.WeightUnit != "lbs" {
		t.Fatalf("weight_unit = %q, want lbs", dto.WeightUnit)
	}
	// Email + admin flag must not be writable from this endpoint —
	// they come from the JWT, not the request body.
	if dto.Email != "upd@example.com" {
		t.Fatalf("email = %q, want upd@example.com (unchanged)", dto.Email)
	}
	if dto.IsAdmin {
		t.Fatal("is_admin should be false for a freshly-registered user")
	}
}

func TestAPIUpdateMe_ClearTargetWeight(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "clr@example.com", "Clr")

	// Seed an existing target weight by updating the user
	// directly through the mock. The HTTP path then clears
	// it by sending target_weight: null.
	seeded := 80.0
	for i := range mockUser.users {
		if mockUser.users[i].Email == "clr@example.com" {
			mockUser.users[i].TargetWeight = &seeded
		}
	}

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, map[string]any{
		"name":          "Clr",
		"target_weight": nil,
		"weight_unit":   "kg",
	})
	dto := decodeAPI[UserDTO](t, rec, http.StatusOK)
	if dto.TargetWeight != nil {
		t.Fatalf("target_weight = %v, want nil after clear", *dto.TargetWeight)
	}
}

func TestAPIUpdateMe_DefaultsWeightUnit(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "def@example.com", "Def")

	// Omit weight_unit entirely — the handler should default
	// it to "kg" rather than write an empty string.
	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, map[string]any{
		"name": "Def",
	})
	dto := decodeAPI[UserDTO](t, rec, http.StatusOK)
	if dto.WeightUnit != "kg" {
		t.Fatalf("weight_unit = %q, want kg (default)", dto.WeightUnit)
	}
	if dto.DistanceUnit != "km" {
		t.Fatalf("distance_unit = %q, want km (default)", dto.DistanceUnit)
	}
}

// TestAPIUpdateMe_DistanceUnit verifies the distance-unit preference
// round-trips through PUT /api/v1/me and normalises like weight_unit.
func TestAPIUpdateMe_DistanceUnit(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "du@example.com", "DU")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name:         "DU",
		DistanceUnit: "mi",
	})
	dto := decodeAPI[UserDTO](t, rec, http.StatusOK)
	if dto.DistanceUnit != "mi" {
		t.Fatalf("distance_unit = %q, want mi", dto.DistanceUnit)
	}

	bad := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name:         "DU",
		DistanceUnit: "nautical-miles",
	})
	decodeAPIError(t, bad, http.StatusBadRequest)
}

func TestAPIUpdateMe_Validation_NameTooShort(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "ns@example.com", "Ns")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name: "A",
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected validation error")
	}
}

func TestAPIUpdateMe_Validation_NameTooLong(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "nl@example.com", "Nl")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name: strings.Repeat("x", 101),
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIUpdateMe_Validation_BadUnit(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "bu@example.com", "Bu")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name:       "Bu",
		WeightUnit: "stone",
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected validation error for unknown unit")
	}
}

func TestAPIUpdateMe_Validation_TargetTooHigh(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "th@example.com", "Th")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name:         "Th",
		TargetWeight: ptrFloat(2000),
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIUpdateMe_Validation_TargetNegative(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "tn@example.com", "Tn")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", token, UpdateMeRequest{
		Name:         "Tn",
		TargetWeight: ptrFloat(-1),
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIUpdateMe_Unauthorized(t *testing.T) {
	_, _, _, e := setupHandler(t)

	// No token — 401 with the same APIError shape /me uses.
	rec := apiDo(t, e, http.MethodPut, "/api/v1/me", "", UpdateMeRequest{
		Name: "Whatever",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// ptrFloat is a tiny helper so the success-path test can
// write `ptrFloat(85.5)` instead of a fresh `f := 85.5; &f`
// dance on every literal.
func ptrFloat(v float64) *float64 { return &v }

// --- /exercises ---

func TestAPIListExercises(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "ex@example.com", "Ex")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercises", token, nil)
	exs := decodeAPI[[]ExerciseDTO](t, rec, http.StatusOK)
	if len(exs) != 2 {
		t.Fatalf("len = %d, want 2", len(exs))
	}
	// Assert the seeded exercises are both present. Ordering
	// is a repository concern (sqlc ORDER BY), not the API
	// surface, so the test should not depend on it.
	names := map[string]bool{}
	for _, e := range exs {
		names[e.Name] = true
		if e.ID == "" {
			t.Fatalf("exercise %q has empty ID", e.Name)
		}
	}
	if !names["Squat"] || !names["Bench Press"] {
		t.Fatalf("missing expected exercise, got %v", names)
	}
}

// --- /exercise-entries ---

func TestAPICreateExerciseEntries_MultiSet(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "create@example.com", "Create")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Notes:      "Felt strong",
		Sets: []CreateSetInput{
			{Reps: 5, Weight: 100, RestTime: 120},
			{Reps: 5, Weight: 100, RestTime: 120},
			{Reps: 5, Weight: 100, RestTime: 120},
		},
	})
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusCreated)
	if len(entries) != 3 {
		t.Fatalf("len = %d, want 3", len(entries))
	}
	for i, entry := range entries {
		if entry.Reps != 5 || entry.Weight != 100 {
			t.Fatalf("entry %d: reps=%d weight=%.1f, want 5/100", i, entry.Reps, entry.Weight)
		}
	}
}

func TestAPICreateExerciseEntries_ValidationError(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "v@example.com", "V")

	// Zero reps is rejected by the validator.
	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets:       []CreateSetInput{{Reps: 0, Weight: 100, RestTime: 0}},
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected validation error message")
	}
}

func TestAPICreateExerciseEntries_UnknownExercise(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "u@example.com", "U")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "does-not-exist",
		Sets:       []CreateSetInput{{Reps: 5, Weight: 100, RestTime: 0}},
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "exercise") {
		t.Fatalf("error = %q, want it to mention exercise", apiErr.Error)
	}
}

// --- /exercise-entries (cardio) ---

// seedCardioExercise appends a cardio-typed exercise to the mock's
// catalogue so the type-driven validation path can be exercised.
func seedCardioExercise(t *testing.T, mock *mockRepository) {
	t.Helper()
	mock.mu.Lock()
	defer mock.mu.Unlock()
	mock.exercises = append(mock.exercises, models.Exercise{
		ID:   "ex-run",
		Name: "Run",
		Type: models.ExerciseTypeCardio,
	})
}

func TestAPICreateExerciseEntries_Cardio(t *testing.T) {
	h, mockRepo, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "run@example.com", "Run")
	seedCardioExercise(t, mockRepo)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-run",
		Notes:      "easy effort",
		Sets: []CreateSetInput{
			{DurationSeconds: 1500, DistanceMeters: 5000, AvgHeartRate: 152, CaloriesBurned: 320},
		},
	})
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusCreated)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ExerciseType != "cardio" {
		t.Errorf("exercise_type = %q, want cardio", entry.ExerciseType)
	}
	if entry.DurationSeconds != 1500 || entry.DistanceMeters != 5000 || entry.AvgHeartRate != 152 || entry.CaloriesBurned != 320 {
		t.Errorf("cardio metrics = %+v", entry)
	}
	// The server must zero strength metrics on cardio entries even if
	// a client sends them.
	if entry.Reps != 0 || entry.Weight != 0 || entry.RestTime != 0 {
		t.Errorf("expected zeroed strength metrics, got reps=%d weight=%.1f rest=%d", entry.Reps, entry.Weight, entry.RestTime)
	}
}

func TestAPICreateExerciseEntries_CardioMissingDistance(t *testing.T) {
	h, mockRepo, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "run2@example.com", "Run2")
	seedCardioExercise(t, mockRepo)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-run",
		Sets:       []CreateSetInput{{DurationSeconds: 1500}},
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "distance") {
		t.Fatalf("error = %q, want it to mention distance", apiErr.Error)
	}
}

func TestAPICreateExerciseEntries_CardioMissingDuration(t *testing.T) {
	h, mockRepo, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "run3@example.com", "Run3")
	seedCardioExercise(t, mockRepo)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-run",
		Sets:       []CreateSetInput{{DistanceMeters: 5000}},
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "duration") {
		t.Fatalf("error = %q, want it to mention duration", apiErr.Error)
	}
}

// TestAPICreateExerciseEntries_StrengthRejectsRepsOnly verifies a
// strength exercise still requires reps — cardio fields alone don't
// satisfy it.
func TestAPICreateExerciseEntries_StrengthRequiresReps(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "str@example.com", "Str")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets:       []CreateSetInput{{Weight: 100}}, // no reps
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "reps") {
		t.Fatalf("error = %q, want it to mention reps", apiErr.Error)
	}
}

func TestAPIListExerciseEntries_DefaultDays(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "l@example.com", "L")

	// Seed two entries: one inside the 7-day window, one outside it.
	now := time.Now()
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-l@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: now.AddDate(0, 0, -2)},
		{ID: "e2", UserID: "user-l@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 90, CreatedAt: now.AddDate(0, 0, -30)},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercise-entries", token, nil)
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusOK)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (only the entry inside the 7-day window)", len(entries))
	}
	if entries[0].ID != "e1" {
		t.Fatalf("id = %q, want e1", entries[0].ID)
	}
}

func TestAPIListExerciseEntries_CustomDays(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "l2@example.com", "L2")

	now := time.Now()
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-l2@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: now.AddDate(0, 0, -2)},
		{ID: "e2", UserID: "user-l2@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 90, CreatedAt: now.AddDate(0, 0, -30)},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercise-entries?days=60", token, nil)
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusOK)
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
}

func TestAPIListExerciseEntries_BadDays(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "bd@example.com", "BD")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercise-entries?days=-1", token, nil)
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected validation error")
	}
}

// --- /exercise-entries from/to range mode ---

// TestAPIListExerciseEntries_DateRange verifies the explicit
// ?from=&to= window (the iOS week calendar's access path):
// only entries inside the inclusive range come back.
func TestAPIListExerciseEntries_DateRange(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "dr@example.com", "DR")

	// Three entries: one before the range, one inside, one after.
	mid := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "before", UserID: "user-dr@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 80, CreatedAt: mid.AddDate(0, 0, -3)},
		{ID: "inside", UserID: "user-dr@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: mid},
		{ID: "after", UserID: "user-dr@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 90, CreatedAt: mid.AddDate(0, 0, 3)},
	}

	from := mid.Add(-1 * time.Hour).Format(time.RFC3339)
	to := mid.Add(1 * time.Hour).Format(time.RFC3339)
	path := "/api/v1/exercise-entries?from=" + from + "&to=" + to

	rec := apiDo(t, e, http.MethodGet, path, token, nil)
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusOK)
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (only the entry inside the range)", len(entries))
	}
	if entries[0].ID != "inside" {
		t.Fatalf("id = %q, want inside", entries[0].ID)
	}
}

// TestAPIListExerciseEntries_DateRangeEncodedOffset proves a
// non-UTC RFC3339 offset survives URL decoding. The '+' in
// "+01:00" must arrive as '+' (clients percent-encode it as
// %2B); if it were decoded as a space the parse would fail.
func TestAPIListExerciseEntries_DateRangeEncodedOffset(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "off@example.com", "OFF")

	inside := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC) // 10:00 +01:00
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "inside", UserID: "user-off@example.com", ExerciseID: "ex-1", ExerciseName: "Run", DurationSeconds: 1800, DistanceMeters: 5000, CreatedAt: inside},
	}

	loc := time.FixedZone("test", 3600)
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, loc).Format(time.RFC3339) // midnight local
	to := time.Date(2026, 8, 11, 0, 0, 0, 0, loc).Add(-time.Second).Format(time.RFC3339)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/exercise-entries?from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusOK)
	if len(entries) != 1 || entries[0].ID != "inside" {
		t.Fatalf("entries = %+v, want just 'inside'", entries)
	}
}

// TestAPIListExerciseEntries_RangeHalfSpecified verifies a lone
// from or to is rejected rather than guessed.
func TestAPIListExerciseEntries_RangeHalfSpecified(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "half@example.com", "HALF")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercise-entries?from="+time.Now().Format(time.RFC3339), token, nil)
	decodeAPIError(t, rec, http.StatusBadRequest)
}

// TestAPIListExerciseEntries_BadRangeFormat verifies malformed
// timestamps are rejected with a 400.
func TestAPIListExerciseEntries_BadRangeFormat(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "bf@example.com", "BF")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercise-entries?from=yesterday&to="+time.Now().Format(time.RFC3339), token, nil)
	decodeAPIError(t, rec, http.StatusBadRequest)
}

// TestAPIListExerciseEntries_InvertedRange verifies from > to
// is rejected.
func TestAPIListExerciseEntries_InvertedRange(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "inv@example.com", "INV")

	now := time.Now()
	path := "/api/v1/exercise-entries?from=" + now.Format(time.RFC3339) +
		"&to=" + now.AddDate(0, 0, -7).Format(time.RFC3339)

	rec := apiDo(t, e, http.MethodGet, path, token, nil)
	decodeAPIError(t, rec, http.StatusBadRequest)
}

// TestAPIListExerciseEntries_RangeTooLarge verifies spans beyond
// maxExerciseEntryRangeSpan are rejected so a caller cannot ask
// for an unbounded scan.
func TestAPIListExerciseEntries_RangeTooLarge(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "big@example.com", "BIG")

	now := time.Now()
	path := "/api/v1/exercise-entries?from=" + now.AddDate(0, 0, -100).Format(time.RFC3339) +
		"&to=" + now.Format(time.RFC3339)

	rec := apiDo(t, e, http.MethodGet, path, token, nil)
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIGetExerciseEntry_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "nf@example.com", "NF")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercise-entries/missing", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIUpdateExerciseEntry(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "up@example.com", "Up")

	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-up@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
	}

	rec := apiDo(t, e, http.MethodPut, "/api/v1/exercise-entries/e1", token, UpdateExerciseEntryRequest{
		ExerciseID: "ex-1",
		Reps:       8,
		Weight:     110,
		RestTime:   90,
	})
	entry := decodeAPI[ExerciseEntryDTO](t, rec, http.StatusOK)
	if entry.Reps != 8 || entry.Weight != 110 {
		t.Fatalf("reps=%d weight=%.1f, want 8/110", entry.Reps, entry.Weight)
	}
}

// TestAPIUpdateExerciseEntry_Cardio verifies the PUT path validates
// cardio edits against the linked exercise's type and normalizes the
// non-applicable metric pair to zero.
func TestAPIUpdateExerciseEntry_Cardio(t *testing.T) {
	h, mockRepo, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "upc@example.com", "UpC")
	seedCardioExercise(t, mockRepo)

	mockRepo.exerciseEntries = []models.ExerciseEntry{
		{ID: "c1", UserID: "user-upc@example.com", ExerciseID: "ex-run", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1800, DistanceMeters: 5000, CreatedAt: time.Now()},
	}

	rec := apiDo(t, e, http.MethodPut, "/api/v1/exercise-entries/c1", token, UpdateExerciseEntryRequest{
		ExerciseID:      "ex-run",
		DurationSeconds: 1500,
		DistanceMeters:  5000,
		AvgHeartRate:    148,
		CaloriesBurned:  300,
	})
	entry := decodeAPI[ExerciseEntryDTO](t, rec, http.StatusOK)
	if entry.DurationSeconds != 1500 || entry.DistanceMeters != 5000 {
		t.Fatalf("cardio metrics = %+v", entry)
	}
	if entry.Reps != 0 || entry.Weight != 0 {
		t.Errorf("expected zeroed strength metrics, got reps=%d weight=%.1f", entry.Reps, entry.Weight)
	}

	// Missing distance → 400 mentioning distance.
	bad := apiDo(t, e, http.MethodPut, "/api/v1/exercise-entries/c1", token, UpdateExerciseEntryRequest{
		ExerciseID:      "ex-run",
		DurationSeconds: 1500,
	})
	apiErr := decodeAPIError(t, bad, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "distance") {
		t.Fatalf("error = %q, want it to mention distance", apiErr.Error)
	}
}

// TestAPIUpdateExerciseEntry_UnknownExercise locks in the guard that
// rejects an update pointing at a non-existent exercise instead of
// silently orphaning the row.
func TestAPIUpdateExerciseEntry_UnknownExercise(t *testing.T) {
	h, mockRepo, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "upx@example.com", "UpX")

	mockRepo.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-upx@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
	}

	rec := apiDo(t, e, http.MethodPut, "/api/v1/exercise-entries/e1", token, UpdateExerciseEntryRequest{
		ExerciseID: "does-not-exist",
		Reps:       8,
		Weight:     110,
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "exercise") {
		t.Fatalf("error = %q, want it to mention exercise", apiErr.Error)
	}
}

func TestAPIDeleteExerciseEntry(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "del@example.com", "Del")

	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-del@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: time.Now()},
	}

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/exercise-entries/e1", token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(mock.exerciseEntries) != 0 {
		t.Fatalf("entries len = %d, want 0 after delete", len(mock.exerciseEntries))
	}
}

// --- /exercises/:id/history ---

func TestAPIGetExerciseHistory(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "h@example.com", "H")

	now := time.Now()
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-h@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: now},
		{ID: "e2", UserID: "user-h@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 110, CreatedAt: now.AddDate(0, 0, -1)},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercises/ex-1/history", token, nil)
	page := decodeAPI[HistoryPageDTO](t, rec, http.StatusOK)
	if len(page.Entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(page.Entries))
	}
	if page.Stats.MaxWeight != 110 {
		t.Fatalf("max weight = %.1f, want 110", page.Stats.MaxWeight)
	}
	if page.Page != 1 {
		t.Fatalf("page = %d, want 1", page.Page)
	}
	if page.HasPrev {
		t.Fatal("has_prev should be false on the first page")
	}
}

func TestAPIGetExerciseHistory_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hnf@example.com", "HNF")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercises/does-not-exist/history", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

// TestAPIGetExerciseHistory_CardioStats verifies the history payload for
// a cardio exercise carries the cardio personal bests (fastest pace,
// longest distance) plus the exercise type so the client can pick the
// right stat cards and chart.
func TestAPIGetExerciseHistory_CardioStats(t *testing.T) {
	h, mockRepo, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "hc@example.com", "HC")
	seedCardioExercise(t, mockRepo)

	now := time.Now()
	mockRepo.exerciseEntries = []models.ExerciseEntry{
		// 30:00 for 5 km → 360 s/km; 25:00 for 5 km → 300 s/km (best).
		{ID: "c1", UserID: "user-hc@example.com", ExerciseID: "ex-run", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1800, DistanceMeters: 5000, CreatedAt: now},
		{ID: "c2", UserID: "user-hc@example.com", ExerciseID: "ex-run", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: now.AddDate(0, 0, -1)},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercises/ex-run/history", token, nil)
	page := decodeAPI[HistoryPageDTO](t, rec, http.StatusOK)
	if page.ExerciseType != "cardio" {
		t.Fatalf("exercise_type = %q, want cardio", page.ExerciseType)
	}
	if page.Stats.BestPaceSecPerKm != 300 {
		t.Errorf("best_pace_sec_per_km = %.1f, want 300", page.Stats.BestPaceSecPerKm)
	}
	if page.Stats.LongestDistanceMeters != 5000 {
		t.Errorf("longest_distance_meters = %.1f, want 5000", page.Stats.LongestDistanceMeters)
	}
}

// --- /exercises/:id/chart ---

func TestAPIGetExerciseChartData(t *testing.T) {
	h, mock, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "c@example.com", "C")

	now := time.Now()
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "user-c@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: now},
		{ID: "e2", UserID: "user-c@example.com", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 110, CreatedAt: now.AddDate(0, 0, -7)},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/exercises/ex-1/chart", token, nil)
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusOK)
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
}

// --- Cookie auth (web app) still works for the API ---

func TestAPIAuthMiddleware_CookieFallback(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "ck@example.com", "CK")

	// Send the request with the token in a cookie instead of
	// the Authorization header. The web app and any other
	// cookie-auth client should still be able to call the API.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var dto UserDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dto.Email != "ck@example.com" {
		t.Fatalf("email = %q, want ck@example.com", dto.Email)
	}
}

// --- /auth/logout (stateless) ---

func TestAPILogout(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/auth/logout", "", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

// --- Smoke: error bodies are always APIError ---

func TestAPIErrorBodyShape(t *testing.T) {
	// Hit a 401 path and assert the response is a clean
	// APIError JSON, not an HTML error page.
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodGet, "/api/v1/me", "", nil)
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("401 content-type = %q, want application/json", ct)
	}
	var e401 APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &e401); err != nil {
		t.Fatalf("401 decode: %v (body=%s)", err, rec.Body.String())
	}
	if e401.Error != "unauthorized" {
		t.Fatalf("401 error = %q, want unauthorized", e401.Error)
	}
}

// --- The web app's redirect-to-login is preserved for HTML routes ---

// TestHTMLLogin_StillRedirectsOnMissingToken guards the HTML
// surface's auth behaviour: a request without a token for a
// protected web page must be a 303 to /login, never the JSON 401
// shape API clients get. It targets /dashboard (the signed-in app's
// landing page) — "/" is now the public marketing page, which is
// intentionally reachable without a session.
func TestHTMLLogin_StillRedirectsOnMissingToken(t *testing.T) {
	_, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (redirect to /login)", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("location = %q, want /login", loc)
	}
}

// --- /goals ---

// goalsResponse mirrors the JSON shape returned by GET /api/v1/goals.
// The iOS client decodes this into a `[GoalDTO]`, but using a
// named struct here keeps the test assertion symmetric with the
// server contract.
type goalsResponse struct {
	Goals []GoalDTO `json:"goals"`
}

func TestAPIListGoals_Empty(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gle@example.com", "GE")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/goals", token, nil)
	resp := decodeAPI[goalsResponse](t, rec, http.StatusOK)
	if len(resp.Goals) != 0 {
		t.Fatalf("goals len = %d, want 0", len(resp.Goals))
	}
}

func TestAPIListGoals_OrderingActiveThenCompleted(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "glo@example.com", "GO")
	userID := "user-glo@example.com"

	now := time.Now()
	completed := now.Add(-24 * time.Hour)

	// Seed the repo directly so we control the active/completed
	// split and verify the ordering. Swap the controller's
	// repository before any other goal test runs (test order is
	// undefined so each test owns its own swap).
	repo := mustSwapGoalsRepo(t, h, newMockGoalRepository())

	// Insert in random order to prove the response reorders.
	repo.goals["g-done"] = &models.Goal{ID: "g-done", UserID: userID, Title: "Done", CompletedAt: &completed, CreatedAt: now}
	repo.goals["g-b"] = &models.Goal{ID: "g-b", UserID: userID, Title: "B", CreatedAt: now}
	repo.goals["g-a"] = &models.Goal{ID: "g-a", UserID: userID, Title: "A", CreatedAt: now.Add(-time.Hour)}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/goals", token, nil)
	resp := decodeAPI[goalsResponse](t, rec, http.StatusOK)
	if len(resp.Goals) != 3 {
		t.Fatalf("goals len = %d, want 3", len(resp.Goals))
	}
	if resp.Goals[0].CompletedAt != nil {
		t.Fatalf("first goal should be active, got completed_at=%v", resp.Goals[0].CompletedAt)
	}
	if resp.Goals[1].CompletedAt != nil {
		t.Fatalf("second goal should be active, got completed_at=%v", resp.Goals[1].CompletedAt)
	}
	if resp.Goals[2].CompletedAt == nil {
		t.Fatal("third goal should be completed")
	}
}

func TestAPICreateGoal_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gc@example.com", "GC")

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	target := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{
		Title:       "Bench 100kg",
		Description: "Working sets",
		StartDate:   &start,
		TargetDate:  &target,
	})
	g := decodeAPI[GoalDTO](t, rec, http.StatusCreated)
	if g.ID == "" {
		t.Fatal("expected generated id")
	}
	if g.Title != "Bench 100kg" {
		t.Fatalf("title = %q, want Bench 100kg", g.Title)
	}
	if g.Description != "Working sets" {
		t.Fatalf("description = %q, want Working sets", g.Description)
	}
	if g.TargetDate == nil || !g.TargetDate.Equal(target) {
		t.Fatalf("target_date = %v, want %v", g.TargetDate, target)
	}
	if g.CompletedAt != nil {
		t.Fatalf("completed_at = %v, want nil", g.CompletedAt)
	}
}

func TestAPICreateGoal_MissingTitle(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gcmt@example.com", "GMT")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: ""})
	e2 := decodeAPIError(t, rec, http.StatusBadRequest)
	if e2.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPICreateGoal_TitleTooLong(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gctl@example.com", "GCTL")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: strings.Repeat("x", 201)})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIGetGoal_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, user := loginUser(t, h, mockUser, "gg@example.com", "GG")

	// Create via the API so we exercise the real write path.
	start := time.Now()
	target := start.AddDate(0, 0, 14)
	created := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{
		Title:      "Run a 5k",
		TargetDate: &target,
	}), http.StatusCreated)

	got := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/goals/"+created.ID, token, nil), http.StatusOK)
	if got.ID != created.ID {
		t.Fatalf("id = %q, want %q", got.ID, created.ID)
	}
	if got.Title != "Run a 5k" {
		t.Fatalf("title = %q, want Run a 5k", got.Title)
	}
	_ = user
}

func TestAPIGetGoal_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "ggnf@example.com", "GGNF")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/goals/missing-id", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIUpdateGoal_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gu@example.com", "GU")

	created := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: "Original"}), http.StatusCreated)

	updated := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/goals/"+created.ID, token, UpdateGoalRequest{
		Title:       "Edited",
		Description: "now with notes",
	}), http.StatusOK)
	if updated.Title != "Edited" {
		t.Fatalf("title = %q, want Edited", updated.Title)
	}
	if updated.Description != "now with notes" {
		t.Fatalf("description = %q, want now with notes", updated.Description)
	}
}

func TestAPIUpdateGoal_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gunf@example.com", "GUNF")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/goals/missing-id", token, UpdateGoalRequest{Title: "x"})
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIMarkGoalComplete_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gmc@example.com", "GMC")

	created := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: "Do it"}), http.StatusCreated)
	if created.CompletedAt != nil {
		t.Fatal("fresh goal should not be complete")
	}

	done := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals/"+created.ID+"/complete", token, nil), http.StatusOK)
	if done.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestAPIMarkGoalComplete_Idempotent(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gmci@example.com", "GMCI")

	created := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: "Do it"}), http.StatusCreated)
	first := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals/"+created.ID+"/complete", token, nil), http.StatusOK)
	second := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals/"+created.ID+"/complete", token, nil), http.StatusOK)

	if first.CompletedAt == nil || second.CompletedAt == nil {
		t.Fatal("expected completed_at to remain set")
	}
	if !second.CompletedAt.Equal(*first.CompletedAt) {
		t.Fatalf("completed_at drifted: first=%v second=%v (server should not overwrite on second call)", *first.CompletedAt, *second.CompletedAt)
	}
}

func TestAPIMarkGoalComplete_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gmcnf@example.com", "GMCNF")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/goals/missing-id/complete", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIReopenGoal_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gr@example.com", "GR")

	created := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: "Do it"}), http.StatusCreated)
	_ = decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals/"+created.ID+"/complete", token, nil), http.StatusOK)
	reopened := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals/"+created.ID+"/reopen", token, nil), http.StatusOK)
	if reopened.CompletedAt != nil {
		t.Fatalf("expected completed_at nil after reopen, got %v", reopened.CompletedAt)
	}
}

func TestAPIReopenGoal_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "grnf@example.com", "GRNF")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/goals/missing-id/reopen", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIDeleteGoal_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gd@example.com", "GD")

	created := decodeAPI[GoalDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/goals", token, CreateGoalRequest{Title: "Do it"}), http.StatusCreated)
	rec := apiDo(t, e, http.MethodDelete, "/api/v1/goals/"+created.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}

	// Subsequent GET must 404.
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/goals/"+created.ID, token, nil), http.StatusNotFound)
}

func TestAPIDeleteGoal_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "gdnf@example.com", "GDNF")

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/goals/missing-id", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIGoals_RequiresAuth(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodGet, "/api/v1/goals", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// mustSwapGoalsRepo replaces h.goalsCtrl with a new controller
// backed by the supplied mockGoalRepository. Used by ordering
// tests that need to seed goals without a real DB. We mutate
// the controller in place (the controller has no exported
// setter) via the same package.
func mustSwapGoalsRepo(t *testing.T, h *Handler, repo *mockGoalRepository) *mockGoalRepository {
	t.Helper()
	h.goalsCtrl = controllers.NewGoalsController(repo)
	return repo
}

// --- /feedback ---

// mustSwapFeedbackRepo replaces h.feedbackCtrl with a new
// controller backed by the supplied mockFeedbackRepository.
// Used by the feedback tests that need to seed/inspect
// persisted rows. Mutates the controller in place via the
// same package, mirroring mustSwapGoalsRepo.
func mustSwapFeedbackRepo(t *testing.T, h *Handler, repo *mockFeedbackRepository) *mockFeedbackRepository {
	t.Helper()
	h.feedbackCtrl = controllers.NewFeedbackController(repo)
	return repo
}

func TestAPISubmitFeedback_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fb@example.com", "Feedbacker")
	repo := mustSwapFeedbackRepo(t, h, newMockFeedbackRepository())

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   "Loving the new dashboard",
		Message: "Just wanted to say thanks for the recent changes.",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.feedback) != 1 {
		t.Fatalf("feedback len = %d, want 1", len(repo.feedback))
	}
	got := repo.feedback[0]
	// user_id must come from the JWT, not from the request
	// body — clients cannot submit feedback on someone else's
	// behalf.
	if got.UserID != "user-fb@example.com" {
		t.Fatalf("user_id = %q, want user-fb@example.com", got.UserID)
	}
	if got.Title != "Loving the new dashboard" {
		t.Fatalf("title = %q, want %q", got.Title, "Loving the new dashboard")
	}
	if got.Message != "Just wanted to say thanks for the recent changes." {
		t.Fatalf("message = %q, want %q", got.Message, "Just wanted to say thanks for the recent changes.")
	}
}

func TestAPISubmitFeedback_TrimsWhitespace(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbt@example.com", "FbT")
	repo := mustSwapFeedbackRepo(t, h, newMockFeedbackRepository())

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   "  Trimmed Title  ",
		Message: "   Trimmed message here.   ",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if repo.feedback[0].Title != "Trimmed Title" {
		t.Fatalf("title = %q, want trimmed %q", repo.feedback[0].Title, "Trimmed Title")
	}
	if repo.feedback[0].Message != "Trimmed message here." {
		t.Fatalf("message = %q, want trimmed %q", repo.feedback[0].Message, "Trimmed message here.")
	}
}

func TestAPISubmitFeedback_TitleTooShort(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbts@example.com", "FbTS")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   "Hi",
		Message: "This is a long enough message body.",
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPISubmitFeedback_TitleTooLong(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbtl@example.com", "FbTL")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   strings.Repeat("x", 101),
		Message: "This is a long enough message body.",
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPISubmitFeedback_MessageTooShort(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbms@example.com", "FbMS")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   "Valid Title Here",
		Message: "Short",
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPISubmitFeedback_MessageTooLong(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbml@example.com", "FbML")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   "Valid Title Here",
		Message: strings.Repeat("x", 1001),
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPISubmitFeedback_InvalidBody(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbib@example.com", "FbIB")

	// Send raw garbage so the JSON decoder in c.Bind rejects it.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPISubmitFeedback_RepoError(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "fbre@example.com", "FbRE")
	repo := mustSwapFeedbackRepo(t, h, newMockFeedbackRepository())
	repo.errCreate = errors.New("database error")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", token, SubmitFeedbackRequest{
		Title:   "Valid Title Here",
		Message: "Valid message body — long enough.",
	})
	apiErr := decodeAPIError(t, rec, http.StatusInternalServerError)
	if apiErr.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPISubmitFeedback_RequiresAuth(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/feedback", "", SubmitFeedbackRequest{
		Title:   "Anonymous Title",
		Message: "Anonymous message body that is long enough.",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// --- /weight ---

// mustSwapWeightRepo replaces h.weightCtrl with a new controller
// backed by the supplied mockWeightRepository. Mirrors the
// mustSwapGoalsRepo / mustSwapFeedbackRepo pattern: swap the
// controller in place so individual tests can seed and assert on
// the repository directly.
func mustSwapWeightRepo(t *testing.T, h *Handler, repo *mockWeightRepository) *mockWeightRepository {
	t.Helper()
	h.weightCtrl = controllers.NewWeightController(repo, nil)
	return repo
}

func TestAPIListWeightEntries_Empty(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wle@example.com", "WLE")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight", token, nil)
	resp := decodeAPI[WeightEntriesResponse](t, rec, http.StatusOK)
	if len(resp.Entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(resp.Entries))
	}
}

func TestAPIListWeightEntries_ReturnsUserEntries(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wleu@example.com", "WLEU")
	userID := "user-wleu@example.com"

	// Load a storage config so utils.PublicURLFor resolves the
	// photo key to a real URL (otherwise the DTO comes back with
	// an empty photo_url even though has_photo is true).
	for _, v := range []string{
		"STORAGE_ENDPOINT", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY",
		"STORAGE_BUCKET", "STORAGE_PUBLIC_URL",
	} {
		t.Setenv(v, "test")
	}
	if _, err := utils.LoadStorageConfig(); err != nil {
		t.Fatalf("LoadStorageConfig: %v", err)
	}

	repo := mustSwapWeightRepo(t, h, newMockWeightRepository())
	now := time.Now()
	repo.entries = []models.WeightEntry{
		{ID: "w1", UserID: userID, Weight: 80, Notes: "morning", CreatedAt: now.Add(-24 * time.Hour)},
		{ID: "w2", UserID: userID, Weight: 79.5, PhotoKey: "weight/u/w2.jpg", CreatedAt: now},
		// Mixed in: a row that belongs to a different user must
		// not appear in the response. The mock's `List` already
		// filters by userID, but the test makes that contract
		// explicit.
		{ID: "w-other", UserID: "other-user", Weight: 70, CreatedAt: now},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight", token, nil)
	resp := decodeAPI[WeightEntriesResponse](t, rec, http.StatusOK)
	if len(resp.Entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(resp.Entries))
	}
	// has_photo should mirror whether PhotoKey is set.
	var withPhoto *WeightEntryDTO
	for i := range resp.Entries {
		if resp.Entries[i].ID == "w2" {
			withPhoto = &resp.Entries[i]
		}
	}
	if withPhoto == nil {
		t.Fatal("w2 not in response")
	}
	if !withPhoto.HasPhoto {
		t.Errorf("has_photo = false, want true (PhotoKey is set)")
	}
	if withPhoto.PhotoURL == "" {
		t.Errorf("photo_url = empty, want resolved via utils.PublicURLFor")
	}
}

func TestAPICreateWeightEntry_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wc@example.com", "WC")
	repo := mustSwapWeightRepo(t, h, newMockWeightRepository())

	rec := apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight: 81.4,
		Notes:  "post-lunch",
	})
	dto := decodeAPI[WeightEntryDTO](t, rec, http.StatusCreated)
	if dto.ID == "" {
		t.Fatal("expected generated id")
	}
	if dto.Weight != 81.4 {
		t.Fatalf("weight = %v, want 81.4", dto.Weight)
	}
	if dto.Notes != "post-lunch" {
		t.Fatalf("notes = %q, want post-lunch", dto.Notes)
	}
	if dto.HasPhoto {
		t.Errorf("has_photo = true, want false for a new entry without a photo")
	}
	if len(repo.entries) != 1 {
		t.Fatalf("repo.size = %d, want 1", len(repo.entries))
	}
}

func TestAPICreateWeightEntry_ValidationError(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wcv@example.com", "WCV")

	// Negative weight is rejected by the validator.
	rec := apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight: -1,
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestAPICreateWeightEntry_WeightTooHigh(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wcth@example.com", "WCTH")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight: 2000,
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPICreateWeightEntry_Backdated(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wcb@example.com", "WCB")

	back := time.Date(2025, 6, 1, 7, 0, 0, 0, time.UTC)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight:    82.0,
		CreatedAt: &back,
	})
	dto := decodeAPI[WeightEntryDTO](t, rec, http.StatusCreated)
	if !dto.CreatedAt.Equal(back) {
		t.Fatalf("created_at = %v, want %v", dto.CreatedAt, back)
	}
}

func TestAPICreateWeightEntry_RequiresAuth(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/weight", "", CreateWeightEntryRequest{Weight: 80})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIGetWeightEntry_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wg@example.com", "WG")

	created := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{Weight: 80.5}), http.StatusCreated)

	got := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/weight/"+created.ID, token, nil), http.StatusOK)
	if got.ID != created.ID {
		t.Fatalf("id = %q, want %q", got.ID, created.ID)
	}
	if got.Weight != 80.5 {
		t.Fatalf("weight = %v, want 80.5", got.Weight)
	}
}

func TestAPIGetWeightEntry_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wgnf@example.com", "WGNF")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight/missing-id", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIUpdateWeightEntry_ReplacePhoto(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wup@example.com", "WUP")

	created := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight:   80,
		PhotoKey: "weight/u/old.jpg",
	}), http.StatusCreated)
	if !created.HasPhoto {
		t.Fatal("created photo state should be true")
	}

	updated := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/weight/"+created.ID, token, UpdateWeightEntryRequest{
		Weight:   80.5,
		PhotoKey: "weight/u/new.jpg",
	}), http.StatusOK)
	if updated.PhotoKey != "weight/u/new.jpg" {
		t.Fatalf("photo_key = %q, want weight/u/new.jpg", updated.PhotoKey)
	}
	if !updated.HasPhoto {
		t.Errorf("has_photo = false, want true")
	}
}

func TestAPIUpdateWeightEntry_RemovePhoto(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wur@example.com", "WUR")

	created := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight:   80,
		PhotoKey: "weight/u/del.jpg",
	}), http.StatusCreated)

	updated := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/weight/"+created.ID, token, UpdateWeightEntryRequest{
		Weight:      80,
		RemovePhoto: true,
	}), http.StatusOK)
	if updated.HasPhoto {
		t.Errorf("has_photo = true, want false after remove_photo=true")
	}
	if updated.PhotoKey != "" {
		t.Errorf("photo_key = %q, want empty after remove_photo=true", updated.PhotoKey)
	}
}

func TestAPIUpdateWeightEntry_PreservesPhotoWhenNotTouched(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wupp@example.com", "WUPP")

	created := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{
		Weight:   80,
		PhotoKey: "weight/u/keep.jpg",
	}), http.StatusCreated)

	// Update only the weight — photo_key must come through unchanged.
	updated := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/weight/"+created.ID, token, UpdateWeightEntryRequest{
		Weight: 80.1,
	}), http.StatusOK)
	if updated.PhotoKey != "weight/u/keep.jpg" {
		t.Fatalf("photo_key = %q, want weight/u/keep.jpg (unchanged)", updated.PhotoKey)
	}
}

func TestAPIUpdateWeightEntry_NotFound(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wunf@example.com", "WUNF")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/weight/missing-id", token, UpdateWeightEntryRequest{Weight: 80})
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIDeleteWeightEntry_Success(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wd@example.com", "WD")

	created := decodeAPI[WeightEntryDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/weight", token, CreateWeightEntryRequest{Weight: 80}), http.StatusCreated)

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/weight/"+created.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}

	// Subsequent GET must 404.
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/weight/"+created.ID, token, nil), http.StatusNotFound)
}

func TestAPIDeleteWeightEntry_Idempotent(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wdnf@example.com", "WDNF")

	// The web's DeleteWeight returns 204 even when the entry
	// doesn't exist (no existence check before the DB delete).
	// The JSON API mirrors that behaviour so the iOS client can
	// rely on "delete then refresh" without a race condition
	// when an entry was deleted from another device first.
	rec := apiDo(t, e, http.MethodDelete, "/api/v1/weight/missing-id", token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (idempotent), body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPICompareWeight_HappyPath(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wcmp@example.com", "WCMP")
	userID := "user-wcmp@example.com"

	repo := mustSwapWeightRepo(t, h, newMockWeightRepository())

	earlier := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	later := time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC)
	repo.entries = []models.WeightEntry{
		{ID: "w-old", UserID: userID, Weight: 90, PhotoKey: "weight/u/w-old.jpg", CreatedAt: earlier},
		{ID: "w-new", UserID: userID, Weight: 85, PhotoKey: "weight/u/w-new.jpg", CreatedAt: later},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight/compare?a=w-old&b=w-new", token, nil)
	resp := decodeAPI[WeightCompareResponse](t, rec, http.StatusOK)

	// The controller sorts by created_at ascending, so the
	// older entry must come back as Before.
	if resp.Before.ID != "w-old" {
		t.Errorf("before.id = %q, want w-old", resp.Before.ID)
	}
	if resp.After.ID != "w-new" {
		t.Errorf("after.id = %q, want w-new", resp.After.ID)
	}
	// 85 - 90 = -5 → "−5.0 kg"
	if resp.DeltaText != "−5.0 kg" {
		t.Errorf("delta_text = %q, want −5.0 kg", resp.DeltaText)
	}
}

func TestAPICompareWeight_MissingPhoto(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wcmpnp@example.com", "WCMPNP")
	userID := "user-wcmpnp@example.com"

	repo := mustSwapWeightRepo(t, h, newMockWeightRepository())
	repo.entries = []models.WeightEntry{
		{ID: "w-a", UserID: userID, Weight: 90, PhotoKey: "weight/u/a.jpg", CreatedAt: time.Now().Add(-24 * time.Hour)},
		{ID: "w-b", UserID: userID, Weight: 85, PhotoKey: "", CreatedAt: time.Now()},
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight/compare?a=w-a&b=w-b", token, nil)
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if !strings.Contains(apiErr.Error, "photo") {
		t.Errorf("error = %q, want it to mention a photo", apiErr.Error)
	}
}

func TestAPICompareWeight_SameID(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "wcmpsi@example.com", "WCMPSI")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight/compare?a=w1&b=w1", token, nil)
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIWeight_RequiresAuth(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := apiDo(t, e, http.MethodGet, "/api/v1/weight", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
