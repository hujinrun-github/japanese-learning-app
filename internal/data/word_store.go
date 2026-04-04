package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/word"
)

// WordStore 实现单词数据访问，对应 words + word_records + word_bookmarks 表。
type WordStore struct {
	db *sql.DB
}

// NewWordStore 创建 WordStore 实例。
func NewWordStore(db *sql.DB) *WordStore {
	return &WordStore{db: db}
}

// GetByID 按 ID 查询单词，不存在时返回 error。
func (s *WordStore) GetByID(id int64) (*word.Word, error) {
	slog.Debug("WordStore.GetByID called", "word_id", id)

	row := s.db.QueryRow(
		`SELECT id, kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level
		 FROM words WHERE id = ?`, id,
	)

	var w word.Word
	var examplesJSON string
	err := row.Scan(&w.ID, &w.KanjiForm, &w.Reading, &w.PartOfSpeech, &w.Meaning, &examplesJSON, &w.JLPTLevel)
	if err == sql.ErrNoRows {
		slog.Error("word not found", "word_id", id)
		return nil, fmt.Errorf("data.WordStore.GetByID %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan word", "err", err, "word_id", id)
		return nil, fmt.Errorf("data.WordStore.GetByID: %w", err)
	}

	if err := json.Unmarshal([]byte(examplesJSON), &w.Examples); err != nil {
		slog.Error("failed to unmarshal examples_json", "err", err, "word_id", id)
		return nil, fmt.Errorf("data.WordStore.GetByID unmarshal examples: %w", err)
	}

	slog.Debug("WordStore.GetByID done", "word_id", id, "kanji", w.KanjiForm)
	return &w, nil
}

// ListByLevel 按 JLPT 等级分页查询单词列表，返回当页数据 + 该等级总数。
func (s *WordStore) ListByLevel(level word.JLPTLevel, page, size int) ([]word.Word, int, error) {
	slog.Debug("WordStore.ListByLevel called", "level", level, "page", page, "size", size)

	// 查总数
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE jlpt_level = ?`, level).Scan(&total); err != nil {
		slog.Error("failed to count words", "err", err, "level", level)
		return nil, 0, fmt.Errorf("data.WordStore.ListByLevel count: %w", err)
	}

	offset := (page - 1) * size
	rows, err := s.db.Query(
		`SELECT id, kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level
		 FROM words WHERE jlpt_level = ? ORDER BY id LIMIT ? OFFSET ?`,
		level, size, offset,
	)
	if err != nil {
		slog.Error("failed to query words", "err", err, "level", level)
		return nil, 0, fmt.Errorf("data.WordStore.ListByLevel query: %w", err)
	}
	defer rows.Close()

	var words []word.Word
	for rows.Next() {
		var w word.Word
		var examplesJSON string
		if err := rows.Scan(&w.ID, &w.KanjiForm, &w.Reading, &w.PartOfSpeech, &w.Meaning, &examplesJSON, &w.JLPTLevel); err != nil {
			slog.Error("failed to scan word row", "err", err)
			return nil, 0, fmt.Errorf("data.WordStore.ListByLevel scan: %w", err)
		}
		if err := json.Unmarshal([]byte(examplesJSON), &w.Examples); err != nil {
			slog.Error("failed to unmarshal examples_json", "err", err, "word_id", w.ID)
			return nil, 0, fmt.Errorf("data.WordStore.ListByLevel unmarshal: %w", err)
		}
		words = append(words, w)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, 0, fmt.Errorf("data.WordStore.ListByLevel rows: %w", err)
	}

	slog.Debug("WordStore.ListByLevel done", "level", level, "count", len(words), "total", total)
	return words, total, nil
}

// GetRecord 查询用户对某单词的学习记录，不存在时返回 error。
func (s *WordStore) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	slog.Debug("WordStore.GetRecord called", "user_id", userID, "word_id", wordID)

	row := s.db.QueryRow(
		`SELECT id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at
		 FROM word_records WHERE user_id = ? AND word_id = ?`,
		userID, wordID,
	)

	var r word.WordRecord
	var historyJSON string
	var nextReviewAt, updatedAt string
	err := row.Scan(&r.ID, &r.UserID, &r.WordID, &r.MasteryLevel, &nextReviewAt, &r.EaseFactor, &r.Interval, &historyJSON, &updatedAt)
	if err == sql.ErrNoRows {
		slog.Error("word_record not found", "user_id", userID, "word_id", wordID)
		return nil, fmt.Errorf("data.WordStore.GetRecord user=%d word=%d: %w", userID, wordID, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan word_record", "err", err)
		return nil, fmt.Errorf("data.WordStore.GetRecord: %w", err)
	}

	r.NextReviewAt, err = parseSQLiteTime(nextReviewAt)
	if err != nil {
		return nil, fmt.Errorf("data.WordStore.GetRecord parse next_review_at: %w", err)
	}
	r.UpdatedAt, err = parseSQLiteTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("data.WordStore.GetRecord parse updated_at: %w", err)
	}
	if err := json.Unmarshal([]byte(historyJSON), &r.ReviewHistory); err != nil {
		slog.Error("failed to unmarshal review_history_json", "err", err)
		return nil, fmt.Errorf("data.WordStore.GetRecord unmarshal history: %w", err)
	}

	return &r, nil
}

// ListDueRecords 查询用户到期待复习的单词记录（next_review_at <= now），按到期时间升序，最多返回 limit 条。
func (s *WordStore) ListDueRecords(userID int64, limit int) ([]word.WordRecord, error) {
	slog.Debug("WordStore.ListDueRecords called", "user_id", userID, "limit", limit)

	rows, err := s.db.Query(
		`SELECT id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at
		 FROM word_records
		 WHERE user_id = ? AND next_review_at <= datetime('now')
		 ORDER BY next_review_at ASC
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		slog.Error("failed to query due word_records", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.WordStore.ListDueRecords query: %w", err)
	}
	defer rows.Close()

	var records []word.WordRecord
	for rows.Next() {
		var r word.WordRecord
		var historyJSON string
		var nextReviewAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.WordID, &r.MasteryLevel, &nextReviewAt, &r.EaseFactor, &r.Interval, &historyJSON, &updatedAt); err != nil {
			slog.Error("failed to scan word_record row", "err", err)
			return nil, fmt.Errorf("data.WordStore.ListDueRecords scan: %w", err)
		}
		r.NextReviewAt, err = parseSQLiteTime(nextReviewAt)
		if err != nil {
			return nil, fmt.Errorf("data.WordStore.ListDueRecords parse next_review_at: %w", err)
		}
		r.UpdatedAt, err = parseSQLiteTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("data.WordStore.ListDueRecords parse updated_at: %w", err)
		}
		if err := json.Unmarshal([]byte(historyJSON), &r.ReviewHistory); err != nil {
			slog.Error("failed to unmarshal review_history_json", "err", err)
			return nil, fmt.Errorf("data.WordStore.ListDueRecords unmarshal: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.WordStore.ListDueRecords rows: %w", err)
	}

	slog.Debug("WordStore.ListDueRecords done", "user_id", userID, "count", len(records))
	return records, nil
}

