package word_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

// --- fakes ---

type fakeWordStore struct {
	words   map[int64]*word.Word
	records map[string]*word.WordRecord // key: "userID:wordID"
}

func newFakeWordStore() *fakeWordStore {
	return &fakeWordStore{
		words:   make(map[int64]*word.Word),
		records: make(map[string]*word.WordRecord),
	}
}

func recordKey(userID, wordID int64) string {
	return fmt.Sprintf("%d:%d", userID, wordID)
}

func (f *fakeWordStore) GetByID(id int64) (*word.Word, error) {
	w, ok := f.words[id]
	if !ok {
		return nil, errors.New("word not found")
	}
	return w, nil
}

func (f *fakeWordStore) ListByLevel(level word.JLPTLevel) ([]word.Word, error) {
	var result []word.Word
	for _, w := range f.words {
		if w.JLPTLevel == level {
			result = append(result, *w)
		}
	}
	return result, nil
}

func (f *fakeWordStore) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	r, ok := f.records[recordKey(userID, wordID)]
	if !ok {
		return nil, nil // no record yet is normal
	}
	return r, nil
}

func (f *fakeWordStore) ListDueRecords(userID int64) ([]word.WordRecord, error) {
	var result []word.WordRecord
	now := time.Now()
	for _, r := range f.records {
		if r.UserID == userID && !r.NextReviewAt.After(now) {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (f *fakeWordStore) UpsertRecord(r word.WordRecord) error {
	key := recordKey(r.UserID, r.WordID)
	cp := r
	f.records[key] = &cp
	return nil
}

func (f *fakeWordStore) BookmarkWord(userID, wordID int64) error {
	// no-op for fake
	return nil
}

// seedWord adds a word to the fake store.
func (f *fakeWordStore) seedWord(w word.Word) {
	f.words[w.ID] = &w
}

// --- tests ---

func TestWordService_GetReviewQueue_NewUser(t *testing.T) {
	store := newFakeWordStore()
	store.seedWord(word.Word{ID: 1, KanjiForm: "一", JLPTLevel: word.LevelN5})
	store.seedWord(word.Word{ID: 2, KanjiForm: "二", JLPTLevel: word.LevelN5})

	svc := word.NewWordService(store)
	cards, err := svc.GetReviewQueue(1, word.LevelN5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards) != 2 {
		t.Errorf("expected 2 cards, got %d", len(cards))
	}
	for _, c := range cards {
		if !c.IsNew {
			t.Errorf("expected IsNew=true for new user, got false for word %d", c.Word.ID)
		}
	}
}

func TestWordService_GetReviewQueue_DueRecords(t *testing.T) {
	store := newFakeWordStore()
	store.seedWord(word.Word{ID: 1, KanjiForm: "一", JLPTLevel: word.LevelN5})
	store.seedWord(word.Word{ID: 2, KanjiForm: "二", JLPTLevel: word.LevelN5})

	// Pre-seed a due record for word 1
	_ = store.UpsertRecord(word.WordRecord{
		UserID:       1,
		WordID:       1,
		MasteryLevel: 1,
		EaseFactor:   2.5,
		Interval:     1,
		NextReviewAt: time.Now().Add(-time.Hour), // already due
	})

	svc := word.NewWordService(store)
	cards, err := svc.GetReviewQueue(1, word.LevelN5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 due card + 1 new card (word 2 has no record)
	dueCount, newCount := 0, 0
	for _, c := range cards {
		if c.IsNew {
			newCount++
		} else {
			dueCount++
		}
	}
	if dueCount != 1 {
		t.Errorf("expected 1 due card, got %d", dueCount)
	}
	if newCount != 1 {
		t.Errorf("expected 1 new card, got %d", newCount)
	}
}

func TestWordService_SubmitRating(t *testing.T) {
	store := newFakeWordStore()
	store.seedWord(word.Word{ID: 1, KanjiForm: "一", JLPTLevel: word.LevelN5})

	svc := word.NewWordService(store)

	tests := []struct {
		name    string
		rating  word.ReviewRating
		wantErr bool
	}{
		{"easy rating", word.RatingEasy, false},
		{"normal rating", word.RatingNormal, false},
		{"hard rating", word.RatingHard, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.SubmitRating(1, 1, tt.rating)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitRating() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// After three ratings, record should exist
	r, err := store.GetRecord(1, 1)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if r == nil {
		t.Fatal("expected record to exist after SubmitRating")
	}
}

func TestWordService_SubmitRating_WordNotFound(t *testing.T) {
	store := newFakeWordStore()
	svc := word.NewWordService(store)

	err := svc.SubmitRating(1, 999, word.RatingEasy)
	if err == nil {
		t.Error("expected error for nonexistent word")
	}
}

func TestWordService_Bookmark(t *testing.T) {
	store := newFakeWordStore()
	store.seedWord(word.Word{ID: 1, KanjiForm: "一", JLPTLevel: word.LevelN5})
	svc := word.NewWordService(store)

	if err := svc.Bookmark(1, 1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
