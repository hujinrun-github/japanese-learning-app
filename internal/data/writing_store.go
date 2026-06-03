package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/writing"
)

// WritingStore 实现写作练习数据访问，对应 writing_questions + writing_records 表。
type WritingStore struct {
	db *sql.DB
}

// NewWritingStore 创建 WritingStore 实例。
func NewWritingStore(db *sql.DB) *WritingStore {
	return &WritingStore{db: db}
}

// GetDailyQueue 获取用户今日写作练习题目队列（3~5 道），随机选取题目。
func (s *WritingStore) GetDailyQueue(userID int64) ([]writing.WritingQuestion, error) {
	slog.Debug("WritingStore.GetDailyQueue called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, type, prompt, grammar_point_id, jlpt_level, expected_answer
		 FROM writing_questions
		 ORDER BY RANDOM()
		 LIMIT 5`,
	)
	if err != nil {
		slog.Error("failed to query writing_questions for daily queue", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.WritingStore.GetDailyQueue query: %w", err)
	}
	defer rows.Close()

	var questions []writing.WritingQuestion
	for rows.Next() {
		var q writing.WritingQuestion
		var grammarPointID sql.NullInt64
		if err := rows.Scan(&q.ID, &q.Type, &q.Prompt, &grammarPointID, &q.JLPTLevel, &q.ExpectedAnswer); err != nil {
			slog.Error("failed to scan writing_question row", "err", err)
			return nil, fmt.Errorf("data.WritingStore.GetDailyQueue scan: %w", err)
		}
		if grammarPointID.Valid {
			q.GrammarPointID = grammarPointID.Int64
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, fmt.Errorf("data.WritingStore.GetDailyQueue rows: %w", err)
	}

	slog.Debug("WritingStore.GetDailyQueue done", "user_id", userID, "count", len(questions))
	return questions, nil
}

// GetQuestionByID 按 ID 查询写作题目。
func (s *WritingStore) GetQuestionByID(id int64) (*writing.WritingQuestion, error) {
	var q writing.WritingQuestion
	var grammarPointID sql.NullInt64
	err := s.db.QueryRow(
		`SELECT id, type, prompt, grammar_point_id, jlpt_level, expected_answer
		 FROM writing_questions WHERE id = ?`, id,
	).Scan(&q.ID, &q.Type, &q.Prompt, &grammarPointID, &q.JLPTLevel, &q.ExpectedAnswer)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("data.WritingStore.GetQuestionByID %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		return nil, fmt.Errorf("data.WritingStore.GetQuestionByID: %w", err)
	}
	if grammarPointID.Valid {
		q.GrammarPointID = grammarPointID.Int64
	}
	return &q, nil
}

// SaveRecord 保存一次写作练习记录。
func (s *WritingStore) SaveRecord(r writing.WritingRecord) error {
	slog.Debug("WritingStore.SaveRecord called", "user_id", r.UserID, "type", r.Type)

	var aiFeedbackJSON string
	if r.AIFeedback != nil {
		b, err := json.Marshal(r.AIFeedback)
		if err != nil {
			slog.Error("failed to marshal ai_feedback", "err", err)
			return fmt.Errorf("data.WritingStore.SaveRecord marshal ai_feedback: %w", err)
		}
		aiFeedbackJSON = string(b)
	} else {
		aiFeedbackJSON = "null"
	}

	_, err := s.db.Exec(
		`INSERT INTO writing_records (user_id, type, question, user_answer, ai_feedback_json, score, practiced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.UserID, r.Type, r.Question, r.UserAnswer, aiFeedbackJSON, r.Score, formatSQLiteTime(r.PracticedAt),
	)
	if err != nil {
		slog.Error("failed to insert writing_record", "err", err, "user_id", r.UserID)
		return fmt.Errorf("data.WritingStore.SaveRecord exec: %w", err)
	}

	slog.Debug("WritingStore.SaveRecord done", "user_id", r.UserID)
	return nil
}

// ListRecords 查询用户所有写作练习记录，按 practiced_at 倒序。
func (s *WritingStore) ListRecords(userID int64) ([]writing.WritingRecord, error) {
	slog.Debug("WritingStore.ListRecords called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, type, question, user_answer, ai_feedback_json, score, practiced_at
		 FROM writing_records WHERE user_id = ?
		 ORDER BY practiced_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query writing_records", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.WritingStore.ListRecords query: %w", err)
	}
	defer rows.Close()

	var records []writing.WritingRecord
	for rows.Next() {
		var r writing.WritingRecord
		var aiFeedbackJSON string
		var practicedAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Type, &r.Question, &r.UserAnswer,
			&aiFeedbackJSON, &r.Score, &practicedAt); err != nil {
			slog.Error("failed to scan writing_record row", "err", err)
			return nil, fmt.Errorf("data.WritingStore.ListRecords scan: %w", err)
		}
		if aiFeedbackJSON != "" && aiFeedbackJSON != "null" {
			var feedback writing.AIFeedback
			if err := json.Unmarshal([]byte(aiFeedbackJSON), &feedback); err != nil {
				slog.Error("failed to unmarshal ai_feedback_json", "err", err)
				return nil, fmt.Errorf("data.WritingStore.ListRecords unmarshal ai_feedback: %w", err)
			}
			r.AIFeedback = &feedback
		}
		r.PracticedAt, err = parseSQLiteTime(practicedAt)
		if err != nil {
			return nil, fmt.Errorf("data.WritingStore.ListRecords parse practiced_at: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, fmt.Errorf("data.WritingStore.ListRecords rows: %w", err)
	}

	slog.Debug("WritingStore.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}

// UpdateQuestion 更新写作题目。
func (s *WritingStore) UpdateQuestion(q writing.WritingQuestion) error {
	slog.Debug("WritingStore.UpdateQuestion called", "id", q.ID)
	_, err := s.db.Exec(
		"UPDATE writing_questions SET type=?, prompt=?, expected_answer=?, grammar_point_id=?, jlpt_level=?, updated_at = datetime('now') WHERE id=?",
		q.Type, q.Prompt, q.ExpectedAnswer, q.GrammarPointID, q.JLPTLevel, q.ID,
	)
	if err != nil {
		slog.Error("failed to update writing_question", "err", err, "id", q.ID)
		return fmt.Errorf("data.WritingStore.UpdateQuestion exec: %w", err)
	}
	return nil
}

// DeleteQuestion 按 ID 删除写作题目。
func (s *WritingStore) DeleteQuestion(id int64) error {
	slog.Debug("WritingStore.DeleteQuestion called", "id", id)
	_, err := s.db.Exec("DELETE FROM writing_questions WHERE id = ?", id)
	if err != nil {
		slog.Error("failed to delete writing_question", "err", err, "id", id)
		return fmt.Errorf("data.WritingStore.DeleteQuestion exec: %w", err)
	}
	return nil
}

// ListAllRecords 分页查询写作练习记录，可选按 user_id 过滤。
// userID == 0 表示不过滤，返回所有用户的记录。
func (s *WritingStore) ListAllRecords(userID int64, offset, limit int) ([]writing.WritingRecord, int, error) {
	slog.Debug("WritingStore.ListAllRecords called", "user_id", userID, "offset", offset, "limit", limit)

	where := "WHERE 1=1"
	var args []any
	if userID > 0 {
		where += " AND user_id = ?"
		args = append(args, userID)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM writing_records "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("data.WritingStore.ListAllRecords count: %w", err)
	}

	var records []writing.WritingRecord
	query := fmt.Sprintf("SELECT id, user_id, type, question, user_answer, ai_feedback_json, score, practiced_at FROM writing_records %s ORDER BY id DESC LIMIT ? OFFSET ?", where)
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("data.WritingStore.ListAllRecords query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r writing.WritingRecord
		var aiFeedbackJSON string
		var practicedAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Type, &r.Question, &r.UserAnswer, &aiFeedbackJSON, &r.Score, &practicedAt); err != nil {
			return nil, 0, fmt.Errorf("data.WritingStore.ListAllRecords scan: %w", err)
		}
		if aiFeedbackJSON != "" && aiFeedbackJSON != "null" {
			var feedback writing.AIFeedback
			if err := json.Unmarshal([]byte(aiFeedbackJSON), &feedback); err != nil {
				return nil, 0, fmt.Errorf("data.WritingStore.ListAllRecords unmarshal ai_feedback: %w", err)
			}
			r.AIFeedback = &feedback
		}
		r.PracticedAt, err = parseSQLiteTime(practicedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("data.WritingStore.ListAllRecords parse practiced_at: %w", err)
		}
		records = append(records, r)
	}

	slog.Debug("WritingStore.ListAllRecords done", "count", len(records), "total", total)
	return records, total, rows.Err()
}

// InsertQuestion 插入一条新的写作题目，返回自动生成的 ID。
func (s *WritingStore) InsertQuestion(q writing.WritingQuestion) (int64, error) {
	slog.Debug("WritingStore.InsertQuestion called", "type", q.Type, "jlpt_level", q.JLPTLevel)
	result, err := s.db.Exec(
		`INSERT INTO writing_questions (type, prompt, expected_answer, grammar_point_id, jlpt_level, updated_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		q.Type, q.Prompt, q.ExpectedAnswer, q.GrammarPointID, q.JLPTLevel,
	)
	if err != nil {
		slog.Error("failed to insert writing_question", "err", err)
		return 0, fmt.Errorf("data.WritingStore.InsertQuestion exec: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		slog.Error("failed to get last insert id", "err", err)
		return 0, fmt.Errorf("data.WritingStore.InsertQuestion last insert id: %w", err)
	}
	slog.Debug("WritingStore.InsertQuestion done", "id", id)
	return id, nil
}

// ListAllQuestions 分页查询写作题目，支持按 jlpt_level 和 type 过滤。
func (s *WritingStore) ListAllQuestions(level, qtype string, offset, limit int) ([]writing.WritingQuestion, int, error) {
	where := "WHERE 1=1"
	var args []any
	if level != "" {
		where += " AND jlpt_level = ?"
		args = append(args, level)
	}
	if qtype != "" {
		where += " AND type = ?"
		args = append(args, qtype)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM writing_questions "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("data.WritingStore.ListAllQuestions count: %w", err)
	}

	query := fmt.Sprintf("SELECT id, type, prompt, grammar_point_id, jlpt_level, expected_answer FROM writing_questions %s ORDER BY updated_at DESC LIMIT ? OFFSET ?", where)
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("data.WritingStore.ListAllQuestions query: %w", err)
	}
	defer rows.Close()

	var questions []writing.WritingQuestion
	for rows.Next() {
		var q writing.WritingQuestion
		var gpid sql.NullInt64
		if err := rows.Scan(&q.ID, &q.Type, &q.Prompt, &gpid, &q.JLPTLevel, &q.ExpectedAnswer); err != nil {
			return nil, 0, fmt.Errorf("data.WritingStore.ListAllQuestions scan: %w", err)
		}
		if gpid.Valid {
			q.GrammarPointID = gpid.Int64
		}
		questions = append(questions, q)
	}
	return questions, total, rows.Err()
}
