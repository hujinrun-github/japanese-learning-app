package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/store"
)

type GrammarStore struct {
	db       queryer
	now      func() time.Time
	location *time.Location
}

func NewGrammarStore(db queryer, deps store.StoreDeps) *GrammarStore {
	return &GrammarStore{
		db:       db,
		now:      currentTimeFunc(deps.Clock),
		location: appLocation(deps.AppTimezone),
	}
}

func (s *GrammarStore) GetByID(id int64) (*grammar.GrammarPoint, error) {
	ctx := context.Background()

	var point grammar.GrammarPoint
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, meaning, conjunction_rule, usage_note, jlpt_level
		 FROM grammar_points
		 WHERE id = $1`,
		id,
	).Scan(
		&point.ID,
		&point.Name,
		&point.Meaning,
		&point.ConjunctionRule,
		&point.UsageNote,
		&point.JLPTLevel,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.GetByID: %w", translateError(err))
	}

	examples, err := s.loadExamplesByGrammarPointID(ctx, id)
	if err != nil {
		return nil, err
	}
	quizQuestions, err := s.loadQuizQuestionsByGrammarPointID(ctx, id)
	if err != nil {
		return nil, err
	}
	point.Examples = examples
	point.QuizQuestions = quizQuestions

	return &point, nil
}

func (s *GrammarStore) ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, name, meaning, conjunction_rule, usage_note, jlpt_level
		 FROM grammar_points
		 WHERE jlpt_level = $1
		 ORDER BY id`,
		level,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.ListByLevel query: %w", translateError(err))
	}
	defer rows.Close()

	return s.scanGrammarPoints(context.Background(), rows)
}

func (s *GrammarStore) ListByLevelWithStatus(userID int64, level grammar.JLPTLevel) ([]grammar.GrammarPointWithStatus, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT gp.id, gp.name, gp.meaning, gp.conjunction_rule, gp.usage_note, gp.jlpt_level,
		        COALESCE(gr.status, 'unlearned'),
		        gr.quiz_history_json::text
		 FROM grammar_points gp
		 LEFT JOIN grammar_records gr ON gr.grammar_point_id = gp.id AND gr.user_id = $1
		 WHERE gp.jlpt_level = $2
		 ORDER BY gp.id`,
		userID,
		level,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.ListByLevelWithStatus query: %w", translateError(err))
	}
	defer rows.Close()

	var items []grammar.GrammarPointWithStatus
	for rows.Next() {
		var item grammar.GrammarPointWithStatus
		var historyJSON sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Meaning,
			&item.ConjunctionRule,
			&item.UsageNote,
			&item.JLPTLevel,
			&item.UserStatus,
			&historyJSON,
		); err != nil {
			return nil, fmt.Errorf("postgres.GrammarStore.ListByLevelWithStatus scan: %w", translateError(err))
		}

		examples, err := s.loadExamplesByGrammarPointID(context.Background(), item.ID)
		if err != nil {
			return nil, err
		}
		quizQuestions, err := s.loadQuizQuestionsByGrammarPointID(context.Background(), item.ID)
		if err != nil {
			return nil, err
		}
		item.Examples = examples
		item.QuizQuestions = quizQuestions
		item.LastQuizScore = lastGrammarQuizScore(historyJSON)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.ListByLevelWithStatus rows: %w", translateError(err))
	}

	return items, nil
}

func (s *GrammarStore) UpdatePoint(point grammar.GrammarPoint) error {
	return withQueryerTx(context.Background(), s.db, func(tx queryer) error {
		_, err := tx.ExecContext(
			context.Background(),
			`UPDATE grammar_points
			 SET name = $2,
			     meaning = $3,
			     conjunction_rule = $4,
			     usage_note = $5,
			     jlpt_level = $6,
			     updated_at = $7
			 WHERE id = $1`,
			point.ID,
			point.Name,
			point.Meaning,
			point.ConjunctionRule,
			point.UsageNote,
			point.JLPTLevel,
			s.now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("postgres.GrammarStore.UpdatePoint update: %w", translateError(err))
		}

		if _, err := tx.ExecContext(context.Background(), `DELETE FROM grammar_examples WHERE grammar_point_id = $1`, point.ID); err != nil {
			return fmt.Errorf("postgres.GrammarStore.UpdatePoint delete examples: %w", translateError(err))
		}
		if _, err := tx.ExecContext(context.Background(), `DELETE FROM grammar_quiz_questions WHERE grammar_point_id = $1`, point.ID); err != nil {
			return fmt.Errorf("postgres.GrammarStore.UpdatePoint delete quiz questions: %w", translateError(err))
		}

		if err := replaceGrammarExamples(context.Background(), tx, point.ID, point.Examples); err != nil {
			return err
		}
		return replaceGrammarQuizQuestions(context.Background(), tx, point.ID, point.QuizQuestions)
	})
}

func (s *GrammarStore) DeletePoint(id int64) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM grammar_points WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres.GrammarStore.DeletePoint: %w", translateError(err))
	}
	return nil
}

func (s *GrammarStore) GetRecord(userID, grammarPointID int64) (*grammar.GrammarRecord, error) {
	record, err := scanGrammarRecord(s.db.QueryRowContext(
		context.Background(),
		`SELECT id, user_id, grammar_point_id, status, next_review_at, quiz_history_json::text
		 FROM grammar_records
		 WHERE user_id = $1 AND grammar_point_id = $2`,
		userID,
		grammarPointID,
	))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

func (s *GrammarStore) UpsertRecord(record grammar.GrammarRecord) error {
	historyJSON, err := json.Marshal(record.QuizHistory)
	if err != nil {
		return fmt.Errorf("postgres.GrammarStore.UpsertRecord marshal history: %w", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO grammar_records (user_id, grammar_point_id, status, next_review_at, quiz_history_json, updated_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		 ON CONFLICT (user_id, grammar_point_id) DO UPDATE SET
		     status = EXCLUDED.status,
		     next_review_at = EXCLUDED.next_review_at,
		     quiz_history_json = EXCLUDED.quiz_history_json,
		     updated_at = EXCLUDED.updated_at`,
		record.UserID,
		record.GrammarPointID,
		record.Status,
		record.NextReviewAt.UTC(),
		string(historyJSON),
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.GrammarStore.UpsertRecord: %w", translateError(err))
	}
	return nil
}

