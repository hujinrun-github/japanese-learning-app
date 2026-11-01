package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"japanese-learning-app/internal/module/summary"
	"japanese-learning-app/internal/store"
)

// SessionStore implements summary.SummaryStoreInterface on top of PostgreSQL.
type SessionStore struct {
	db  queryer
	now func() time.Time
}

// NewSessionStore creates a SessionStore.
func NewSessionStore(db queryer, deps store.StoreDeps) *SessionStore {
	return &SessionStore{
		db:  db,
		now: currentTimeFunc(deps.Clock),
	}
}

// SaveSession persists a study session, generating a random session ID.
func (s *SessionStore) SaveSession(sess summary.StudySession) error {
	sessionID := sess.SessionID
	if sessionID == "" {
		generated, err := generateSessionID()
		if err != nil {
			return fmt.Errorf("postgres.SessionStore.SaveSession generate id: %w", err)
		}
		sessionID = generated
	}

	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO study_sessions (session_id, user_id, module, duration_seconds, completed_count, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		sessionID,
		sess.UserID,
		sess.Module,
		sess.DurationSeconds,
		sess.CompletedCount,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.SessionStore.SaveSession: %w", translateError(err))
	}
	sess.SessionID = sessionID
	return nil
}

// GetSession retrieves a study session by its public session ID.
func (s *SessionStore) GetSession(sessionID string) (*summary.StudySession, error) {
	var sess summary.StudySession
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT id, session_id, user_id, module, duration_seconds, completed_count, started_at
		 FROM study_sessions WHERE session_id = $1`,
		sessionID,
	).Scan(
		&sess.ID,
		&sess.SessionID,
		&sess.UserID,
		&sess.Module,
		&sess.DurationSeconds,
		&sess.CompletedCount,
		&sess.StartedAt,
	)
	if err != nil {
		if errors.Is(translateError(err), sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres.SessionStore.GetSession %q: %w", sessionID, sql.ErrNoRows)
		}
		return nil, fmt.Errorf("postgres.SessionStore.GetSession: %w", translateError(err))
	}
	return &sess, nil
}

// SaveSummary persists a generated session summary.
func (s *SessionStore) SaveSummary(sum summary.SessionSummary) error {
	scoreSummary, err := json.Marshal(sum.ScoreSummary)
	if err != nil {
		return fmt.Errorf("postgres.SessionStore.SaveSummary marshal score_summary: %w", err)
	}
	strengths, err := marshalJSONList(sum.Strengths)
	if err != nil {
		return fmt.Errorf("postgres.SessionStore.SaveSummary marshal strengths: %w", err)
	}
	weaknesses, err := marshalJSONList(sum.Weaknesses)
	if err != nil {
		return fmt.Errorf("postgres.SessionStore.SaveSummary marshal weaknesses: %w", err)
	}
	suggestions, err := marshalJSONList(sum.ImprovementSuggestions)
	if err != nil {
		return fmt.Errorf("postgres.SessionStore.SaveSummary marshal suggestions: %w", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO session_summaries
		   (user_id, session_id, module, score_summary_json, strengths_json, weaknesses_json, suggestions_json, generated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8)`,
		sum.UserID,
		sum.SessionID,
		sum.Module,
		string(scoreSummary),
		string(strengths),
		string(weaknesses),
		string(suggestions),
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.SessionStore.SaveSummary: %w", translateError(err))
	}
	return nil
}

// ListSummaries returns all summaries for a user, newest first.
func (s *SessionStore) ListSummaries(userID int64) ([]summary.SessionSummary, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, user_id, session_id, module, score_summary_json::text, strengths_json::text,
		        weaknesses_json::text, suggestions_json::text, generated_at
		 FROM session_summaries WHERE user_id = $1
		 ORDER BY generated_at DESC, id DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.SessionStore.ListSummaries query: %w", translateError(err))
	}
	defer rows.Close()

	items := make([]summary.SessionSummary, 0)
	for rows.Next() {
		item, err := scanSessionSummary(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.SessionStore.ListSummaries rows: %w", translateError(err))
	}
	return items, nil
}

type sessionSummaryScanner interface {
	Scan(dest ...any) error
}

func scanSessionSummary(row sessionSummaryScanner) (*summary.SessionSummary, error) {
	var sum summary.SessionSummary
	var scoreSummaryJSON, strengthsJSON, weaknessesJSON, suggestionsJSON string
	if err := row.Scan(
		&sum.ID,
		&sum.UserID,
		&sum.SessionID,
		&sum.Module,
		&scoreSummaryJSON,
		&strengthsJSON,
		&weaknessesJSON,
		&suggestionsJSON,
		&sum.GeneratedAt,
	); err != nil {
		return nil, fmt.Errorf("postgres.scanSessionSummary scan: %w", translateError(err))
	}
	if scoreSummaryJSON != "" {
		if err := json.Unmarshal([]byte(scoreSummaryJSON), &sum.ScoreSummary); err != nil {
			return nil, fmt.Errorf("postgres.scanSessionSummary unmarshal score_summary: %w", err)
		}
	}
	if err := json.Unmarshal([]byte(orJSONArray(strengthsJSON)), &sum.Strengths); err != nil {
		return nil, fmt.Errorf("postgres.scanSessionSummary unmarshal strengths: %w", err)
	}
	if err := json.Unmarshal([]byte(orJSONArray(weaknessesJSON)), &sum.Weaknesses); err != nil {
		return nil, fmt.Errorf("postgres.scanSessionSummary unmarshal weaknesses: %w", err)
	}
	if err := json.Unmarshal([]byte(orJSONArray(suggestionsJSON)), &sum.ImprovementSuggestions); err != nil {
		return nil, fmt.Errorf("postgres.scanSessionSummary unmarshal suggestions: %w", err)
	}
	return &sum, nil
}

func marshalJSONList(value any) ([]byte, error) {
	if value == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(value)
}

func orJSONArray(raw string) string {
	if raw == "" {
		return "[]"
	}
	return raw
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
