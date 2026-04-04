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
		`SELECT id, type, prompt, grammar_point_id, expected_answer
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
		if err := rows.Scan(&q.ID, &q.Type, &q.Prompt, &grammarPointID, &q.ExpectedAnswer); err != nil {
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
