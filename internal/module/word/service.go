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
	// words (by id), both scoped to level. Both limits must be pushed down to
	// SQL: loading a whole JLPT level costs thousands of rows on PostgreSQL.
	ListReviewCandidates(userID int64, level JLPTLevel, dueLimit, newLimit int) ([]ReviewCandidate, error)
	UpsertRecord(r WordRecord) error
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

// GetReviewQueue returns at most limit review cards for a user at the given
// JLPT level. Due words come first (most overdue first), then new words fill
// the rest of the round.
//
// Two rules keep learned words from showing up over and over:
//   - a due word already reviewed today is skipped (a page refresh must not
//     replay the same batch);
//   - due words take at most half of the round, so a backlog of overdue words
//     cannot crowd out new words for days.
//
// Both candidate groups are read in a single batched store call with the
// limits pushed down to SQL. Loading a whole JLPT level or calling GetRecord
// once per word costs thousands of serial round trips on PostgreSQL.
func (s *WordService) GetReviewQueue(userID int64, level JLPTLevel, limit int) ([]WordCard, error) {
	slog.Debug("WordService.GetReviewQueue called", "user_id", userID, "level", level, "limit", limit)

	if limit <= 0 {
		return []WordCard{}, nil
	}

	dueLimit := limit / 2
	if dueLimit < 1 {
		dueLimit = 1
	}

	candidates, err := s.store.ListReviewCandidates(userID, level, dueLimit, limit)
	if err != nil {
		slog.Error("WordService.GetReviewQueue: ListReviewCandidates failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("word.WordService.GetReviewQueue ListReviewCandidates: %w", err)
	}

	now := time.Now()
	cards := make([]WordCard, 0, limit)
	dueCount := 0
	for _, c := range candidates {
		if len(cards) >= limit {
			break
		}
		if c.Record != nil {
			if dueCount >= dueLimit {
				continue
			}
			if sameDay(c.Record.UpdatedAt, now) {
				slog.Debug("WordService.GetReviewQueue: skip word already reviewed today", "word_id", c.Word.ID)
				continue
			}
			dueCount++
			cards = append(cards, WordCard{Word: c.Word, Record: *c.Record, IsNew: false})
			continue
		}
		cards = append(cards, WordCard{
			Word:   c.Word,
			Record: WordRecord{UserID: userID, WordID: c.Word.ID, EaseFactor: 2.5},
			IsNew:  true,
		})
	}

	slog.Debug("WordService.GetReviewQueue done", "user_id", userID, "level", level, "count", len(cards), "due_count", dueCount)
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
	if err := s.store.UpsertRecord(record); err != nil {
		slog.Error("WordService.MarkSeen: UpsertRecord failed", "err", err, "word_id", wordID)
		return fmt.Errorf("word.WordService.MarkSeen UpsertRecord: %w", err)
	}

	slog.Debug("WordService.MarkSeen done", "word_id", wordID, "next_review_at", record.NextReviewAt)
	return nil
}

// sameDay reports whether t falls on the same calendar day as now.
func sameDay(t, now time.Time) bool {
	if t.IsZero() {
		return false
	}
	return t.Year() == now.Year() && t.YearDay() == now.YearDay()
}

// SubmitRating records the user's rating for a word review and advances the SM-2 schedule.
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
	updated := WordRecord{
		UserID:        userID,
		WordID:        wordID,
		MasteryLevel:  newMastery,
		Interval:      newInterval,
		EaseFactor:    newEF,
		NextReviewAt:  nextReview,
		ReviewHistory: newHistory,
		UpdatedAt:     time.Now(),
	}

	if err := s.store.UpsertRecord(updated); err != nil {
		slog.Error("WordService.SubmitRating: UpsertRecord failed", "err", err)
		return fmt.Errorf("word.WordService.SubmitRating UpsertRecord: %w", err)
	}

	slog.Debug("WordService.SubmitRating done", "user_id", userID, "word_id", wordID)
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
