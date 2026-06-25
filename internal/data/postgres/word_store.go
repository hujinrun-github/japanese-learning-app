package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"japanese-learning-app/internal/module/word"
	"japanese-learning-app/internal/store"
)

type WordStore struct {
	db       queryer
	now      func() time.Time
	location *time.Location
}

func NewWordStore(db queryer, deps store.StoreDeps) *WordStore {
	return &WordStore{
		db:       db,
		now:      currentTimeFunc(deps.Clock),
		location: appLocation(deps.AppTimezone),
	}
}

func (s *WordStore) GetByID(id int64) (*word.Word, error) {
	ctx := context.Background()

	var item word.Word
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, kanji_form, reading, part_of_speech, meaning, jlpt_level, reading_type
		 FROM words
		 WHERE id = $1`,
		id,
	).Scan(
		&item.ID,
		&item.KanjiForm,
		&item.Reading,
		&item.PartOfSpeech,
		&item.Meaning,
		&item.JLPTLevel,
		&item.ReadingType,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.WordStore.GetByID: %w", translateError(err))
	}

	examples, err := s.loadExamplesByWordID(ctx, id)
	if err != nil {
		return nil, err
	}
	item.Examples = examples

	return &item, nil
}

func (s *WordStore) ListByLevel(level word.JLPTLevel) ([]word.Word, error) {
	ctx := context.Background()

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, kanji_form, reading, part_of_speech, meaning, jlpt_level, reading_type
		 FROM words
		 WHERE jlpt_level = $1
		 ORDER BY id`,
		level,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.WordStore.ListByLevel query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := s.scanWords(ctx, rows)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func (s *WordStore) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	record, err := scanWordRecord(s.db.QueryRowContext(
		context.Background(),
		`SELECT id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval_days, review_history_json::text, updated_at
		 FROM word_records
		 WHERE user_id = $1 AND word_id = $2`,
		userID,
		wordID,
	))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

func (s *WordStore) ListDueRecords(userID int64) ([]word.WordRecord, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval_days, review_history_json::text, updated_at
		 FROM word_records
		 WHERE user_id = $1 AND next_review_at <= $2
		 ORDER BY next_review_at ASC`,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.WordStore.ListDueRecords query: %w", translateError(err))
	}
	defer rows.Close()

	var records []word.WordRecord
	for rows.Next() {
		record, err := scanWordRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.WordStore.ListDueRecords rows: %w", translateError(err))
	}

	return records, nil
}

func (s *WordStore) UpsertRecord(record word.WordRecord) error {
	historyJSON, err := json.Marshal(record.ReviewHistory)
	if err != nil {
		return fmt.Errorf("postgres.WordStore.UpsertRecord marshal history: %w", err)
	}

	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = s.now().UTC()
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO word_records (user_id, word_id, mastery_level, next_review_at, ease_factor, interval_days, review_history_json, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		 ON CONFLICT (user_id, word_id) DO UPDATE SET
		     mastery_level = EXCLUDED.mastery_level,
		     next_review_at = EXCLUDED.next_review_at,
		     ease_factor = EXCLUDED.ease_factor,
		     interval_days = EXCLUDED.interval_days,
		     review_history_json = EXCLUDED.review_history_json,
		     updated_at = EXCLUDED.updated_at`,
		record.UserID,
		record.WordID,
		record.MasteryLevel,
		record.NextReviewAt.UTC(),
		record.EaseFactor,
		record.Interval,
		string(historyJSON),
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres.WordStore.UpsertRecord: %w", translateError(err))
	}

	return nil
}

func (s *WordStore) BookmarkWord(userID, wordID int64) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO word_bookmarks (user_id, word_id)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, word_id) DO NOTHING`,
		userID,
		wordID,
	)
	if err != nil {
		return fmt.Errorf("postgres.WordStore.BookmarkWord: %w", translateError(err))
	}
	return nil
}

func (s *WordStore) ListAll(level word.JLPTLevel, search string, offset, limit int) ([]word.Word, int, error) {
	where, args := buildWordListFilter(level, search)

	total, err := s.countWords(`SELECT COUNT(*) FROM words `+where, args...)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, kanji_form, reading, part_of_speech, meaning, jlpt_level, reading_type
		 FROM words
		 %s
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.WordStore.ListAll query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := s.scanWords(context.Background(), rows)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (s *WordStore) InsertWord(item word.Word) (int64, error) {
	var id int64

	err := withQueryerTx(context.Background(), s.db, func(tx queryer) error {
		err := tx.QueryRowContext(
			context.Background(),
			`INSERT INTO words (kanji_form, reading, part_of_speech, meaning, jlpt_level, reading_type, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id`,
			item.KanjiForm,
			item.Reading,
			item.PartOfSpeech,
			item.Meaning,
			item.JLPTLevel,
			item.ReadingType,
			s.now().UTC(),
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("postgres.WordStore.InsertWord insert: %w", translateError(err))
		}

		return replaceWordExamples(context.Background(), tx, id, item.Examples)
	})
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *WordStore) UpdateWord(item word.Word) error {
	return withQueryerTx(context.Background(), s.db, func(tx queryer) error {
		_, err := tx.ExecContext(
			context.Background(),
			`UPDATE words
			 SET kanji_form = $2,
			     reading = $3,
			     part_of_speech = $4,
			     meaning = $5,
			     jlpt_level = $6,
			     reading_type = $7,
			     updated_at = $8
			 WHERE id = $1`,
			item.ID,
			item.KanjiForm,
			item.Reading,
			item.PartOfSpeech,
			item.Meaning,
			item.JLPTLevel,
			item.ReadingType,
			s.now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("postgres.WordStore.UpdateWord update: %w", translateError(err))
		}

		if _, err := tx.ExecContext(
			context.Background(),
			`DELETE FROM word_examples WHERE word_id = $1`,
			item.ID,
		); err != nil {
			return fmt.Errorf("postgres.WordStore.UpdateWord delete examples: %w", translateError(err))
		}

		return replaceWordExamples(context.Background(), tx, item.ID, item.Examples)
	})
}

func (s *WordStore) DeleteWord(id int64) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`DELETE FROM words WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("postgres.WordStore.DeleteWord: %w", translateError(err))
	}
	return nil
}

