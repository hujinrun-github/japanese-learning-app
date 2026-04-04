package word

import (
	"math"
	"time"
)

// CalcNextReview applies the SM-2 spaced repetition algorithm to compute the
// next review schedule for a word, given the user's self-rating.
//
// SM-2 rules:
//   - hard  → reset mastery to 0, interval = 1, EF -= 0.2 (min 1.3)
//   - normal→ advance mastery, EF unchanged
//   - easy  → advance mastery, EF += 0.1 (max 3.0)
//
// Interval schedule:
//   - mastery 0 (first learn): interval = 1
//   - mastery 1: interval = 6
//   - mastery ≥ 2: interval = floor(prev_interval * EF)
//
// The function is pure (no side effects, no DB).
func CalcNextReview(record WordRecord, rating ReviewRating) WordRecord {
	r := record // copy

	switch rating {
	case RatingHard:
		r.MasteryLevel = 0
		r.Interval = 1
		r.EaseFactor = math.Max(1.3, r.EaseFactor-0.2)
	case RatingNormal:
		r.Interval = nextInterval(r.MasteryLevel, r.Interval, r.EaseFactor)
		r.MasteryLevel++
	case RatingEasy:
		r.Interval = nextInterval(r.MasteryLevel, r.Interval, r.EaseFactor)
		r.MasteryLevel++
		r.EaseFactor = math.Min(3.0, r.EaseFactor+0.1)
	}

	r.NextReviewAt = time.Now().Add(time.Duration(r.Interval) * 24 * time.Hour)
	r.UpdatedAt = time.Now()
	r.ReviewHistory = append(r.ReviewHistory, ReviewEvent{
		Rating:     rating,
		ReviewedAt: time.Now(),
	})

	return r
}

// nextInterval computes the interval for the next review based on current mastery.
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
