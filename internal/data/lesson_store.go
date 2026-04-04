package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/lesson"
)

// LessonStore 实现课文数据访问，对应 lessons 表。
type LessonStore struct {
	db *sql.DB
}

// NewLessonStore 创建 LessonStore 实例。
func NewLessonStore(db *sql.DB) *LessonStore {
	return &LessonStore{db: db}
}

// ListSummaries 按 JLPT 等级查询课文列表。若 tag 非空，则只返回含该 tag 的课文。
func (s *LessonStore) ListSummaries(level lesson.JLPTLevel, tag string) ([]lesson.LessonSummary, error) {
	slog.Debug("LessonStore.ListSummaries called", "level", level, "tag", tag)

	var rows *sql.Rows
	var err error

	if tag == "" {
		rows, err = s.db.Query(
			`SELECT id, title, jlpt_level, tags_json, char_count, audio_url
			 FROM lessons WHERE jlpt_level = ? ORDER BY id`,
			level,
		)
	} else {
		// 使用 LIKE 对 JSON 数组字符串进行 tag 过滤
		rows, err = s.db.Query(
			`SELECT id, title, jlpt_level, tags_json, char_count, audio_url
			 FROM lessons WHERE jlpt_level = ?
			   AND tags_json LIKE ?
			 ORDER BY id`,
			level, "%\""+tag+"\"%",
		)
	}
	if err != nil {
		slog.Error("failed to query lessons", "err", err, "level", level, "tag", tag)
		return nil, fmt.Errorf("data.LessonStore.ListSummaries query: %w", err)
	}
	defer rows.Close()

	var summaries []lesson.LessonSummary
	for rows.Next() {
		var ls lesson.LessonSummary
		var tagsJSON string
		if err := rows.Scan(&ls.ID, &ls.Title, &ls.JLPTLevel, &tagsJSON, &ls.CharCount, &ls.AudioURL); err != nil {
			slog.Error("failed to scan lesson row", "err", err)
			return nil, fmt.Errorf("data.LessonStore.ListSummaries scan: %w", err)
		}
		if err := json.Unmarshal([]byte(tagsJSON), &ls.Tags); err != nil {
			slog.Error("failed to unmarshal tags_json", "err", err, "lesson_id", ls.ID)
			return nil, fmt.Errorf("data.LessonStore.ListSummaries unmarshal tags: %w", err)
		}
		summaries = append(summaries, ls)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, fmt.Errorf("data.LessonStore.ListSummaries rows: %w", err)
	}

	slog.Debug("LessonStore.ListSummaries done", "level", level, "tag", tag, "count", len(summaries))
	return summaries, nil
}

// GetDetail 查询课文详情（含句子列表和词汇 ID 列表），不存在时返回 error。
func (s *LessonStore) GetDetail(id int64) (*lesson.Lesson, error) {
	slog.Debug("LessonStore.GetDetail called", "lesson_id", id)

	row := s.db.QueryRow(
		`SELECT id, title, jlpt_level, tags_json, char_count, audio_url,
		        content_furigana_json, word_ids_json
		 FROM lessons WHERE id = ?`, id,
	)

	var l lesson.Lesson
	var tagsJSON, contentJSON, wordIDsJSON string
	err := row.Scan(&l.ID, &l.Title, &l.JLPTLevel, &tagsJSON, &l.CharCount, &l.AudioURL,
		&contentJSON, &wordIDsJSON)
	if err == sql.ErrNoRows {
		slog.Error("lesson not found", "lesson_id", id)
		return nil, fmt.Errorf("data.LessonStore.GetDetail %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan lesson", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("data.LessonStore.GetDetail: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &l.Tags); err != nil {
		slog.Error("failed to unmarshal tags_json", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("data.LessonStore.GetDetail unmarshal tags: %w", err)
	}
	if err := json.Unmarshal([]byte(contentJSON), &l.Sentences); err != nil {
		slog.Error("failed to unmarshal content_furigana_json", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("data.LessonStore.GetDetail unmarshal sentences: %w", err)
	}
	if err := json.Unmarshal([]byte(wordIDsJSON), &l.WordIDs); err != nil {
		slog.Error("failed to unmarshal word_ids_json", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("data.LessonStore.GetDetail unmarshal word_ids: %w", err)
	}

	slog.Debug("LessonStore.GetDetail done", "lesson_id", id, "sentences", len(l.Sentences))
	return &l, nil
}

// GetSentences 查询课文的句子列表，按 index 升序排列。
func (s *LessonStore) GetSentences(lessonID int64) ([]lesson.Sentence, error) {
	slog.Debug("LessonStore.GetSentences called", "lesson_id", lessonID)

	row := s.db.QueryRow(
		`SELECT content_furigana_json FROM lessons WHERE id = ?`, lessonID,
	)

	var contentJSON string
	if err := row.Scan(&contentJSON); err == sql.ErrNoRows {
		slog.Error("lesson not found for GetSentences", "lesson_id", lessonID)
		return nil, fmt.Errorf("data.LessonStore.GetSentences %d: %w", lessonID, sql.ErrNoRows)
	} else if err != nil {
		slog.Error("failed to scan lesson content", "err", err, "lesson_id", lessonID)
		return nil, fmt.Errorf("data.LessonStore.GetSentences: %w", err)
	}

	var sentences []lesson.Sentence
	if err := json.Unmarshal([]byte(contentJSON), &sentences); err != nil {
		slog.Error("failed to unmarshal content_furigana_json", "err", err, "lesson_id", lessonID)
		return nil, fmt.Errorf("data.LessonStore.GetSentences unmarshal: %w", err)
	}

	// sentences 已经按 index 存储，但显式排序保证正确性
	for i := 1; i < len(sentences); i++ {
		if sentences[i].Index < sentences[i-1].Index {
			// 排序（冒泡，列表通常很短）
			sentences[i], sentences[i-1] = sentences[i-1], sentences[i]
		}
	}

	slog.Debug("LessonStore.GetSentences done", "lesson_id", lessonID, "count", len(sentences))
	return sentences, nil
}
