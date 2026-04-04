package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeSpeakingStore struct {
	records map[int64]*speaking.SpeakingRecord
	nextID  int64
}

func (f *fakeSpeakingStore) SaveRecord(r speaking.SpeakingRecord) error {
	f.nextID++
	r.ID = f.nextID
	f.records[r.ID] = &r
	return nil
}

func (f *fakeSpeakingStore) ListRecords(userID int64) ([]speaking.SpeakingRecord, error) {
	var result []speaking.SpeakingRecord
	for _, r := range f.records {
		if r.UserID == userID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (f *fakeSpeakingStore) GetRecord(id int64) (*speaking.SpeakingRecord, error) {
	r, ok := f.records[id]
	if !ok {
		return nil, fmt.Errorf("speaking_record %d: %w", id, errors.New("not found"))
	}
	return r, nil
}

// --- tests ---

func TestSpeakingService_SaveRecord(t *testing.T) {
	store := &fakeSpeakingStore{records: map[int64]*speaking.SpeakingRecord{}}
	svc := service.NewSpeakingService(store)

	r := speaking.SpeakingRecord{
		UserID:      1,
		Type:        speaking.PracticeTypeShadow,
		MaterialID:  10,
		Score:       85,
		AudioRef:    "audio/user1/session1.wav",
		PracticedAt: time.Now(),
	}
	if err := svc.SaveRecord(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.records) != 1 {
		t.Errorf("expected 1 record, got %d", len(store.records))
	}
}

func TestSpeakingService_ListRecords(t *testing.T) {
	now := time.Now()
	store := &fakeSpeakingStore{
		records: map[int64]*speaking.SpeakingRecord{
			1: {ID: 1, UserID: 1, Type: speaking.PracticeTypeShadow, MaterialID: 10, Score: 80, PracticedAt: now.Add(-time.Hour)},
			2: {ID: 2, UserID: 1, Type: speaking.PracticeTypeFree, MaterialID: 11, Score: 90, PracticedAt: now},
			3: {ID: 3, UserID: 2, Type: speaking.PracticeTypeShadow, MaterialID: 10, Score: 70, PracticedAt: now},
		},
	}

	svc := service.NewSpeakingService(store)
	records, err := svc.ListRecords(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records for user 1, got %d", len(records))
	}
}

func TestSpeakingService_GetRecord(t *testing.T) {
	store := &fakeSpeakingStore{
		records: map[int64]*speaking.SpeakingRecord{
			5: {ID: 5, UserID: 1, Type: speaking.PracticeTypeFree, MaterialID: 3, Score: 75},
		},
	}

	svc := service.NewSpeakingService(store)
	r, err := svc.GetRecord(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ID != 5 {
		t.Errorf("expected id 5, got %d", r.ID)
	}
}

func TestSpeakingService_GetRecord_NotFound(t *testing.T) {
	store := &fakeSpeakingStore{records: map[int64]*speaking.SpeakingRecord{}}
	svc := service.NewSpeakingService(store)
	_, err := svc.GetRecord(999)
	if err == nil {
		t.Fatal("expected error for nonexistent record")
	}
}
