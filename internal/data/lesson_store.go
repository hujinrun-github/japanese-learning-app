package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/lesson"
)

// LessonStore implements lesson data access for the SQLite lessons table.
type LessonStore struct {
	db *sql.DB
}

// NewLessonStore creates a LessonStore instance.
func NewLessonStore(db *sql.DB) *LessonStore {
	return &LessonStore{db: db}
}

// ListSummaries queries lessons by JLPT level and optionally filters by tag.
func (s *LessonStore) ListSummaries(level lesson.JLPTLevel, tag string) ([]lesson.LessonSummary, error) {
	slog.Debug("LessonStore.ListSummaries called", "level", level, "tag", tag)

	query := `SELECT id, title, jlpt_level, tags_json, char_count, audio_url,
	                 video_url, shadowing_enabled, shadowing_version, shadowing_config_json
	          FROM lessons WHERE jlpt_level = ? ORDER BY id`
	args := []any{level}
	if tag != "" {
		query = `SELECT id, title, jlpt_level, tags_json, char_count, audio_url,
		                video_url, shadowing_enabled, shadowing_version, shadowing_config_json
		         FROM lessons WHERE jlpt_level = ?
		           AND tags_json LIKE ?
		         ORDER BY id`
		args = append(args, "%\""+tag+"\"%")
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		slog.Error("failed to query lessons", "err", err, "level", level, "tag", tag)
		return nil, fmt.Errorf("data.LessonStore.ListSummaries query: %w", err)
	}
	defer rows.Close()

	var summaries []lesson.LessonSummary
	for rows.Next() {
		var ls lesson.LessonSummary
		var tagsJSON string
		var shadowingEnabled int
		var shadowingConfigJSON string
		if err := rows.Scan(
			&ls.ID,
			&ls.Title,
			&ls.JLPTLevel,
			&tagsJSON,
			&ls.CharCount,
			&ls.AudioURL,
			&ls.VideoURL,
			&shadowingEnabled,
			&ls.ShadowingVersion,
			&shadowingConfigJSON,
		); err != nil {
			slog.Error("failed to scan lesson row", "err", err)
			return nil, fmt.Errorf("data.LessonStore.ListSummaries scan: %w", err)
		}
		if err := json.Unmarshal([]byte(tagsJSON), &ls.Tags); err != nil {
			slog.Error("failed to unmarshal tags_json", "err", err, "lesson_id", ls.ID)
			return nil, fmt.Errorf("data.LessonStore.ListSummaries unmarshal tags: %w", err)
		}
		ls.ShadowingEnabled = shadowingEnabled != 0
		if err := decodeShadowingConfig(shadowingConfigJSON, &ls.ShadowingConfig); err != nil {
			slog.Error("failed to unmarshal shadowing_config_json", "err", err, "lesson_id", ls.ID)
			return nil, fmt.Errorf("data.LessonStore.ListSummaries unmarshal shadowing config: %w", err)
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

// GetDetail queries a lesson with sentences, word IDs, and shadowing metadata.
func (s *LessonStore) GetDetail(id int64) (*lesson.Lesson, error) {
	slog.Debug("LessonStore.GetDetail called", "lesson_id", id)

	row := s.db.QueryRow(
		`SELECT id, title, jlpt_level, tags_json, char_count, audio_url,
		        video_url, shadowing_enabled, shadowing_version, shadowing_config_json,
		        content_furigana_json, word_ids_json
		 FROM lessons WHERE id = ?`, id,
	)

	var l lesson.Lesson
	var tagsJSON, contentJSON, wordIDsJSON, shadowingConfigJSON string
	var shadowingEnabled int
	err := row.Scan(
		&l.ID,
		&l.Title,
		&l.JLPTLevel,
		&tagsJSON,
		&l.CharCount,
		&l.AudioURL,
		&l.VideoURL,
		&shadowingEnabled,
		&l.ShadowingVersion,
		&shadowingConfigJSON,
		&contentJSON,
		&wordIDsJSON,
	)
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
	l.ShadowingEnabled = shadowingEnabled != 0
	if err := decodeShadowingConfig(shadowingConfigJSON, &l.ShadowingConfig); err != nil {
		slog.Error("failed to unmarshal shadowing_config_json", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("data.LessonStore.GetDetail unmarshal shadowing config: %w", err)
	}

	slog.Debug("LessonStore.GetDetail done", "lesson_id", id, "sentences", len(l.Sentences))
	return &l, nil
}

func decodeShadowingConfig(raw string, out *map[string]any) error {
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return err
	}
	if *out == nil {
		*out = map[string]any{}
	}
	return nil
}

// GetSentences returns lesson sentences ordered by index.
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

	for i := 1; i < len(sentences); i++ {
		if sentences[i].Index < sentences[i-1].Index {
			sentences[i], sentences[i-1] = sentences[i-1], sentences[i]
		}
	}

	slog.Debug("LessonStore.GetSentences done", "lesson_id", lessonID, "count", len(sentences))
	return sentences, nil
}
