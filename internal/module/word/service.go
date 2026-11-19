package word

import (
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/sm2"
)

// WordStoreInterface defines data access methods required by WordService.
type WordStoreInterface interface {
	GetByID(id int64) (*Word, error)
	GetRecord(userID, wordID int64) (*WordRecord, error)
	// ListReviewCandidates returns at most dueLimit overdue words (oldest
	// first, excluding ones reviewed today) followed by at most newLimit new
	// words (by id), scoped to level (empty means all levels for reviews).
	// A zero limit excludes that group. Both limits must be pushed down to
	// SQL: loading a whole JLPT level costs thousands of rows on PostgreSQL.
	ListReviewCandidates(userID int64, level JLPTLevel, dueLimit, newLimit int) ([]ReviewCandidate, error)
	UpsertRecord(r WordRecord) error
	InsertRecordIfAbsent(r WordRecord) error
	BookmarkWord(userID, wordID int64) error
}

// WordService handles business logic for vocabulary learning.
type WordService struct {
	store WordStoreInterface
}

// NewWordService creates a WordService instance.
func NewWordService(store WordStoreInterface) *WordService {
	return &WordService{store: store}
}

// GetByID returns a word by ID.
func (s *WordService) GetByID(id int64) (*Word, error) {
	return s.store.GetByID(id)
}

// GetReviewQueue returns only learned, due words, oldest first. An empty
// level covers all JLPT levels, matching the dashboard's due count.
func (s *WordService) GetReviewQueue(userID int64, level JLPTLevel, limit int) ([]WordCard, error) {
	return s.getQueue(userID, level, limit, false)
}

// GetNewWordQueue returns unseen words only, after an explicit learner choice.
func (s *WordService) GetNewWordQueue(userID int64, level JLPTLevel, limit int) ([]WordCard, error) {
	return s.getQueue(userID, level, limit, true)
}

func (s *WordService) getQueue(userID int64, level JLPTLevel, limit int, newWords bool) ([]WordCard, error) {
	slog.Debug("WordService.getQueue called", "user_id", userID, "level", level, "limit", limit, "new_words", newWords)
	if limit <= 0 {
		return []WordCard{}, nil
	}
	dueLimit, newLimit := limit, 0
	if newWords {
		dueLimit, newLimit = 0, limit
	}
	candidates, err := s.store.ListReviewCandidates(userID, level, dueLimit, newLimit)
	if err != nil {
		slog.Error("WordService.getQueue: ListReviewCandidates failed", "err", err, "user_id", userID, "level", level, "new_words", newWords)
		return nil, fmt.Errorf("word.WordService.getQueue ListReviewCandidates: %w", err)
	}

	cards := make([]WordCard, 0, limit)
	for _, c := range candidates {
		if len(cards) >= limit {
			break
		}
		if (c.Record == nil) != newWords {
			continue
		}
		if c.Record != nil {
			cards = append(cards, WordCard{Word: c.Word, Record: *c.Record, IsNew: false})
			continue
		}
		cards = append(cards, WordCard{
			Word:   c.Word,
			Record: WordRecord{UserID: userID, WordID: c.Word.ID, EaseFactor: 2.5},
			IsNew:  true,
		})
	}

	slog.Debug("WordService.getQueue done", "user_id", userID, "level", level, "count", len(cards), "new_words", newWords)
	return cards, nil
}

