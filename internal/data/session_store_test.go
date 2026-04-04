package data

import (
	"testing"

	"japanese-learning-app/internal/module/summary"
)

func insertTestUser(t *testing.T, id int64, email string) {
	t.Helper()
	_, err := testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		id, email, "hash", "N5",
	)
	if err != nil {
		t.Fatalf("insertTestUser error: %v", err)
	}
}

func TestSessionStore_CreateSession(t *testing.T) {
	store := &SessionStore{db: testDB}

	insertTestUser(t, 9100, "session_create@example.com")

	session := summary.StudySession{
		UserID:          9100,
		Module:          summary.ModuleWord,
		DurationSeconds: 300,
		CompletedCount:  10,
	}

	sessionID, err := store.CreateSession(session)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if sessionID == "" {
		t.Error("CreateSession returned empty session_id")
	}

	// CreateSession 应返回唯一 ID
	sessionID2, err := store.CreateSession(session)
	if err != nil {
		t.Fatalf("CreateSession second call error: %v", err)
	}
	if sessionID == sessionID2 {
		t.Error("CreateSession returned duplicate session_id")
	}
}

func TestSessionStore_GetSessionData(t *testing.T) {
	store := &SessionStore{db: testDB}

	insertTestUser(t, 9101, "session_get@example.com")

	session := summary.StudySession{
		UserID:          9101,
		Module:          summary.ModuleGrammar,
		DurationSeconds: 600,
		CompletedCount:  5,
	}

	sessionID, err := store.CreateSession(session)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	got, err := store.GetSessionData(sessionID)
	if err != nil {
		t.Fatalf("GetSessionData error: %v", err)
	}
	if got == nil {
		t.Fatal("GetSessionData returned nil")
	}
	if got.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, sessionID)
	}
	if got.UserID != 9101 {
		t.Errorf("UserID = %d, want 9101", got.UserID)
	}
	if got.Module != summary.ModuleGrammar {
		t.Errorf("Module = %q, want grammar", got.Module)
	}
	if got.DurationSeconds != 600 {
		t.Errorf("DurationSeconds = %d, want 600", got.DurationSeconds)
	}
}

func TestSessionStore_SaveAndGetSummary(t *testing.T) {
	store := &SessionStore{db: testDB}

	insertTestUser(t, 9102, "session_summary@example.com")

	session := summary.StudySession{
		UserID:          9102,
		Module:          summary.ModuleWord,
		DurationSeconds: 120,
		CompletedCount:  8,
	}
	sessionID, err := store.CreateSession(session)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	s := summary.SessionSummary{
		UserID:    9102,
		SessionID: sessionID,
		Module:    summary.ModuleWord,
		ScoreSummary: summary.ScoreSummary{
			"reviewed":   8,
			"easy_rate":  0.625,
			"hard_count": 1,
		},
		Strengths:              []summary.SummaryItem{{Label: "食べる", Note: "连续3次easy"}},
		Weaknesses:             []summary.SummaryItem{{Label: "帰る", Note: "评分hard"}},
		ImprovementSuggestions: []string{"多复习难词"},
	}

	if err := store.SaveSummary(s); err != nil {
		t.Fatalf("SaveSummary error: %v", err)
	}

	got, err := store.GetSummary(sessionID)
	if err != nil {
		t.Fatalf("GetSummary error: %v", err)
	}
	if got == nil {
		t.Fatal("GetSummary returned nil")
	}
	if got.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, sessionID)
	}
	if len(got.Strengths) != 1 {
		t.Errorf("Strengths len = %d, want 1", len(got.Strengths))
	}
	if len(got.Weaknesses) != 1 {
		t.Errorf("Weaknesses len = %d, want 1", len(got.Weaknesses))
	}
	if len(got.ImprovementSuggestions) != 1 {
		t.Errorf("ImprovementSuggestions len = %d, want 1", len(got.ImprovementSuggestions))
	}
}

func TestSessionStore_GetSummary_NotFound(t *testing.T) {
	store := &SessionStore{db: testDB}

	got, err := store.GetSummary("nonexistent-session-id")
	if err == nil {
		t.Fatal("GetSummary(nonexistent) expected error, got nil")
	}
	if got != nil {
		t.Errorf("GetSummary(nonexistent) expected nil, got %+v", got)
	}
}
