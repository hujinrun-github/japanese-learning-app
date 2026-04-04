package word

import (
	"fmt"
	"log/slog"
)

// WordStoreInterface defines data access methods required by WordService.
type WordStoreInterface interface {
	GetByID(id int64) (*Word, error)
	ListByLevel(level JLPTLevel) ([]Word, error)
	GetRecord(userID, wordID int64) (*WordRecord, error)
	ListDueRecords(userID int64) ([]WordRecord, error)
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

// GetReviewQueue returns the review queue for a user at the given JLPT level.
// It merges due records (existing, overdue) with new words (no record yet).
func (s *WordService) GetReviewQueue(userID int64, level JLPTLevel) ([]WordCard, error) {
	slog.Debug("WordService.GetReviewQueue called", "user_id", userID, "level", level)

	dueRecords, err := s.store.ListDueRecords(userID)
	if err != nil {
		slog.Error("WordService.GetReviewQueue: ListDueRecords failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("word.WordService.GetReviewQueue ListDueRecords: %w", err)
	}

	allWords, err := s.store.ListByLevel(level)
	if err != nil {
		slog.Error("WordService.GetReviewQueue: ListByLevel failed", "err", err, "level", level)
		return nil, fmt.Errorf("word.WordService.GetReviewQueue ListByLevel: %w", err)
	}

	// Build set of word IDs that already have due records
	dueWordIDs := make(map[int64]WordRecord, len(dueRecords))
	for _, r := range dueRecords {
		dueWordIDs[r.WordID] = r
	}

	var cards []WordCard

	// Add due cards first
	for _, r := range dueRecords {
		w, err := s.store.GetByID(r.WordID)
		if err != nil {
			slog.Error("WordService.GetReviewQueue: GetByID failed", "err", err, "word_id", r.WordID)
			return nil, fmt.Errorf("word.WordService.GetReviewQueue GetByID: %w", err)
		}
		cards = append(cards, WordCard{Word: *w, Record: r, IsNew: false})
	}

	// Add new words (those in the level list but with no record yet)
	for _, w := range allWords {
		if _, hasDue := dueWordIDs[w.ID]; hasDue {
			continue
		}
		rec, err := s.store.GetRecord(userID, w.ID)
		if err != nil {
			slog.Error("WordService.GetReviewQueue: GetRecord failed", "err", err, "word_id", w.ID)
			return nil, fmt.Errorf("word.WordService.GetReviewQueue GetRecord: %w", err)
		}
		if rec == nil {
			// No record yet — it's a new word
			cards = append(cards, WordCard{Word: w, Record: WordRecord{UserID: userID, WordID: w.ID, EaseFactor: 2.5}, IsNew: true})
		}
	}

	slog.Debug("WordService.GetReviewQueue done", "user_id", userID, "count", len(cards))
	return cards, nil
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

	updated := CalcNextReview(base, rating)
	updated.UserID = userID
	updated.WordID = wordID

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
