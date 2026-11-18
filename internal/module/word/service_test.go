package word_test

import (
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

// --- fakes ---

type fakeWordStore struct {
	words                     map[int64]*word.Word
	records                   map[string]*word.WordRecord // key: "userID:wordID"
	getByIDCalls              int
	getRecordCalls            int
	listByLevelCalls          int
	listReviewCandidatesCalls int
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
	f.getByIDCalls++
	w, ok := f.words[id]
	if !ok {
		return nil, errors.New("word not found")
	}
	return w, nil
}

func (f *fakeWordStore) ListByLevel(level word.JLPTLevel) ([]word.Word, error) {
	f.listByLevelCalls++
	var result []word.Word
	for _, w := range f.words {
		if w.JLPTLevel == level {
			result = append(result, *w)
		}
	}
	// 真实 store 按 id 升序返回；fake 也排序，断言才可预测。
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (f *fakeWordStore) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	f.getRecordCalls++
	r, ok := f.records[recordKey(userID, wordID)]
	if !ok {
		return nil, nil // no record yet is normal
	}
	return r, nil
}

// ListReviewCandidates mirrors the SQL the real stores run: at most dueLimit
// overdue words (excluding ones reviewed today) then at most newLimit words
// with no record, all scoped to the requested level.
func (f *fakeWordStore) ListReviewCandidates(userID int64, level word.JLPTLevel, dueLimit, newLimit int) ([]word.ReviewCandidate, error) {
	f.listReviewCandidatesCalls++
	now := time.Now()

	var due []word.WordRecord
	for _, r := range f.records {
		w, ok := f.words[r.WordID]
		if !ok || w.JLPTLevel != level || r.UserID != userID || r.NextReviewAt.After(now) {
			continue
		}
		if r.UpdatedAt.Year() == now.Year() && r.UpdatedAt.YearDay() == now.YearDay() {
			continue
		}
		due = append(due, *r)
	}
	sort.Slice(due, func(i, j int) bool { return due[i].NextReviewAt.Before(due[j].NextReviewAt) })
	if len(due) > dueLimit {
		due = due[:dueLimit]
	}

	result := make([]word.ReviewCandidate, 0, dueLimit+newLimit)
	for _, r := range due {
		rec := r
		result = append(result, word.ReviewCandidate{Word: *f.words[r.WordID], Record: &rec})
	}

	var fresh []word.Word
	for _, w := range f.words {
		if w.JLPTLevel != level {
			continue
		}
		if _, ok := f.records[recordKey(userID, w.ID)]; ok {
			continue
		}
		fresh = append(fresh, *w)
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].ID < fresh[j].ID })
	if len(fresh) > newLimit {
		fresh = fresh[:newLimit]
	}
	for _, w := range fresh {
		result = append(result, word.ReviewCandidate{Word: w})
	}
	return result, nil
}

