package service

import (
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/writing"
)

// WritingStoreInterface defines the data access methods required by WritingService.
type WritingStoreInterface interface {
	GetDailyQueue(userID int64) ([]writing.WritingQuestion, error)
	SaveRecord(r writing.WritingRecord) error
	ListRecords(userID int64) ([]writing.WritingRecord, error)
}

// WritingService handles business logic for writing practice.
type WritingService struct {
	store WritingStoreInterface
}

// NewWritingService creates a WritingService instance.
func NewWritingService(store WritingStoreInterface) *WritingService {
	return &WritingService{store: store}
}

// GetDailyQueue returns today's writing questions for the user.
// The ExpectedAnswer field is cleared before returning to clients.
func (s *WritingService) GetDailyQueue(userID int64) ([]writing.WritingQuestion, error) {
	slog.Debug("WritingService.GetDailyQueue called", "user_id", userID)

	questions, err := s.store.GetDailyQueue(userID)
	if err != nil {
		slog.Error("WritingService.GetDailyQueue: failed", "err", err)
		return nil, fmt.Errorf("service.WritingService.GetDailyQueue: %w", err)
	}

	// Strip expected answers — must not be exposed to the client
	for i := range questions {
		questions[i].ExpectedAnswer = ""
	}

	slog.Debug("WritingService.GetDailyQueue done", "user_id", userID, "count", len(questions))
	return questions, nil
}

// SaveRecord persists a writing practice record.
func (s *WritingService) SaveRecord(r writing.WritingRecord) error {
	slog.Debug("WritingService.SaveRecord called", "user_id", r.UserID, "type", r.Type)

	if err := s.store.SaveRecord(r); err != nil {
		slog.Error("WritingService.SaveRecord: failed", "err", err)
		return fmt.Errorf("service.WritingService.SaveRecord: %w", err)
	}

	slog.Debug("WritingService.SaveRecord done", "user_id", r.UserID)
	return nil
}

// ListRecords returns all writing records for the user.
func (s *WritingService) ListRecords(userID int64) ([]writing.WritingRecord, error) {
	slog.Debug("WritingService.ListRecords called", "user_id", userID)

	records, err := s.store.ListRecords(userID)
	if err != nil {
		slog.Error("WritingService.ListRecords: failed", "err", err)
		return nil, fmt.Errorf("service.WritingService.ListRecords: %w", err)
	}

	slog.Debug("WritingService.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}
