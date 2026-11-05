package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/store"
)

type TranslationStore struct {
	db  queryer
	now func() time.Time
}

func NewTranslationStore(db queryer, deps store.StoreDeps) *TranslationStore {
	return &TranslationStore{
		db:  db,
		now: currentTimeFunc(deps.Clock),
	}
}

func (s *TranslationStore) ListAllSentences(sourceID int64, direction string, offset, limit int) ([]translation.TranslationSentence, int, error) {
	where, args := buildTranslationSentenceFilter(sourceID, direction)

	total, err := countRows(context.Background(), s.db, `SELECT COUNT(*) FROM translation_sentences `+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.TranslationStore.ListAllSentences count: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM translation_sentences
		 %s
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.TranslationStore.ListAllSentences query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanTranslationSentences(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *TranslationStore) SaveSentence(sent translation.TranslationSentence) (int64, error) {
	sourceID, err := s.ensureSourceID(sent.SourceID)
	if err != nil {
		return 0, err
	}

	var id int64
	err = s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO translation_sentences (source_id, direction, source_text, reference_translation, position, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		sourceID,
		sent.Direction,
		sent.SourceText,
		sent.ReferenceTranslation,
		sent.Position,
		s.now().UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.TranslationStore.SaveSentence: %w", translateError(err))
	}
	return id, nil
}

func (s *TranslationStore) UpdateSentence(sent translation.TranslationSentence) error {
	sourceID, err := s.ensureSourceID(sent.SourceID)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`UPDATE translation_sentences
		 SET source_id = $2,
		     direction = $3,
		     source_text = $4,
		     reference_translation = $5,
		     position = $6,
		     updated_at = $7
		 WHERE id = $1`,
		sent.ID,
		sourceID,
		sent.Direction,
		sent.SourceText,
		sent.ReferenceTranslation,
		sent.Position,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.TranslationStore.UpdateSentence: %w", translateError(err))
	}
	return nil
}

func (s *TranslationStore) DeleteSentence(id int64) error {
	return withQueryerTx(context.Background(), s.db, func(tx queryer) error {
		if _, err := tx.ExecContext(context.Background(), `DELETE FROM translation_records WHERE sentence_id = $1`, id); err != nil {
			return fmt.Errorf("postgres.TranslationStore.DeleteSentence delete records: %w", translateError(err))
		}
		if _, err := tx.ExecContext(context.Background(), `DELETE FROM translation_sentences WHERE id = $1`, id); err != nil {
			return fmt.Errorf("postgres.TranslationStore.DeleteSentence delete sentence: %w", translateError(err))
		}
		return nil
	})
}

func (s *TranslationStore) ensureSourceID(sourceID int64) (int64, error) {
	if sourceID > 0 {
		return sourceID, nil
	}

	var id int64
	err := s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content, created_at)
		 VALUES ($1, 'manual', '', '', '', $2)
		 RETURNING id`,
		"Admin Manual",
		s.now().UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.TranslationStore.ensureSourceID: %w", translateError(err))
	}
	return id, nil
}

func buildTranslationSentenceFilter(sourceID int64, direction string) (string, []any) {
	var clauses []string
	var args []any
	if sourceID > 0 {
		args = append(args, sourceID)
		clauses = append(clauses, fmt.Sprintf("source_id = $%d", len(args)))
	}
	if direction != "" {
		args = append(args, direction)
		clauses = append(clauses, fmt.Sprintf("direction = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// ── App-side methods (translation.StoreInterface) ──────────────────────────

func (s *TranslationStore) SaveSource(src translation.TranslationSource) (int64, error) {
	if src.SourceType == "" {
		src.SourceType = "manual"
	}

	var id int64
	err := s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		src.Title,
		src.SourceType,
		src.SourceURL,
		src.APIEndpoint,
		src.RawContent,
		s.now().UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.TranslationStore.SaveSource: %w", translateError(err))
	}
	return id, nil
}

func (s *TranslationStore) ListSources() ([]translation.TranslationSource, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, title, source_type, source_url, api_endpoint, raw_content, created_at
		 FROM translation_sources
		 ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.ListSources query: %w", translateError(err))
	}
	defer rows.Close()

	var items []translation.TranslationSource
	for rows.Next() {
		var item translation.TranslationSource
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.SourceType,
			&item.SourceURL,
			&item.APIEndpoint,
			&item.RawContent,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres.TranslationStore.ListSources scan: %w", translateError(err))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.ListSources rows: %w", translateError(err))
	}
	if items == nil {
		items = make([]translation.TranslationSource, 0)
	}
	return items, nil
}

func (s *TranslationStore) ListSentencesBySource(sourceID int64) ([]translation.TranslationSentence, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		// 练习题乱序返回，避免按 position 连续出题时相邻句子互相泄露上下文。
		`SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM translation_sentences
		 WHERE source_id = $1
		 ORDER BY RANDOM()`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.ListSentencesBySource query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanTranslationSentences(rows)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]translation.TranslationSentence, 0)
	}
	return items, nil
}

func (s *TranslationStore) GetSentenceByID(id int64) (*translation.TranslationSentence, error) {
	var item translation.TranslationSentence
	var pointsJSON string
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT id, source_id, direction, source_text, reference_translation, position, array_to_json(points)::text
		 FROM translation_sentences WHERE id = $1`,
		id,
	).Scan(
		&item.ID,
		&item.SourceID,
		&item.Direction,
		&item.SourceText,
		&item.ReferenceTranslation,
		&item.Position,
		&pointsJSON,
	)
	if err != nil {
		if errors.Is(translateError(err), sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres.TranslationStore.GetSentenceByID: %w", translateError(err))
	}
	if err := json.Unmarshal([]byte(pointsJSON), &item.GrammarPoints); err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.GetSentenceByID grammar points: %w", err)
	}
	return &item, nil
}