func (f *fakeWordStore) ListDueRecords(userID int64) ([]word.WordRecord, error) {
	var result []word.WordRecord
	now := time.Now()
	for _, r := range f.records {
		if r.UserID == userID && !r.NextReviewAt.After(now) {
			result = append(result, *r)
		}
	}
	// 真实 store 按 next_review_at 升序返回。
	sort.Slice(result, func(i, j int) bool { return result[i].NextReviewAt.Before(result[j].NextReviewAt) })
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
	cards, err := svc.GetReviewQueue(1, word.LevelN5, 20)
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
	cards, err := svc.GetReviewQueue(1, word.LevelN5, 20)
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

// TestWordService_MarkSeen 验证「卡片被看过」就会留下学习记录，
// 这样没点评分就离开页面也不会让同一个词作为新词反复出现。
func TestWordService_MarkSeen(t *testing.T) {
	tests := []struct {
		name        string
		wordID      int64
		seed        *word.WordRecord
		wantErr     bool
		wantCreated bool
		wantMastery int
		wantEF      float64
	}{
		{"新词创建保底记录", 1, nil, false, true, 0, 2.5},
		{
			"已有记录保持原样",
			1,
			&word.WordRecord{UserID: 1, WordID: 1, MasteryLevel: 3, EaseFactor: 2.8, NextReviewAt: time.Now().Add(-time.Hour)},
			false, true, 3, 2.8,
		},
		{
			"其它用户的记录不影响当前用户",
			1,
			&word.WordRecord{UserID: 99, WordID: 1, MasteryLevel: 4, EaseFactor: 2.9},
			false, true, 0, 2.5,
		},
		{"词不存在返回错误", 999, nil, true, false, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeWordStore()
			store.seedWord(word.Word{ID: 1, KanjiForm: "一", JLPTLevel: word.LevelN5})
			if tt.seed != nil {
				_ = store.UpsertRecord(*tt.seed)
			}

			err := word.NewWordService(store).MarkSeen(1, tt.wordID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarkSeen error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			got, err := store.GetRecord(1, tt.wordID)
			if err != nil {
				t.Fatalf("GetRecord error: %v", err)
			}
			if got == nil {
				t.Fatal("MarkSeen should leave a record behind")
			}
			if got.MasteryLevel != tt.wantMastery {
				t.Errorf("MasteryLevel = %d, want %d", got.MasteryLevel, tt.wantMastery)
			}
			if got.EaseFactor != tt.wantEF {
				t.Errorf("EaseFactor = %v, want %v", got.EaseFactor, tt.wantEF)
			}
			if tt.wantCreated && tt.seed == nil {
				if got.Interval != 1 {
					t.Errorf("Interval = %d, want 1", got.Interval)
				}
				delta := time.Until(got.NextReviewAt)
				if delta < 23*time.Hour || delta > 25*time.Hour {
					t.Errorf("NextReviewAt = %v, want about 24h later (delta=%v)", got.NextReviewAt, delta)
				}
				if len(got.ReviewHistory) != 0 {
					t.Errorf("ReviewHistory = %v, want empty for a provisional record", got.ReviewHistory)
				}
			}
		})
	}
}

// TestWordService_GetReviewQueue_NoRepeat 验证队列不会把「今天学过的词」
// 又推回给用户，且到期词不会占满整轮、新词能排进来。
func TestWordService_GetReviewQueue_NoRepeat(t *testing.T) {
	tests := []struct {
		name         string
		level        word.JLPTLevel
		limit        int
		allReviewed  bool
		wantIDs      []int64
		wantNewFlags []bool
	}{
		{
			"到期词限量并用新词补足", word.LevelN5, 4, false,
			[]int64{1, 4, 5, 6}, []bool{false, true, true, true},
		},
		{
			"小 limit 仍保留一张到期词", word.LevelN5, 2, false,
			[]int64{1, 4}, []bool{false, true},
		},
		{
			"limit 1 只给到期词", word.LevelN5, 1, false,
			[]int64{1}, []bool{false},
		},
		{
			"今天已复习的到期词全部跳过", word.LevelN5, 3, true,
			[]int64{4, 5, 6}, []bool{true, true, true},
		},
		{"其它等级无词", word.LevelN4, 4, false, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeWordStore()
			for id := int64(1); id <= 6; id++ {
				store.seedWord(word.Word{ID: id, KanjiForm: fmt.Sprintf("語%d", id), JLPTLevel: word.LevelN5})
			}
			old := time.Now().AddDate(0, 0, -3)
			today := time.Now()
			word1UpdatedAt := old
			if tt.allReviewed {
				word1UpdatedAt = today
			}
			records := []word.WordRecord{
				{UserID: 1, WordID: 1, MasteryLevel: 1, NextReviewAt: time.Now().Add(-2 * time.Hour), EaseFactor: 2.5, UpdatedAt: word1UpdatedAt},
				// word 2 is always reviewed today, so it must never reappear.
				{UserID: 1, WordID: 2, MasteryLevel: 1, NextReviewAt: time.Now().Add(-1 * time.Hour), EaseFactor: 2.5, UpdatedAt: today},
				{UserID: 1, WordID: 3, MasteryLevel: 2, NextReviewAt: time.Now().Add(7 * 24 * time.Hour), EaseFactor: 2.6, UpdatedAt: old},
			}
			for _, r := range records {
				_ = store.UpsertRecord(r)
			}

			cards, err := word.NewWordService(store).GetReviewQueue(1, tt.level, tt.limit)
			if err != nil {
				t.Fatalf("GetReviewQueue error: %v", err)
			}
			if len(cards) != len(tt.wantIDs) {
				t.Fatalf("len = %d, want %d (cards=%+v)", len(cards), len(tt.wantIDs), cards)
			}
			for i, c := range cards {
				if c.Word.ID != tt.wantIDs[i] {
					t.Errorf("cards[%d].Word.ID = %d, want %d", i, c.Word.ID, tt.wantIDs[i])
				}
				if c.IsNew != tt.wantNewFlags[i] {
					t.Errorf("cards[%d].IsNew = %v, want %v", i, c.IsNew, tt.wantNewFlags[i])
				}
			}
		})
	}
}

