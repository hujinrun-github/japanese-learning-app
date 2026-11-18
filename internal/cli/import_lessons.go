package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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
	Index     int    `json:"index"`
	Tokens    any    `json:"tokens"`
	Chinese   string `json:"chinese"`
	StartMS   int64  `json:"start_ms"`
	EndMS     int64  `json:"end_ms"`
	Paragraph int    `json:"paragraph"` // 所属段落序号；缺省 0 表示整篇第一段
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
	return ImportLessonsToPostgresFromFile(db, filePath)
}
func ImportLessonFromJSON(db *sql.DB, jsonStr string) (int, error) {
	return ImportLessonToPostgresFromJSON(db, jsonStr)
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
