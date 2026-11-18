package word_test

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

func TestWordSelfAssessmentSchedule(t *testing.T) {
	tests := []struct {
		name         string
		mastery      int
		interval     int
		rating       word.ReviewRating
		wantMastery  int
		wantInterval int
	}{
		{"new familiar word", 0, 0, word.RatingNormal, 1, 1},
		{"familiar word needs more reviews", 2, 6, word.RatingNormal, 2, 1},
		{"formerly mastered word becomes familiar", 5, 30, word.RatingNormal, 4, 1},
		{"new fully mastered word", 0, 0, word.RatingEasy, 5, 7},
		{"second fully mastered review", 1, 1, word.RatingEasy, 5, 7},
		{"fully mastered review grows interval", 2, 6, word.RatingEasy, 5, 15},
		{"repeatedly mastered stays at maximum mastery", 5, 30, word.RatingEasy, 5, 75},
		{"mastered interval never gets shorter", 1, 30, word.RatingEasy, 5, 31},
		{"wrong answers still need daily reviews", 5, 30, word.RatingHard, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeWordStore()
			store.seedWord(word.Word{ID: 1, JLPTLevel: word.LevelN5})
			previous := word.ReviewEvent{Rating: word.RatingNormal, ReviewedAt: time.Now().Add(-48 * time.Hour)}
			if err := store.UpsertRecord(word.WordRecord{
				UserID: 1, WordID: 1, MasteryLevel: tt.mastery,
				Interval: tt.interval, EaseFactor: 2.5,
				ReviewHistory: []word.ReviewEvent{previous},
			}); err != nil {
				t.Fatal(err)
			}
			before := time.Now()
			if err := word.NewWordService(store).SubmitRating(1, 1, tt.rating); err != nil {
				t.Fatal(err)
			}
			after := time.Now()
			got, err := store.GetRecord(1, 1)
			if err != nil || got == nil {
				t.Fatalf("GetRecord: record=%+v err=%v", got, err)
			}
			if got.MasteryLevel != tt.wantMastery || got.Interval != tt.wantInterval {
				t.Errorf("mastery/interval=%d/%d, want %d/%d", got.MasteryLevel, got.Interval, tt.wantMastery, tt.wantInterval)
			}
			delay := time.Duration(tt.wantInterval) * 24 * time.Hour
			if got.NextReviewAt.Before(before.Add(delay)) || got.NextReviewAt.After(after.Add(delay)) {
				t.Errorf("next review %v does not match %d-day interval", got.NextReviewAt, tt.wantInterval)
			}
			if len(got.ReviewHistory) != 2 || got.ReviewHistory[0] != previous || got.ReviewHistory[1].Rating != tt.rating {
				t.Errorf("review history lost or incorrect: %+v", got.ReviewHistory)
			}
		})
	}
}