func (s *TranslationStore) ListUnpracticedSentences(userID, sourceID int64, limit int) ([]translation.TranslationSentence, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`WITH practiced AS (
		    SELECT DISTINCT ts.direction, TRIM(ts.source_text) AS prompt
		    FROM translation_records tr
		    JOIN translation_sentences ts ON ts.id = tr.sentence_id
		    WHERE tr.user_id = $1
		 ), candidates AS (
		    SELECT ts.id, ts.source_id, ts.direction, ts.source_text, ts.reference_translation, ts.position,
		    ROW_NUMBER() OVER (
		        PARTITION BY ts.direction, TRIM(ts.source_text) ORDER BY ts.position, ts.id
		    ) AS prompt_rank
		    FROM translation_sentences ts
		    WHERE ($2::bigint = 0 OR ts.source_id = $2)
		      AND NOT EXISTS (
		          SELECT 1 FROM practiced p
		          WHERE p.direction = ts.direction AND p.prompt = TRIM(ts.source_text)
		      )
		 )
		 SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM candidates
		 WHERE prompt_rank = 1
		 ORDER BY RANDOM()
		 LIMIT NULLIF($3::int, 0)`,
		userID, sourceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.ListUnpracticedSentences query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanTranslationSentences(rows)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]translation.TranslationSentence, 0)
	}
	return items, nil
}

func (s *TranslationStore) SaveRecord(r translation.TranslationRecord) (int64, error) {
	feedbackJSON := []byte("{}")
	if r.AIFeedback != nil {
		raw, err := json.Marshal(r.AIFeedback)
		if err != nil {
			return 0, fmt.Errorf("postgres.TranslationStore.SaveRecord marshal ai_feedback: %w", err)
		}
		feedbackJSON = raw
	}

	sent, err := s.GetSentenceByID(r.SentenceID)
	if err != nil {
		return 0, err
	}
	if sent == nil {
		return 0, fmt.Errorf("postgres.TranslationStore.SaveRecord sentence %d: %w", r.SentenceID, sql.ErrNoRows)
	}

	practicedAt := r.PracticedAt
	if practicedAt.IsZero() {
		practicedAt = s.now().UTC()
	}

	var id int64
	err = s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO translation_records
		 (user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json,
		  source_text_snapshot, direction_snapshot, reference_translation_snapshot, position_snapshot, practiced_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11)
		 RETURNING id`,
		r.UserID,
		r.SentenceID,
		r.UserTranslation,
		r.Score,
		r.RuleScore,
		string(feedbackJSON),
		sent.SourceText,
		sent.Direction,
		sent.ReferenceTranslation,
		sent.Position,
		practicedAt.UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.TranslationStore.SaveRecord: %w", translateError(err))
	}
	return id, nil
}

func (s *TranslationStore) GetRecord(id int64) (*translation.TranslationRecord, error) {
	row := s.db.QueryRowContext(
		context.Background(),
		`SELECT id, user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json::text, practiced_at,
		        reference_translation_snapshot
		 FROM translation_records WHERE id = $1`,
		id,
	)
	record, err := scanTranslationRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres.TranslationStore.GetRecord %d: %w", id, sql.ErrNoRows)
		}
		return nil, err
	}
	return record, nil
}

func (s *TranslationStore) ListRecords(userID int64) ([]translation.TranslationRecord, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json::text, practiced_at,
		        reference_translation_snapshot
		 FROM translation_records WHERE user_id = $1
		 ORDER BY practiced_at DESC, id DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.ListRecords query: %w", translateError(err))
	}
	defer rows.Close()

	items := make([]translation.TranslationRecord, 0)
	for rows.Next() {
		record, err := scanTranslationRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.TranslationStore.ListRecords rows: %w", translateError(err))
	}
	return items, nil
}

func (s *TranslationStore) UpdateRecordFeedback(recordID int64, score int, feedbackJSON string) error {
	if feedbackJSON == "" {
		feedbackJSON = "{}"
	}

	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE translation_records SET score = $2, ai_feedback_json = $3::jsonb WHERE id = $1`,
		recordID,
		score,
		feedbackJSON,
	)
	if err != nil {
		return fmt.Errorf("postgres.TranslationStore.UpdateRecordFeedback: %w", translateError(err))
	}
	return nil
}

