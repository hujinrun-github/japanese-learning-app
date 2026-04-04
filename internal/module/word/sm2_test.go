package word_test

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

func TestCalcNextReview(t *testing.T) {
	baseRecord := func(mastery int, ef float64, interval int) word.WordRecord {
		return word.WordRecord{
			UserID:       1,
			WordID:       1,
			MasteryLevel: mastery,
			EaseFactor:   ef,
			Interval:     interval,
		}
	}

	tests := []struct {
		name            string
		record          word.WordRecord
		rating          word.ReviewRating
		wantMastery     int
		wantInterval    int
		wantEFMin       float64
		wantEFMax       float64
		wantNextAtLeast time.Duration // at least N days from now
	}{
		{
			name:         "first learning easy",
			record:       baseRecord(0, 2.5, 0),
			rating:       word.RatingEasy,
			wantMastery:  1,
			wantInterval: 1,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "first learning normal",
			record:       baseRecord(0, 2.5, 0),
			rating:       word.RatingNormal,
			wantMastery:  1,
			wantInterval: 1,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "first learning hard",
			record:       baseRecord(0, 2.5, 0),
			rating:       word.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 1 easy -> interval 6",
			record:       baseRecord(1, 2.5, 1),
			rating:       word.RatingEasy,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "mastery 1 normal -> interval 6",
			record:       baseRecord(1, 2.5, 1),
			rating:       word.RatingNormal,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 1 hard -> reset",
			record:       baseRecord(1, 2.5, 1),
			rating:       word.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 2 normal -> interval = prev*EF",
			record:       baseRecord(2, 2.5, 6),
			rating:       word.RatingNormal,
			wantMastery:  3,
			wantInterval: 15, // floor(6 * 2.5) = 15
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 2 easy -> ef increases",
			record:       baseRecord(2, 2.5, 6),
			rating:       word.RatingEasy,
			wantMastery:  3,
			wantInterval: 15, // based on previous interval before EF update
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "mastery 3 hard -> reset regardless of mastery",
			record:       baseRecord(3, 2.5, 15),
			rating:       word.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.49,
		},
		{
			name:         "ef already min 1.3 stays at 1.3 on hard",
			record:       baseRecord(1, 1.3, 1),
			rating:       word.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    1.3,
		},
		{
			name:         "ef cap at 3.0 on easy",
			record:       baseRecord(1, 2.9, 1),
			rating:       word.RatingEasy,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    3.0,
			wantEFMax:    3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			result := word.CalcNextReview(tt.record, tt.rating)
			after := time.Now()

			if result.MasteryLevel != tt.wantMastery {
				t.Errorf("MasteryLevel = %d, want %d", result.MasteryLevel, tt.wantMastery)
			}
			if result.Interval != tt.wantInterval {
				t.Errorf("Interval = %d, want %d", result.Interval, tt.wantInterval)
			}
			if result.EaseFactor < tt.wantEFMin-0.001 || result.EaseFactor > tt.wantEFMax+0.001 {
				t.Errorf("EaseFactor = %.4f, want [%.4f, %.4f]", result.EaseFactor, tt.wantEFMin, tt.wantEFMax)
			}
			// NextReviewAt should be ~Interval days from now (within 5 seconds)
			wantLo := before.Add(time.Duration(result.Interval)*24*time.Hour - 5*time.Second)
			wantHi := after.Add(time.Duration(result.Interval)*24*time.Hour + 5*time.Second)
			if result.NextReviewAt.Before(wantLo) || result.NextReviewAt.After(wantHi) {
				t.Errorf("NextReviewAt = %v, want between %v and %v", result.NextReviewAt, wantLo, wantHi)
			}
			// ReviewHistory grows by 1
			if len(result.ReviewHistory) != len(tt.record.ReviewHistory)+1 {
				t.Errorf("ReviewHistory len = %d, want %d", len(result.ReviewHistory), len(tt.record.ReviewHistory)+1)
			}
			// Last event has correct rating
			last := result.ReviewHistory[len(result.ReviewHistory)-1]
			if last.Rating != tt.rating {
				t.Errorf("last ReviewEvent.Rating = %s, want %s", last.Rating, tt.rating)
			}
		})
	}
}
