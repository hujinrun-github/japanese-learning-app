package speaking_test

import (
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
)

// --- stub scorer ---

type stubScorer struct {
	score int
	err   error
}

func (s *stubScorer) Score(_, _ []byte) (speaking.ScoreResult, error) {
	if s.err != nil {
		return speaking.ScoreResult{}, s.err
	}
	return speaking.ScoreResult{OverallScore: s.score}, nil
}

// --- fake store ---

type fakeSpeakingStore struct {
	records []speaking.SpeakingRecord
	nextID  int64
}

func (f *fakeSpeakingStore) SaveRecord(r speaking.SpeakingRecord) error {
	f.nextID++
	r.ID = f.nextID
	f.records = append(f.records, r)
	return nil
}

func (f *fakeSpeakingStore) ListRecords(userID int64) ([]speaking.SpeakingRecord, error) {
	var result []speaking.SpeakingRecord
	for _, r := range f.records {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (f *fakeSpeakingStore) GetRecord(id int64) (*speaking.SpeakingRecord, error) {
	for _, r := range f.records {
		if r.ID == id {
			cp := r
			return &cp, nil
		}
	}
	return nil, errors.New("record not found")
}

// --- tests ---

func TestSpeakingService_Practice(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{score: 85}
	svc := speaking.NewSpeakingService(store, scorer)

	result, err := svc.Practice(1, speaking.PracticeTypeShadow, 1, []byte("ref"), []byte("user"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallScore != 85 {
		t.Errorf("expected score 85, got %d", result.OverallScore)
	}
	if len(store.records) != 1 {
		t.Errorf("expected 1 record saved, got %d", len(store.records))
	}
	if store.records[0].Score != 85 {
		t.Errorf("expected saved score 85, got %d", store.records[0].Score)
	}
	if store.records[0].UserID != 1 {
		t.Errorf("expected UserID 1, got %d", store.records[0].UserID)
	}
}

func TestSpeakingService_Practice_ScorerError(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{err: errors.New("scorer failed")}
	svc := speaking.NewSpeakingService(store, scorer)

	_, err := svc.Practice(1, speaking.PracticeTypeShadow, 1, []byte("ref"), []byte("user"))
	if err == nil {
		t.Error("expected error when scorer fails")
	}
}

func TestSpeakingService_ListRecords(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{score: 70}
	svc := speaking.NewSpeakingService(store, scorer)

	// Create some records for user 1 and user 2
	_, _ = svc.Practice(1, speaking.PracticeTypeShadow, 1, []byte("r"), []byte("u"))
	_, _ = svc.Practice(1, speaking.PracticeTypeFree, 2, []byte("r"), []byte("u"))
	_, _ = svc.Practice(2, speaking.PracticeTypeShadow, 1, []byte("r"), []byte("u"))

	records, err := svc.ListRecords(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records for user 1, got %d", len(records))
	}
	for _, r := range records {
		if r.UserID != 1 {
			t.Errorf("expected UserID 1, got %d", r.UserID)
		}
	}
}

func TestSpeakingService_Practice_StoresPracticeType(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{score: 60}
	svc := speaking.NewSpeakingService(store, scorer)

	tests := []speaking.PracticeType{speaking.PracticeTypeShadow, speaking.PracticeTypeFree}
	for _, pt := range tests {
		_, err := svc.Practice(1, pt, 1, []byte("r"), []byte("u"))
		if err != nil {
			t.Fatalf("Practice(%s) unexpected error: %v", pt, err)
		}
	}
	if len(store.records) != 2 {
		t.Errorf("expected 2 records, got %d", len(store.records))
	}
	if store.records[0].Type != speaking.PracticeTypeShadow {
		t.Errorf("expected shadow type, got %s", store.records[0].Type)
	}
	if store.records[1].Type != speaking.PracticeTypeFree {
		t.Errorf("expected free type, got %s", store.records[1].Type)
	}
	_ = time.Now() // just to use the import
}