func (s *TranslationStore) UpdateSentenceReference(sentenceID int64, reference string) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE translation_sentences SET reference_translation = $2 WHERE id = $1 AND TRIM(reference_translation) = ''`,
		sentenceID,
		reference,
	)
	if err != nil {
		return fmt.Errorf("postgres.TranslationStore.UpdateSentenceReference: %w", translateError(err))
	}
	return nil
}

func (s *TranslationStore) DeleteSource(id int64) error {
	return withQueryerTx(context.Background(), s.db, func(tx queryer) error {
		if _, err := tx.ExecContext(
			context.Background(),
			`DELETE FROM translation_records
			 WHERE sentence_id IN (SELECT id FROM translation_sentences WHERE source_id = $1)`,
			id,
		); err != nil {
			return fmt.Errorf("postgres.TranslationStore.DeleteSource delete records: %w", translateError(err))
		}
		if _, err := tx.ExecContext(
			context.Background(),
			`DELETE FROM translation_sentences WHERE source_id = $1`,
			id,
		); err != nil {
			return fmt.Errorf("postgres.TranslationStore.DeleteSource delete sentences: %w", translateError(err))
		}
		result, err := tx.ExecContext(context.Background(), `DELETE FROM translation_sources WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("postgres.TranslationStore.DeleteSource delete source: %w", translateError(err))
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("postgres.TranslationStore.DeleteSource rows affected: %w", translateError(err))
		}
		if affected == 0 {
			return fmt.Errorf("postgres.TranslationStore.DeleteSource: source %d: %w", id, sql.ErrNoRows)
		}
		return nil
	})
}

// scanTranslationRecord scans a single translation record row. The caller must
// guarantee the column order matches the query used above.
func scanTranslationRecord(row interface {
	Scan(dest ...any) error
}) (*translation.TranslationRecord, error) {
	var record translation.TranslationRecord
	var feedbackJSON string
	if err := row.Scan(
		&record.ID,
		&record.UserID,
		&record.SentenceID,
		&record.UserTranslation,
		&record.Score,
		&record.RuleScore,
		&feedbackJSON,
		&record.PracticedAt,
		&record.ReferenceTranslation,
	); err != nil {
		return nil, fmt.Errorf("postgres.scanTranslationRecord scan: %w", translateError(err))
	}
	if feedbackJSON != "" && feedbackJSON != "null" && feedbackJSON != "{}" {
		var feedback translation.TranslationFeedback
		if err := json.Unmarshal([]byte(feedbackJSON), &feedback); err != nil {
			return nil, fmt.Errorf("postgres.scanTranslationRecord unmarshal ai_feedback: %w", err)
		}
		record.AIFeedback = &feedback
	}
	return &record, nil
}

func scanTranslationSentences(rows *sql.Rows) ([]translation.TranslationSentence, error) {
	var items []translation.TranslationSentence
	for rows.Next() {
		var item translation.TranslationSentence
		if err := rows.Scan(
			&item.ID,
			&item.SourceID,
			&item.Direction,
			&item.SourceText,
			&item.ReferenceTranslation,
			&item.Position,
		); err != nil {
			return nil, fmt.Errorf("postgres.scanTranslationSentences scan: %w", translateError(err))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.scanTranslationSentences rows: %w", translateError(err))
	}
	return items, nil
}
