package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// lessonImport is the JSON shape expected for each element in the lesson import file.
type lessonImport struct {
	Title            string           `json:"title"`
	JLPTLevel        string           `json:"jlpt_level"`
	Tags             any              `json:"tags"`
	AudioURL         string           `json:"audio_url"`
	VideoURL         string           `json:"video_url"`
	WordIDs          any              `json:"word_ids"`
	ShadowingEnabled bool             `json:"shadowing_enabled"`
	ShadowingVersion int              `json:"shadowing_version"`
	ShadowingConfig  map[string]any   `json:"shadowing_config"`
	Sentences        []sentenceImport `json:"sentences"`
}

// sentenceImport represents a single sentence in the lesson.
type sentenceImport struct {
	Index   int    `json:"index"`
	Tokens  any    `json:"tokens"`
	Chinese string `json:"chinese"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
}

type lessonRow struct {
	title               string
	jlptLevel           string
	contentJSON         string
	translationJSON     string
	tagsJSON            string
	audioURL            string
	videoURL            string
	timestampsJSON      string
	charCount           int
	wordIDsJSON         string
	shadowingEnabled    int
	shadowingVersion    int
	shadowingConfigJSON string
}

// ImportLessonsFromFile reads a JSON array of lessons from filePath and upserts
// them by (title, jlpt_level). It returns the number of rows newly inserted.
func ImportLessonsFromFile(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportLessonsFromFile called", "file", filePath)

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportLessonsFromFile ReadFile: %w", err)
	}

	var items []lessonImport
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("cli.ImportLessonsFromFile Unmarshal: %w", err)
	}

	slog.Debug("ImportLessonsFromFile parsed items", "file", filePath, "count", len(items))
	return insertLessons(db, items)
}

// ImportLessonFromJSON parses a single JSON object string and upserts it into
// the lessons table. Returns 1 if inserted, 0 if an existing lesson was updated.
func ImportLessonFromJSON(db *sql.DB, jsonStr string) (int, error) {
	slog.Debug("ImportLessonFromJSON called", "json", jsonStr)

	var item lessonImport
	if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
		return 0, fmt.Errorf("cli.ImportLessonFromJSON Unmarshal: %w", err)
	}

	return insertLessons(db, []lessonImport{item})
}

func insertLessons(db *sql.DB, items []lessonImport) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.insertLessons Begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	insertStmt, err := tx.Prepare(`
		INSERT INTO lessons
			(title, content_furigana_json, translation_json, jlpt_level,
			 tags_json, audio_url, video_url, sentence_timestamps_json, char_count,
			 word_ids_json, shadowing_enabled, shadowing_version, shadowing_config_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertLessons Prepare insert: %w", err)
	}
	defer insertStmt.Close()

	updateStmt, err := tx.Prepare(`
		UPDATE lessons
		SET content_furigana_json = ?,
		    translation_json = ?,
		    tags_json = ?,
		    audio_url = ?,
		    video_url = ?,
		    sentence_timestamps_json = ?,
		    char_count = ?,
		    word_ids_json = ?,
		    shadowing_enabled = ?,
		    shadowing_version = ?,
		    shadowing_config_json = ?
		WHERE id = ?
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertLessons Prepare update: %w", err)
	}
	defer updateStmt.Close()

	inserted := 0
	for _, item := range items {
		row, buildErr := buildLessonRow(item)
		if buildErr != nil {
			err = buildErr
			return 0, err
		}

		existingID, findErr := findLessonID(tx, item.Title, item.JLPTLevel)
		if findErr != nil {
			err = findErr
			return 0, err
		}

		if existingID == 0 {
			result, execErr := insertStmt.Exec(
				row.title,
				row.contentJSON,
				row.translationJSON,
				row.jlptLevel,
				row.tagsJSON,
				row.audioURL,
				row.videoURL,
				row.timestampsJSON,
				row.charCount,
				row.wordIDsJSON,
				row.shadowingEnabled,
				row.shadowingVersion,
				row.shadowingConfigJSON,
			)
			if execErr != nil {
				err = fmt.Errorf("cli.insertLessons insert for %q: %w", item.Title, execErr)
				slog.Error("insertLessons insert failed", "title", item.Title, "err", execErr)
				return 0, err
			}
			n, _ := result.RowsAffected()
			inserted += int(n)
			continue
		}

		if _, execErr := updateStmt.Exec(
			row.contentJSON,
			row.translationJSON,
			row.tagsJSON,
			row.audioURL,
			row.videoURL,
			row.timestampsJSON,
			row.charCount,
			row.wordIDsJSON,
			row.shadowingEnabled,
			row.shadowingVersion,
			row.shadowingConfigJSON,
			existingID,
		); execErr != nil {
			err = fmt.Errorf("cli.insertLessons update for %q: %w", item.Title, execErr)
			slog.Error("insertLessons update failed", "title", item.Title, "err", execErr)
			return 0, err
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertLessons Commit: %w", commitErr)
		slog.Error("insertLessons Commit failed", "err", commitErr)
		return 0, err
	}

	slog.Debug("insertLessons done", "inserted", inserted)
	return inserted, nil
}

func findLessonID(tx *sql.Tx, title, level string) (int64, error) {
	rows, err := tx.Query(
		`SELECT id FROM lessons WHERE title = ? AND jlpt_level = ? ORDER BY id`,
		title,
		level,
	)
	if err != nil {
		return 0, fmt.Errorf("cli.findLessonID %q/%s: %w", title, level, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("cli.findLessonID scan %q/%s: %w", title, level, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("cli.findLessonID rows %q/%s: %w", title, level, err)
	}

	switch len(ids) {
	case 0:
		return 0, nil
	case 1:
		return ids[0], nil
	default:
		return 0, fmt.Errorf("duplicate lessons for title %q and jlpt_level %q (ids %v); run cleanup-lesson-duplicates before import", title, level, ids)
	}
}

func buildLessonRow(item lessonImport) (lessonRow, error) {
	contentJSON, err := json.Marshal(item.Sentences)
	if err != nil {
		slog.Error("insertLessons marshal sentences failed", "title", item.Title, "err", err)
		return lessonRow{}, fmt.Errorf("cli.insertLessons marshal sentences for %q: %w", item.Title, err)
	}

	translations := make([]string, len(item.Sentences))
	for i, s := range item.Sentences {
		translations[i] = s.Chinese
	}
	translationJSON, err := json.Marshal(translations)
	if err != nil {
		slog.Error("insertLessons marshal translation failed", "title", item.Title, "err", err)
		return lessonRow{}, fmt.Errorf("cli.insertLessons marshal translation for %q: %w", item.Title, err)
	}

	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		slog.Error("insertLessons marshal tags failed", "title", item.Title, "err", err)
		return lessonRow{}, fmt.Errorf("cli.insertLessons marshal tags for %q: %w", item.Title, err)
	}

	type tsEntry struct {
		Index   int   `json:"index"`
		StartMS int64 `json:"start_ms"`
		EndMS   int64 `json:"end_ms"`
	}
	timestamps := make([]tsEntry, len(item.Sentences))
	for i, s := range item.Sentences {
		timestamps[i] = tsEntry{Index: s.Index, StartMS: s.StartMS, EndMS: s.EndMS}
	}
	timestampsJSON, err := json.Marshal(timestamps)
	if err != nil {
		slog.Error("insertLessons marshal timestamps failed", "title", item.Title, "err", err)
		return lessonRow{}, fmt.Errorf("cli.insertLessons marshal timestamps for %q: %w", item.Title, err)
	}

	wordIDsJSON, err := json.Marshal(item.WordIDs)
	if err != nil {
		slog.Error("insertLessons marshal word_ids failed", "title", item.Title, "err", err)
		return lessonRow{}, fmt.Errorf("cli.insertLessons marshal word_ids for %q: %w", item.Title, err)
	}

	shadowingVersion := item.ShadowingVersion
	if shadowingVersion == 0 {
		shadowingVersion = 1
	}
	shadowingConfig := item.ShadowingConfig
	if shadowingConfig == nil {
		shadowingConfig = map[string]any{}
	}
	shadowingConfigJSON, err := json.Marshal(shadowingConfig)
	if err != nil {
		slog.Error("insertLessons marshal shadowing_config failed", "title", item.Title, "err", err)
		return lessonRow{}, fmt.Errorf("cli.insertLessons marshal shadowing_config for %q: %w", item.Title, err)
	}

	return lessonRow{
		title:               item.Title,
		jlptLevel:           item.JLPTLevel,
		contentJSON:         string(contentJSON),
		translationJSON:     string(translationJSON),
		tagsJSON:            string(tagsJSON),
		audioURL:            item.AudioURL,
		videoURL:            item.VideoURL,
		timestampsJSON:      string(timestampsJSON),
		charCount:           lessonCharCount(item.Sentences),
		wordIDsJSON:         string(wordIDsJSON),
		shadowingEnabled:    boolToInt(item.ShadowingEnabled),
		shadowingVersion:    shadowingVersion,
		shadowingConfigJSON: string(shadowingConfigJSON),
	}, nil
}

func lessonCharCount(sentences []sentenceImport) int {
	charCount := 0
	for _, s := range sentences {
		tokens, ok := s.Tokens.([]interface{})
		if !ok {
			continue
		}
		for _, tok := range tokens {
			m, ok := tok.(map[string]interface{})
			if !ok {
				continue
			}
			surface, ok := m["surface"].(string)
			if ok {
				charCount += len([]rune(surface))
			}
		}
	}
	return charCount
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
