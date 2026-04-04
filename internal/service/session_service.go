package service

import (
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/summary"
)

// SessionStoreInterface defines the data access methods required by SessionService.
type SessionStoreInterface interface {
	CreateSession(sess summary.StudySession) (string, error)
	GetSessionData(sessionID string) (*summary.StudySession, error)
	SaveSummary(sum summary.SessionSummary) error
	GetSummary(sessionID string) (*summary.SessionSummary, error)
}

// SessionService handles business logic for study session tracking and summaries.
type SessionService struct {
	store SessionStoreInterface
}

// NewSessionService creates a SessionService instance.
func NewSessionService(store SessionStoreInterface) *SessionService {
	return &SessionService{store: store}
}

// CreateSession persists a new study session and returns its unique ID.
func (s *SessionService) CreateSession(sess summary.StudySession) (string, error) {
	slog.Debug("SessionService.CreateSession called", "user_id", sess.UserID, "module", sess.Module)

	sessionID, err := s.store.CreateSession(sess)
	if err != nil {
		slog.Error("SessionService.CreateSession: failed", "err", err)
		return "", fmt.Errorf("service.SessionService.CreateSession: %w", err)
	}

	slog.Debug("SessionService.CreateSession done", "session_id", sessionID, "user_id", sess.UserID)
	return sessionID, nil
}

// GetSessionData retrieves a study session by its ID.
func (s *SessionService) GetSessionData(sessionID string) (*summary.StudySession, error) {
	slog.Debug("SessionService.GetSessionData called", "session_id", sessionID)

	sess, err := s.store.GetSessionData(sessionID)
	if err != nil {
		slog.Error("SessionService.GetSessionData: failed", "err", err, "session_id", sessionID)
		return nil, fmt.Errorf("service.SessionService.GetSessionData: %w", err)
	}

	slog.Debug("SessionService.GetSessionData done", "session_id", sessionID, "user_id", sess.UserID)
	return sess, nil
}

// SaveSummary persists an AI-generated session summary.
func (s *SessionService) SaveSummary(sum summary.SessionSummary) error {
	slog.Debug("SessionService.SaveSummary called", "session_id", sum.SessionID, "user_id", sum.UserID)

	if err := s.store.SaveSummary(sum); err != nil {
		slog.Error("SessionService.SaveSummary: failed", "err", err)
		return fmt.Errorf("service.SessionService.SaveSummary: %w", err)
	}

	slog.Debug("SessionService.SaveSummary done", "session_id", sum.SessionID)
	return nil
}

// GetSummary retrieves the session summary for the given session ID.
func (s *SessionService) GetSummary(sessionID string) (*summary.SessionSummary, error) {
	slog.Debug("SessionService.GetSummary called", "session_id", sessionID)

	sum, err := s.store.GetSummary(sessionID)
	if err != nil {
		slog.Error("SessionService.GetSummary: failed", "err", err, "session_id", sessionID)
		return nil, fmt.Errorf("service.SessionService.GetSummary: %w", err)
	}

	slog.Debug("SessionService.GetSummary done", "session_id", sessionID)
	return sum, nil
}