func (s *GrammarStore) ListDueRecords(userID int64) ([]grammar.GrammarRecord, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, user_id, grammar_point_id, status, next_review_at, quiz_history_json::text
		 FROM grammar_records
		 WHERE user_id = $1 AND next_review_at <= $2
		 ORDER BY next_review_at ASC`,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.ListDueRecords query: %w", translateError(err))
	}
	defer rows.Close()

	var records []grammar.GrammarRecord
	for rows.Next() {
		record, err := scanGrammarRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.ListDueRecords rows: %w", translateError(err))
	}

	return records, nil
}

func (s *GrammarStore) ListAll(level, search string, offset, limit int) ([]grammar.GrammarPoint, int, error) {
	where, args := buildGrammarListFilter(level, search)

	total, err := s.countGrammarRows(`SELECT COUNT(*) FROM grammar_points `+where, args...)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, name, meaning, conjunction_rule, usage_note, jlpt_level
		 FROM grammar_points
		 %s
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.GrammarStore.ListAll query: %w", translateError(err))
	}
	defer rows.Close()

	points, err := s.scanGrammarPoints(context.Background(), rows)
	if err != nil {
		return nil, 0, err
	}

	return points, total, nil
}

func (s *GrammarStore) InsertPoint(point grammar.GrammarPoint) (int64, error) {
	var id int64

	err := withQueryerTx(context.Background(), s.db, func(tx queryer) error {
		err := tx.QueryRowContext(
			context.Background(),
			`INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, jlpt_level, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id`,
			point.Name,
			point.Meaning,
			point.ConjunctionRule,
			point.UsageNote,
			point.JLPTLevel,
			s.now().UTC(),
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("postgres.GrammarStore.InsertPoint insert: %w", translateError(err))
		}

		if err := replaceGrammarExamples(context.Background(), tx, id, point.Examples); err != nil {
			return err
		}
		return replaceGrammarQuizQuestions(context.Background(), tx, id, point.QuizQuestions)
	})
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *GrammarStore) ListAllRecords(userID int64, offset, limit int) ([]grammar.GrammarRecord, int, error) {
	queryArgs := []any{}
	where := ""
	if userID > 0 {
		where = "WHERE user_id = $1"
		queryArgs = append(queryArgs, userID)
	}

	totalQuery := `SELECT COUNT(*) FROM grammar_records`
	if where != "" {
		totalQuery += " " + where
	}
	total, err := s.countGrammarRows(totalQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}

	queryArgs = append(queryArgs, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, user_id, grammar_point_id, status, next_review_at, quiz_history_json::text
		 FROM grammar_records
		 %s
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(queryArgs)-1,
		len(queryArgs),
	)

	rows, err := s.db.QueryContext(context.Background(), query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.GrammarStore.ListAllRecords query: %w", translateError(err))
	}
	defer rows.Close()

	var records []grammar.GrammarRecord
	for rows.Next() {
		record, err := scanGrammarRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("postgres.GrammarStore.ListAllRecords rows: %w", translateError(err))
	}

	return records, total, nil
}

func (s *GrammarStore) scanGrammarPoints(ctx context.Context, rows *sql.Rows) ([]grammar.GrammarPoint, error) {
	var points []grammar.GrammarPoint
	for rows.Next() {
		var point grammar.GrammarPoint
		if err := rows.Scan(
			&point.ID,
			&point.Name,
			&point.Meaning,
			&point.ConjunctionRule,
			&point.UsageNote,
			&point.JLPTLevel,
		); err != nil {
			return nil, fmt.Errorf("postgres.GrammarStore.scanGrammarPoints scan: %w", translateError(err))
		}

		examples, err := s.loadExamplesByGrammarPointID(ctx, point.ID)
		if err != nil {
			return nil, err
		}
		quizQuestions, err := s.loadQuizQuestionsByGrammarPointID(ctx, point.ID)
		if err != nil {
			return nil, err
		}
		point.Examples = examples
		point.QuizQuestions = quizQuestions

		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.scanGrammarPoints rows: %w", translateError(err))
	}
	return points, nil
}

func (s *GrammarStore) loadExamplesByGrammarPointID(ctx context.Context, grammarPointID int64) ([]grammar.GrammarExample, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT japanese, chinese, furigana_html, COALESCE(array_to_json(linked_word_ids)::text, '[]')
		 FROM grammar_examples
		 WHERE grammar_point_id = $1
		 ORDER BY position`,
		grammarPointID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.loadExamplesByGrammarPointID query: %w", translateError(err))
	}
	defer rows.Close()

	var examples []grammar.GrammarExample
	for rows.Next() {
		var example grammar.GrammarExample
		var linkedWordIDsJSON string
		if err := rows.Scan(&example.Japanese, &example.Chinese, &example.FuriganaHTML, &linkedWordIDsJSON); err != nil {
			return nil, fmt.Errorf("postgres.GrammarStore.loadExamplesByGrammarPointID scan: %w", translateError(err))
		}
		if err := json.Unmarshal([]byte(linkedWordIDsJSON), &example.LinkedWords); err != nil {
			return nil, fmt.Errorf("postgres.GrammarStore.loadExamplesByGrammarPointID unmarshal linked words: %w", err)
		}
		examples = append(examples, example)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.loadExamplesByGrammarPointID rows: %w", translateError(err))
	}

	return examples, nil
}

