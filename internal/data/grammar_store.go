package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/grammar"
)

// GrammarStore 实现语法数据访问，对应 grammar_points + grammar_records 表。
type GrammarStore struct {
	db *sql.DB
}

// NewGrammarStore 创建 GrammarStore 实例。
func NewGrammarStore(db *sql.DB) *GrammarStore {
	return &GrammarStore{db: db}
}

// GetByID 按 ID 查询语法点，不存在时返回 error。
func (s *GrammarStore) GetByID(id int64) (*grammar.GrammarPoint, error) {
	slog.Debug("GrammarStore.GetByID called", "grammar_point_id", id)

	row := s.db.QueryRow(
		`SELECT id, name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level
		 FROM grammar_points WHERE id = ?`, id,
	)

	var gp grammar.GrammarPoint
	var examplesJSON, quizJSON string
	err := row.Scan(&gp.ID, &gp.Name, &gp.Meaning, &gp.ConjunctionRule, &gp.UsageNote,
		&examplesJSON, &quizJSON, &gp.JLPTLevel)
	if err == sql.ErrNoRows {
		slog.Error("grammar_point not found", "grammar_point_id", id)
		return nil, fmt.Errorf("data.GrammarStore.GetByID %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan grammar_point", "err", err, "grammar_point_id", id)
		return nil, fmt.Errorf("data.GrammarStore.GetByID: %w", err)
	}

	if err := json.Unmarshal([]byte(examplesJSON), &gp.Examples); err != nil {
		slog.Error("failed to unmarshal examples_json", "err", err, "grammar_point_id", id)
		return nil, fmt.Errorf("data.GrammarStore.GetByID unmarshal examples: %w", err)
	}
	if err := json.Unmarshal([]byte(quizJSON), &gp.QuizQuestions); err != nil {
		slog.Error("failed to unmarshal quiz_questions_json", "err", err, "grammar_point_id", id)
		return nil, fmt.Errorf("data.GrammarStore.GetByID unmarshal quiz: %w", err)
	}

	slog.Debug("GrammarStore.GetByID done", "grammar_point_id", id, "name", gp.Name)
	return &gp, nil
}

// ListByLevel 查询指定 JLPT 等级的所有语法点。
func (s *GrammarStore) ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error) {
	slog.Debug("GrammarStore.ListByLevel called", "level", level)

	rows, err := s.db.Query(
		`SELECT id, name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level
		 FROM grammar_points WHERE jlpt_level = ? ORDER BY id`,
		level,
	)
	if err != nil {
		slog.Error("failed to query grammar_points", "err", err, "level", level)
		return nil, fmt.Errorf("data.GrammarStore.ListByLevel query: %w", err)
	}
	defer rows.Close()

	var points []grammar.GrammarPoint
	for rows.Next() {
		var gp grammar.GrammarPoint
		var examplesJSON, quizJSON string
		if err := rows.Scan(&gp.ID, &gp.Name, &gp.Meaning, &gp.ConjunctionRule, &gp.UsageNote,
			&examplesJSON, &quizJSON, &gp.JLPTLevel); err != nil {
			slog.Error("failed to scan grammar_point row", "err", err)
			return nil, fmt.Errorf("data.GrammarStore.ListByLevel scan: %w", err)
		}
		if err := json.Unmarshal([]byte(examplesJSON), &gp.Examples); err != nil {
			slog.Error("failed to unmarshal examples_json", "err", err, "grammar_point_id", gp.ID)
			return nil, fmt.Errorf("data.GrammarStore.ListByLevel unmarshal examples: %w", err)
		}
		if err := json.Unmarshal([]byte(quizJSON), &gp.QuizQuestions); err != nil {
			slog.Error("failed to unmarshal quiz_questions_json", "err", err, "grammar_point_id", gp.ID)
			return nil, fmt.Errorf("data.GrammarStore.ListByLevel unmarshal quiz: %w", err)
		}
		points = append(points, gp)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, fmt.Errorf("data.GrammarStore.ListByLevel rows: %w", err)
	}

	slog.Debug("GrammarStore.ListByLevel done", "level", level, "count", len(points))
	return points, nil
}

