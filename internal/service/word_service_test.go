package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeWordStore struct {
	words      map[int64]*word.Word
	records    map[string]*word.WordRecord // key: "userID:wordID"
	bookmarks  []string
}

func (f *fakeWordStore) GetByID(id int64) (*word.Word, error) {
	w, ok := f.words[id]
	if !ok {
		return nil, fmt.Errorf("word %d: %w", id, errors.New("not found"))
	}
	return w, nil
}

func (f *fakeWordStore) ListByLevel(level word.JLPTLevel, page, size int) ([]word.Word, int, error) {
	var result []word.Word
	for _, w := range f.words {
		if w.JLPTLevel == level {
			result = append(result, *w)
		}
	}
	return result, len(result), nil
}

func (f *fakeWordStore) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	key := fmt.Sprintf("%d:%d", userID, wordID)
	r, ok := f.records[key]
	if !ok {
		return nil, fmt.Errorf("record not found: %w", errors.New("not found"))
	}
	return r, nil
}

func (f *fakeWordStore) ListDueRecords(userID int64, limit int) ([]word.WordRecord, error) {
	var result []word.WordRecord
	for _, r := range f.records {
		if r.UserID == userID && !r.NextReviewAt.After(time.Now()) {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (f *fakeWordStore) UpsertRecord(r word.WordRecord) error {
	key := fmt.Sprintf("%d:%d", r.UserID, r.WordID)
	cp := r
	f.records[key] = &cp
	return nil
}

func (f *fakeWordStore) BookmarkWord(userID, wordID int64) error {
	f.bookmarks = append(f.bookmarks, fmt.Sprintf("%d:%d", userID, wordID))
	return nil
}

// --- tests ---

func TestWordService_GetDueQueue(t *testing.T) {
	now := time.Now()
	store := &fakeWordStore{
		words: map[int64]*word.Word{
			1: {ID: 1, KanjiForm: "勉強", Reading: "べんきょう", JLPTLevel: word.LevelN5},
			2: {ID: 2, KanjiForm: "先生", Reading: "せんせい", JLPTLevel: word.LevelN5},
		},
		records: map[string]*word.WordRecord{
			"1:1": {ID: 1, UserID: 1, WordID: 1, MasteryLevel: 1, NextReviewAt: now.Add(-time.Hour), EaseFactor: 2.5, Interval: 1},
			"1:2": {ID: 2, UserID: 1, WordID: 2, MasteryLevel: 0, NextReviewAt: now.Add(time.Hour), EaseFactor: 2.5, Interval: 1},
		},
	}

	svc := service.NewWordService(store)
	cards, err := svc.GetDueQueue(1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards) != 1 {
		t.Errorf("expected 1 due card, got %d", len(cards))
	}
	if cards[0].Word.ID != 1 {
		t.Errorf("expected word id 1, got %d", cards[0].Word.ID)
	}
	if cards[0].IsNew {
		t.Error("existing record should not be marked as new")
	}
}

func TestWordService_SubmitReview_Easy(t *testing.T) {
	now := time.Now()
	store := &fakeWordStore{
		words: map[int64]*word.Word{
			1: {ID: 1, KanjiForm: "勉強", Reading: "べんきょう", JLPTLevel: word.LevelN5},
		},
		records: map[string]*word.WordRecord{
			"1:1": {
				ID: 1, UserID: 1, WordID: 1,
				MasteryLevel: 1, EaseFactor: 2.5, Interval: 1,
				NextReviewAt: now.Add(-time.Hour),
			},
		},
	}

	svc := service.NewWordService(store)
	if err := svc.SubmitReview(1, 1, word.RatingEasy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := store.records["1:1"]
	if r.MasteryLevel <= 1 {
		t.Errorf("mastery should have increased, got %d", r.MasteryLevel)
	}
	if r.Interval <= 1 {
		t.Errorf("interval should have grown, got %d", r.Interval)
	}
	if !r.NextReviewAt.After(now) {
		t.Error("next review should be in the future")
	}
}

func TestWordService_SubmitReview_Hard(t *testing.T) {
	now := time.Now()
	store := &fakeWordStore{
		words: map[int64]*word.Word{
			1: {ID: 1, KanjiForm: "勉強", JLPTLevel: word.LevelN5},
		},
		records: map[string]*word.WordRecord{
			"1:1": {
				ID: 1, UserID: 1, WordID: 1,
				MasteryLevel: 3, EaseFactor: 2.5, Interval: 4,
				NextReviewAt: now.Add(-time.Hour),
			},
		},
	}

	svc := service.NewWordService(store)
	if err := svc.SubmitReview(1, 1, word.RatingHard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := store.records["1:1"]
	if r.MasteryLevel != 0 {
		t.Errorf("hard rating should reset mastery to 0, got %d", r.MasteryLevel)
	}
	if r.Interval != 1 {
		t.Errorf("hard rating should reset interval to 1, got %d", r.Interval)
	}
}

func TestWordService_SubmitReview_NewWord(t *testing.T) {
	store := &fakeWordStore{
		words: map[int64]*word.Word{
			1: {ID: 1, KanjiForm: "勉強", JLPTLevel: word.LevelN5},
		},
		records: map[string]*word.WordRecord{},
	}

	svc := service.NewWordService(store)
	if err := svc.SubmitReview(1, 1, word.RatingNormal); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := store.records["1:1"]
	if r == nil {
		t.Fatal("expected record to be created")
	}
	if r.UserID != 1 || r.WordID != 1 {
		t.Errorf("record has wrong ids: user=%d word=%d", r.UserID, r.WordID)
	}
}

func TestWordService_BookmarkWord(t *testing.T) {
	store := &fakeWordStore{
		words:   map[int64]*word.Word{},
		records: map[string]*word.WordRecord{},
	}
	svc := service.NewWordService(store)
	if err := svc.BookmarkWord(1, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.bookmarks) != 1 || store.bookmarks[0] != "1:5" {
		t.Error("expected bookmark to be recorded")
	}
}
