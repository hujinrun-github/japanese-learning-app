package service

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/word"
)

// WordStoreInterface defines the data access methods required by WordService.
type WordStoreInterface interface {
	GetByID(id int64) (*word.Word, error)
	ListByLevel(level word.JLPTLevel, page, size int) ([]word.Word, int, error)
	GetRecord(userID, wordID int64) (*word.WordRecord, error)
	ListDueRecords(userID int64, limit int) ([]word.WordRecord, error)
	UpsertRecord(r word.WordRecord) error
	BookmarkWord(userID, wordID int64) error
}

// WordService handles business logic for vocabulary learning and spaced repetition.
type WordService struct {
	store WordStoreInterface
}

// NewWordService creates a WordService instance.
func NewWordService(store WordStoreInterface) *WordService {
	return &WordService{store: store}
}

// GetDueQueue returns up to limit word cards due for review for the user.
// Cards without a record are excluded; use the store's ListDueRecords.
func (s *WordService) GetDueQueue(userID int64, limit int) ([]word.WordCard, error) {
	slog.Debug("WordService.GetDueQueue called", "user_id", userID, "limit", limit)

	records, err := s.store.ListDueRecords(userID, limit)
	if err != nil {
		slog.Error("WordService.GetDueQueue: failed to list due records", "err", err)
		return nil, fmt.Errorf("service.WordService.GetDueQueue: %w", err)
	}

	cards := make([]word.WordCard, 0, len(records))
	for _, r := range records {
		w, err := s.store.GetByID(r.WordID)
		if err != nil {
			slog.Error("WordService.GetDueQueue: failed to get word", "err", err, "word_id", r.WordID)
			return nil, fmt.Errorf("service.WordService.GetDueQueue get word %d: %w", r.WordID, err)
		}
		cards = append(cards, word.WordCard{
			Word:   *w,
			Record: r,
			IsNew:  false,
		})
	}

	slog.Debug("WordService.GetDueQueue done", "user_id", userID, "count", len(cards))
	return cards, nil
}

// SubmitReview processes a review rating for a word, applying SM-2 algorithm.
// If no record exists, a new one is created.
func (s *WordService) SubmitReview(userID, wordID int64, rating word.ReviewRating) error {
	slog.Debug("WordService.SubmitReview called", "user_id", userID, "word_id", wordID, "rating", rating)

	r, err := s.store.GetRecord(userID, wordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isNotFound(err) {
			// First review — create new record
			r = &word.WordRecord{
				UserID:     userID,
				WordID:     wordID,
				EaseFactor: 2.5,
				Interval:   1,
			}
		} else {
			slog.Error("WordService.SubmitReview: failed to get record", "err", err)
			return fmt.Errorf("service.WordService.SubmitReview get record: %w", err)
		}
	}

	// Apply SM-2
	applyRating(r, rating)

	if err := s.store.UpsertRecord(*r); err != nil {
		slog.Error("WordService.SubmitReview: failed to upsert record", "err", err)
		return fmt.Errorf("service.WordService.SubmitReview upsert: %w", err)
	}

	slog.Debug("WordService.SubmitReview done", "user_id", userID, "word_id", wordID, "new_interval", r.Interval)
	return nil
}

// BookmarkWord saves a word to the user's bookmark list (idempotent).
func (s *WordService) BookmarkWord(userID, wordID int64) error {
	slog.Debug("WordService.BookmarkWord called", "user_id", userID, "word_id", wordID)

	if err := s.store.BookmarkWord(userID, wordID); err != nil {
		slog.Error("WordService.BookmarkWord: failed", "err", err)
		return fmt.Errorf("service.WordService.BookmarkWord: %w", err)
	}

	slog.Debug("WordService.BookmarkWord done", "user_id", userID, "word_id", wordID)
	return nil
}

// ListByLevel returns paginated words for the given JLPT level.
func (s *WordService) ListByLevel(level word.JLPTLevel, page, size int) ([]word.Word, int, error) {
	slog.Debug("WordService.ListByLevel called", "level", level, "page", page, "size", size)

	words, total, err := s.store.ListByLevel(level, page, size)
	if err != nil {
		slog.Error("WordService.ListByLevel: failed", "err", err)
		return nil, 0, fmt.Errorf("service.WordService.ListByLevel: %w", err)
	}

	slog.Debug("WordService.ListByLevel done", "level", level, "count", len(words), "total", total)
	return words, total, nil
}

// applyRating applies the SM-2 algorithm to the word record based on the rating.
//
// SM-2 mapping:
//
//	easy   → quality 5
//	normal → quality 3
//	hard   → quality 1 (resets)
func applyRating(r *word.WordRecord, rating word.ReviewRating) {
	now := time.Now()

	event := word.ReviewEvent{Rating: rating, ReviewedAt: now}
	r.ReviewHistory = append(r.ReviewHistory, event)

	switch rating {
	case word.RatingHard:
		// Reset
		r.MasteryLevel = 0
		r.Interval = 1
		r.EaseFactor = max64(1.3, r.EaseFactor-0.2)
	case word.RatingNormal:
		r.MasteryLevel++
		r.Interval = nextInterval(r.MasteryLevel, r.Interval, r.EaseFactor)
		// EF unchanged for normal
	case word.RatingEasy:
		r.MasteryLevel++
		r.Interval = nextInterval(r.MasteryLevel, r.Interval, r.EaseFactor)
		r.EaseFactor = min64(3.0, r.EaseFactor+0.1)
	}

	r.NextReviewAt = now.Add(time.Duration(r.Interval) * 24 * time.Hour)
}

func nextInterval(mastery, prevInterval int, ef float64) int {
	switch mastery {
	case 1:
		return 1
	case 2:
		return 6
	default:
		next := int(float64(prevInterval) * ef)
		if next < 1 {
			return 1
		}
		return next
	}
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// isNotFound returns true if the error message contains "not found" — used for fake stores in tests.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, sql.ErrNoRows) || containsNotFound(err.Error())
}

func containsNotFound(s string) bool {
	// Simple check for common not-found indicators from both the DB layer and fakes
	for i := 0; i+8 <= len(s); i++ {
		if s[i:i+9] == "not found" {
			return true
		}
	}
	return false
}