// MarkSeen records that a learner has seen a card. It is idempotent: a word
// with an existing record is left untouched. Without this, a learner who
// answers correctly and leaves before picking a rating has no record at all,
// so the same word is served as "new" on every visit.
func (s *WordService) MarkSeen(userID, wordID int64) error {
	slog.Debug("WordService.MarkSeen called", "user_id", userID, "word_id", wordID)

	if _, err := s.store.GetByID(wordID); err != nil {
		slog.Error("WordService.MarkSeen: word not found", "err", err, "word_id", wordID)
		return fmt.Errorf("word.WordService.MarkSeen GetByID: %w", err)
	}

	rec, err := s.store.GetRecord(userID, wordID)
	if err != nil {
		slog.Error("WordService.MarkSeen: GetRecord failed", "err", err, "word_id", wordID)
		return fmt.Errorf("word.WordService.MarkSeen GetRecord: %w", err)
	}
	if rec != nil {
		slog.Debug("WordService.MarkSeen: record already exists", "word_id", wordID)
		return nil
	}

	now := time.Now()
	record := WordRecord{
		UserID:       userID,
		WordID:       wordID,
		MasteryLevel: 0,
		EaseFactor:   2.5,
		Interval:     1,
		NextReviewAt: now.Add(24 * time.Hour),
		UpdatedAt:    now,
	}
	if err := s.store.InsertRecordIfAbsent(record); err != nil {
		slog.Error("WordService.MarkSeen: InsertRecordIfAbsent failed", "err", err, "word_id", wordID)
		return fmt.Errorf("word.WordService.MarkSeen InsertRecordIfAbsent: %w", err)
	}

	slog.Debug("WordService.MarkSeen done", "word_id", wordID, "next_review_at", record.NextReviewAt)
	return nil
}

// SubmitRating records a word review. Normal means familiar (daily practice),
// easy means fully mastered (at least a week between reviews), and hard is
// reserved for wrong answers. Other modules keep the shared SM-2 schedule.
func (s *WordService) SubmitRating(userID, wordID int64, rating ReviewRating) error {
	slog.Debug("WordService.SubmitRating called", "user_id", userID, "word_id", wordID, "rating", rating)

	// Verify word exists
	if _, err := s.store.GetByID(wordID); err != nil {
		slog.Error("WordService.SubmitRating: word not found", "err", err, "word_id", wordID)
		return fmt.Errorf("word.WordService.SubmitRating GetByID: %w", err)
	}

	rec, err := s.store.GetRecord(userID, wordID)
	if err != nil {
		slog.Error("WordService.SubmitRating: GetRecord failed", "err", err)
		return fmt.Errorf("word.WordService.SubmitRating GetRecord: %w", err)
	}

	var base WordRecord
	if rec != nil {
		base = *rec
	} else {
		base = WordRecord{UserID: userID, WordID: wordID, EaseFactor: 2.5}
	}

	newMastery, newInterval, newEF, nextReview, newHistory := sm2.CalcNextReview(
		base.MasteryLevel, base.Interval, base.EaseFactor,
		rating, base.ReviewHistory,
	)
	switch rating {
	case RatingNormal:
		newMastery = max(1, min(base.MasteryLevel, 4))
		newInterval = 1
	case RatingEasy:
		newMastery = 5
		newInterval = max(7, base.Interval+1, newInterval)
	}
	now := time.Now()
	nextReview = now.Add(time.Duration(newInterval) * 24 * time.Hour)
	updated := WordRecord{
		UserID:        userID,
		WordID:        wordID,
		MasteryLevel:  newMastery,
		Interval:      newInterval,
		EaseFactor:    newEF,
		NextReviewAt:  nextReview,
		ReviewHistory: newHistory,
		UpdatedAt:     now,
	}

	if err := s.store.UpsertRecord(updated); err != nil {
		slog.Error("WordService.SubmitRating: UpsertRecord failed", "err", err)
		return fmt.Errorf("word.WordService.SubmitRating UpsertRecord: %w", err)
	}

	slog.Debug("WordService.SubmitRating done", "user_id", userID, "word_id", wordID,
		"rating", rating, "previous_interval_days", base.Interval, "interval_days", newInterval,
		"mastery_level", newMastery, "next_review_at", nextReview)
	return nil
}

// Bookmark bookmarks a word for a user.
func (s *WordService) Bookmark(userID, wordID int64) error {
	slog.Debug("WordService.Bookmark called", "user_id", userID, "word_id", wordID)

	if err := s.store.BookmarkWord(userID, wordID); err != nil {
		slog.Error("WordService.Bookmark: failed", "err", err)
		return fmt.Errorf("word.WordService.Bookmark: %w", err)
	}

	slog.Debug("WordService.Bookmark done", "user_id", userID, "word_id", wordID)
	return nil
}
