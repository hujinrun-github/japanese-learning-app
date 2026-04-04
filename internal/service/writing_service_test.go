package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/writing"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeWritingStore struct {
	questions []writing.WritingQuestion
	records   map[int64]*writing.WritingRecord
	nextID    int64
}

func (f *fakeWritingStore) GetDailyQueue(userID int64) ([]writing.WritingQuestion, error) {
	return f.questions, nil
}

func (f *fakeWritingStore) SaveRecord(r writing.WritingRecord) error {
	f.nextID++
	r.ID = f.nextID
	f.records[r.ID] = &r
	return nil
}

func (f *fakeWritingStore) ListRecords(userID int64) ([]writing.WritingRecord, error) {
	var result []writing.WritingRecord
	for _, r := range f.records {
		if r.UserID == userID {
			result = append(result, *r)
		}
	}
	return result, nil
}

// --- tests ---

func TestWritingService_GetDailyQueue(t *testing.T) {
	store := &fakeWritingStore{
		questions: []writing.WritingQuestion{
			{ID: 1, Type: writing.WritingTypeInput, Prompt: "にほんご"},
			{ID: 2, Type: writing.WritingTypeSentence, Prompt: "我可以吃这个吗？"},
		},
		records: map[int64]*writing.WritingRecord{},
	}

	svc := service.NewWritingService(store)
	questions, err := svc.GetDailyQueue(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(questions))
	}
	// ExpectedAnswer must NOT be exposed via the queue
	for _, q := range questions {
		if q.ExpectedAnswer != "" {
			t.Errorf("ExpectedAnswer should be hidden in daily queue, question id=%d", q.ID)
		}
	}
}

func TestWritingService_SaveRecord(t *testing.T) {
	store := &fakeWritingStore{
		questions: nil,
		records:   map[int64]*writing.WritingRecord{},
	}

	svc := service.NewWritingService(store)
	r := writing.WritingRecord{
		UserID:     1,
		Type:       writing.WritingTypeInput,
		Question:   "にほんご",
		UserAnswer: "にほんご",
		Score:      100,
		PracticedAt: time.Now(),
	}
	if err := svc.SaveRecord(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.records) != 1 {
		t.Errorf("expected 1 record, got %d", len(store.records))
	}
}

func TestWritingService_ListRecords(t *testing.T) {
	now := time.Now()
	store := &fakeWritingStore{
		questions: nil,
		records: map[int64]*writing.WritingRecord{
			1: {ID: 1, UserID: 1, Type: writing.WritingTypeInput, Score: 100, PracticedAt: now.Add(-time.Hour)},
			2: {ID: 2, UserID: 1, Type: writing.WritingTypeSentence, Score: 80, PracticedAt: now},
			3: {ID: 3, UserID: 2, Type: writing.WritingTypeInput, Score: 90, PracticedAt: now},
		},
	}

	svc := service.NewWritingService(store)
	records, err := svc.ListRecords(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records for user 1, got %d", len(records))
	}
}

// ensure unused import is used
var _ = fmt.Sprintf
var _ = errors.New
