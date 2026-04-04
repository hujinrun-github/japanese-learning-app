package writing

import (
	"fmt"
	"log/slog"
	"time"
)

// WritingStoreInterface defines data access methods required by WritingService.
type WritingStoreInterface interface {
	GetDailyQueue(userID int64) ([]WritingQuestion, error)
	SaveRecord(r WritingRecord) error
	ListRecords(userID int64) ([]WritingRecord, error)
}

// WritingService handles business logic for writing practice.
type WritingService struct {
	store    WritingStoreInterface
	reviewer AIReviewer
}

// NewWritingService creates a WritingService instance.
func NewWritingService(store WritingStoreInterface, reviewer AIReviewer) *WritingService {
	return &WritingService{store: store, reviewer: reviewer}
}

// GetDailyQueue returns today's writing questions for the user.
// ExpectedAnswer is stripped before returning so it is never sent to the client.
func (s *WritingService) GetDailyQueue(userID int64) ([]WritingQuestion, error) {
	slog.Debug("WritingService.GetDailyQueue called", "user_id", userID)

	questions, err := s.store.GetDailyQueue(userID)
	if err != nil {
		slog.Error("WritingService.GetDailyQueue: store failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("writing.WritingService.GetDailyQueue: %w", err)
	}

	// Strip expected answers before returning to client.
	stripped := make([]WritingQuestion, len(questions))
	for i, q := range questions {
		q.ExpectedAnswer = ""
		stripped[i] = q
	}

	slog.Debug("WritingService.GetDailyQueue done", "user_id", userID, "count", len(stripped))
	return stripped, nil
}

// SubmitInput grades a keyboard-input practice answer (exact-match scoring)
// and saves the record.
func (s *WritingService) SubmitInput(userID int64, question, userAnswer, expected string) (WritingRecord, error) {
	slog.Debug("WritingService.SubmitInput called", "user_id", userID, "question", question)

	score := 0
	if userAnswer == expected {
		score = 100
	}

	rec := WritingRecord{
		UserID:      userID,
		Type:        WritingTypeInput,
		Question:    question,
		UserAnswer:  userAnswer,
		Score:       score,
		PracticedAt: time.Now(),
	}

	if err := s.store.SaveRecord(rec); err != nil {
		slog.Error("WritingService.SubmitInput: SaveRecord failed", "err", err, "user_id", userID)
		return WritingRecord{}, fmt.Errorf("writing.WritingService.SubmitInput SaveRecord: %w", err)
	}

	slog.Debug("WritingService.SubmitInput done", "user_id", userID, "score", score)
	return rec, nil
}

// SubmitSentence sends the user's sentence to the AI reviewer, saves the record,
// and returns it with AI feedback attached.
func (s *WritingService) SubmitSentence(userID int64, question, userAnswer string) (WritingRecord, error) {
	slog.Debug("WritingService.SubmitSentence called", "user_id", userID, "question", question)

	feedback, err := s.reviewer.Review(question, userAnswer)
	if err != nil {
		slog.Error("WritingService.SubmitSentence: reviewer failed", "err", err, "user_id", userID)
		return WritingRecord{}, fmt.Errorf("writing.WritingService.SubmitSentence Review: %w", err)
	}

	rec := WritingRecord{
		UserID:      userID,
		Type:        WritingTypeSentence,
		Question:    question,
		UserAnswer:  userAnswer,
		AIFeedback:  &feedback,
		Score:       feedback.Score,
		PracticedAt: time.Now(),
	}

	if err := s.store.SaveRecord(rec); err != nil {
		slog.Error("WritingService.SubmitSentence: SaveRecord failed", "err", err, "user_id", userID)
		return WritingRecord{}, fmt.Errorf("writing.WritingService.SubmitSentence SaveRecord: %w", err)
	}

	slog.Debug("WritingService.SubmitSentence done", "user_id", userID, "score", feedback.Score)
	return rec, nil
}

// ListRecords returns all writing records for the user.
func (s *WritingService) ListRecords(userID int64) ([]WritingRecord, error) {
	slog.Debug("WritingService.ListRecords called", "user_id", userID)

	records, err := s.store.ListRecords(userID)
	if err != nil {
		slog.Error("WritingService.ListRecords: failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("writing.WritingService.ListRecords: %w", err)
	}

	slog.Debug("WritingService.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}
