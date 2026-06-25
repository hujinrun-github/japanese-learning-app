package postgres

import (
	"context"
	"database/sql"
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
