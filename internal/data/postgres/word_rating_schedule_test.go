package postgres

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
	"japanese-learning-app/internal/store"
)

func TestWordSelfAssessmentPersistsAndControlsReviewQueue(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	u, err := newPostgresUserStoreForDB(db).CreateUser(user.User{
		Email: "word-rating@example.test", JLPTLevels: []string{"N5"},
	}, "unused")
	if err != nil {
		t.Fatal(err)
	}
	s := newPostgresWordStoreForDB(db)
	svc := word.NewWordService(s)
	for _, tt := range []struct {
		name         string
		rating       word.ReviewRating
		wantInterval int
		wantMastery  int
		wantDue      bool
	}{
		{"familiar", word.RatingNormal, 1, 1, true},
		{"mastered", word.RatingEasy, 7, 5, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := s.InsertWord(word.Word{
				KanjiForm: tt.name, Reading: "てすと", Meaning: "测试", JLPTLevel: word.LevelN5,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.MarkSeen(u.ID, id); err != nil {
				t.Fatal(err)
			}
			if err := svc.SubmitRating(u.ID, id, tt.rating); err != nil {
				t.Fatal(err)
			}
			got, err := s.GetRecord(u.ID, id)
			if err != nil || got == nil {
				t.Fatalf("GetRecord: record=%+v err=%v", got, err)
			}
			if got.Interval != tt.wantInterval || got.MasteryLevel != tt.wantMastery || len(got.ReviewHistory) != 1 || got.ReviewHistory[0].Rating != tt.rating {
				t.Fatalf("unexpected saved assessment: %+v", got)
			}
			futureStore := NewWordStore(db, store.StoreDeps{
				Clock: fixedClock{now: time.Now().Add(48 * time.Hour)}, Logger: testLogger{}, AppTimezone: time.UTC,
			})
			candidates, err := futureStore.ListReviewCandidates(u.ID, word.LevelN5, 20, 20)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, candidate := range candidates {
				if candidate.Word.ID == id {
					found = true
				}
			}
			if found != tt.wantDue {
				t.Errorf("in review queue after two days = %v, want %v", found, tt.wantDue)
			}
		})
	}
}
