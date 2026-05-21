package speaking

import (
	"fmt"
	"log/slog"
	"time"
)

// SpeakingStoreInterface defines data access methods required by SpeakingService.
type SpeakingStoreInterface interface {
	SaveRecord(r SpeakingRecord) (int64, error)
	ListRecords(userID int64) ([]SpeakingRecord, error)
	GetRecord(id int64) (*SpeakingRecord, error)
	ListMaterials(practiceType, level string) ([]SpeakingMaterial, error)
	GetMaterialByID(id int64) (*SpeakingMaterial, error)
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

	if _, err := s.store.SaveRecord(rec); err != nil {
		slog.Error("SpeakingService.Practice: SaveRecord failed", "err", err)
		return ScoreResult{}, fmt.Errorf("speaking.SpeakingService.Practice SaveRecord: %w", err)
	}

	slog.Debug("SpeakingService.Practice done", "user_id", userID, "score", result.OverallScore)
	return result, nil
}

// RecordPractice saves a self-rated practice record and returns it with the assigned ID.
func (s *SpeakingService) RecordPractice(userID int64, practiceType PracticeType, materialID int64, score int) (*SpeakingRecord, error) {
	slog.Debug("SpeakingService.RecordPractice called", "user_id", userID, "type", practiceType, "material_id", materialID, "score", score)

	rec := SpeakingRecord{
		UserID:      userID,
		Type:        practiceType,
		MaterialID:  materialID,
		Score:       score,
		PracticedAt: time.Now(),
	}

	id, err := s.store.SaveRecord(rec)
	if err != nil {
		slog.Error("SpeakingService.RecordPractice: SaveRecord failed", "err", err)
		return nil, fmt.Errorf("speaking.SpeakingService.RecordPractice SaveRecord: %w", err)
	}

	rec.ID = id
	slog.Debug("SpeakingService.RecordPractice done", "record_id", id, "user_id", userID)
	return &rec, nil
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
// ListMaterials returns speaking practice materials with optional type and level filters.
func (s *SpeakingService) ListMaterials(practiceType, level string) ([]SpeakingMaterial, error) {
	slog.Debug("SpeakingService.ListMaterials called", "type", practiceType, "level", level)

	materials, err := s.store.ListMaterials(practiceType, level)
	if err != nil {
		slog.Error("SpeakingService.ListMaterials: failed", "err", err)
		return nil, fmt.Errorf("speaking.SpeakingService.ListMaterials: %w", err)
	}

	slog.Debug("SpeakingService.ListMaterials done", "count", len(materials))
	return materials, nil
}

// GetMaterialByID returns a single speaking material by id.
func (s *SpeakingService) GetMaterialByID(id int64) (*SpeakingMaterial, error) {
	slog.Debug("SpeakingService.GetMaterialByID called", "id", id)

	material, err := s.store.GetMaterialByID(id)
	if err != nil {
		slog.Error("SpeakingService.GetMaterialByID: failed", "err", err, "id", id)
		return nil, fmt.Errorf("speaking.SpeakingService.GetMaterialByID: %w", err)
	}

	slog.Debug("SpeakingService.GetMaterialByID done", "id", id)
	return material, nil
}
