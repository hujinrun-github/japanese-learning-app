package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"japanese-learning-app/internal/module/lesson"
)

type LessonStore struct {
	db queryer
}

func NewLessonStore(db queryer) *LessonStore {
	return &LessonStore{db: db}
}

func (s *LessonStore) ListSummaries(level lesson.JLPTLevel) ([]lesson.LessonSummary, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT l.id,
		        l.title,
		        l.jlpt_level,
		        COALESCE(array_to_json(l.tags)::text, '[]'),
		        l.char_count,
		        COALESCE(
		            l.shadowing_config_json->>'media_url',
		            l.shadowing_config_json->>'audio_url',
		            CASE WHEN ao.id IS NOT NULL THEN '/api/v1/audio/' || ao.id::text || '/stream' ELSE '' END
		        ) AS audio_url,
		        l.shadowing_enabled,
		        l.shadowing_version,
		        l.shadowing_config_json::text
		 FROM lessons l
		 LEFT JOIN audio_objects ao ON ao.id = l.audio_object_id AND ao.deleted_at IS NULL
		 WHERE l.jlpt_level = $1
		 ORDER BY l.id`,
		level,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.ListSummaries query: %w", translateError(err))
	}
	defer rows.Close()

	var summaries []lesson.LessonSummary
	for rows.Next() {
		var summary lesson.LessonSummary
		var tagsJSON string
		var shadowingConfigJSON string
		if err := rows.Scan(
			&summary.ID,
			&summary.Title,
			&summary.JLPTLevel,
			&tagsJSON,
			&summary.CharCount,
			&summary.AudioURL,
			&summary.ShadowingEnabled,
			&summary.ShadowingVersion,
			&shadowingConfigJSON,
		); err != nil {
			return nil, fmt.Errorf("postgres.LessonStore.ListSummaries scan: %w", translateError(err))
		}
		if err := json.Unmarshal([]byte(tagsJSON), &summary.Tags); err != nil {
			return nil, fmt.Errorf("postgres.LessonStore.ListSummaries unmarshal tags: %w", err)
		}
		if err := decodeLessonShadowingConfig(shadowingConfigJSON, &summary.ShadowingConfig); err != nil {
			return nil, fmt.Errorf("postgres.LessonStore.ListSummaries unmarshal shadowing config: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.ListSummaries rows: %w", translateError(err))
	}

	return summaries, nil
}

func (s *LessonStore) GetDetail(id int64) (*lesson.Lesson, error) {
	var detail lesson.Lesson
	var tagsJSON string
	var shadowingConfigJSON string

	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT l.id,
		        l.title,
		        l.jlpt_level,
		        COALESCE(array_to_json(l.tags)::text, '[]'),
		        l.char_count,
		        COALESCE(
		            l.shadowing_config_json->>'media_url',
		            l.shadowing_config_json->>'audio_url',
		            CASE WHEN ao.id IS NOT NULL THEN '/api/v1/audio/' || ao.id::text || '/stream' ELSE '' END
		        ) AS audio_url,
		        l.shadowing_enabled,
		        l.shadowing_version,
		        l.shadowing_config_json::text
		 FROM lessons l
		 LEFT JOIN audio_objects ao ON ao.id = l.audio_object_id AND ao.deleted_at IS NULL
		 WHERE l.id = $1`,
		id,
	).Scan(
		&detail.ID,
		&detail.Title,
		&detail.JLPTLevel,
		&tagsJSON,
		&detail.CharCount,
		&detail.AudioURL,
		&detail.ShadowingEnabled,
		&detail.ShadowingVersion,
		&shadowingConfigJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.GetDetail: %w", translateError(err))
	}
	if err := json.Unmarshal([]byte(tagsJSON), &detail.Tags); err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.GetDetail unmarshal tags: %w", err)
	}
	if err := decodeLessonShadowingConfig(shadowingConfigJSON, &detail.ShadowingConfig); err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.GetDetail unmarshal shadowing config: %w", err)
	}

	sentences, err := s.GetSentences(id)
	if err != nil {
		return nil, err
	}
	wordIDs, err := s.loadLessonWordIDs(id)
	if err != nil {
		return nil, err
	}

	detail.Sentences = sentences
	detail.WordIDs = wordIDs
	return &detail, nil
}

func decodeLessonShadowingConfig(raw string, out *map[string]any) error {
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

func (s *LessonStore) GetSentences(lessonID int64) ([]lesson.Sentence, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT position, tokens::text, chinese, start_ms, end_ms
		 FROM lesson_sentences
		 WHERE lesson_id = $1
		 ORDER BY position`,
		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.GetSentences query: %w", translateError(err))
	}
	defer rows.Close()

	var sentences []lesson.Sentence
	for rows.Next() {
		var sentence lesson.Sentence
		var tokensJSON string
		if err := rows.Scan(&sentence.Index, &tokensJSON, &sentence.Chinese, &sentence.StartMS, &sentence.EndMS); err != nil {
			return nil, fmt.Errorf("postgres.LessonStore.GetSentences scan: %w", translateError(err))
		}
		if err := json.Unmarshal([]byte(tokensJSON), &sentence.Tokens); err != nil {
			return nil, fmt.Errorf("postgres.LessonStore.GetSentences unmarshal tokens: %w", err)
		}
		sentences = append(sentences, sentence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.GetSentences rows: %w", translateError(err))
	}
	if len(sentences) > 0 {
		return sentences, nil
	}

	var exists int
	if err := s.db.QueryRowContext(context.Background(), `SELECT 1 FROM lessons WHERE id = $1`, lessonID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.GetSentences lesson lookup: %w", translateError(err))
	}

	return sentences, nil
}

func (s *LessonStore) loadLessonWordIDs(lessonID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT word_id
		 FROM lesson_words
		 WHERE lesson_id = $1
		 ORDER BY position`,
		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.loadLessonWordIDs query: %w", translateError(err))
	}
	defer rows.Close()

	var wordIDs []int64
	for rows.Next() {
		var wordID int64
		if err := rows.Scan(&wordID); err != nil {
			return nil, fmt.Errorf("postgres.LessonStore.loadLessonWordIDs scan: %w", translateError(err))
		}
		wordIDs = append(wordIDs, wordID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.LessonStore.loadLessonWordIDs rows: %w", translateError(err))
	}

	return wordIDs, nil
}