// TestWordService_GetReviewQueue_AvoidsPerWordLookups 锁定队列路径不做逐词
// GetByID：在 PostgreSQL 上每条 due record 单独查词 + 例句会变成上百次往返。
func TestWordService_GetReviewQueue_AvoidsPerWordLookups(t *testing.T) {
	tests := []struct {
		name       string
		level      word.JLPTLevel
		limit      int
		dueWordIDs []int64
		wantIDs    []int64
	}{
		{"到期词与新词混合", word.LevelN5, 4, []int64{1, 2}, []int64{1, 2, 3, 4}},
		{"没有到期词", word.LevelN5, 3, nil, []int64{1, 2, 3}},
		{"到期词属于其它等级", word.LevelN4, 3, []int64{1, 2}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeWordStore()
			for id := int64(1); id <= 5; id++ {
				store.seedWord(word.Word{ID: id, KanjiForm: fmt.Sprintf("語%d", id), JLPTLevel: word.LevelN5})
			}
			for i, id := range tt.dueWordIDs {
				_ = store.UpsertRecord(word.WordRecord{
					UserID:       1,
					WordID:       id,
					MasteryLevel: 1,
					// 先插入的到期更早，队列顺序应为 1, 2, ...
					NextReviewAt: time.Now().Add(-time.Duration(len(tt.dueWordIDs)-i) * time.Hour),
					EaseFactor:   2.5,
					UpdatedAt:    time.Now().AddDate(0, 0, -3),
				})
			}

			cards, err := word.NewWordService(store).GetReviewQueue(1, tt.level, tt.limit)
			if err != nil {
				t.Fatalf("GetReviewQueue error: %v", err)
			}
			if store.getByIDCalls != 0 {
				t.Errorf("GetByID calls = %d, want 0 (one per-word query per due record)", store.getByIDCalls)
			}
			if store.getRecordCalls != 0 {
				t.Errorf("GetRecord calls = %d, want 0 (one round trip per candidate)", store.getRecordCalls)
			}
			if store.listByLevelCalls != 0 {
				t.Errorf("ListByLevel calls = %d, want 0 (whole level must not be loaded)", store.listByLevelCalls)
			}
			if store.listReviewCandidatesCalls != 1 {
				t.Errorf("ListReviewCandidates calls = %d, want 1", store.listReviewCandidatesCalls)
			}
			if len(cards) != len(tt.wantIDs) {
				t.Fatalf("len = %d, want %d (cards=%+v)", len(cards), len(tt.wantIDs), cards)
			}
			for i, c := range cards {
				if c.Word.ID != tt.wantIDs[i] {
					t.Errorf("cards[%d].Word.ID = %d, want %d", i, c.Word.ID, tt.wantIDs[i])
				}
				if c.Word.KanjiForm == "" {
					t.Errorf("cards[%d] lost word content", i)
				}
			}
		})
	}
}

func (f *fakeWordStore) InsertRecordIfAbsent(r word.WordRecord) error {
	if _, ok := f.records[recordKey(r.UserID, r.WordID)]; ok {
		return nil
	}
	return f.UpsertRecord(r)
}
