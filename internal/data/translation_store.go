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

// DeleteSource deletes a source and its sentences/records in a transaction.
func (s *TranslationStore) DeleteSource(id int64) error {
	slog.Debug("TranslationStore.DeleteSource called", "source_id", id)

	tx, err := s.db.Begin()
	if err != nil {
		slog.Error("TranslationStore.DeleteSource begin tx failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSource: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete practice records for sentences belonging to this source.
	_, err = tx.Exec(
		`DELETE FROM translation_records WHERE sentence_id IN (SELECT id FROM translation_sentences WHERE source_id = ?)`,
		id,
	)
	if err != nil {
		slog.Error("TranslationStore.DeleteSource delete records failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSource: delete records: %w", err)
	}

	// Delete sentences (cascade is defined on sentences, but explicit for clarity).
	_, err = tx.Exec(`DELETE FROM translation_sentences WHERE source_id = ?`, id)
	if err != nil {
		slog.Error("TranslationStore.DeleteSource delete sentences failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSource: delete sentences: %w", err)
	}

	// Delete the source.
	result, err := tx.Exec(`DELETE FROM translation_sources WHERE id = ?`, id)
	if err != nil {
		slog.Error("TranslationStore.DeleteSource delete source failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSource: delete source: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("data.TranslationStore.DeleteSource: source %d not found", id)
	}

	if err := tx.Commit(); err != nil {
		slog.Error("TranslationStore.DeleteSource commit failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSource: commit: %w", err)
	}

	slog.Debug("TranslationStore.DeleteSource done", "source_id", id)
	return nil
}

// UpdateSentenceReference sets the reference translation for a sentence.
func (s *TranslationStore) UpdateSentenceReference(sentenceID int64, reference string) error {
	slog.Debug("TranslationStore.UpdateSentenceReference called", "sentence_id", sentenceID)

	_, err := s.db.Exec(
		`UPDATE translation_sentences SET reference_translation = ? WHERE id = ?`,
		reference, sentenceID,
	)
	if err != nil {
		return fmt.Errorf("data.TranslationStore.UpdateSentenceReference: %w", err)
	}

	slog.Debug("TranslationStore.UpdateSentenceReference done", "sentence_id", sentenceID)
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

// UpdateSentence 更新翻译句子的所有字段。
func (s *TranslationStore) UpdateSentence(sent translation.TranslationSentence) error {
	slog.Debug("TranslationStore.UpdateSentence called", "id", sent.ID)
	_, err := s.db.Exec(
		`UPDATE translation_sentences SET source_id=?, direction=?, source_text=?, reference_translation=?, position=? WHERE id=?`,
		sent.SourceID, sent.Direction, sent.SourceText, sent.ReferenceTranslation, sent.Position, sent.ID,
	)
	if err != nil {
		slog.Error("TranslationStore.UpdateSentence failed", "err", err, "id", sent.ID)
		return fmt.Errorf("data.TranslationStore.UpdateSentence: %w", err)
	}
	slog.Debug("TranslationStore.UpdateSentence done", "id", sent.ID)
	return nil
}

// DeleteSentence 按 ID 删除翻译句子及其关联的练习记录。
func (s *TranslationStore) DeleteSentence(id int64) error {
	slog.Debug("TranslationStore.DeleteSentence called", "id", id)

	tx, err := s.db.Begin()
	if err != nil {
		slog.Error("TranslationStore.DeleteSentence begin tx failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSentence: begin tx: %w", err)
	}
	defer tx.Rollback()

	// 删除关联的练习记录
	_, err = tx.Exec(`DELETE FROM translation_records WHERE sentence_id = ?`, id)
	if err != nil {
		slog.Error("TranslationStore.DeleteSentence delete records failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSentence: delete records: %w", err)
	}

	// 删除句子
	_, err = tx.Exec(`DELETE FROM translation_sentences WHERE id = ?`, id)
	if err != nil {
		slog.Error("TranslationStore.DeleteSentence delete sentence failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSentence: delete sentence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		slog.Error("TranslationStore.DeleteSentence commit failed", "err", err)
		return fmt.Errorf("data.TranslationStore.DeleteSentence: commit: %w", err)
	}

	slog.Debug("TranslationStore.DeleteSentence done", "id", id)
	return nil
}

// ListAllSentences 分页查询翻译句子，支持按 direction 和 source_id 过滤。
func (s *TranslationStore) ListAllSentences(sourceID int64, direction string, offset, limit int) ([]translation.TranslationSentence, int, error) {
	where := "WHERE 1=1"
	var args []any
	if sourceID > 0 {
		where += " AND source_id = ?"
		args = append(args, sourceID)
	}
	if direction != "" {
		where += " AND direction = ?"
		args = append(args, direction)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM translation_sentences "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("data.TranslationStore.ListAllSentences count: %w", err)
	}

	query := fmt.Sprintf("SELECT id, source_id, direction, source_text, reference_translation, position FROM translation_sentences %s ORDER BY id LIMIT ? OFFSET ?", where)
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("data.TranslationStore.ListAllSentences query: %w", err)
	}
	defer rows.Close()

	sentences := make([]translation.TranslationSentence, 0)
	for rows.Next() {
		var sent translation.TranslationSentence
		if err := rows.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position); err != nil {
			return nil, 0, fmt.Errorf("data.TranslationStore.ListAllSentences scan: %w", err)
		}
		sentences = append(sentences, sent)
	}

	slog.Debug("TranslationStore.ListAllSentences done", "count", len(sentences), "total", total)
	return sentences, total, rows.Err()
}