// UpsertRecord 插入或更新单词学习记录（ON CONFLICT (user_id, word_id) DO UPDATE）。
func (s *WordStore) UpsertRecord(r word.WordRecord) error {
	slog.Debug("WordStore.UpsertRecord called", "user_id", r.UserID, "word_id", r.WordID)

	historyJSON, err := json.Marshal(r.ReviewHistory)
	if err != nil {
		slog.Error("failed to marshal review_history", "err", err)
		return fmt.Errorf("data.WordStore.UpsertRecord marshal: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO word_records (user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT (user_id, word_id) DO UPDATE SET
		   mastery_level       = excluded.mastery_level,
		   next_review_at      = excluded.next_review_at,
		   ease_factor         = excluded.ease_factor,
		   interval            = excluded.interval,
		   review_history_json = excluded.review_history_json,
		   updated_at          = excluded.updated_at`,
		r.UserID, r.WordID, r.MasteryLevel, formatSQLiteTime(r.NextReviewAt),
		r.EaseFactor, r.Interval, string(historyJSON),
	)
	if err != nil {
		slog.Error("failed to upsert word_record", "err", err, "user_id", r.UserID, "word_id", r.WordID)
		return fmt.Errorf("data.WordStore.UpsertRecord exec: %w", err)
	}

	slog.Debug("WordStore.UpsertRecord done", "user_id", r.UserID, "word_id", r.WordID)
	return nil
}

// BookmarkWord 收藏单词（幂等，已收藏则忽略）。
func (s *WordStore) BookmarkWord(userID, wordID int64) error {
	slog.Debug("WordStore.BookmarkWord called", "user_id", userID, "word_id", wordID)

	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO word_bookmarks (user_id, word_id) VALUES (?, ?)`,
		userID, wordID,
	)
	if err != nil {
		slog.Error("failed to bookmark word", "err", err, "user_id", userID, "word_id", wordID)
		return fmt.Errorf("data.WordStore.BookmarkWord: %w", err)
	}

	return nil
}