func (s *GrammarStore) loadQuizQuestionsByGrammarPointID(ctx context.Context, grammarPointID int64) ([]grammar.QuizQuestion, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, type, prompt, options::text, answer, explanation
		 FROM grammar_quiz_questions
		 WHERE grammar_point_id = $1
		 ORDER BY position`,
		grammarPointID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.loadQuizQuestionsByGrammarPointID query: %w", translateError(err))
	}
	defer rows.Close()

	var questions []grammar.QuizQuestion
	for rows.Next() {
		var question grammar.QuizQuestion
		var optionsJSON string
		if err := rows.Scan(&question.ID, &question.Type, &question.Prompt, &optionsJSON, &question.Answer, &question.Explanation); err != nil {
			return nil, fmt.Errorf("postgres.GrammarStore.loadQuizQuestionsByGrammarPointID scan: %w", translateError(err))
		}
		if err := json.Unmarshal([]byte(optionsJSON), &question.Options); err != nil {
			return nil, fmt.Errorf("postgres.GrammarStore.loadQuizQuestionsByGrammarPointID unmarshal options: %w", err)
		}
		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.GrammarStore.loadQuizQuestionsByGrammarPointID rows: %w", translateError(err))
	}

	return questions, nil
}

func (s *GrammarStore) countGrammarRows(query string, args ...any) (int, error) {
	var count int
	if err := s.db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres.GrammarStore.countGrammarRows: %w", translateError(err))
	}
	return count, nil
}

func buildGrammarListFilter(level, search string) (string, []any) {
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
		clauses = append(clauses, fmt.Sprintf("(name ILIKE $%[1]d OR meaning ILIKE $%[1]d)", index))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func replaceGrammarExamples(ctx context.Context, db queryer, grammarPointID int64, examples []grammar.GrammarExample) error {
	for idx, example := range examples {
		linkedWordIDsJSON, err := json.Marshal(normalizeInt64Slice(example.LinkedWords))
		if err != nil {
			return fmt.Errorf("postgres.replaceGrammarExamples marshal linked words: %w", err)
		}
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO grammar_examples (grammar_point_id, position, japanese, chinese, furigana_html, linked_word_ids)
			 VALUES (
			     $1,
			     $2,
			     $3,
			     $4,
			     $5,
			     COALESCE(
			         (SELECT array_agg(value::bigint) FROM jsonb_array_elements_text($6::jsonb) AS value),
			         ARRAY[]::bigint[]
			     )
			 )`,
			grammarPointID,
			idx,
			example.Japanese,
			example.Chinese,
			example.FuriganaHTML,
			string(linkedWordIDsJSON),
		); err != nil {
			return fmt.Errorf("postgres.replaceGrammarExamples: %w", translateError(err))
		}
	}
	return nil
}

