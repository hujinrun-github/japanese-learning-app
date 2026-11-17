package word_test

import (
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

// batchQueueStore is a queue-only fake. It records how many data-access calls
// the service makes, so the tests below can lock in "one batch, no per-word
// lookups" — the property that keeps PostgreSQL deployments fast.
type batchQueueStore struct {
	candidates     []word.ReviewCandidate
	queueCalls     int
	getRecordCalls int
	getByIDCalls   int
	gotDueLimit    int
	gotNewLimit    int
	queueErr       error
}

func (f *batchQueueStore) GetByID(id int64) (*word.Word, error) {
	f.getByIDCalls++
	return nil, errors.New("word not found")
}

func (f *batchQueueStore) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	f.getRecordCalls++
	return nil, nil
}

func (f *batchQueueStore) ListReviewCandidates(userID int64, level word.JLPTLevel, dueLimit, newLimit int) ([]word.ReviewCandidate, error) {
	f.queueCalls++
	f.gotDueLimit, f.gotNewLimit = dueLimit, newLimit
	return f.candidates, f.queueErr
}

func (f *batchQueueStore) UpsertRecord(r word.WordRecord) error { return nil }

func (f *batchQueueStore) BookmarkWord(userID, wordID int64) error { return nil }

func newCandidate(id int64, due bool) word.ReviewCandidate {
	c := word.ReviewCandidate{
		Word: word.Word{ID: id, KanjiForm: "語", JLPTLevel: word.LevelN5},
	}
	if due {
		c.Record = &word.WordRecord{
			UserID:       1,
			WordID:       id,
			MasteryLevel: 1,
			EaseFactor:   2.5,
			NextReviewAt: time.Now().Add(-time.Hour),
			UpdatedAt:    time.Now().AddDate(0, 0, -3),
		}
	}
	return c
}

// TestWordService_GetReviewQueue_UsesSingleBatch locks in that a queue request
// performs exactly one batch read: on PostgreSQL, loading a whole JLPT level
// plus one GetRecord per candidate costs seconds of serial round trips.
func TestWordService_GetReviewQueue_UsesSingleBatch(t *testing.T) {
	tests := []struct {
		name         string
		limit        int
		candidates   []word.ReviewCandidate
		wantIDs      []int64
		wantNewFlags []bool
		wantDueLimit int
		wantNewLimit int
	}{
		{
			name:  "到期词与新词混合",
			limit: 4,
			candidates: []word.ReviewCandidate{
				newCandidate(1, true), newCandidate(2, true),
				newCandidate(3, false), newCandidate(4, false), newCandidate(5, false),
			},
			wantIDs:      []int64{1, 2, 3, 4},
			wantNewFlags: []bool{false, false, true, true},
			wantDueLimit: 2, wantNewLimit: 4,
		},
		{
			name:         "到期词超过半轮则截断",
			limit:        4,
			candidates:   []word.ReviewCandidate{newCandidate(1, true), newCandidate(2, true), newCandidate(3, true), newCandidate(4, false), newCandidate(5, false), newCandidate(6, false)},
			wantIDs:      []int64{1, 2, 4, 5},
			wantNewFlags: []bool{false, false, true, true},
			wantDueLimit: 2, wantNewLimit: 4,
		},
		{
			name:         "limit 1 仍保留一张到期词",
			limit:        1,
			candidates:   []word.ReviewCandidate{newCandidate(1, true), newCandidate(2, false)},
			wantIDs:      []int64{1},
			wantNewFlags: []bool{false},
			wantDueLimit: 1, wantNewLimit: 1,
		},
		{
			name:         "没有到期词时全是新词",
			limit:        2,
			candidates:   []word.ReviewCandidate{newCandidate(7, false), newCandidate(8, false)},
			wantIDs:      []int64{7, 8},
			wantNewFlags: []bool{true, true},
			wantDueLimit: 1, wantNewLimit: 2,
		},
		{
			name:  "今天已复习的到期词跳过",
			limit: 3,
			candidates: func() []word.ReviewCandidate {
				c := newCandidate(1, true)
				c.Record.UpdatedAt = time.Now()
				return []word.ReviewCandidate{c, newCandidate(2, false), newCandidate(3, false)}
			}(),
			wantIDs:      []int64{2, 3},
			wantNewFlags: []bool{true, true},
			wantDueLimit: 1, wantNewLimit: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &batchQueueStore{candidates: tt.candidates}
			cards, err := word.NewWordService(store).GetReviewQueue(1, word.LevelN5, tt.limit)
			if err != nil {
				t.Fatalf("GetReviewQueue error: %v", err)
			}
			if store.queueCalls != 1 {
				t.Errorf("ListReviewCandidates calls = %d, want 1", store.queueCalls)
			}
			if store.getRecordCalls != 0 {
				t.Errorf("GetRecord calls = %d, want 0 (per-word round trips)", store.getRecordCalls)
			}
			if store.getByIDCalls != 0 {
				t.Errorf("GetByID calls = %d, want 0 (per-word round trips)", store.getByIDCalls)
			}
			if store.gotDueLimit != tt.wantDueLimit {
				t.Errorf("dueLimit = %d, want %d", store.gotDueLimit, tt.wantDueLimit)
			}
			if store.gotNewLimit != tt.wantNewLimit {
				t.Errorf("newLimit = %d, want %d", store.gotNewLimit, tt.wantNewLimit)
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

// TestWordService_GetReviewQueue_PropagatesBatchError checks errors are wrapped
// and logged rather than silently producing an empty queue.
func TestWordService_GetReviewQueue_PropagatesBatchError(t *testing.T) {
	store := &batchQueueStore{queueErr: errors.New("db down")}
	_, err := word.NewWordService(store).GetReviewQueue(1, word.LevelN5, 20)
	if err == nil {
		t.Fatal("expected an error when the batch query fails")
	}
	if !errors.Is(err, store.queueErr) {
		t.Errorf("error = %v, want it to wrap the store error", err)
	}
}
