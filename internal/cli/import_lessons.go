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
	Title     string          `json:"title"`
	JLPTLevel string          `json:"jlpt_level"`
	Tags      any             `json:"tags"`
	AudioURL  string          `json:"audio_url"`
	WordIDs   any             `json:"word_ids"`
	Sentences []sentenceImport `json:"sentences"`
}

// sentenceImport represents a single sentence in the lesson.
type sentenceImport struct {
	Index   int              `json:"index"`
	Tokens  any              `json:"tokens"`
	Chinese string           `json:"chinese"`
	StartMS int64            `json:"start_ms"`
	EndMS   int64            `json:"end_ms"`
}

// ImportLessonsFromFile reads a JSON array of lessons from filePath and inserts them
// into the lessons table using INSERT OR IGNORE (idempotent – duplicate titles per
// jlpt_level are silently skipped).
// It returns the number of rows actually inserted.
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

// ImportLessonFromJSON parses a single JSON object string and inserts it into the
// lessons table. Returns 1 if inserted, 0 if it was a duplicate.
func ImportLessonFromJSON(db *sql.DB, jsonStr string) (int, error) {
	slog.Debug("ImportLessonFromJSON called", "json", jsonStr)

	var item lessonImport
	if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
		return 0, fmt.Errorf("cli.ImportLessonFromJSON Unmarshal: %w", err)
	}

	return insertLessons(db, []lessonImport{item})
}

// insertLessons is the shared core: opens a transaction and bulk-inserts lessons.
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

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO lessons
			(title, content_furigana_json, translation_json, jlpt_level,
			 tags_json, audio_url, sentence_timestamps_json, char_count, word_ids_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertLessons Prepare: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, item := range items {
		// Build content_furigana_json (array of Sentence tokens)
		contentJSON, jsonErr := json.Marshal(item.Sentences)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertLessons marshal sentences for %q: %w", item.Title, jsonErr)
			slog.Error("insertLessons marshal sentences failed", "title", item.Title, "err", jsonErr)
			return 0, err
		}

		// Build translation_json (array of Chinese translations per sentence)
		translations := make([]string, len(item.Sentences))
		for i, s := range item.Sentences {
			translations[i] = s.Chinese
		}
		translationJSON, jsonErr := json.Marshal(translations)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertLessons marshal translation for %q: %w", item.Title, jsonErr)
			slog.Error("insertLessons marshal translation failed", "title", item.Title, "err", jsonErr)
			return 0, err
		}

		tagsJSON, jsonErr := json.Marshal(item.Tags)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertLessons marshal tags for %q: %w", item.Title, jsonErr)
			slog.Error("insertLessons marshal tags failed", "title", item.Title, "err", jsonErr)
			return 0, err
		}

		// Build sentence_timestamps_json
		type tsEntry struct {
			Index   int   `json:"index"`
			StartMS int64 `json:"start_ms"`
			EndMS   int64 `json:"end_ms"`
		}
		timestamps := make([]tsEntry, len(item.Sentences))
		for i, s := range item.Sentences {
			timestamps[i] = tsEntry{Index: s.Index, StartMS: s.StartMS, EndMS: s.EndMS}
		}
		timestampsJSON, jsonErr := json.Marshal(timestamps)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertLessons marshal timestamps for %q: %w", item.Title, jsonErr)
			slog.Error("insertLessons marshal timestamps failed", "title", item.Title, "err", jsonErr)
			return 0, err
		}

		wordIDsJSON, jsonErr := json.Marshal(item.WordIDs)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertLessons marshal word_ids for %q: %w", item.Title, jsonErr)
			slog.Error("insertLessons marshal word_ids failed", "title", item.Title, "err", jsonErr)
			return 0, err
		}

		// Calculate total char count from all sentence tokens
		charCount := 0
		for _, s := range item.Sentences {
			// Rough char count: sum surface lengths
			if tokens, ok := s.Tokens.([]interface{}); ok {
				for _, tok := range tokens {
					if m, ok := tok.(map[string]interface{}); ok {
						if surface, ok := m["surface"].(string); ok {
							charCount += len([]rune(surface))
						}
					}
				}
			}
		}

		result, execErr := stmt.Exec(
			item.Title,
			string(contentJSON),
			string(translationJSON),
			item.JLPTLevel,
			string(tagsJSON),
			item.AudioURL,
			string(timestampsJSON),
			charCount,
			string(wordIDsJSON),
		)
		if execErr != nil {
			err = fmt.Errorf("cli.insertLessons Exec for %q: %w", item.Title, execErr)
			slog.Error("insertLessons Exec failed", "title", item.Title, "err", execErr)
			return 0, err
		}

		n, _ := result.RowsAffected()
		inserted += int(n)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertLessons Commit: %w", commitErr)
		slog.Error("insertLessons Commit failed", "err", commitErr)
		return 0, err
	}

	slog.Debug("insertLessons done", "inserted", inserted)
	return inserted, nil
}
