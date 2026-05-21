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
	records   []speaking.SpeakingRecord
	materials []speaking.SpeakingMaterial
	nextID    int64
}

func (f *fakeSpeakingStore) SaveRecord(r speaking.SpeakingRecord) (int64, error) {
	f.nextID++
	r.ID = f.nextID
	f.records = append(f.records, r)
	return f.nextID, nil
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

// --- materials methods for fake store ---

func (f *fakeSpeakingStore) ListMaterials(practiceType, level string) ([]speaking.SpeakingMaterial, error) {
	var result []speaking.SpeakingMaterial
	for _, m := range f.materials {
		if (practiceType == "" || m.Type == practiceType) &&
			(level == "" || m.JLPTLevel == level) {
			result = append(result, m)
		}
	}
	return result, nil
}

func (f *fakeSpeakingStore) GetMaterialByID(id int64) (*speaking.SpeakingMaterial, error) {
	for _, m := range f.materials {
		if m.ID == id {
			cp := m
			return &cp, nil
		}
	}
	return nil, nil
}

// --- material tests ---

func TestSpeakingService_ListMaterials(t *testing.T) {
	store := &fakeSpeakingStore{
		materials: []speaking.SpeakingMaterial{
			{ID: 1, Type: "shadow", Title: "M1", JLPTLevel: "N5"},
			{ID: 2, Type: "free", Title: "M2", JLPTLevel: "N4"},
			{ID: 3, Type: "shadow", Title: "M3", JLPTLevel: "N5"},
		},
	}
	scorer := &stubScorer{score: 70}
	svc := speaking.NewSpeakingService(store, scorer)

	materials, err := svc.ListMaterials("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(materials) != 3 {
		t.Errorf("expected 3 materials, got %d", len(materials))
	}
}

func TestSpeakingService_ListMaterials_FilterByType(t *testing.T) {
	store := &fakeSpeakingStore{
		materials: []speaking.SpeakingMaterial{
			{ID: 1, Type: "shadow", Title: "S1", JLPTLevel: "N5"},
			{ID: 2, Type: "free", Title: "F1", JLPTLevel: "N5"},
			{ID: 3, Type: "shadow", Title: "S2", JLPTLevel: "N4"},
		},
	}
	scorer := &stubScorer{score: 70}
	svc := speaking.NewSpeakingService(store, scorer)

	materials, err := svc.ListMaterials("free", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(materials) != 1 {
		t.Errorf("expected 1 free material, got %d", len(materials))
	}
	if materials[0].Title != "F1" {
		t.Errorf("expected F1, got %s", materials[0].Title)
	}
}

func TestSpeakingService_GetMaterialByID(t *testing.T) {
	store := &fakeSpeakingStore{
		materials: []speaking.SpeakingMaterial{
			{ID: 1, Type: "shadow", Title: "Test", JLPTLevel: "N5"},
			{ID: 2, Type: "free", Title: "Other", JLPTLevel: "N4"},
		},
	}
	scorer := &stubScorer{score: 70}
	svc := speaking.NewSpeakingService(store, scorer)

	got, err := svc.GetMaterialByID(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil material")
	}
	if got.Title != "Test" {
		t.Errorf("expected Test, got %s", got.Title)
	}
}

func TestSpeakingService_GetMaterialByID_NotFound(t *testing.T) {
	store := &fakeSpeakingStore{
		materials: []speaking.SpeakingMaterial{
			{ID: 1, Type: "shadow", Title: "Test", JLPTLevel: "N5"},
		},
	}
	scorer := &stubScorer{score: 70}
	svc := speaking.NewSpeakingService(store, scorer)

	got, err := svc.GetMaterialByID(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent material, got %+v", got)
	}
}

func TestSpeakingService_RecordPractice(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{score: 0}
	svc := speaking.NewSpeakingService(store, scorer)

	rec, err := svc.RecordPractice(1, speaking.PracticeTypeShadow, 1, 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID == 0 {
		t.Error("expected non-zero record ID")
	}
	if rec.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", rec.UserID)
	}
	if rec.Type != speaking.PracticeTypeShadow {
		t.Errorf("expected shadow type, got %s", rec.Type)
	}
	if rec.MaterialID != 1 {
		t.Errorf("expected MaterialID 1, got %d", rec.MaterialID)
	}
	if rec.Score != 80 {
		t.Errorf("expected Score 80, got %d", rec.Score)
	}
}

func TestSpeakingService_RecordPractice_SavesRecord(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{score: 0}
	svc := speaking.NewSpeakingService(store, scorer)

	_, err := svc.RecordPractice(1, speaking.PracticeTypeFree, 5, 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records, err := svc.ListRecords(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Score != 90 {
		t.Errorf("expected Score 90, got %d", records[0].Score)
	}
}

func TestSpeakingService_RecordPractice_InvalidScore(t *testing.T) {
	store := &fakeSpeakingStore{}
	scorer := &stubScorer{score: 0}
	svc := speaking.NewSpeakingService(store, scorer)

	// invalid scores should still be saved (validation is at handler level)
	rec, err := svc.RecordPractice(1, speaking.PracticeTypeShadow, 1, 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Score != 150 {
		t.Errorf("expected Score 150, got %d", rec.Score)
	}
}
