package summary_test

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/module/summary"
)

// --- fake store ---

type fakeSummaryStore struct {
	sessions  []summary.StudySession
	summaries []summary.SessionSummary
	nextID    int64
}

func (f *fakeSummaryStore) SaveSession(s summary.StudySession) error {
	f.nextID++
	s.ID = f.nextID
	f.sessions = append(f.sessions, s)
	return nil
}

func (f *fakeSummaryStore) GetSession(sessionID string) (*summary.StudySession, error) {
	for _, s := range f.sessions {
		if s.SessionID == sessionID {
			cp := s
			return &cp, nil
		}
	}
	return nil, errors.New("session not found")
}

func (f *fakeSummaryStore) SaveSummary(s summary.SessionSummary) error {
	f.nextID++
	s.ID = f.nextID
	f.summaries = append(f.summaries, s)
	return nil
}

func (f *fakeSummaryStore) ListSummaries(userID int64) ([]summary.SessionSummary, error) {
	var result []summary.SessionSummary
	for _, s := range f.summaries {
		if s.UserID == userID {
			result = append(result, s)
		}
	}
	return result, nil
}

// --- tests ---

func TestSummaryService_RecordSession(t *testing.T) {
	store := &fakeSummaryStore{}
	svc := summary.NewSummaryService(store)

	session := summary.StudySession{
		SessionID:       "sess-001",
		UserID:          1,
		Module:          summary.ModuleWord,
		DurationSeconds: 300,
		CompletedCount:  10,
	}

	err := svc.RecordSession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.sessions) != 1 {
		t.Errorf("expected 1 session saved, got %d", len(store.sessions))
	}
	if store.sessions[0].SessionID != "sess-001" {
		t.Errorf("expected session_id=sess-001, got %s", store.sessions[0].SessionID)
	}
}

func TestSummaryService_GenerateSummary(t *testing.T) {
	store := &fakeSummaryStore{}
	svc := summary.NewSummaryService(store)

	// Save a session first
	session := summary.StudySession{
		SessionID:       "sess-002",
		UserID:          2,
		Module:          summary.ModuleGrammar,
		DurationSeconds: 180,
		CompletedCount:  5,
	}
	_ = svc.RecordSession(session)

	sum, err := svc.GenerateSummary(2, "sess-002", summary.ModuleGrammar, summary.ScoreSummary{
		"score":   80,
		"correct": 4,
		"total":   5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.UserID != 2 {
		t.Errorf("expected UserID=2, got %d", sum.UserID)
	}
	if sum.SessionID != "sess-002" {
		t.Errorf("expected SessionID=sess-002, got %s", sum.SessionID)
	}
	if sum.Module != summary.ModuleGrammar {
		t.Errorf("expected module=grammar, got %s", sum.Module)
	}
	if len(store.summaries) != 1 {
		t.Errorf("expected 1 summary saved, got %d", len(store.summaries))
	}
}

func TestSummaryService_ListSummaries(t *testing.T) {
	store := &fakeSummaryStore{}
	svc := summary.NewSummaryService(store)

	// Generate summaries for two users
	for i, userID := range []int64{1, 1, 2} {
		_ = svc.RecordSession(summary.StudySession{
			SessionID: string(rune('a' + i)),
			UserID:    userID,
			Module:    summary.ModuleWord,
		})
		_, _ = svc.GenerateSummary(userID, string(rune('a'+i)), summary.ModuleWord, summary.ScoreSummary{})
	}

	summaries, err := svc.ListSummaries(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("expected 2 summaries for user 1, got %d", len(summaries))
	}
}

func TestSummaryService_GenerateSummary_SessionNotFound(t *testing.T) {
	store := &fakeSummaryStore{}
	svc := summary.NewSummaryService(store)

	_, err := svc.GenerateSummary(1, "nonexistent", summary.ModuleWord, summary.ScoreSummary{})
	if err == nil {
		t.Error("expected error when session not found")
	}
}
