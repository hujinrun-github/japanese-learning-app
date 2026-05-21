package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/translation"
)

// TranslationStore implements translation.StoreInterface.
type TranslationStore struct {
	db *sql.DB
}

// NewTranslationStore creates a TranslationStore.
func NewTranslationStore(db *sql.DB) *TranslationStore {
	return &TranslationStore{db: db}
}

func (s *TranslationStore) SaveSource(src translation.TranslationSource) (int64, error) {
	slog.Debug("TranslationStore.SaveSource called", "title", src.Title, "source_type", src.SourceType)

	result, err := s.db.Exec(
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content)
		 VALUES (?, ?, ?, ?, ?)`,
		src.Title, src.SourceType, src.SourceURL, src.APIEndpoint, src.RawContent,
	)
	if err != nil {
		slog.Error("TranslationStore.SaveSource failed", "err", err)
		return 0, fmt.Errorf("data.TranslationStore.SaveSource: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.TranslationStore.SaveSource LastInsertId: %w", err)
	}

	slog.Debug("TranslationStore.SaveSource done", "id", id)
	return id, nil
}

func (s *TranslationStore) SaveSentence(sent translation.TranslationSentence) (int64, error) {
	slog.Debug("TranslationStore.SaveSentence called", "source_id", sent.SourceID, "direction", sent.Direction)

	result, err := s.db.Exec(
		`INSERT INTO translation_sentences (source_id, direction, source_text, reference_translation, position)
		 VALUES (?, ?, ?, ?, ?)`,
		sent.SourceID, sent.Direction, sent.SourceText, sent.ReferenceTranslation, sent.Position,
	)
	if err != nil {
		slog.Error("TranslationStore.SaveSentence failed", "err", err)
		return 0, fmt.Errorf("data.TranslationStore.SaveSentence: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.TranslationStore.SaveSentence LastInsertId: %w", err)
	}

	slog.Debug("TranslationStore.SaveSentence done", "id", id)
	return id, nil
}

func (s *TranslationStore) ListSources() ([]translation.TranslationSource, error) {
	slog.Debug("TranslationStore.ListSources called")

	rows, err := s.db.Query(
		`SELECT id, title, source_type, source_url, api_endpoint, raw_content, created_at
		 FROM translation_sources ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.ListSources: %w", err)
	}
	defer rows.Close()

	var sources []translation.TranslationSource
	for rows.Next() {
		var src translation.TranslationSource
		var createdAt string
		if err := rows.Scan(&src.ID, &src.Title, &src.SourceType, &src.SourceURL, &src.APIEndpoint, &src.RawContent, &createdAt); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListSources scan: %w", err)
		}
		src.CreatedAt, err = parseSQLiteTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListSources parse created_at: %w", err)
		}
		sources = append(sources, src)
	}

	slog.Debug("TranslationStore.ListSources done", "count", len(sources))
	return sources, rows.Err()
}

func (s *TranslationStore) ListSentencesBySource(sourceID int64) ([]translation.TranslationSentence, error) {
	slog.Debug("TranslationStore.ListSentencesBySource called", "source_id", sourceID)

	rows, err := s.db.Query(
		`SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM translation_sentences WHERE source_id = ? ORDER BY position`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.ListSentencesBySource: %w", err)
	}
	defer rows.Close()

	sentences := make([]translation.TranslationSentence, 0)
	for rows.Next() {
		var sent translation.TranslationSentence
		if err := rows.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListSentencesBySource scan: %w", err)
		}
		sentences = append(sentences, sent)
	}

	slog.Debug("TranslationStore.ListSentencesBySource done", "source_id", sourceID, "count", len(sentences))
	return sentences, rows.Err()
}

func (s *TranslationStore) GetSentenceByID(id int64) (*translation.TranslationSentence, error) {
	slog.Debug("TranslationStore.GetSentenceByID called", "id", id)

	row := s.db.QueryRow(
		`SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM translation_sentences WHERE id = ?`, id,
	)
	var sent translation.TranslationSentence
	err := row.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position)
	if err == sql.ErrNoRows {
		slog.Debug("translation_sentence not found", "id", id)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetSentenceByID: %w", err)
	}

	slog.Debug("TranslationStore.GetSentenceByID done", "id", id)
	return &sent, nil
}

