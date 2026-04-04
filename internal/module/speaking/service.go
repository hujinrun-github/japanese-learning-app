package speaking

import (
	"fmt"
	"log/slog"
	"time"
)

// SpeakingStoreInterface defines data access methods required by SpeakingService.
type SpeakingStoreInterface interface {
	SaveRecord(r SpeakingRecord) error
	ListRecords(userID int64) ([]SpeakingRecord, error)
	GetRecord(id int64) (*SpeakingRecord, error)
}

// SpeakingService handles business logic for speaking practice.
type SpeakingService struct {
	store  SpeakingStoreInterface
	scorer AudioScorer
}

// NewSpeakingService creates a SpeakingService instance.
func NewSpeakingService(store SpeakingStoreInterface, scorer AudioScorer) *SpeakingService {
	return &SpeakingService{store: store, scorer: scorer}
}

// Practice scores the user's speaking audio against reference audio,
// saves the result, and returns the ScoreResult.
func (s *SpeakingService) Practice(userID int64, practiceType PracticeType, materialID int64, referenceAudio, userAudio []byte) (ScoreResult, error) {
	slog.Debug("SpeakingService.Practice called", "user_id", userID, "type", practiceType, "material_id", materialID)

	result, err := s.scorer.Score(referenceAudio, userAudio)
	if err != nil {
		slog.Error("SpeakingService.Practice: scorer failed", "err", err)
		return ScoreResult{}, fmt.Errorf("speaking.SpeakingService.Practice Score: %w", err)
	}

	rec := SpeakingRecord{
		UserID:      userID,
		Type:        practiceType,
		MaterialID:  materialID,
		Score:       result.OverallScore,
		PracticedAt: time.Now(),
	}

	if err := s.store.SaveRecord(rec); err != nil {
		slog.Error("SpeakingService.Practice: SaveRecord failed", "err", err)
		return ScoreResult{}, fmt.Errorf("speaking.SpeakingService.Practice SaveRecord: %w", err)
	}

	slog.Debug("SpeakingService.Practice done", "user_id", userID, "score", result.OverallScore)
	return result, nil
}

// ListRecords returns all speaking records for the user.
func (s *SpeakingService) ListRecords(userID int64) ([]SpeakingRecord, error) {
	slog.Debug("SpeakingService.ListRecords called", "user_id", userID)

	records, err := s.store.ListRecords(userID)
	if err != nil {
		slog.Error("SpeakingService.ListRecords: failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("speaking.SpeakingService.ListRecords: %w", err)
	}

	slog.Debug("SpeakingService.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}