// GetRecord 查询用户对某语法点的学习记录，不存在时返回 error。
func (s *GrammarStore) GetRecord(userID, grammarPointID int64) (*grammar.GrammarRecord, error) {
	slog.Debug("GrammarStore.GetRecord called", "user_id", userID, "grammar_point_id", grammarPointID)

	row := s.db.QueryRow(
		`SELECT id, user_id, grammar_point_id, status, next_review_at, quiz_history_json
		 FROM grammar_records WHERE user_id = ? AND grammar_point_id = ?`,
		userID, grammarPointID,
	)

	var r grammar.GrammarRecord
	var historyJSON, nextReviewAt string
	err := row.Scan(&r.ID, &r.UserID, &r.GrammarPointID, &r.Status, &nextReviewAt, &historyJSON)
	if err == sql.ErrNoRows {
		slog.Error("grammar_record not found", "user_id", userID, "grammar_point_id", grammarPointID)
		return nil, fmt.Errorf("data.GrammarStore.GetRecord user=%d grammar_point=%d: %w", userID, grammarPointID, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan grammar_record", "err", err)
		return nil, fmt.Errorf("data.GrammarStore.GetRecord: %w", err)
	}

	r.NextReviewAt, err = parseSQLiteTime(nextReviewAt)
	if err != nil {
		return nil, fmt.Errorf("data.GrammarStore.GetRecord parse next_review_at: %w", err)
	}
	if err := json.Unmarshal([]byte(historyJSON), &r.QuizHistory); err != nil {
		slog.Error("failed to unmarshal quiz_history_json", "err", err)
		return nil, fmt.Errorf("data.GrammarStore.GetRecord unmarshal history: %w", err)
	}

	slog.Debug("GrammarStore.GetRecord done", "user_id", userID, "grammar_point_id", grammarPointID)
	return &r, nil
}

// UpsertRecord 插入或更新语法学习记录（ON CONFLICT (user_id, grammar_point_id) DO UPDATE）。
func (s *GrammarStore) UpsertRecord(r grammar.GrammarRecord) error {
	slog.Debug("GrammarStore.UpsertRecord called", "user_id", r.UserID, "grammar_point_id", r.GrammarPointID)

	historyJSON, err := json.Marshal(r.QuizHistory)
	if err != nil {
		slog.Error("failed to marshal quiz_history", "err", err)
		return fmt.Errorf("data.GrammarStore.UpsertRecord marshal: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO grammar_records (user_id, grammar_point_id, status, next_review_at, quiz_history_json)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, grammar_point_id) DO UPDATE SET
		   status            = excluded.status,
		   next_review_at    = excluded.next_review_at,
		   quiz_history_json = excluded.quiz_history_json`,
		r.UserID, r.GrammarPointID, r.Status, formatSQLiteTime(r.NextReviewAt), string(historyJSON),
	)
	if err != nil {
		slog.Error("failed to upsert grammar_record", "err", err, "user_id", r.UserID, "grammar_point_id", r.GrammarPointID)
		return fmt.Errorf("data.GrammarStore.UpsertRecord exec: %w", err)
	}

	slog.Debug("GrammarStore.UpsertRecord done", "user_id", r.UserID, "grammar_point_id", r.GrammarPointID)
	return nil
}

// ListDueRecords 查询用户到期待复习的语法记录（next_review_at <= now）。
func (s *GrammarStore) ListDueRecords(userID int64) ([]grammar.GrammarRecord, error) {
	slog.Debug("GrammarStore.ListDueRecords called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, grammar_point_id, status, next_review_at, quiz_history_json
		 FROM grammar_records
		 WHERE user_id = ? AND next_review_at <= datetime('now')
		 ORDER BY next_review_at ASC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query due grammar_records", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.GrammarStore.ListDueRecords query: %w", err)
	}
	defer rows.Close()

	var records []grammar.GrammarRecord
	for rows.Next() {
		var r grammar.GrammarRecord
		var historyJSON, nextReviewAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.GrammarPointID, &r.Status, &nextReviewAt, &historyJSON); err != nil {
			slog.Error("failed to scan grammar_record row", "err", err)
			return nil, fmt.Errorf("data.GrammarStore.ListDueRecords scan: %w", err)
		}
		r.NextReviewAt, err = parseSQLiteTime(nextReviewAt)
		if err != nil {
			return nil, fmt.Errorf("data.GrammarStore.ListDueRecords parse next_review_at: %w", err)
		}
		if err := json.Unmarshal([]byte(historyJSON), &r.QuizHistory); err != nil {
			slog.Error("failed to unmarshal quiz_history_json", "err", err)
			return nil, fmt.Errorf("data.GrammarStore.ListDueRecords unmarshal: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.GrammarStore.ListDueRecords rows: %w", err)
	}

	slog.Debug("GrammarStore.ListDueRecords done", "user_id", userID, "count", len(records))
	return records, nil
}