func replaceGrammarQuizQuestions(ctx context.Context, db queryer, grammarPointID int64, questions []grammar.QuizQuestion) error {
	for idx, question := range questions {
		optionsJSON, err := json.Marshal(normalizeStringSlice(question.Options))
		if err != nil {
			return fmt.Errorf("postgres.replaceGrammarQuizQuestions marshal options: %w", err)
		}
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO grammar_quiz_questions (grammar_point_id, position, type, prompt, options, answer, explanation)
			 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
			grammarPointID,
			idx,
			question.Type,
			question.Prompt,
			string(optionsJSON),
			question.Answer,
			question.Explanation,
		); err != nil {
			return fmt.Errorf("postgres.replaceGrammarQuizQuestions: %w", translateError(err))
		}
	}
	return nil
}

func scanGrammarRecord(scanner rowScanner) (*grammar.GrammarRecord, error) {
	var record grammar.GrammarRecord
	var historyJSON string
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.GrammarPointID,
		&record.Status,
		&record.NextReviewAt,
		&historyJSON,
	); err != nil {
		return nil, fmt.Errorf("postgres.scanGrammarRecord: %w", translateError(err))
	}
	if historyJSON != "" {
		if err := json.Unmarshal([]byte(historyJSON), &record.QuizHistory); err != nil {
			return nil, fmt.Errorf("postgres.scanGrammarRecord unmarshal history: %w", err)
		}
	}
	return &record, nil
}

func lastGrammarQuizScore(historyJSON sql.NullString) int {
	if !historyJSON.Valid || historyJSON.String == "" {
		return -1
	}
	var history []grammar.QuizAttempt
	if err := json.Unmarshal([]byte(historyJSON.String), &history); err != nil {
		return -1
	}
	if len(history) == 0 {
		return -1
	}
	return history[len(history)-1].Score
}

func normalizeInt64Slice(values []int64) []int64 {
	if values == nil {
		return []int64{}
	}
	return values
}

func normalizeStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
