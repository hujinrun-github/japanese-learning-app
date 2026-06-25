package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"japanese-learning-app/internal/module/writing"
	"japanese-learning-app/internal/store"
)

type WritingStore struct {
	db  queryer
	now func() time.Time
}

func NewWritingStore(db queryer, deps store.StoreDeps) *WritingStore {
	return &WritingStore{
		db:  db,
		now: currentTimeFunc(deps.Clock),
	}
}

func (s *WritingStore) ListAllQuestions(level, qtype string, offset, limit int) ([]writing.WritingQuestion, int, error) {
	where, args := buildWritingQuestionFilter(level, qtype)

	total, err := countRows(context.Background(), s.db, `SELECT COUNT(*) FROM writing_questions `+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllQuestions count: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, type, prompt, grammar_point_id, jlpt_level, expected_answer
		 FROM writing_questions
		 %s
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllQuestions query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanWritingQuestions(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *WritingStore) InsertQuestion(q writing.WritingQuestion) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO writing_questions (type, prompt, expected_answer, grammar_point_id, jlpt_level, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		q.Type,
		q.Prompt,
		q.ExpectedAnswer,
		nullablePositiveInt64(q.GrammarPointID),
		q.JLPTLevel,
		s.now().UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.WritingStore.InsertQuestion: %w", translateError(err))
	}
	return id, nil
}

func (s *WritingStore) UpdateQuestion(q writing.WritingQuestion) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE writing_questions
		 SET type = $2,
		     prompt = $3,
		     expected_answer = $4,
		     grammar_point_id = $5,
		     jlpt_level = $6,
		     updated_at = $7
		 WHERE id = $1`,
		q.ID,
		q.Type,
		q.Prompt,
		q.ExpectedAnswer,
		nullablePositiveInt64(q.GrammarPointID),
		q.JLPTLevel,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.WritingStore.UpdateQuestion: %w", translateError(err))
	}
	return nil
}

func (s *WritingStore) DeleteQuestion(id int64) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM writing_questions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres.WritingStore.DeleteQuestion: %w", translateError(err))
	}
	return nil
}

func (s *WritingStore) ListAllRecords(userID int64, offset, limit int) ([]writing.WritingRecord, int, error) {
	where := ""
	args := []any{}
	if userID > 0 {
		where = "WHERE user_id = $1"
		args = append(args, userID)
	}

	totalQuery := `SELECT COUNT(*) FROM writing_records`
	if where != "" {
		totalQuery += " " + where
	}
	total, err := countRows(context.Background(), s.db, totalQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllRecords count: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, user_id, type, question, user_answer, ai_feedback_json::text, score, practiced_at
		 FROM writing_records
		 %s
		 ORDER BY id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllRecords query: %w", translateError(err))
	}
	defer rows.Close()

	var records []writing.WritingRecord
	for rows.Next() {
		var record writing.WritingRecord
		var feedbackJSON string
		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.Type,
			&record.Question,
			&record.UserAnswer,
			&feedbackJSON,
			&record.Score,
			&record.PracticedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllRecords scan: %w", translateError(err))
		}
		if feedbackJSON != "" && feedbackJSON != "null" {
			var feedback writing.AIFeedback
			if err := json.Unmarshal([]byte(feedbackJSON), &feedback); err != nil {
				return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllRecords unmarshal feedback: %w", err)
			}
			record.AIFeedback = &feedback
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("postgres.WritingStore.ListAllRecords rows: %w", translateError(err))
	}
	return records, total, nil
}

func buildWritingQuestionFilter(level, qtype string) (string, []any) {
	var clauses []string
	var args []any
	if level != "" {
		args = append(args, level)
		clauses = append(clauses, fmt.Sprintf("jlpt_level = $%d", len(args)))
	}
	if qtype != "" {
		args = append(args, qtype)
		clauses = append(clauses, fmt.Sprintf("type = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func scanWritingQuestions(rows *sql.Rows) ([]writing.WritingQuestion, error) {
	var items []writing.WritingQuestion
	for rows.Next() {
		var item writing.WritingQuestion
		var grammarPointID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Prompt,
			&grammarPointID,
			&item.JLPTLevel,
			&item.ExpectedAnswer,
		); err != nil {
			return nil, fmt.Errorf("postgres.scanWritingQuestions scan: %w", translateError(err))
		}
		if grammarPointID.Valid {
			item.GrammarPointID = grammarPointID.Int64
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.scanWritingQuestions rows: %w", translateError(err))
	}
	return items, nil
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
