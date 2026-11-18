package postgres

import (
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

func TestSeenInsertNeverOverwritesRating(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	u, err := newPostgresUserStoreForDB(db).CreateUser(user.User{Email: "seen@example.test", JLPTLevels: []string{"N5"}}, "unused")
	if err != nil {
		t.Fatal(err)
	}
	s := newPostgresWordStoreForDB(db)
	id, err := s.InsertWord(word.Word{KanjiForm: "猫", Reading: "ねこ", Meaning: "猫", JLPTLevel: word.LevelN5})
	if err != nil {
		t.Fatal(err)
	}
	provisional := word.WordRecord{UserID: u.ID, WordID: id, EaseFactor: 2.5, Interval: 1, NextReviewAt: time.Now().Add(24 * time.Hour), UpdatedAt: time.Now()}
	if err := s.InsertRecordIfAbsent(provisional); err != nil {
		t.Fatal(err)
	}
	if err := word.NewWordService(s).SubmitRating(u.ID, id, word.RatingEasy); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetRecord(u.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRecordIfAbsent(provisional); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetRecord(u.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.MasteryLevel != before.MasteryLevel || after.Interval != before.Interval || len(after.ReviewHistory) != 1 || !after.NextReviewAt.Equal(before.NextReviewAt) {
		t.Fatalf("rating changed: before=%+v after=%+v", before, after)
	}
}

func TestGrammarSearchNormalizesAndRanksBeforeLimit(t *testing.T) {
	s := newPostgresGrammarStoreForTest(t)
	id, err := s.InsertPoint(grammar.GrammarPoint{Name: "〜から", Meaning: "因为", JLPTLevel: grammar.LevelN5})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 55; i++ {
		if _, err := s.InsertPoint(grammar.GrammarPoint{Name: fmt.Sprintf("〜ことから%d", i), Meaning: "由于", JLPTLevel: grammar.LevelN3}); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"から", "「から」", "～から", "『〜から』", "から（理由）"} {
		t.Run(query, func(t *testing.T) {
			got, err := grammar.NewGrammarService(s).SearchByName(query)
			if err != nil {
				t.Fatal(err)
			}
			if got.ExactMatchID != id || len(got.Points) > 50 || len(got.Points) == 0 || got.Points[0].ID != id {
				t.Fatalf("query=%q result=%+v", query, got)
			}
		})
	}
}
