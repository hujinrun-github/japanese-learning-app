package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/summary"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeSessionStore struct {
	sessions  map[string]*summary.StudySession
	summaries map[string]*summary.SessionSummary
}

func (f *fakeSessionStore) CreateSession(sess summary.StudySession) (string, error) {
	id := fmt.Sprintf("sess-%d", len(f.sessions)+1)
	sess.SessionID = id
	cp := sess
	f.sessions[id] = &cp
	return id, nil
}

func (f *fakeSessionStore) GetSessionData(sessionID string) (*summary.StudySession, error) {
	s, ok := f.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q: %w", sessionID, errors.New("not found"))
	}
	return s, nil
}

func (f *fakeSessionStore) SaveSummary(sum summary.SessionSummary) error {
	cp := sum
	f.summaries[sum.SessionID] = &cp
	return nil
}

func (f *fakeSessionStore) GetSummary(sessionID string) (*summary.SessionSummary, error) {
	s, ok := f.summaries[sessionID]
	if !ok {
		return nil, fmt.Errorf("summary %q: %w", sessionID, errors.New("not found"))
	}
	return s, nil
}

// --- tests ---

func TestSessionService_CreateSession(t *testing.T) {
	store := &fakeSessionStore{
		sessions:  map[string]*summary.StudySession{},
		summaries: map[string]*summary.SessionSummary{},
	}
	svc := service.NewSessionService(store)

	sess := summary.StudySession{
		UserID:          1,
		Module:          summary.ModuleWord,
		DurationSeconds: 300,
		CompletedCount:  10,
	}
	sessionID, err := svc.CreateSession(sess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID == "" {
		t.Error("expected non-empty session id")
	}
}

func TestSessionService_GetSessionData(t *testing.T) {
	store := &fakeSessionStore{
		sessions: map[string]*summary.StudySession{
			"sess-abc": {
				ID:              1,
				SessionID:       "sess-abc",
				UserID:          2,
				Module:          summary.ModuleGrammar,
				DurationSeconds: 600,
				CompletedCount:  5,
				StartedAt:       time.Now(),
			},
		},
		summaries: map[string]*summary.SessionSummary{},
	}
	svc := service.NewSessionService(store)

	got, err := svc.GetSessionData("sess-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Module != summary.ModuleGrammar {
		t.Errorf("expected module grammar, got %s", got.Module)
	}
}

func TestSessionService_SaveAndGetSummary(t *testing.T) {
	store := &fakeSessionStore{
		sessions:  map[string]*summary.StudySession{},
		summaries: map[string]*summary.SessionSummary{},
	}
	svc := service.NewSessionService(store)

	sum := summary.SessionSummary{
		UserID:    1,
		SessionID: "sess-xyz",
		Module:    summary.ModuleWriting,
		ScoreSummary: summary.ScoreSummary{
			"completed": 4,
			"avg_score": 82,
		},
		Strengths:              []summary.SummaryItem{{Label: "てもいい", Note: "全对"}},
		Weaknesses:             []summary.SummaryItem{{Label: "なければならない", Note: "答错2次"}},
		ImprovementSuggestions: []string{"多练习〜なければならない", "复习N4语法"},
	}
	if err := svc.SaveSummary(sum); err != nil {
		t.Fatalf("unexpected error saving summary: %v", err)
	}

	got, err := svc.GetSummary("sess-xyz")
	if err != nil {
		t.Fatalf("unexpected error getting summary: %v", err)
	}
	if got.Module != summary.ModuleWriting {
		t.Errorf("expected module writing, got %s", got.Module)
	}
	if len(got.ImprovementSuggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(got.ImprovementSuggestions))
	}
}

func TestSessionService_GetSummary_NotFound(t *testing.T) {
	store := &fakeSessionStore{
		sessions:  map[string]*summary.StudySession{},
		summaries: map[string]*summary.SessionSummary{},
	}
	svc := service.NewSessionService(store)
	_, err := svc.GetSummary("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent summary")
	}
}
