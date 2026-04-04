package service

import (
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/speaking"
)

// SpeakingStoreInterface defines the data access methods required by SpeakingService.
type SpeakingStoreInterface interface {
	SaveRecord(r speaking.SpeakingRecord) error
	ListRecords(userID int64) ([]speaking.SpeakingRecord, error)
	GetRecord(id int64) (*speaking.SpeakingRecord, error)
}

// SpeakingService handles business logic for speaking practice.
type SpeakingService struct {
	store SpeakingStoreInterface
}

// NewSpeakingService creates a SpeakingService instance.
func NewSpeakingService(store SpeakingStoreInterface) *SpeakingService {
	return &SpeakingService{store: store}
}

// SaveRecord persists a speaking practice record.
func (s *SpeakingService) SaveRecord(r speaking.SpeakingRecord) error {
	slog.Debug("SpeakingService.SaveRecord called", "user_id", r.UserID, "type", r.Type)

	if err := s.store.SaveRecord(r); err != nil {
		slog.Error("SpeakingService.SaveRecord: failed", "err", err)
		return fmt.Errorf("service.SpeakingService.SaveRecord: %w", err)
	}

	slog.Debug("SpeakingService.SaveRecord done", "user_id", r.UserID)
	return nil
}

// ListRecords returns all speaking records for the user, ordered by practiced_at desc.
func (s *SpeakingService) ListRecords(userID int64) ([]speaking.SpeakingRecord, error) {
	slog.Debug("SpeakingService.ListRecords called", "user_id", userID)

	records, err := s.store.ListRecords(userID)
	if err != nil {
		slog.Error("SpeakingService.ListRecords: failed", "err", err)
		return nil, fmt.Errorf("service.SpeakingService.ListRecords: %w", err)
	}

	slog.Debug("SpeakingService.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}

// GetRecord returns a single speaking record by ID.
func (s *SpeakingService) GetRecord(id int64) (*speaking.SpeakingRecord, error) {
	slog.Debug("SpeakingService.GetRecord called", "record_id", id)

	r, err := s.store.GetRecord(id)
	if err != nil {
		slog.Error("SpeakingService.GetRecord: failed", "err", err, "record_id", id)
		return nil, fmt.Errorf("service.SpeakingService.GetRecord: %w", err)
	}

	slog.Debug("SpeakingService.GetRecord done", "record_id", id, "user_id", r.UserID)
	return r, nil
}
