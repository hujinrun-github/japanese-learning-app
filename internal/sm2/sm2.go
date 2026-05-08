package sm2

import (
	"math"
	"time"
)

// Rating is a self-assessment rating for a review.
type Rating string

const (
	RatingEasy   Rating = "easy"
	RatingNormal Rating = "normal"
	RatingHard   Rating = "hard"
)

// ReviewEvent records a single review attempt.
type ReviewEvent struct {
	Rating     Rating    `json:"rating"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// CalcNextReview applies the SM-2 spaced repetition algorithm.
// Returns: newMastery, newInterval, newEaseFactor, nextReviewAt, newHistory.
//
// SM-2 rules:
//   - hard  → reset mastery to 0, interval = 1, EF -= 0.2 (min 1.3)
//   - normal→ advance mastery, EF unchanged
//   - easy  → advance mastery, EF += 0.1 (max 3.0)
//
// Interval schedule:
//   - mastery 0 (first learn): interval = 1
//   - mastery 1: interval = 6
//   - mastery >= 2: interval = floor(prev_interval * EF)
func CalcNextReview(mastery int, interval int, easeFactor float64, rating Rating, history []ReviewEvent) (int, int, float64, time.Time, []ReviewEvent) {
	switch rating {
	case RatingHard:
		mastery = 0
		interval = 1
		easeFactor = math.Max(1.3, easeFactor-0.2)
	case RatingNormal:
		interval = nextInterval(mastery, interval, easeFactor)
		mastery++
	case RatingEasy:
		interval = nextInterval(mastery, interval, easeFactor)
		mastery++
		easeFactor = math.Min(3.0, easeFactor+0.1)
	}

	nextReviewAt := time.Now().Add(time.Duration(interval) * 24 * time.Hour)
	history = append(history, ReviewEvent{Rating: rating, ReviewedAt: time.Now()})

	return mastery, interval, easeFactor, nextReviewAt, history
}

func nextInterval(mastery, prevInterval int, ef float64) int {
	switch mastery {
	case 0:
		return 1
	case 1:
		return 6
	default:
		return int(math.Floor(float64(prevInterval) * ef))
	}
}
