package word

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
)

const testJWTSecret = "test-secret"

// fakeWordStore is the minimum WordStoreInterface needed to rate a word.
type fakeWordStore struct {
	records map[int64]WordRecord
}

func newFakeWordStore() *fakeWordStore {
	return &fakeWordStore{records: make(map[int64]WordRecord)}
}

func (f *fakeWordStore) GetByID(id int64) (*Word, error) {
	return &Word{ID: id, KanjiForm: "勉強", Reading: "べんきょう", Meaning: "学习", JLPTLevel: LevelN5}, nil
}

func (f *fakeWordStore) ListByLevel(level JLPTLevel) ([]Word, error) { return nil, nil }

func (f *fakeWordStore) ListReviewCandidates(userID int64, level JLPTLevel, dueLimit, newLimit int) ([]ReviewCandidate, error) {
	return nil, nil
}

func (f *fakeWordStore) GetRecord(userID, wordID int64) (*WordRecord, error) {
	rec, ok := f.records[wordID]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}

func (f *fakeWordStore) ListDueRecords(userID int64) ([]WordRecord, error) { return nil, nil }

func (f *fakeWordStore) UpsertRecord(r WordRecord) error {
	f.records[r.WordID] = r
	return nil
}

func (f *fakeWordStore) BookmarkWord(userID, wordID int64) error { return nil }

// fakeStreakRecorder records check-ins so tests can assert the wiring.
type fakeStreakRecorder struct {
	calls []int64
	err   error
}

func (f *fakeStreakRecorder) RecordStudyDay(userID int64) (int, error) {
	f.calls = append(f.calls, userID)
	if f.err != nil {
		return 0, f.err
	}
	return len(f.calls), nil
}

func seenRequest(t *testing.T) *http.Request {
	t.Helper()
	token, _, err := user.SignToken(42, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/words/7/seen", nil)
	req.SetPathValue("id", "7")
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func seenMux(svc *WordService) *http.ServeMux {
	h := NewWordHandler(svc)
	protected := http.NewServeMux()
	h.RegisterRoutes(protected)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/words/", user.AuthMiddleware(testJWTSecret, protected))
	return mux
}

// TestMarkSeenCreatesProvisionalRecord proves a card that was only looked at
// still leaves a record, so it will not be served as new again on reload.
func TestMarkSeenCreatesProvisionalRecord(t *testing.T) {
	store := newFakeWordStore()
	rec := httptest.NewRecorder()
	seenMux(NewWordService(store)).ServeHTTP(rec, seenRequest(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got, err := store.GetRecord(42, 7)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if got == nil {
		t.Fatal("MarkSeen did not persist a record")
	}
	if got.MasteryLevel != 0 || got.EaseFactor != 2.5 {
		t.Errorf("provisional record = %+v, want mastery 0 / EF 2.5", got)
	}
	if !got.NextReviewAt.After(time.Now()) {
		t.Errorf("NextReviewAt = %v, want in the future", got.NextReviewAt)
	}
}

// TestMarkSeenIsIdempotent keeps a real rating from being overwritten by a
// late "seen" call (e.g. two tabs open on the same card).
func TestMarkSeenIsIdempotent(t *testing.T) {
	store := newFakeWordStore()
	_ = store.UpsertRecord(WordRecord{UserID: 42, WordID: 7, MasteryLevel: 4, EaseFactor: 2.9})

	rec := httptest.NewRecorder()
	seenMux(NewWordService(store)).ServeHTTP(rec, seenRequest(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	got, err := store.GetRecord(42, 7)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if got.MasteryLevel != 4 || got.EaseFactor != 2.9 {
		t.Errorf("existing record was overwritten: %+v", got)
	}
}

func ratingRequest(t *testing.T) *http.Request {
	t.Helper()
	token, _, err := user.SignToken(42, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/words/7/rate", strings.NewReader(`{"rating":"normal"}`))
	req.SetPathValue("id", "7")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestSubmitRatingChecksInForStreak proves a completed rating advances the
// streak: this is the wiring that previously did not exist at all.
func TestSubmitRatingChecksInForStreak(t *testing.T) {
	svc := NewWordService(newFakeWordStore())
	recorder := &fakeStreakRecorder{}
	h := NewWordHandler(svc)
	h.SetStreakRecorder(recorder)

	protected := http.NewServeMux()
	h.RegisterRoutes(protected)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/words/", user.AuthMiddleware(testJWTSecret, protected))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, ratingRequest(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(recorder.calls) != 1 || recorder.calls[0] != 42 {
		t.Errorf("streak calls = %v, want [42]", recorder.calls)
	}
}

// TestSubmitRatingSurvivesStreakFailure keeps a bookkeeping error from failing
// a submission the learner has already completed.
func TestSubmitRatingSurvivesStreakFailure(t *testing.T) {
	svc := NewWordService(newFakeWordStore())
	recorder := &fakeStreakRecorder{err: errors.New("db down")}
	h := NewWordHandler(svc)
	h.SetStreakRecorder(recorder)

	protected := http.NewServeMux()
	h.RegisterRoutes(protected)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/words/", user.AuthMiddleware(testJWTSecret, protected))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, ratingRequest(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: a streak failure must not fail the request", rec.Code, http.StatusOK)
	}
	if len(recorder.calls) != 1 {
		t.Errorf("streak calls = %v, want one attempt", recorder.calls)
	}
}

// TestSubmitRatingWithoutStreakRecorder stays safe when streak tracking is
// not wired up.
func TestSubmitRatingWithoutStreakRecorder(t *testing.T) {
	svc := NewWordService(newFakeWordStore())
	h := NewWordHandler(svc)

	protected := http.NewServeMux()
	h.RegisterRoutes(protected)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/words/", user.AuthMiddleware(testJWTSecret, protected))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, ratingRequest(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var envelope struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	if envelope.Data["status"] != "ok" {
		t.Errorf("status = %q, want ok", envelope.Data["status"])
	}
}
