package summary

import (
	"fmt"
	"log/slog"
	"time"
)

// SummaryStoreInterface defines data access methods required by SummaryService.
type SummaryStoreInterface interface {
	SaveSession(s StudySession) error
	GetSession(sessionID string) (*StudySession, error)
	SaveSummary(s SessionSummary) error
	ListSummaries(userID int64) ([]SessionSummary, error)
}

// SummaryService handles business logic for study session recording and summary generation.
type SummaryService struct {
	store SummaryStoreInterface
}

// NewSummaryService creates a SummaryService instance.
func NewSummaryService(store SummaryStoreInterface) *SummaryService {
	return &SummaryService{store: store}
}

// RecordSession persists a study session.
func (s *SummaryService) RecordSession(session StudySession) error {
	slog.Debug("SummaryService.RecordSession called", "session_id", session.SessionID, "user_id", session.UserID)

	if err := s.store.SaveSession(session); err != nil {
		slog.Error("SummaryService.RecordSession: SaveSession failed", "err", err, "session_id", session.SessionID)
		return fmt.Errorf("summary.SummaryService.RecordSession SaveSession: %w", err)
	}

	slog.Debug("SummaryService.RecordSession done", "session_id", session.SessionID)
	return nil
}

// GenerateSummary builds and persists a SessionSummary for the given session.
// It looks up the session to validate it exists, then creates the summary.
func (s *SummaryService) GenerateSummary(userID int64, sessionID string, module ModuleType, scores ScoreSummary) (SessionSummary, error) {
	slog.Debug("SummaryService.GenerateSummary called", "user_id", userID, "session_id", sessionID)

	_, err := s.store.GetSession(sessionID)
	if err != nil {
		slog.Error("SummaryService.GenerateSummary: GetSession failed", "err", err, "session_id", sessionID)
		return SessionSummary{}, fmt.Errorf("summary.SummaryService.GenerateSummary GetSession: %w", err)
	}

	sum := SessionSummary{
		UserID:                 userID,
		SessionID:              sessionID,
		Module:                 module,
		ScoreSummary:           scores,
		Strengths:              []SummaryItem{},
		Weaknesses:             []SummaryItem{},
		ImprovementSuggestions: []string{},
		GeneratedAt:            time.Now(),
	}

	if err := s.store.SaveSummary(sum); err != nil {
		slog.Error("SummaryService.GenerateSummary: SaveSummary failed", "err", err, "session_id", sessionID)
		return SessionSummary{}, fmt.Errorf("summary.SummaryService.GenerateSummary SaveSummary: %w", err)
	}

	slog.Debug("SummaryService.GenerateSummary done", "user_id", userID, "session_id", sessionID)
	return sum, nil
}

// ListSummaries returns all session summaries for the user.
func (s *SummaryService) ListSummaries(userID int64) ([]SessionSummary, error) {
	slog.Debug("SummaryService.ListSummaries called", "user_id", userID)

	summaries, err := s.store.ListSummaries(userID)
	if err != nil {
		slog.Error("SummaryService.ListSummaries: failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("summary.SummaryService.ListSummaries: %w", err)
	}

	slog.Debug("SummaryService.ListSummaries done", "user_id", userID, "count", len(summaries))
	return summaries, nil
}