func (s *TranslationStore) GetDailyQueue(userID int64, limit int) ([]translation.TranslationSentence, error) {
	slog.Debug("TranslationStore.GetDailyQueue called", "user_id", userID, "limit", limit)

	rows, err := s.db.Query(
		`SELECT ts.id, ts.source_id, ts.direction, ts.source_text, ts.reference_translation, ts.position
		 FROM translation_sentences ts
		 WHERE ts.id NOT IN (
		 	SELECT tr.sentence_id FROM translation_records tr
		 	WHERE tr.user_id = ? AND tr.score >= 80
		 )
		 ORDER BY RANDOM()
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetDailyQueue: %w", err)
	}
	defer rows.Close()

	sentences := make([]translation.TranslationSentence, 0)
	for rows.Next() {
		var sent translation.TranslationSentence
		if err := rows.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.GetDailyQueue scan: %w", err)
		}
		sentences = append(sentences, sent)
	}

	slog.Debug("TranslationStore.GetDailyQueue done", "user_id", userID, "count", len(sentences))
	return sentences, rows.Err()
}

func (s *TranslationStore) SaveRecord(r translation.TranslationRecord) (int64, error) {
	slog.Debug("TranslationStore.SaveRecord called", "user_id", r.UserID, "sentence_id", r.SentenceID)

	aiJSON := ""
	if r.AIFeedback != nil {
		raw, err := json.Marshal(r.AIFeedback)
		if err != nil {
			return 0, fmt.Errorf("data.TranslationStore.SaveRecord marshal ai_feedback: %w", err)
		}
		aiJSON = string(raw)
	}

	result, err := s.db.Exec(
		`INSERT OR REPLACE INTO translation_records
		 (user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json, practiced_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		r.UserID, r.SentenceID, r.UserTranslation, r.Score, r.RuleScore, aiJSON,
	)
	if err != nil {
		slog.Error("TranslationStore.SaveRecord failed", "err", err)
		return 0, fmt.Errorf("data.TranslationStore.SaveRecord: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.TranslationStore.SaveRecord LastInsertId: %w", err)
	}

	slog.Debug("TranslationStore.SaveRecord done", "id", id)
	return id, nil
}

func (s *TranslationStore) UpdateRecordFeedback(recordID int64, score int, feedbackJSON string) error {
	slog.Debug("TranslationStore.UpdateRecordFeedback called", "record_id", recordID)

	_, err := s.db.Exec(
		`UPDATE translation_records SET score = ?, ai_feedback_json = ? WHERE id = ?`,
		score, feedbackJSON, recordID,
	)
	if err != nil {
		return fmt.Errorf("data.TranslationStore.UpdateRecordFeedback: %w", err)
	}

	slog.Debug("TranslationStore.UpdateRecordFeedback done", "record_id", recordID)
	return nil
}

func (s *TranslationStore) GetRecord(id int64) (*translation.TranslationRecord, error) {
	slog.Debug("TranslationStore.GetRecord called", "id", id)

	row := s.db.QueryRow(
		`SELECT id, user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json, practiced_at
		 FROM translation_records WHERE id = ?`, id,
	)
	var r translation.TranslationRecord
	var aiJSON string
	var practicedAt string
	err := row.Scan(&r.ID, &r.UserID, &r.SentenceID, &r.UserTranslation, &r.Score, &r.RuleScore, &aiJSON, &practicedAt)
	if err == sql.ErrNoRows {
		slog.Debug("translation_record not found", "id", id)
		return nil, fmt.Errorf("data.TranslationStore.GetRecord %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetRecord: %w", err)
	}

	r.PracticedAt, err = parseSQLiteTime(practicedAt)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetRecord parse practiced_at: %w", err)
	}
	if aiJSON != "" {
		var fb translation.TranslationFeedback
		if err := json.Unmarshal([]byte(aiJSON), &fb); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.GetRecord unmarshal ai_feedback: %w", err)
		}
		r.AIFeedback = &fb
	}

	slog.Debug("TranslationStore.GetRecord done", "id", id, "user_id", r.UserID)
	return &r, nil
}

func (s *TranslationStore) ListRecords(userID int64) ([]translation.TranslationRecord, error) {
	slog.Debug("TranslationStore.ListRecords called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json, practiced_at
		 FROM translation_records WHERE user_id = ? ORDER BY practiced_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.ListRecords: %w", err)
	}
	defer rows.Close()

	records := make([]translation.TranslationRecord, 0)
	for rows.Next() {
		var r translation.TranslationRecord
		var aiJSON string
		var practicedAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.SentenceID, &r.UserTranslation, &r.Score, &r.RuleScore, &aiJSON, &practicedAt); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListRecords scan: %w", err)
		}
		r.PracticedAt, err = parseSQLiteTime(practicedAt)
		if err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListRecords parse practiced_at: %w", err)
		}
		if aiJSON != "" {
			var fb translation.TranslationFeedback
			if err := json.Unmarshal([]byte(aiJSON), &fb); err != nil {
				return nil, fmt.Errorf("data.TranslationStore.ListRecords unmarshal ai_feedback: %w", err)
			}
			r.AIFeedback = &fb
		}
		records = append(records, r)
	}

	slog.Debug("TranslationStore.ListRecords done", "user_id", userID, "count", len(records))
	return records, rows.Err()
}
