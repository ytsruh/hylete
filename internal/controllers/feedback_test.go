package controllers

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"hylete/internal/models"
)

type mockFeedbackRepository struct {
	mu       sync.Mutex
	feedback []*models.Feedback

	errCreate  error
	errGetAll  error
	errGetByID error
	errUpdate  error
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
	feedback.ID = "feedback-" + feedback.Title
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

func TestFeedbackController_Submit_Success(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	err := ctrl.Submit("Valid Title", "This is a valid message with enough characters", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.feedback) != 1 {
		t.Fatalf("expected 1 feedback, got %d", len(mock.feedback))
	}
	if mock.feedback[0].Title != "Valid Title" {
		t.Errorf("expected title 'Valid Title', got %q", mock.feedback[0].Title)
	}
}

func TestFeedbackController_Submit_TitleTooShort(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	err := ctrl.Submit("Hi", "This is a valid message with enough characters", "user-1")
	if !errors.Is(err, ErrTitleTooShort) {
		t.Errorf("expected ErrTitleTooShort, got %v", err)
	}
}

func TestFeedbackController_Submit_MessageTooShort(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	err := ctrl.Submit("Valid Title", "Short", "user-1")
	if !errors.Is(err, ErrMessageTooShort) {
		t.Errorf("expected ErrMessageTooShort, got %v", err)
	}
}

func TestFeedbackController_Submit_TitleTooLong(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	longTitle := strings.Repeat("a", 101)
	err := ctrl.Submit(longTitle, "This is a valid message", "user-1")
	if !errors.Is(err, ErrTitleTooLong) {
		t.Errorf("expected ErrTitleTooLong, got %v", err)
	}
}

func TestFeedbackController_Submit_MessageTooLong(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	longMessage := strings.Repeat("a", 1001)
	err := ctrl.Submit("Valid Title", longMessage, "user-1")
	if !errors.Is(err, ErrMessageTooLong) {
		t.Errorf("expected ErrMessageTooLong, got %v", err)
	}
}

func TestFeedbackController_Submit_TrimsWhitespace(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	err := ctrl.Submit("  Valid Title  ", "   Valid message here   ", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.feedback[0].Title != "Valid Title" {
		t.Errorf("expected trimmed title 'Valid Title', got %q", mock.feedback[0].Title)
	}
	if mock.feedback[0].Message != "Valid message here" {
		t.Errorf("expected trimmed message 'Valid message here', got %q", mock.feedback[0].Message)
	}
}

func TestFeedbackController_AdminList_All(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.feedback = []*models.Feedback{
		{ID: "fb-1", Title: "First", Message: "Message 1"},
		{ID: "fb-2", Title: "Second", Message: "Message 2"},
	}

	ctrl := NewFeedbackController(mock)

	feedback, err := ctrl.AdminList("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feedback) != 2 {
		t.Fatalf("expected 2 feedback items, got %d", len(feedback))
	}
}

func TestFeedbackController_AdminList_Open(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.feedback = []*models.Feedback{
		{ID: "fb-1", Title: "Open", Message: "Message", IsClosed: false},
		{ID: "fb-2", Title: "Closed", Message: "Message", IsClosed: true},
	}

	ctrl := NewFeedbackController(mock)

	feedback, err := ctrl.AdminList("open")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feedback) != 1 {
		t.Fatalf("expected 1 open feedback, got %d", len(feedback))
	}
	if feedback[0].Title != "Open" {
		t.Errorf("expected open feedback title 'Open', got %q", feedback[0].Title)
	}
}

func TestFeedbackController_AdminList_Closed(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.feedback = []*models.Feedback{
		{ID: "fb-1", Title: "Open", Message: "Message", IsClosed: false},
		{ID: "fb-2", Title: "Closed", Message: "Message", IsClosed: true},
	}

	ctrl := NewFeedbackController(mock)

	feedback, err := ctrl.AdminList("closed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feedback) != 1 {
		t.Fatalf("expected 1 closed feedback, got %d", len(feedback))
	}
	if feedback[0].Title != "Closed" {
		t.Errorf("expected closed feedback title 'Closed', got %q", feedback[0].Title)
	}
}

func TestFeedbackController_AdminDetail_Found(t *testing.T) {
	mockFeedback := newMockFeedbackRepository()
	mockFeedback.feedback = []*models.Feedback{
		{ID: "fb-1", UserID: "user-1", UserName: "Test User", Title: "Found", Message: "Found message"},
	}

	ctrl := NewFeedbackController(mockFeedback)

	feedback, err := ctrl.AdminDetail("fb-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feedback.Title != "Found" {
		t.Errorf("expected title 'Found', got %q", feedback.Title)
	}
	if feedback.UserName != "Test User" {
		t.Errorf("expected user name 'Test User', got %q", feedback.UserName)
	}
}

func TestFeedbackController_AdminDetail_NotFound(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	_, err := ctrl.AdminDetail("non-existent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFeedbackController_AdminDetail_GetByIDError(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.errGetByID = errors.New("database error")
	ctrl := NewFeedbackController(mock)

	_, err := ctrl.AdminDetail("fb-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFeedbackController_Close_Success(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.feedback = []*models.Feedback{
		{ID: "fb-1", Title: "To Close", Message: "Message", IsClosed: false},
	}

	ctrl := NewFeedbackController(mock)

	err := ctrl.Close("fb-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.feedback[0].IsClosed {
		t.Fatal("expected IsClosed to be true")
	}
}

func TestFeedbackController_Close_Reopen(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.feedback = []*models.Feedback{
		{ID: "fb-1", Title: "To Reopen", Message: "Message", IsClosed: true},
	}

	ctrl := NewFeedbackController(mock)

	err := ctrl.Close("fb-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.feedback[0].IsClosed {
		t.Fatal("expected IsClosed to be false")
	}
}

func TestFeedbackController_Close_NotFound(t *testing.T) {
	mock := newMockFeedbackRepository()
	ctrl := NewFeedbackController(mock)

	err := ctrl.Close("non-existent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFeedbackController_Close_UpdateStatusError(t *testing.T) {
	mock := newMockFeedbackRepository()
	mock.feedback = []*models.Feedback{
		{ID: "fb-1", Title: "Error", Message: "Message"},
	}
	mock.errUpdate = errors.New("database error")
	ctrl := NewFeedbackController(mock)

	err := ctrl.Close("fb-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
