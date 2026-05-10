package sm2_test

import (
	"testing"
	"time"

	"japanese-learning-app/internal/sm2"
)

func TestCalcNextReview_NonNilHistory(t *testing.T) {
	oldHistory := []sm2.ReviewEvent{
		{Rating: sm2.RatingEasy, ReviewedAt: time.Now().Add(-48 * time.Hour)},
	}

	_, _, _, _, newHistory := sm2.CalcNextReview(1, 1, 2.5, sm2.RatingNormal, oldHistory)

	if len(newHistory) != len(oldHistory)+1 {
		t.Errorf("history len = %d, want %d", len(newHistory), len(oldHistory)+1)
	}
	if newHistory[0].Rating != oldHistory[0].Rating {
		t.Errorf("history[0].Rating = %s, want %s", newHistory[0].Rating, oldHistory[0].Rating)
	}
	if !newHistory[0].ReviewedAt.Equal(oldHistory[0].ReviewedAt) {
		t.Errorf("history[0].ReviewedAt = %v, want %v", newHistory[0].ReviewedAt, oldHistory[0].ReviewedAt)
	}
	if newHistory[1].Rating != sm2.RatingNormal {
		t.Errorf("history[1].Rating = %s, want %s", newHistory[1].Rating, sm2.RatingNormal)
	}
}

func TestCalcNextReview(t *testing.T) {
	tests := []struct {
		name         string
		mastery      int
		interval     int
		easeFactor   float64
		rating       sm2.Rating
		wantMastery  int
		wantInterval int
		wantEFMin    float64
		wantEFMax    float64
	}{
		{
			name:         "first learning easy",
			mastery:      0,
			interval:     0,
			easeFactor:   2.5,
			rating:       sm2.RatingEasy,
			wantMastery:  1,
			wantInterval: 1,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "first learning normal",
			mastery:      0,
			interval:     0,
			easeFactor:   2.5,
			rating:       sm2.RatingNormal,
			wantMastery:  1,
			wantInterval: 1,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "first learning hard",
			mastery:      0,
			interval:     0,
			easeFactor:   2.5,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 1 easy -> interval 6",
			mastery:      1,
			interval:     1,
			easeFactor:   2.5,
			rating:       sm2.RatingEasy,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "mastery 1 normal -> interval 6",
			mastery:      1,
			interval:     1,
			easeFactor:   2.5,
			rating:       sm2.RatingNormal,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 1 hard -> reset",
			mastery:      1,
			interval:     1,
			easeFactor:   2.5,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 2 normal -> interval = prev*EF",
			mastery:      2,
			interval:     6,
			easeFactor:   2.5,
			rating:       sm2.RatingNormal,
			wantMastery:  3,
			wantInterval: 15, // floor(6 * 2.5) = 15
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 2 easy -> ef increases",
			mastery:      2,
			interval:     6,
			easeFactor:   2.5,
			rating:       sm2.RatingEasy,
			wantMastery:  3,
			wantInterval: 15,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "mastery 3 hard -> reset regardless of mastery",
			mastery:      3,
			interval:     15,
			easeFactor:   2.5,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.49,
		},
		{
			name:         "ef already min 1.3 stays at 1.3 on hard",
			mastery:      1,
			interval:     1,
			easeFactor:   1.3,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    1.3,
		},
		{
			name:         "ef cap at 3.0 on easy",
			mastery:      1,
			interval:     1,
			easeFactor:   2.9,
			rating:       sm2.RatingEasy,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    3.0,
			wantEFMax:    3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			mastery, interval, ef, nextReview, history := sm2.CalcNextReview(tt.mastery, tt.interval, tt.easeFactor, tt.rating, nil)
			after := time.Now()

			if mastery != tt.wantMastery {
				t.Errorf("mastery = %d, want %d", mastery, tt.wantMastery)
			}
			if interval != tt.wantInterval {
				t.Errorf("interval = %d, want %d", interval, tt.wantInterval)
			}
			if ef < tt.wantEFMin-0.001 || ef > tt.wantEFMax+0.001 {
				t.Errorf("easeFactor = %.4f, want [%.4f, %.4f]", ef, tt.wantEFMin, tt.wantEFMax)
			}
			wantLo := before.Add(time.Duration(interval)*24*time.Hour - 5*time.Second)
			wantHi := after.Add(time.Duration(interval)*24*time.Hour + 5*time.Second)
			if nextReview.Before(wantLo) || nextReview.After(wantHi) {
				t.Errorf("nextReviewAt = %v, want between %v and %v", nextReview, wantLo, wantHi)
			}
			if len(history) != 1 {
				t.Errorf("history len = %d, want 1", len(history))
			}
			if history[0].Rating != tt.rating {
				t.Errorf("history[0].Rating = %s, want %s", history[0].Rating, tt.rating)
			}
		})
	}
}
