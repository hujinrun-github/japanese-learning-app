package data

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/summary"
)

// SessionStore 实现学习会话与会话总结的数据访问，对应 study_sessions + session_summaries 表。
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore 创建 SessionStore 实例。
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// generateSessionID 生成 32 字节随机十六进制字符串作为会话 ID。
func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generateSessionID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateSession 创建新的学习会话记录，返回唯一 session_id。
func (s *SessionStore) CreateSession(sess summary.StudySession) (string, error) {
	slog.Debug("SessionStore.CreateSession called", "user_id", sess.UserID, "module", sess.Module)

	sessionID, err := generateSessionID()
	if err != nil {
		slog.Error("failed to generate session id", "err", err)
		return "", fmt.Errorf("data.SessionStore.CreateSession: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO study_sessions (session_id, user_id, module, duration_seconds, completed_count, started_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		sessionID, sess.UserID, sess.Module, sess.DurationSeconds, sess.CompletedCount,
	)
	if err != nil {
		slog.Error("failed to insert study_session", "err", err, "user_id", sess.UserID)
		return "", fmt.Errorf("data.SessionStore.CreateSession exec: %w", err)
	}

	slog.Debug("SessionStore.CreateSession done", "session_id", sessionID, "user_id", sess.UserID)
	return sessionID, nil
}

// GetSessionData 按 session_id 查询学习会话数据，不存在时返回 error。
func (s *SessionStore) GetSessionData(sessionID string) (*summary.StudySession, error) {
	slog.Debug("SessionStore.GetSessionData called", "session_id", sessionID)

	row := s.db.QueryRow(
		`SELECT id, session_id, user_id, module, duration_seconds, completed_count, started_at
		 FROM study_sessions WHERE session_id = ?`, sessionID,
	)

	var sess summary.StudySession
	var startedAt string
	err := row.Scan(&sess.ID, &sess.SessionID, &sess.UserID, &sess.Module,
		&sess.DurationSeconds, &sess.CompletedCount, &startedAt)
	if err == sql.ErrNoRows {
		slog.Error("study_session not found", "session_id", sessionID)
		return nil, fmt.Errorf("data.SessionStore.GetSessionData %q: %w", sessionID, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan study_session", "err", err, "session_id", sessionID)
		return nil, fmt.Errorf("data.SessionStore.GetSessionData: %w", err)
	}

	sess.StartedAt, err = parseSQLiteTime(startedAt)
	if err != nil {
		return nil, fmt.Errorf("data.SessionStore.GetSessionData parse started_at: %w", err)
	}

	slog.Debug("SessionStore.GetSessionData done", "session_id", sessionID, "user_id", sess.UserID)
	return &sess, nil
}

// SaveSummary 保存会话总结。
func (s *SessionStore) SaveSummary(sum summary.SessionSummary) error {
	slog.Debug("SessionStore.SaveSummary called", "session_id", sum.SessionID, "user_id", sum.UserID)

	scoreSummaryJSON, err := json.Marshal(sum.ScoreSummary)
	if err != nil {
		slog.Error("failed to marshal score_summary", "err", err)
		return fmt.Errorf("data.SessionStore.SaveSummary marshal score_summary: %w", err)
	}
	strengthsJSON, err := json.Marshal(sum.Strengths)
	if err != nil {
		slog.Error("failed to marshal strengths", "err", err)
		return fmt.Errorf("data.SessionStore.SaveSummary marshal strengths: %w", err)
	}
	weaknessesJSON, err := json.Marshal(sum.Weaknesses)
	if err != nil {
		slog.Error("failed to marshal weaknesses", "err", err)
		return fmt.Errorf("data.SessionStore.SaveSummary marshal weaknesses: %w", err)
	}
	suggestionsJSON, err := json.Marshal(sum.ImprovementSuggestions)
	if err != nil {
		slog.Error("failed to marshal improvement_suggestions", "err", err)
		return fmt.Errorf("data.SessionStore.SaveSummary marshal suggestions: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO session_summaries
		   (user_id, session_id, module, score_summary_json, strengths_json, weaknesses_json, suggestions_json, generated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		sum.UserID, sum.SessionID, sum.Module,
		string(scoreSummaryJSON), string(strengthsJSON), string(weaknessesJSON), string(suggestionsJSON),
	)
	if err != nil {
		slog.Error("failed to insert session_summary", "err", err, "session_id", sum.SessionID)
		return fmt.Errorf("data.SessionStore.SaveSummary exec: %w", err)
	}

	slog.Debug("SessionStore.SaveSummary done", "session_id", sum.SessionID)
	return nil
}

// ListSummaries 查询某用户的所有会话总结，按生成时间降序。
func (s *SessionStore) ListSummaries(userID int64) ([]summary.SessionSummary, error) {
	slog.Debug("SessionStore.ListSummaries called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, session_id, module, score_summary_json, strengths_json,
		        weaknesses_json, suggestions_json, generated_at
		 FROM session_summaries WHERE user_id = ? ORDER BY generated_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query session_summaries", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.SessionStore.ListSummaries query: %w", err)
	}
	defer rows.Close()

	var result []summary.SessionSummary
	for rows.Next() {
		var sum summary.SessionSummary
		var scoreSummaryJSON, strengthsJSON, weaknessesJSON, suggestionsJSON, generatedAt string
		if err := rows.Scan(&sum.ID, &sum.UserID, &sum.SessionID, &sum.Module,
			&scoreSummaryJSON, &strengthsJSON, &weaknessesJSON, &suggestionsJSON, &generatedAt); err != nil {
			slog.Error("failed to scan session_summary row", "err", err)
			return nil, fmt.Errorf("data.SessionStore.ListSummaries scan: %w", err)
		}
		if err := json.Unmarshal([]byte(scoreSummaryJSON), &sum.ScoreSummary); err != nil {
			return nil, fmt.Errorf("data.SessionStore.ListSummaries unmarshal score_summary: %w", err)
		}
		if err := json.Unmarshal([]byte(strengthsJSON), &sum.Strengths); err != nil {
			return nil, fmt.Errorf("data.SessionStore.ListSummaries unmarshal strengths: %w", err)
		}
		if err := json.Unmarshal([]byte(weaknessesJSON), &sum.Weaknesses); err != nil {
			return nil, fmt.Errorf("data.SessionStore.ListSummaries unmarshal weaknesses: %w", err)
		}
		if err := json.Unmarshal([]byte(suggestionsJSON), &sum.ImprovementSuggestions); err != nil {
			return nil, fmt.Errorf("data.SessionStore.ListSummaries unmarshal suggestions: %w", err)
		}
		var parseErr error
		sum.GeneratedAt, parseErr = parseSQLiteTime(generatedAt)
		if parseErr != nil {
			return nil, fmt.Errorf("data.SessionStore.ListSummaries parse generated_at: %w", parseErr)
		}
		result = append(result, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.SessionStore.ListSummaries rows: %w", err)
	}

	slog.Debug("SessionStore.ListSummaries done", "user_id", userID, "count", len(result))
	return result, nil
}

// GetSummary 按 session_id 查询会话总结，不存在时返回 error。
func (s *SessionStore) GetSummary(sessionID string) (*summary.SessionSummary, error) {
	slog.Debug("SessionStore.GetSummary called", "session_id", sessionID)

	row := s.db.QueryRow(
		`SELECT id, user_id, session_id, module, score_summary_json, strengths_json,
		        weaknesses_json, suggestions_json, generated_at
		 FROM session_summaries WHERE session_id = ?`, sessionID,
	)

	var sum summary.SessionSummary
	var scoreSummaryJSON, strengthsJSON, weaknessesJSON, suggestionsJSON, generatedAt string
	err := row.Scan(&sum.ID, &sum.UserID, &sum.SessionID, &sum.Module,
		&scoreSummaryJSON, &strengthsJSON, &weaknessesJSON, &suggestionsJSON, &generatedAt)

	if err == sql.ErrNoRows {
		slog.Error("session_summary not found", "session_id", sessionID)
		return nil, fmt.Errorf("data.SessionStore.GetSummary %q: %w", sessionID, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan session_summary", "err", err, "session_id", sessionID)
		return nil, fmt.Errorf("data.SessionStore.GetSummary: %w", err)
	}

	if err := json.Unmarshal([]byte(scoreSummaryJSON), &sum.ScoreSummary); err != nil {
		slog.Error("failed to unmarshal score_summary_json", "err", err)
		return nil, fmt.Errorf("data.SessionStore.GetSummary unmarshal score_summary: %w", err)
	}
	if err := json.Unmarshal([]byte(strengthsJSON), &sum.Strengths); err != nil {
		slog.Error("failed to unmarshal strengths_json", "err", err)
		return nil, fmt.Errorf("data.SessionStore.GetSummary unmarshal strengths: %w", err)
	}
	if err := json.Unmarshal([]byte(weaknessesJSON), &sum.Weaknesses); err != nil {
		slog.Error("failed to unmarshal weaknesses_json", "err", err)
		return nil, fmt.Errorf("data.SessionStore.GetSummary unmarshal weaknesses: %w", err)
	}
	if err := json.Unmarshal([]byte(suggestionsJSON), &sum.ImprovementSuggestions); err != nil {
		slog.Error("failed to unmarshal improvement_suggestions_json", "err", err)
		return nil, fmt.Errorf("data.SessionStore.GetSummary unmarshal suggestions: %w", err)
	}
	sum.GeneratedAt, err = parseSQLiteTime(generatedAt)
	if err != nil {
		return nil, fmt.Errorf("data.SessionStore.GetSummary parse generated_at: %w", err)
	}

	slog.Debug("SessionStore.GetSummary done", "session_id", sessionID)
	return &sum, nil
}
