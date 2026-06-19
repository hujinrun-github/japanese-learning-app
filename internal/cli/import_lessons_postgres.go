package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

func ImportLessonsToPostgresFromFile(db *sql.DB, filePath string) (int, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportLessonsToPostgresFromFile ReadFile: %w", err)
	}

	var items []lessonImport
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("cli.ImportLessonsToPostgresFromFile Unmarshal: %w", err)
	}

	return insertLessonsPostgres(db, items)
}

func ImportLessonToPostgresFromJSON(db *sql.DB, jsonStr string) (int, error) {
	var item lessonImport
	if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
		return 0, fmt.Errorf("cli.ImportLessonToPostgresFromJSON Unmarshal: %w", err)
	}

	return insertLessonsPostgres(db, []lessonImport{item})
}

func insertLessonsPostgres(db *sql.DB, items []lessonImport) (int, error) {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cli.insertLessonsPostgres BeginTx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	inserted := 0
	for _, item := range items {
		lessonID, wasInserted, upsertErr := upsertPostgresLesson(ctx, tx, item)
		if upsertErr != nil {
			err = upsertErr
			return 0, err
		}
		if wasInserted {
			inserted++
		}
		if replaceErr := replacePostgresLessonChildren(ctx, tx, lessonID, item); replaceErr != nil {
			err = replaceErr
			return 0, err
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertLessonsPostgres Commit: %w", commitErr)
		return 0, err
	}
	return inserted, nil
}

func upsertPostgresLesson(ctx context.Context, tx *sql.Tx, item lessonImport) (int64, bool, error) {
	row, err := buildPostgresLessonRow(item)
	if err != nil {
		return 0, false, err
	}

	var existingID int64
	findErr := tx.QueryRowContext(
		ctx,
		`SELECT id FROM lessons WHERE title = $1 AND jlpt_level = $2`,
		row.title,
		row.jlptLevel,
	).Scan(&existingID)
	switch findErr {
	case nil:
		_, err := tx.ExecContext(
			ctx,
			`UPDATE lessons
			 SET tags = COALESCE((SELECT array_agg(value) FROM jsonb_array_elements_text($2::jsonb) AS value), ARRAY[]::text[]),
			     char_count = $3,
			     shadowing_enabled = $4,
			     shadowing_version = $5,
			     shadowing_config_json = $6::jsonb,
			     updated_at = now()
			 WHERE id = $1`,
			existingID,
			row.tagsJSON,
			row.charCount,
			row.shadowingEnabled != 0,
			row.shadowingVersion,
			row.shadowingConfigJSON,
		)
		if err != nil {
			return 0, false, fmt.Errorf("cli.upsertPostgresLesson update %q: %w", item.Title, err)
		}
		return existingID, false, nil
	case sql.ErrNoRows:
		var lessonID int64
		err := tx.QueryRowContext(
			ctx,
			`INSERT INTO lessons (
			     title, jlpt_level, tags, char_count,
			     shadowing_enabled, shadowing_version, shadowing_config_json, updated_at
			 )
			 VALUES (
			     $1,
			     $2,
			     COALESCE((SELECT array_agg(value) FROM jsonb_array_elements_text($3::jsonb) AS value), ARRAY[]::text[]),
			     $4,
			     $5,
			     $6,
			     $7::jsonb,
			     now()
			 )
			 RETURNING id`,
			row.title,
			row.jlptLevel,
			row.tagsJSON,
			row.charCount,
			row.shadowingEnabled != 0,
			row.shadowingVersion,
			row.shadowingConfigJSON,
		).Scan(&lessonID)
		if err != nil {
			return 0, false, fmt.Errorf("cli.upsertPostgresLesson insert %q: %w", item.Title, err)
		}
		return lessonID, true, nil
	default:
		return 0, false, fmt.Errorf("cli.upsertPostgresLesson find %q/%s: %w", item.Title, item.JLPTLevel, findErr)
	}
}

func buildPostgresLessonRow(item lessonImport) (lessonRow, error) {
	row, err := buildLessonRow(item)
	if err != nil {
		return lessonRow{}, err
	}

	shadowingConfig := item.ShadowingConfig
	if shadowingConfig == nil {
		shadowingConfig = map[string]any{}
	}
	if item.AudioURL != "" {
		if _, ok := shadowingConfig["media_url"]; !ok {
			if _, ok := shadowingConfig["audio_url"]; !ok {
				shadowingConfig["media_url"] = item.AudioURL
			}
		}
	}
	shadowingConfigJSON, err := json.Marshal(shadowingConfig)
	if err != nil {
		return lessonRow{}, fmt.Errorf("cli.buildPostgresLessonRow marshal shadowing_config for %q: %w", item.Title, err)
	}
	row.shadowingConfigJSON = string(shadowingConfigJSON)

	tagsJSON, err := json.Marshal(normalizeAnyStringSlice(item.Tags))
	if err != nil {
		return lessonRow{}, fmt.Errorf("cli.buildPostgresLessonRow marshal tags for %q: %w", item.Title, err)
	}
	row.tagsJSON = string(tagsJSON)

	return row, nil
}

func replacePostgresLessonChildren(ctx context.Context, tx *sql.Tx, lessonID int64, item lessonImport) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM lesson_words WHERE lesson_id = $1`, lessonID); err != nil {
		return fmt.Errorf("cli.replacePostgresLessonChildren delete words: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM lesson_sentences WHERE lesson_id = $1`, lessonID); err != nil {
		return fmt.Errorf("cli.replacePostgresLessonChildren delete sentences: %w", err)
	}

	for _, sentence := range item.Sentences {
		tokensJSON, err := json.Marshal(sentence.Tokens)
		if err != nil {
			return fmt.Errorf("cli.replacePostgresLessonChildren marshal tokens for %q sentence %d: %w", item.Title, sentence.Index, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO lesson_sentences (lesson_id, position, tokens, chinese, start_ms, end_ms)
			 VALUES ($1, $2, $3::jsonb, $4, $5, $6)`,
			lessonID,
			sentence.Index,
			string(tokensJSON),
			sentence.Chinese,
			sentence.StartMS,
			sentence.EndMS,
		); err != nil {
			return fmt.Errorf("cli.replacePostgresLessonChildren insert sentence %d for %q: %w", sentence.Index, item.Title, err)
		}
	}

	wordIDs, err := normalizeAnyInt64Slice(item.WordIDs)
	if err != nil {
		return fmt.Errorf("cli.replacePostgresLessonChildren normalize word_ids for %q: %w", item.Title, err)
	}
	for idx, wordID := range wordIDs {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO lesson_words (lesson_id, word_id, position) VALUES ($1, $2, $3)`,
			lessonID,
			wordID,
			idx,
		); err != nil {
			return fmt.Errorf("cli.replacePostgresLessonChildren insert word %d for %q: %w", wordID, item.Title, err)
		}
	}

	return nil
}

func normalizeAnyStringSlice(value any) []string {
	switch typed := value.(type) {
	case nil:
		return []string{}
	case []string:
		if typed == nil {
			return []string{}
		}
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
	}
}

func normalizeAnyInt64Slice(value any) ([]int64, error) {
	switch typed := value.(type) {
	case nil:
		return []int64{}, nil
	case []int64:
		if typed == nil {
			return []int64{}, nil
		}
		return typed, nil
	case []int:
		out := make([]int64, len(typed))
		for i, item := range typed {
			out[i] = int64(item)
		}
		return out, nil
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			switch v := item.(type) {
			case float64:
				out = append(out, int64(v))
			case int:
				out = append(out, int64(v))
			case int64:
				out = append(out, v)
			default:
				return nil, fmt.Errorf("unsupported word id value %T", item)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported word_ids value %T", value)
	}
}