func (s *WordStore) ListAllRecords(userID int64, offset, limit int) ([]word.WordRecord, int, error) {
	queryArgs := []any{}
	where := ""
	if userID > 0 {
		where = "WHERE user_id = $1"
		queryArgs = append(queryArgs, userID)
	}

	totalQuery := `SELECT COUNT(*) FROM word_records`
	if where != "" {
		totalQuery += " " + where
	}
	total, err := s.countWords(totalQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}

	queryArgs = append(queryArgs, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval_days, review_history_json::text, updated_at
		 FROM word_records
		 %s
		 ORDER BY id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(queryArgs)-1,
		len(queryArgs),
	)

	rows, err := s.db.QueryContext(context.Background(), query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.WordStore.ListAllRecords query: %w", translateError(err))
	}
	defer rows.Close()

	var records []word.WordRecord
	for rows.Next() {
		record, err := scanWordRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("postgres.WordStore.ListAllRecords rows: %w", translateError(err))
	}

	return records, total, nil
}

func (s *WordStore) scanWords(ctx context.Context, rows *sql.Rows) ([]word.Word, error) {
	var items []word.Word
	for rows.Next() {
		var item word.Word
		if err := rows.Scan(
			&item.ID,
			&item.KanjiForm,
			&item.Reading,
			&item.PartOfSpeech,
			&item.Meaning,
			&item.JLPTLevel,
			&item.ReadingType,
		); err != nil {
			return nil, fmt.Errorf("postgres.WordStore.scanWords scan: %w", translateError(err))
		}

		examples, err := s.loadExamplesByWordID(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		item.Examples = examples

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.WordStore.scanWords rows: %w", translateError(err))
	}
	return items, nil
}

func (s *WordStore) loadExamplesByWordID(ctx context.Context, wordID int64) ([]word.WordExample, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT japanese, chinese, furigana_html
		 FROM word_examples
		 WHERE word_id = $1
		 ORDER BY position`,
		wordID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.WordStore.loadExamplesByWordID query: %w", translateError(err))
	}
	defer rows.Close()

	var examples []word.WordExample
	for rows.Next() {
		var example word.WordExample
		if err := rows.Scan(&example.Japanese, &example.Chinese, &example.FuriganaHTML); err != nil {
			return nil, fmt.Errorf("postgres.WordStore.loadExamplesByWordID scan: %w", translateError(err))
		}
		examples = append(examples, example)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.WordStore.loadExamplesByWordID rows: %w", translateError(err))
	}

	return examples, nil
}

func (s *WordStore) countWords(query string, args ...any) (int, error) {
	var count int
	if err := s.db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres.WordStore.countWords: %w", translateError(err))
	}
	return count, nil
}

func buildWordListFilter(level word.JLPTLevel, search string) (string, []any) {
	var clauses []string
	var args []any

	if level != "" {
		args = append(args, level)
		clauses = append(clauses, fmt.Sprintf("jlpt_level = $%d", len(args)))
	}

	if search != "" {
		like := "%" + search + "%"
		args = append(args, like)
		index := len(args)
		clauses = append(clauses, fmt.Sprintf("(kanji_form ILIKE $%[1]d OR reading ILIKE $%[1]d OR meaning ILIKE $%[1]d)", index))
	}

	if len(clauses) == 0 {
		return "", args
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func replaceWordExamples(ctx context.Context, db queryer, wordID int64, examples []word.WordExample) error {
	for idx, example := range examples {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO word_examples (word_id, position, japanese, chinese, furigana_html)
			 VALUES ($1, $2, $3, $4, $5)`,
			wordID,
			idx,
			example.Japanese,
			example.Chinese,
			example.FuriganaHTML,
		); err != nil {
			return fmt.Errorf("postgres.replaceWordExamples: %w", translateError(err))
		}
	}
	return nil
}

func scanWordRecord(scanner rowScanner) (*word.WordRecord, error) {
	var record word.WordRecord
	var historyJSON string
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.WordID,
		&record.MasteryLevel,
		&record.NextReviewAt,
		&record.EaseFactor,
		&record.Interval,
		&historyJSON,
		&record.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("postgres.scanWordRecord: %w", translateError(err))
	}

	if historyJSON != "" {
		if err := json.Unmarshal([]byte(historyJSON), &record.ReviewHistory); err != nil {
			return nil, fmt.Errorf("postgres.scanWordRecord unmarshal history: %w", err)
		}
	}

	return &record, nil
}
