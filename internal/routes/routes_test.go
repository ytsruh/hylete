package routes

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/utils"
)

type mockRepository struct {
	mu              sync.Mutex
	exercises       []models.Exercise
	exerciseEntries []models.ExerciseEntry

	errCreate                       error
	errGetByName                    error
	errGetByID                      error
	errUpdate                       error
	errList                         error
	errCreateExerciseEntry                  error
	errGetExerciseEntry                     error
	errUpdateExerciseEntry                  error
	errUpdateExerciseEntryWithDate          error
	errDeleteExerciseEntry                  error
	errListExerciseEntries                  error
	errGetExerciseEntriesByExercisePaginated error
	errGetExerciseEntriesByDateRange        error
	errGetExerciseByID              error
	errGetMaxWeightByExercise       error
	errGetBestPaceByExercise        error
	errGetLongestDistanceByExercise error
	errGetLastSetByExercise         error
}

func newMockRepository() *mockRepository {
	return &mockRepository{}
}

func (m *mockRepository) Create(_ *sql.Tx, name string) (string, error) {
	if m.errCreate != nil {
		return "", m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == name {
			return e.ID, nil
		}
	}
	id := "ex-" + name
	m.exercises = append(m.exercises, models.Exercise{ID: id, Name: name})
	return id, nil
}

func (m *mockRepository) CreateNoTx(params models.CreateExerciseParams) (string, error) {
	if m.errCreate != nil {
		return "", m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == params.Name {
			return e.ID, nil
		}
	}
	id := "ex-" + params.Name
	m.exercises = append(m.exercises, models.Exercise{ID: id, Name: params.Name, Description: params.Description, VideoURL: params.VideoURL, ImgURL: params.ImgURL, ImgURLOriginal: params.ImgURLOriginal, Type: params.Type})
	return id, nil
}

