package word_test

import (
	"testing"

	"japanese-learning-app/internal/module/word"
)

// A rating arriving between the seen read and write must survive. This
// deterministic interleaving exercises the service contract without timing.
type seenRaceStore struct {
	*fakeWordStore
	afterRead func()
}

func (s *seenRaceStore) GetRecord(uid, wid int64) (*word.WordRecord, error) {
	r, err := s.fakeWordStore.GetRecord(uid, wid)
	if s.afterRead != nil {
		f := s.afterRead
		s.afterRead = nil
		f()
	}
	return r, err
}

func TestMarkSeenPreservesConcurrentRating(t *testing.T) {
	s := &seenRaceStore{fakeWordStore: newFakeWordStore()}
	s.seedWord(word.Word{ID: 1, JLPTLevel: word.LevelN5})
	svc := word.NewWordService(s)
	s.afterRead = func() {
		if err := svc.SubmitRating(1, 1, word.RatingEasy); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.MarkSeen(1, 1); err != nil {
		t.Fatal(err)
	}
	r, err := s.fakeWordStore.GetRecord(1, 1)
	if err != nil || r == nil || len(r.ReviewHistory) != 1 || r.MasteryLevel == 0 {
		t.Fatalf("concurrent rating was overwritten: record=%+v err=%v", r, err)
	}
}