func (m *mockRepository) GetByID(id string) (*models.Exercise, error) {
	if m.errGetByID != nil {
		return nil, m.errGetByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.ID == id {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) GetExerciseByID(id string, userID string) (*models.Exercise, error) {
	if m.errGetExerciseByID != nil {
		return nil, m.errGetExerciseByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.ID == id {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) GetByName(name string) (*models.Exercise, error) {
	if m.errGetByName != nil {
		return nil, m.errGetByName
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == name {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) Update(id string, params models.UpdateExerciseParams) (*models.Exercise, error) {
	if m.errUpdate != nil {
		return nil, m.errUpdate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exercises {
		if e.ID == id {
			m.exercises[i].Name = params.Name
			m.exercises[i].Description = params.Description
			m.exercises[i].VideoURL = params.VideoURL
			m.exercises[i].ImgURL = params.ImgURL
			m.exercises[i].ImgURLOriginal = params.ImgURLOriginal
			m.exercises[i].Type = params.Type
			cp := m.exercises[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) List() ([]models.Exercise, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]models.Exercise, len(m.exercises))
	copy(result, m.exercises)
	return result, nil
}

func (m *mockRepository) CreateExerciseEntry(entry *models.ExerciseEntry) error {
	if m.errCreateExerciseEntry != nil {
		return m.errCreateExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var exerciseID string
	for _, e := range m.exercises {
		if e.Name == entry.ExerciseName {
			exerciseID = e.ID
			break
		}
	}
	if exerciseID == "" {
		exerciseID = "ex-" + entry.ExerciseName
		m.exercises = append(m.exercises, models.Exercise{ID: exerciseID, Name: entry.ExerciseName})
	}
	// Mirror the real repository's JOIN: the persisted row carries the
	// linked exercise's name and type so responses can branch on type.
	for _, e := range m.exercises {
		if e.ID == entry.ExerciseID {
			entry.ExerciseName = e.Name
			entry.ExerciseType = e.Type
			break
		}
	}
	entry.ExerciseID = exerciseID
	entry.ID = "entry-" + entry.ExerciseName
	m.exerciseEntries = append(m.exerciseEntries, *entry)
	return nil
}

func (m *mockRepository) GetExerciseEntry(id string, userID string) (*models.ExerciseEntry, error) {
	if m.errGetExerciseEntry != nil {
		return nil, m.errGetExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exerciseEntries {
		if e.ID == id && e.UserID == userID {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) UpdateExerciseEntry(entry *models.ExerciseEntry, userID string) error {
	if m.errUpdateExerciseEntry != nil {
		return m.errUpdateExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exerciseEntries {
		if e.ID == entry.ID && e.UserID == userID {
			m.exerciseEntries[i] = *entry
			return nil
		}
	}
	return nil
}

func (m *mockRepository) UpdateExerciseEntryWithDate(entry *models.ExerciseEntry, userID string) error {
	if m.errUpdateExerciseEntryWithDate != nil {
		return m.errUpdateExerciseEntryWithDate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exerciseEntries {
		if e.ID == entry.ID && e.UserID == userID {
			m.exerciseEntries[i] = *entry
			return nil
		}
	}
	return nil
}

func (m *mockRepository) DeleteExerciseEntry(id string, userID string) error {
	if m.errDeleteExerciseEntry != nil {
		return m.errDeleteExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exerciseEntries {
		if e.ID == id && e.UserID == userID {
			m.exerciseEntries = append(m.exerciseEntries[:i], m.exerciseEntries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockRepository) ListExerciseEntries(userID string, limit int) ([]models.ExerciseEntry, error) {
	if m.errListExerciseEntries != nil {
		return nil, m.errListExerciseEntries
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if e.UserID == userID {
			result = append(result, e)
		}
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepository) GetExerciseEntriesByExercisePaginated(exerciseID string, userID string, limit, offset int) ([]models.ExerciseEntry, error) {
	if m.errGetExerciseEntriesByExercisePaginated != nil {
		return nil, m.errGetExerciseEntriesByExercisePaginated
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if e.ExerciseID == exerciseID && e.UserID == userID {
			all = append(all, e)
		}
	}
	if offset >= len(all) {
		return []models.ExerciseEntry{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (m *mockRepository) GetMaxWeightByExercise(exerciseID string, userID string) (float64, error) {
	if m.errGetMaxWeightByExercise != nil {
		return 0, m.errGetMaxWeightByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var max float64
	for _, e := range m.exerciseEntries {
		if e.ExerciseID == exerciseID && e.UserID == userID && e.Weight > max {
			max = e.Weight
		}
	}
	return max, nil
}

func (m *mockRepository) GetBestPaceByExercise(exerciseID string, userID string) (float64, error) {
	if m.errGetBestPaceByExercise != nil {
		return 0, m.errGetBestPaceByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	best := 0.0
	for _, e := range m.exerciseEntries {
		if e.ExerciseID != exerciseID || e.UserID != userID {
			continue
		}
		if pace := e.PaceSecPerKm(); pace > 0 && (best == 0 || pace < best) {
			best = pace
		}
	}
	return best, nil
}

func (m *mockRepository) GetLongestDistanceByExercise(exerciseID string, userID string) (float64, error) {
	if m.errGetLongestDistanceByExercise != nil {
		return 0, m.errGetLongestDistanceByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var longest float64
	for _, e := range m.exerciseEntries {
		if e.ExerciseID == exerciseID && e.UserID == userID && e.DistanceMeters > longest {
			longest = e.DistanceMeters
		}
	}
	return longest, nil
}

func (m *mockRepository) GetLastSetByExercise(exerciseID string, userID string) (*models.ExerciseEntry, error) {
	if m.errGetLastSetByExercise != nil {
		return nil, m.errGetLastSetByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var last *models.ExerciseEntry
	for i, e := range m.exerciseEntries {
		if e.ExerciseID != exerciseID || e.UserID != userID {
			continue
		}
		if last == nil || e.CreatedAt.After(last.CreatedAt) {
			cp := m.exerciseEntries[i]
			last = &cp
		}
	}
	return last, nil
}

func (m *mockRepository) GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]models.ExerciseEntry, error) {
	if m.errGetExerciseEntriesByDateRange != nil {
		return nil, m.errGetExerciseEntriesByDateRange
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if !e.CreatedAt.Before(start) && !e.CreatedAt.After(end) && e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockRepository) ListExerciseEntriesLast7Days(userID string) ([]models.ExerciseEntry, error) {
	if m.errListExerciseEntries != nil {
		return nil, m.errListExerciseEntries
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	var result []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if e.CreatedAt.After(sevenDaysAgo) && e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

type mockUserRepository struct {
	mu    sync.Mutex
	users []models.User
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{}
}

func (m *mockUserRepository) CreateUser(user *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == user.Email {
			return errors.New("email already exists")
		}
	}
	user.ID = "user-" + user.Email
	m.users = append(m.users, *user)
	return nil
}

func (m *mockUserRepository) GetUserByEmail(email string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepository) GetUserByID(id string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepository) UpdateUser(user *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, u := range m.users {
		if u.ID == user.ID {
			m.users[i] = *user
			return nil
		}
	}
	return errors.New("user not found")
}

func (m *mockUserRepository) UpdateUserPassword(userID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if passwordHash == "" {
		return errors.New("password hash is empty")
	}
	for i, u := range m.users {
		if u.ID == userID {
			m.users[i].PasswordHash = passwordHash
			return nil
		}
	}
	return errors.New("user not found")
}

// UpdateUserReminder stores the reminder preferences on the
// matching user row. The route tests that exercise the
// reminder save path (e.g. TestProfileUpdateReminder) check
// the stored values via GetUserByID, so this is the only
// piece of plumbing the mock needs.
func (m *mockUserRepository) UpdateUserReminder(userID string, prefs models.ReminderPreferences) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, u := range m.users {
		if u.ID == userID {
			m.users[i].ReminderEnabled = prefs.Enabled
			m.users[i].ReminderFrequency = prefs.Frequency
			m.users[i].ReminderDayOfWeek = prefs.DayOfWeek
			m.users[i].ReminderTime = prefs.Time
			m.users[i].ReminderNextFireAt = prefs.NextFireAt
			return nil
		}
	}
	return errors.New("user not found")
}

// mockAuthTokenRepo is a no-op AuthTokenRepo used by the
// route tests that don't exercise the password-reset flow.
// The recovery controller's tests in
// auth_recovery_test.go use a richer mock.
type mockAuthTokenRepo struct{}

func newMockAuthTokenRepo() *mockAuthTokenRepo { return &mockAuthTokenRepo{} }

func (m *mockAuthTokenRepo) CreatePasswordResetToken(ctx context.Context, userID, raw string, ttl time.Duration) (string, error) {
	return "", nil
}

func (m *mockAuthTokenRepo) ConsumePasswordResetToken(ctx context.Context, raw string) (string, error) {
	return "", models.ErrAuthTokenInvalid
}

type mockAdminUserRepository struct {
	mu    sync.Mutex
	users []models.User
}

func newMockAdminUserRepository() *mockAdminUserRepository {
	return &mockAdminUserRepository{}
}

func (m *mockAdminUserRepository) ListUsers(_ context.Context) ([]models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.users, nil
}

// GetUserByID returns a copy of the matching admin-user row, or nil
// when the ID is unknown. Used by the admin user action tests to
// verify the controller validated the target user first.
func (m *mockAdminUserRepository) GetUserByID(_ context.Context, id string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.users {
		if m.users[i].ID == id {
			u := m.users[i]
			return &u, nil
		}
	}
	return nil, nil
}

// SetUserAdmin flips the admin flag on the matching row. Unknown IDs
// error so a silent no-op cannot hide a broken test expectation.
func (m *mockAdminUserRepository) SetUserAdmin(_ context.Context, userID string, isAdmin bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.users {
		if m.users[i].ID == userID {
			m.users[i].IsAdmin = isAdmin
			return nil
		}
	}
	return errors.New("user not found")
}

type mockFeedbackRepository struct {
	mu       sync.Mutex
	feedback []*models.Feedback

	errCreate   error
	errGetAll   error
	errGetByID  error
	errUpdate   error
}

func newMockFeedbackRepository() *mockFeedbackRepository {
	return &mockFeedbackRepository{}
}

func (m *mockFeedbackRepository) Create(feedback *models.Feedback) error {
	if m.errCreate != nil {
		return m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	feedback.ID = "fb-" + feedback.Title
	m.feedback = append(m.feedback, feedback)
	return nil
}

func (m *mockFeedbackRepository) GetAll(filter string) ([]*models.Feedback, error) {
	if m.errGetAll != nil {
		return nil, m.errGetAll
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*models.Feedback
	for _, f := range m.feedback {
		switch filter {
		case "open":
			if !f.IsClosed {
				result = append(result, f)
			}
		case "closed":
			if f.IsClosed {
				result = append(result, f)
			}
		default:
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockFeedbackRepository) GetByID(id string) (*models.Feedback, error) {
	if m.errGetByID != nil {
		return nil, m.errGetByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.feedback {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, nil
}

func (m *mockFeedbackRepository) UpdateStatus(id string, isClosed bool) error {
	if m.errUpdate != nil {
		return m.errUpdate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.feedback {
		if f.ID == id {
			f.IsClosed = isClosed
			return nil
		}
	}
	return nil
}

type mockWeightRepository struct {
	mu       sync.Mutex
	entries  []models.WeightEntry

	errCreate    error
	errGetByID   error
	errList      error
	errUpdate    error
	errDelete    error
}

func newMockWeightRepository() *mockWeightRepository {
	return &mockWeightRepository{}
}

func (m *mockWeightRepository) Create(entry *models.WeightEntry) error {
	if m.errCreate != nil {
		return m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.ID = "weight-" + fmt.Sprintf("%d", len(m.entries)+1)
	m.entries = append(m.entries, *entry)
	return nil
}

func (m *mockWeightRepository) GetByID(id string, userID string) (*models.WeightEntry, error) {
	if m.errGetByID != nil {
		return nil, m.errGetByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.ID == id && e.UserID == userID {
			return &e, nil
		}
	}
	return nil, nil
}

func (m *mockWeightRepository) List(userID string) ([]models.WeightEntry, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.WeightEntry
	for _, e := range m.entries {
		if e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockWeightRepository) Update(entry *models.WeightEntry, userID string) error {
	if m.errUpdate != nil {
		return m.errUpdate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.ID == entry.ID && e.UserID == userID {
			m.entries[i] = *entry
			return nil
		}
	}
	return nil
}

func (m *mockWeightRepository) Delete(id string, userID string) error {
	if m.errDelete != nil {
		return m.errDelete
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.ID == id && e.UserID == userID {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockWeightRepository) GetByIDs(idA, idB, userID string) ([]models.WeightEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.WeightEntry
	for _, e := range m.entries {
		if e.UserID != userID {
			continue
		}
		if e.ID == idA || e.ID == idB {
			result = append(result, e)
		}
	}
	return result, nil
}

// mockGoalRepository satisfies the models.GoalRepo interface for the
// route tests. It tracks goals per user and supports per-method
// error injection so handler-level error paths can be exercised
// without a live DB.
type mockGoalRepository struct {
	mu    sync.Mutex
	goals map[string]*models.Goal

	errCreate       error
	errGetByID      error
	errList         error
	errUpdate       error
	errMarkComplete error
	errReopen       error
	errDelete       error
}

func newMockGoalRepository() *mockGoalRepository {
	return &mockGoalRepository{
		goals: map[string]*models.Goal{},
	}
}

func (m *mockGoalRepository) Create(g *models.Goal) error {
	if m.errCreate != nil {
		return m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g.ID = "goal-" + fmt.Sprintf("%d", len(m.goals)+1)
	cp := *g
	m.goals[g.ID] = &cp
	*g = cp
	return nil
}

func (m *mockGoalRepository) GetByID(id, userID string) (*models.Goal, error) {
	if m.errGetByID != nil {
		return nil, m.errGetByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil, nil
	}
	cp := *g
	return &cp, nil
}

func (m *mockGoalRepository) List(userID string) ([]models.Goal, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var active, completed []models.Goal
	for _, g := range m.goals {
		if g.UserID != userID {
			continue
		}
		cp := *g
		if g.CompletedAt != nil {
			completed = append(completed, cp)
		} else {
			active = append(active, cp)
		}
	}
	out := append([]models.Goal{}, active...)
	out = append(out, completed...)
	return out, nil
}

func (m *mockGoalRepository) Update(g *models.Goal, userID string) error {
	if m.errUpdate != nil {
		return m.errUpdate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.goals[g.ID]
	if !ok || existing.UserID != userID {
		return nil
	}
	existing.Title = g.Title
	existing.Description = g.Description
	existing.StartDate = g.StartDate
	existing.TargetDate = g.TargetDate
	existing.EndDate = g.EndDate
	return nil
}

func (m *mockGoalRepository) MarkComplete(id, userID string, completedAt time.Time) error {
	if m.errMarkComplete != nil {
		return m.errMarkComplete
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil
	}
	if g.CompletedAt != nil {
		return nil
	}
	cp := completedAt
	g.CompletedAt = &cp
	return nil
}

func (m *mockGoalRepository) Reopen(id, userID string) error {
	if m.errReopen != nil {
		return m.errReopen
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil
	}
	g.CompletedAt = nil
	return nil
}

func (m *mockGoalRepository) Delete(id, userID string) error {
	if m.errDelete != nil {
		return m.errDelete
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil
	}
	delete(m.goals, id)
	return nil
}

func setupHandler(t *testing.T) (*Handler, *mockRepository, *mockUserRepository, *echo.Echo) {
	t.Helper()
	e := echo.New()
	mock := newMockRepository()
	mockUser := newMockUserRepository()
	mockAdminUser := newMockAdminUserRepository()
	mockFeedback := newMockFeedbackRepository()
	mockWeight := newMockWeightRepository()
	mockGoals := newMockGoalRepository()
	jwtService := utils.NewJWTService("test-secret")
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
		{ID: "ex-2", Name: "Bench Press"},
	}
	authCtrl := controllers.NewAuthController(mockUser, jwtService, nil)
	authRecoveryCtrl := controllers.NewAuthRecoveryController(mockUser, newMockAuthTokenRepo(), nil)
	entryCtrl := controllers.NewExerciseEntryController(mock)
	adminCtrl := controllers.NewAdminController(mock)
	adminUserCtrl := controllers.NewAdminUserController(mockAdminUser, newMockAuthTokenRepo(), nil)
	feedbackCtrl := controllers.NewFeedbackController(mockFeedback)
	weightCtrl := controllers.NewWeightController(mockWeight, nil)
	goalsCtrl := controllers.NewGoalsController(mockGoals)
	validator := utils.NewValidator()
	// The default test wiring uses a fake image processor and
	// uploader so the upload route can be exercised without
	// pulling in the real imaging package or a live S3 client.
	proc, upl := newFakeImagePipeline()
	h := NewHandler(
		authCtrl, authRecoveryCtrl, entryCtrl, adminCtrl, adminUserCtrl,
		feedbackCtrl, weightCtrl,
		goalsCtrl,
		mockUser, jwtService, validator,
		proc, upl, DefaultExerciseImageConfig,
	)
	// Register routes so the full HTTP path (including
	// middleware) is exercised. Tests that need to call
	// handler methods directly can still do so — the
	// middleware runs only through e.ServeHTTP.
	h.RegisterRoutes(e)
	return h, mock, mockUser, e
}

func setAuthContext(c echo.Context, userID string, email, name string, isAdmin bool) {
	claims := &utils.Claims{
		UserID:  userID,
		Email:   email,
		Name:    name,
		IsAdmin: isAdmin,
	}
	c.Set("auth_claims", claims)
}

func chdirToProjectRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(wd) == "routes" {
		if err := os.Chdir("../.."); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			os.Chdir(wd)
		})
	}
}

func TestDashboard(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
	}

	req := httptest.NewRequest(http.MethodGet, dashboardPath, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.Dashboard(c); err != nil {
		t.Fatalf("Dashboard failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestDashboard_RepositoryError(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.errListExerciseEntries = errors.New("db error")

	req := httptest.NewRequest(http.MethodGet, dashboardPath, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	err := h.Dashboard(c)
	if err == nil {
		t.Fatal("expected error from repository, got nil")
	}
}

func TestExerciseHistory(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
		{ID: "entry-2", UserID: "user-1", ExerciseName: "Bench Press", Reps: 8, Weight: 80},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseHistory(c); err != nil {
		t.Fatalf("ExerciseHistory failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestExerciseHistory_PageQuery(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	now := time.Now()
	var entries []models.ExerciseEntry
	for i := 0; i < 30; i++ {
		entries = append(entries, models.ExerciseEntry{
			ID:         fmt.Sprintf("entry-%d", i),
			UserID:     "user-1",
			ExerciseID: "uuid-exercise-1",
			Reps:       5,
			Weight:     float64(100 + i),
			CreatedAt:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	mock.exerciseEntries = entries

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1?page=2", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseHistory(c); err != nil {
		t.Fatalf("ExerciseHistory (?page=2) failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Page 2") {
		t.Error("expected 'Page 2' indicator in full-page response")
	}
}

func TestExerciseHistory_HtmxFragment(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	now := time.Now()
	var entries []models.ExerciseEntry
	for i := 0; i < 30; i++ {
		entries = append(entries, models.ExerciseEntry{
			ID:         fmt.Sprintf("entry-%d", i),
			UserID:     "user-1",
			ExerciseID: "uuid-exercise-1",
			Reps:       5,
			Weight:     float64(100 + i),
			CreatedAt:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	mock.exerciseEntries = entries

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1?page=2", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseHistory(c); err != nil {
		t.Fatalf("ExerciseHistory (htmx) failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "HX-Request") {
		t.Errorf("expected Vary: HX-Request header, got %q", vary)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "history-table-wrap") {
		t.Error("expected fragment to contain swappable wrap id")
	}
	// Fragment must NOT include the full page chrome.
	if strings.Contains(body, "<html") {
		t.Error("fragment response should not include the full HTML document")
	}
	if !strings.Contains(body, "Page 2") {
		t.Error("expected 'Page 2' indicator inside the fragment")
	}
}

// TestExerciseChart verifies the chart placeholder route returns a 200
// with the expected sub-view links and the chart sub-view marked as
// active. Mirrors the TestExerciseHistory happy-path pattern.
func TestExerciseChart(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1/chart", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseChart(c); err != nil {
		t.Fatalf("ExerciseChart failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1/chart"`) {
		t.Error("expected Chart link in button group")
	}
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1/chart" class="btn" aria-current="page"`) {
		t.Error("expected Chart link to be the active sub-view")
	}
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1/chart/advanced"`) {
		t.Error("expected Advanced link in button group")
	}
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1"`) {
		t.Error("expected History (back) link in button group")
	}
}

// TestExerciseChart_NotFound asserts the chart route returns a 404 when
// the exercise ID does not match a row the user can see. We surface this
// explicitly (rather than letting an unhandled nil panic) so a bad URL
// fails cleanly in the browser.
func TestExerciseChart_NotFound(t *testing.T) {
	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/exercises/missing/chart", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("missing")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	err := h.ExerciseChart(c)
	if err == nil {
		t.Fatal("expected error for missing exercise")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
}

// TestExerciseChart_Populated asserts the dedicated chart route renders
// the full-width chart card and the dedicated exercise-chart canvas
// when the user has at least 2 unique days of data. Multiple sets on
// the same day are aggregated to a single point on the line.
func TestExerciseChart_Populated(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	day1 := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 17, 9, 0, 0, 0, time.UTC)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 100, CreatedAt: day1},
		{ID: "e2", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 3, Weight: 105, CreatedAt: day1.Add(8 * time.Hour)},
		{ID: "e3", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 110, CreatedAt: day2},
		{ID: "e4", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 3, Weight: 108, CreatedAt: day2.Add(8 * time.Hour)},
		// Other-exercise entries must be ignored.
		{ID: "e5", ExerciseID: "uuid-exercise-other", UserID: "user-1", Reps: 5, Weight: 200, CreatedAt: day1},
		// Other-user entries must be ignored.
		{ID: "e6", ExerciseID: "uuid-exercise-1", UserID: "user-other", Reps: 5, Weight: 999, CreatedAt: day1},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1/chart", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseChart(c); err != nil {
		t.Fatalf("ExerciseChart failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Page header copy.
	if !strings.Contains(body, "Squat Progression") {
		t.Error("expected page header with exercise name + Progression")
	}
	if !strings.Contains(body, "Max weight per training day") {
		t.Error("expected subtitle describing the chart aggregation")
	}
	// Full-width chart card chrome.
	if !strings.Contains(body, `<div class="card p-4">`) {
		t.Error("expected full-width card container")
	}
	if !strings.Contains(body, `class="h-[60vh] min-h-96"`) {
		t.Error("expected tall fixed-height chart wrapper")
	}
	// Distinct canvas id and JSON data block.
	if !strings.Contains(body, `<canvas id="exercise-chart">`) {
		t.Error("expected canvas with id exercise-chart")
	}
	if !strings.Contains(body, `id="exercise-chart-data"`) {
		t.Error("expected exercise-chart-data JSON payload")
	}
	// Dataset label reflects the exercise name.
	if !strings.Contains(body, "Squat (kg)") {
		t.Error("expected dataset label to include the exercise name + (kg)")
	}
	// Aggregation: only 2 unique days -> 2 chart points. Heaviest per
	// day wins, so day1 = 105 and day2 = 110.
	re := regexp.MustCompile(`<script id="exercise-chart-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatal("could not find exercise-chart-data script block")
	}
	var parsed struct {
		Labels   []string `json:"labels"`
		Datasets []struct {
			Values []float64 `json:"values"`
		} `json:"datasets"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("data block is not valid JSON: %v\ncontent: %s", err, m[1])
	}
	if len(parsed.Labels) != 2 {
		t.Errorf("expected 2 day-bucketed labels, got %d (%v)", len(parsed.Labels), parsed.Labels)
	}
	if len(parsed.Datasets) != 1 {
		t.Fatalf("expected 1 dataset, got %d", len(parsed.Datasets))
	}
	if len(parsed.Datasets[0].Values) != 2 || parsed.Datasets[0].Values[0] != 105 || parsed.Datasets[0].Values[1] != 110 {
		t.Errorf("expected max-weight-per-day values [105, 110], got %v", parsed.Datasets[0].Values)
	}
	// Empty state must NOT be rendered in the populated case.
	if strings.Contains(body, "Log at least 2 sessions") {
		t.Error("did not expect empty-state message in populated chart view")
	}
}

// TestExerciseChart_EmptyState asserts the empty-state message is
// rendered (and the chart canvas is not) when the user has fewer than
// 2 unique days of data for the exercise.
func TestExerciseChart_EmptyState(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	// Single entry, single day — below the 2-day threshold.
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 100, CreatedAt: time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1/chart", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseChart(c); err != nil {
		t.Fatalf("ExerciseChart failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "Log at least 2 sessions to see your progression.") {
		t.Error("expected friendly empty-state message in the empty-state route test")
	}
	if strings.Contains(body, `<canvas id="exercise-chart">`) {
		t.Error("did not expect chart canvas in empty-state route response")
	}
	// The page header copy should still be rendered so the user
	// understands which exercise the empty state applies to.
	if !strings.Contains(body, "Squat Progression") {
		t.Error("expected page header copy even in the empty state")
	}
}

// TestExerciseChartAdvanced asserts the dedicated advanced route returns
// 200 OK, the Advanced link is marked active, the line chart link is
// NOT marked active, and all three sub-view links are present in the
// shared button group.
//
// Two sets of data are passed so the chart (and its subtitle) is
// rendered. The empty-state case is covered by
// TestExerciseChartAdvanced_EmptyState.
func TestExerciseChartAdvanced(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	day1 := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 17, 9, 0, 0, 0, time.UTC)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 100, CreatedAt: day1},
		{ID: "e2", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 105, CreatedAt: day2},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1/chart/advanced", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseChartAdvanced(c); err != nil {
		t.Fatalf("ExerciseChartAdvanced failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1/chart/advanced" class="btn" aria-current="page"`) {
		t.Error("expected Advanced link to be the active sub-view")
	}
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1/chart"`) {
		t.Error("expected Chart link in button group")
	}
	if !strings.Contains(body, `href="/exercises/uuid-exercise-1"`) {
		t.Error("expected History (back) link in button group")
	}
	// Page header copy is rendered on the advanced view too.
	if !strings.Contains(body, "Squat Volume") {
		t.Error("expected page header with exercise name + Volume")
	}
	if !strings.Contains(body, "Every set plotted by reps and weight") {
		t.Error("expected subtitle describing the scatter plot")
	}
	// Scatter canvas must be present (2 entries -> 2 dots).
	if !strings.Contains(body, `<canvas id="exercise-chart-advanced">`) {
		t.Error("expected scatter canvas when entries exist")
	}
	if strings.Contains(body, "Log at least 2 sets to see your volume profile.") {
		t.Error("did not expect empty-state message when entries exist")
	}
}

// TestExerciseChartAdvanced_Populated asserts the dedicated advanced
// route renders the full-width scatter card and the dedicated
// exercise-chart-advanced canvas when the user has at least 2 sets of
// data. Each set is plotted as its own (reps, weight) point — no
// per-day collapse.
func TestExerciseChartAdvanced_Populated(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	day1 := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 17, 9, 0, 0, 0, time.UTC)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 100, CreatedAt: day1},
		{ID: "e2", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 3, Weight: 110, CreatedAt: day1.Add(8 * time.Hour)},
		{ID: "e3", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 115, CreatedAt: day2},
		// Other-exercise entries must be ignored.
		{ID: "e4", ExerciseID: "uuid-exercise-other", UserID: "user-1", Reps: 5, Weight: 200, CreatedAt: day1},
		// Other-user entries must be ignored.
		{ID: "e5", ExerciseID: "uuid-exercise-1", UserID: "user-other", Reps: 5, Weight: 999, CreatedAt: day1},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1/chart/advanced", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseChartAdvanced(c); err != nil {
		t.Fatalf("ExerciseChartAdvanced failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Full-width scatter card chrome.
	if !strings.Contains(body, `<div class="card p-4">`) {
		t.Error("expected full-width card container")
	}
	if !strings.Contains(body, `class="h-[60vh] min-h-96"`) {
		t.Error("expected tall fixed-height scatter wrapper")
	}
	// Distinct canvas id from the line chart.
	if !strings.Contains(body, `<canvas id="exercise-chart-advanced">`) {
		t.Error("expected canvas with id exercise-chart-advanced")
	}
	if strings.Contains(body, `<canvas id="exercise-chart">`) {
		t.Error("did not expect the line-chart canvas id on the advanced view")
	}
	if !strings.Contains(body, `id="exercise-chart-advanced-data"`) {
		t.Error("expected exercise-chart-advanced-data JSON payload")
	}
	// Y-axis label reflects the exercise name.
	if !strings.Contains(body, "Squat (kg)") {
		t.Error("expected y-axis label to include the exercise name + (kg)")
	}
	// No per-day collapse: 3 owned sets -> 3 scatter points.
	re := regexp.MustCompile(`<script id="exercise-chart-advanced-data" type="application/json">([\s\S]*?)</script>`)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatal("could not find exercise-chart-advanced-data script block")
	}
	var parsed struct {
		XLabel   string `json:"xLabel"`
		YLabel   string `json:"yLabel"`
		Points   []struct {
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			Date string  `json:"date"`
		} `json:"points"`
		HideAxes bool `json:"hideAxes"`
	}
	if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
		t.Fatalf("data block is not valid JSON: %v\ncontent: %s", err, m[1])
	}
	if parsed.XLabel != "Reps" {
		t.Errorf("expected xLabel 'Reps', got %q", parsed.XLabel)
	}
	if parsed.YLabel != "Squat (kg)" {
		t.Errorf("expected yLabel 'Squat (kg)', got %q", parsed.YLabel)
	}
	if parsed.HideAxes {
		t.Error("expected hideAxes=false at full width so axis tick labels are visible")
	}
	if len(parsed.Points) != 3 {
		t.Errorf("expected 3 per-set scatter points, got %d", len(parsed.Points))
	}
	// Empty state must NOT be rendered in the populated case.
	if strings.Contains(body, "Log at least 2 sets") {
		t.Error("did not expect empty-state message in populated advanced view")
	}
}

// TestExerciseChartAdvanced_EmptyState asserts the empty-state message
// is rendered (and the scatter canvas is not) when the user has fewer
// than 2 sets for the exercise.
func TestExerciseChartAdvanced_EmptyState(t *testing.T) {
	h, mock, _, e := setupHandler(t)
	mock.exercises = []models.Exercise{
		{ID: "uuid-exercise-1", Name: "Squat"},
	}
	// Single set — below the 2-set threshold.
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", ExerciseID: "uuid-exercise-1", UserID: "user-1", Reps: 5, Weight: 100, CreatedAt: time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/uuid-exercise-1/chart/advanced", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("uuid-exercise-1")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ExerciseChartAdvanced(c); err != nil {
		t.Fatalf("ExerciseChartAdvanced failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "Log at least 2 sets to see your volume profile.") {
		t.Error("expected friendly empty-state message in the empty-state route test")
	}
	if strings.Contains(body, `<canvas id="exercise-chart-advanced">`) {
		t.Error("did not expect scatter canvas in empty-state route response")
	}
	// The page header copy should still be rendered so the user
	// understands which exercise the empty state applies to.
	if !strings.Contains(body, "Squat Volume") {
		t.Error("expected page header copy even in the empty state")
	}
}

// TestExerciseChartAdvanced_NotFound asserts the advanced route also
// returns 404 for unknown exercise IDs.
func TestExerciseChartAdvanced_NotFound(t *testing.T) {
	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/exercises/missing/chart/advanced", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("missing")
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	err := h.ExerciseChartAdvanced(c)
	if err == nil {
		t.Fatal("expected error for missing exercise")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestParsePage(t *testing.T) {
	tests := []struct {
		raw      string
		expected int
	}{
		{"", 1},
		{"0", 1},
		{"-5", 1},
		{"abc", 1},
		{"1", 1},
		{"3", 3},
		{"42", 42},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := parsePage(tt.raw); got != tt.expected {
				t.Errorf("parsePage(%q) = %d, want %d", tt.raw, got, tt.expected)
			}
		})
	}
}

func TestServeManifest(t *testing.T) {
	chdirToProjectRoot(t)

	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.ServeManifest(c); err != nil {
		t.Fatalf("ServeManifest failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	ct := rec.Header().Get(echo.HeaderContentType)
	if ct != "application/manifest+json" {
		t.Fatalf("expected content-type 'application/manifest+json', got %q", ct)
	}
}

func TestLoginForm(t *testing.T) {
	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.LoginForm(c); err != nil {
		t.Fatalf("LoginForm failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Login") {
		t.Fatalf("expected login form, got %q", body)
	}
}

func TestRegisterForm(t *testing.T) {
	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.RegisterForm(c); err != nil {
		t.Fatalf("RegisterForm failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Register") {
		t.Fatalf("expected register form, got %q", body)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := url.Values{}
	form.Set("name", "Alice")
	form.Set("email", "alice@example.com")
	form.Set("password", "secret123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after register, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != dashboardPath {
		t.Fatalf("expected redirect to %s, got %q", dashboardPath, loc)
	}

	cookies := rec.Result().Cookies()
	var hasAuth bool
	for _, cookie := range cookies {
		if cookie.Name == utils.CookieName {
			hasAuth = true
			break
		}
	}
	if !hasAuth {
		t.Fatal("expected auth cookie to be set after registration")
	}

	form = url.Values{}
	form.Set("email", "alice@example.com")
	form.Set("password", "secret123")

	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after login, got %d", rec.Code)
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	mockUser.users = append(mockUser.users, models.User{
		ID:           "user-1",
		Name:         "Bob",
		Email:        "bob@example.com",
		PasswordHash: string(hash),
	})

	form := url.Values{}
	form.Set("email", "bob@example.com")
	form.Set("password", "wrong")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (rendered error), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid") {
		t.Fatalf("expected error message, got %q", body)
	}
}

func TestLogin_InvalidEmail(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := url.Values{}
	form.Set("email", "not-an-email")
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (rendered error), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "email") {
		t.Fatalf("expected email validation error, got %q", body)
	}
}

func TestLogin_EmptyFields(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := url.Values{}
	form.Set("email", "")
	form.Set("password", "")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (rendered error), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "required") {
		t.Fatalf("expected required field error, got %q", body)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := url.Values{}
	form.Set("name", "Alice")
	form.Set("email", "not-an-email")
	form.Set("password", "secret123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (rendered error), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "email") {
		t.Fatalf("expected email validation error, got %q", body)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := url.Values{}
	form.Set("name", "Alice")
	form.Set("email", "alice@example.com")
	form.Set("password", "short")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (rendered error), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Password") {
		t.Fatalf("expected password validation error, got %q", body)
	}
}

func TestRegister_EmptyFields(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := url.Values{}
	form.Set("name", "")
	form.Set("email", "")
	form.Set("password", "")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (rendered error), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "required") {
		t.Fatalf("expected required field error, got %q", body)
	}
}

func TestLogout(t *testing.T) {
	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.Logout(c); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after logout, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/login" {
		t.Fatalf("expected redirect to '/login', got %q", loc)
	}

	cookies := rec.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == utils.CookieName && cookie.MaxAge >= 0 {
			t.Fatal("expected auth cookie to be cleared")
		}
	}
}

// stubPhotoGetter is an export.PhotoGetter backed by an in-memory map.
// The route test uses it to assert end-to-end zip streaming without
// pulling in R2.
type stubPhotoGetter struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *stubPhotoGetter) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.data[key]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return nil, errors.New("stub: not found: " + key)
}

// TestExportWeightZip_StreamsZip confirms GET /export/weight responds
// with a valid zip containing the user's entries. The route runs the
// export in a background goroutine, so we have to read the whole body
// before asserting on the zip — the in-memory httptest.ResponseRecorder
// does that for us.
func TestExportWeightZip_StreamsZip(t *testing.T) {
	h, _, _, e := setupHandler(t)

	// Need to inject a stub photo getter. The shared setupHandler
	// passes nil, so rebuild the weight controller with one for this
	// test only.
	stub := &stubPhotoGetter{data: map[string][]byte{
		"weight/u1/photo.jpg": {0xFF, 0xD8, 0xFF, 0xE0}, // jpeg magic
	}}
	mockWeight := newMockWeightRepository()
	mockWeight.entries = []models.WeightEntry{
		{ID: "w1", UserID: "u1", Weight: 80, Notes: "morning", PhotoKey: "weight/u1/photo.jpg", CreatedAt: time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)},
		{ID: "w2", UserID: "u1", Weight: 79, Notes: "evening", PhotoKey: "", CreatedAt: time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)},
	}
	h.weightCtrl = controllers.NewWeightController(mockWeight, stub)

	req := httptest.NewRequest(http.MethodGet, "/export/weight", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "u1", "test@example.com", "Test User", false)

	if err := h.ExportWeightZip(c); err != nil {
		t.Fatalf("ExportWeightZip failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(echo.HeaderContentType); got != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", got)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "hylete-weight-export-") {
		t.Errorf("Content-Disposition = %q, want attachment with hylete-weight-export-*", cd)
	}

	// Validate the body is a real zip with the expected files.
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("body is not a valid zip: %v", err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	for _, want := range []string{"weight.csv", "manifest.json", "photos/2026-01-09_w1.jpg"} {
		if !got[want] {
			t.Errorf("expected %q in zip, got files: %v", want, keys(got))
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestExportWeightZip_Unauthenticated asserts the route requires a
// valid auth context. Echo's auth middleware is responsible for
// redirecting unauthenticated requests before they reach the handler,
// so here we just confirm that the handler relies on GetClaims (the
// production middleware layer would have already gated this).
func TestExportWeightZip_RequiresAuthContext(t *testing.T) {
	h, _, _, e := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/export/weight", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// No setAuthContext call. The handler dereferences
	// claims.UserID, so a missing claim should surface as a panic
	// — production traffic always has one (the middleware sets
	// it). This test pins that contract: callers must wire up
	// auth before invoking the handler.
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic when auth context is missing")
		}
	}()
	_ = h.ExportWeightZip(c)
}

// TestExportExerciseEntriesZip_StreamsZip confirms GET /export/exercises
// responds with a valid zip containing the user's exercise entries.
// Mirrors TestExportWeightZip_StreamsZip; exercise entries carry no
// photos so the archive must contain only exercise_entries.csv and
// manifest.json.
func TestExportExerciseEntriesZip_StreamsZip(t *testing.T) {
	h, mock, _, e := setupHandler(t)

	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "e1", UserID: "u1", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, RestTime: 180, CreatedAt: time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)},
		{ID: "e2", UserID: "u1", ExerciseID: "ex-2", ExerciseName: "Bench Press", Reps: 8, Weight: 60, RestTime: 90, CreatedAt: time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)},
	}

	req := httptest.NewRequest(http.MethodGet, "/exercises/export", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "u1", "test@example.com", "Test User", false)

	if err := h.ExportExerciseEntriesZip(c); err != nil {
		t.Fatalf("ExportExerciseEntriesZip failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(echo.HeaderContentType); got != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", got)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "hylete-exercise-entries-export-") {
		t.Errorf("Content-Disposition = %q, want attachment with hylete-exercise-entries-export-*", cd)
	}

	// Validate the body is a real zip with exactly the expected files.
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("body is not a valid zip: %v", err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	for _, want := range []string{"exercise_entries.csv", "manifest.json"} {
		if !got[want] {
			t.Errorf("expected %q in zip, got files: %v", want, keys(got))
		}
	}
	for name := range got {
		if strings.HasPrefix(name, "photos/") {
			t.Errorf("unexpected photo file %q in exercise entries export", name)
		}
	}
}

// TestExportExerciseEntriesZip_RequiresAuthContext asserts the route
// requires a valid auth context. Echo's auth middleware is responsible
// for redirecting unauthenticated requests before they reach the
// handler, so here we just confirm that the handler relies on
// GetClaims (the production middleware layer would have already
// gated this).
func TestExportExerciseEntriesZip_RequiresAuthContext(t *testing.T) {
	h, _, _, e := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/export/exercises", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// No setAuthContext call. The handler dereferences
	// claims.UserID, so a missing claim should surface as a panic
	// — production traffic always has one (the middleware sets
	// it). This test pins that contract: callers must wire up
	// auth before invoking the handler.
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic when auth context is missing")
		}
	}()
	_ = h.ExportExerciseEntriesZip(c)
}

// TestDataExportPage confirms GET /export renders the Data Export
// page with both download links (weight + exercise entries).
func TestDataExportPage(t *testing.T) {
	h, _, _, e := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "u1", "test@example.com", "Test User", false)

	if err := h.DataExportPage(c); err != nil {
		t.Fatalf("DataExportPage failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Data Export", `href="/export/weight"`, `href="/export/exercises"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected page body to contain %q", want)
		}
	}
}
// --- Goals route tests ---

// --- Admin create / update exercise route tests ---
//
// These cover the form → route → controller → repo path for the
// image-upload keys (`img_key`, `img_key_original`, `clear_image`).
// The bug being guarded against: the file input inside the upload
// widget shipped without a `name` attribute, so htmx dropped the
// file from the multipart body, the upload failed silently, and
// the admin's "save" form posted empty `img_key` values that
// overwrote the exercise's image with empty strings in the DB.
// With the fix in place the keys flow correctly; these tests
// assert the full path on both the create and update routes.

// adminCreateForm builds the url.Values payload the admin exercise
// create route expects.
func adminCreateForm(name, typeStr string, imgKey, imgKeyOriginal string) url.Values {
	form := url.Values{}
	form.Set("name", name)
	form.Set("type", typeStr)
	form.Set("description", "A description")
	form.Set("video_url", "")
	form.Set("img_key", imgKey)
	form.Set("img_key_original", imgKeyOriginal)
	return form
}

// adminPostForm returns a request + context pair that simulates the
// admin exercise form being submitted as an HTMX form-urlencoded
// POST. The handler is invoked directly so the AdminMiddleware is
// bypassed (setAuthContext provides the admin claims).
func adminPostForm(t *testing.T, h *Handler, e *echo.Echo, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	// Path-parameter parsing for /admin/exercises/:id: we extract
	// the id from the path and pass it through c.SetParamNames /
	// c.SetParamValues. The id segment may contain spaces or other
	// URL-unsafe chars in the test names, but Echo's path parser
	// expects them to be url.PathUnescape'd first.
	var exerciseID string
	if strings.HasPrefix(path, "/admin/exercises/") {
		raw := strings.TrimPrefix(path, "/admin/exercises/")
		if i := strings.Index(raw, "/"); i >= 0 {
			raw = raw[:i]
		}
		if decoded, derr := url.PathUnescape(raw); derr == nil {
			exerciseID = decoded
		} else {
			exerciseID = raw
		}
		// The request line must be a syntactically valid URL; if
		// the id contains a space we substitute a placeholder and
		// drive the handler off the synthetic context below.
		reqPath := path
		if strings.ContainsAny(exerciseID, " \t\n") {
			reqPath = "/admin/exercises/__id__"
		}
		req := httptest.NewRequest(http.MethodPost, reqPath, strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(exerciseID)
		setAuthContext(c, "admin-1", "admin@example.com", "Admin", true)
		if err := h.AdminUpdateExercise(c); err != nil {
			t.Fatalf("admin update handler returned error: %v", err)
		}
		return rec
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "admin-1", "admin@example.com", "Admin", true)
	if err := h.AdminCreateExercise(c); err != nil {
		t.Fatalf("admin create handler returned error: %v", err)
	}
	return rec
}

// findExerciseByName looks up an exercise in the mock repository
// by its (case-sensitive) name. Returns nil if not found.
func findExerciseByName(m *mockRepository, name string) *models.Exercise {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.exercises {
		if m.exercises[i].Name == name {
			cp := m.exercises[i]
			return &cp
		}
	}
	return nil
}

func TestAdminCreateExercise_PersistsImageKeys(t *testing.T) {
	// Regression: when an admin uploads a new image and then
	// saves the create form, the storage keys returned by the
	// upload route must end up on the persisted exercise. A
	// regression that dropped the file from the multipart body
	// (e.g. file input without a `name` attribute) would leave
	// these fields empty on the saved record.
	h, mock, _, e := setupHandler(t)

	form := adminCreateForm("Deadlift", "strength", "exercises/abc.jpg", "exercises/abc_original.jpg")
	rec := adminPostForm(t, h, e, "/admin/exercises", form)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Exercise created!") {
		t.Errorf("expected success toast in body, got: %s", body)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "triggerRedirect") {
		t.Errorf("expected HX-Trigger to redirect, got %q", trigger)
	}

	// The mock repository must have stored the keys the form
	// submitted. Look it up by name to assert against the
	// persisted record.
	got := findExerciseByName(mock, "Deadlift")
	if got == nil {
		t.Fatal("expected the new exercise to be persisted")
	}
	if got.ImgURL != "exercises/abc.jpg" {
		t.Errorf("ImgURL = %q, want %q", got.ImgURL, "exercises/abc.jpg")
	}
	if got.ImgURLOriginal != "exercises/abc_original.jpg" {
		t.Errorf("ImgURLOriginal = %q, want %q", got.ImgURLOriginal, "exercises/abc_original.jpg")
	}
}

func TestAdminCreateExercise_EmptyImageKeysAreAllowed(t *testing.T) {
	// Creating an exercise without uploading an image must
	// succeed and leave the image keys empty. This guards
	// against a regression where empty form fields were treated
	// as a hard error.
	h, mock, _, e := setupHandler(t)

	form := adminCreateForm("Bodyweight Squat", "strength", "", "")
	rec := adminPostForm(t, h, e, "/admin/exercises", form)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	got := findExerciseByName(mock, "Bodyweight Squat")
	if got == nil {
		t.Fatal("expected the new exercise to be persisted")
	}
	if got.ImgURL != "" || got.ImgURLOriginal != "" {
		t.Errorf("expected empty image keys, got ImgURL=%q ImgURLOriginal=%q", got.ImgURL, got.ImgURLOriginal)
	}
}

func TestAdminCreateExercise_RequiresName(t *testing.T) {
	h, _, _, e := setupHandler(t)

	form := adminCreateForm("", "strength", "exercises/abc.jpg", "exercises/abc_original.jpg")
	rec := adminPostForm(t, h, e, "/admin/exercises", form)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Exercise name is required") {
		t.Errorf("expected name-required error toast, got: %s", rec.Body.String())
	}
}

func TestAdminUpdateExercise_ReplacesImageOnNewKey(t *testing.T) {
	// The replace flow: exercise has an old image, the admin
	// uploads a new one and saves. The persisted keys must be
	// the new ones (the route only swaps the keys when the
	// upload widget populated a non-empty img_key).
	h, mock, _, e := setupHandler(t)

	seedForm := adminCreateForm("Back Squat", "strength", "exercises/old.jpg", "exercises/old_original.jpg")
	adminPostForm(t, h, e, "/admin/exercises", seedForm)
	before := findExerciseByName(mock, "Back Squat")
	if before == nil {
		t.Fatal("expected seed exercise to be persisted")
	}
	if before.ImgURL != "exercises/old.jpg" {
		t.Fatalf("seed ImgURL = %q, want %q", before.ImgURL, "exercises/old.jpg")
	}

	update := adminCreateForm("Back Squat", "strength", "exercises/new.jpg", "exercises/new_original.jpg")
	rec := adminPostForm(t, h, e, "/admin/exercises/"+before.ID, update)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Exercise updated!") {
		t.Errorf("expected success toast, got: %s", rec.Body.String())
	}

	after := findExerciseByName(mock, "Back Squat")
	if after == nil {
		t.Fatal("expected updated exercise to be persisted")
	}
	if after.ImgURL != "exercises/new.jpg" {
		t.Errorf("ImgURL after update = %q, want %q", after.ImgURL, "exercises/new.jpg")
	}
	if after.ImgURLOriginal != "exercises/new_original.jpg" {
		t.Errorf("ImgURLOriginal after update = %q, want %q", after.ImgURLOriginal, "exercises/new_original.jpg")
	}
}

func TestAdminUpdateExercise_ClearsImageOnClearImageFlag(t *testing.T) {
	// When the admin clicks "Remove img" the widget flips a
	// hidden `clear_image` input to "true". On save, the route
	// must wipe both keys regardless of the hidden `img_key`
	// inputs (which are populated from the upload widget).
	h, mock, _, e := setupHandler(t)

	seedForm := adminCreateForm("Incline Press", "strength", "exercises/ip.jpg", "exercises/ip_original.jpg")
	adminPostForm(t, h, e, "/admin/exercises", seedForm)
	before := findExerciseByName(mock, "Incline Press")
	if before == nil {
		t.Fatal("expected seed exercise to be persisted")
	}

	update := adminCreateForm("Incline Press", "strength", "", "")
	update.Set("clear_image", "true")
	rec := adminPostForm(t, h, e, "/admin/exercises/"+before.ID, update)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	after := findExerciseByName(mock, "Incline Press")
	if after == nil {
		t.Fatal("expected updated exercise to be persisted")
	}
	if after.ImgURL != "" {
		t.Errorf("expected ImgURL cleared, got %q", after.ImgURL)
	}
	if after.ImgURLOriginal != "" {
		t.Errorf("expected ImgURLOriginal cleared, got %q", after.ImgURLOriginal)
	}
}

func TestAdminUpdateExercise_KeepsImageWhenNoNewKey(t *testing.T) {
	// No upload, no clear flag — saving an unrelated field on
	// an exercise that already has an image must preserve the
	// existing keys. Without this, the upload-widget bug
	// (empty `img_key` reaches the route) would silently wipe
	// the exercise's image on every save.
	h, mock, _, e := setupHandler(t)

	seedForm := adminCreateForm("Romanian Deadlift", "strength", "exercises/rdl.jpg", "exercises/rdl_original.jpg")
	adminPostForm(t, h, e, "/admin/exercises", seedForm)
	before := findExerciseByName(mock, "Romanian Deadlift")
	if before == nil {
		t.Fatal("expected seed exercise to be persisted")
	}

	update := adminCreateForm("Romanian Deadlift", "strength", "", "")
	rec := adminPostForm(t, h, e, "/admin/exercises/"+before.ID, update)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	after := findExerciseByName(mock, "Romanian Deadlift")
	if after == nil {
		t.Fatal("expected updated exercise to be persisted")
	}
	if after.ImgURL != "exercises/rdl.jpg" {
		t.Errorf("ImgURL after empty-key update = %q, want preserved %q", after.ImgURL, "exercises/rdl.jpg")
	}
	if after.ImgURLOriginal != "exercises/rdl_original.jpg" {
		t.Errorf("ImgURLOriginal after empty-key update = %q, want preserved %q", after.ImgURLOriginal, "exercises/rdl_original.jpg")
	}
}

func TestAdminUpdateExercise_RequiresName(t *testing.T) {
	h, mock, _, e := setupHandler(t)

	seedForm := adminCreateForm("Lat Pulldown", "strength", "exercises/lp.jpg", "exercises/lp_original.jpg")
	adminPostForm(t, h, e, "/admin/exercises", seedForm)
	before := findExerciseByName(mock, "Lat Pulldown")
	if before == nil {
		t.Fatal("expected seed exercise to be persisted")
	}

	update := adminCreateForm("", "strength", "", "")
	rec := adminPostForm(t, h, e, "/admin/exercises/"+before.ID, update)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Exercise name is required") {
		t.Errorf("expected name-required error toast, got: %s", rec.Body.String())
	}
}
