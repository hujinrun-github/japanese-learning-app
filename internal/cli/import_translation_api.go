package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"japanese-learning-app/internal/module/translation"
)

type translationAPIConfig struct {
	Title     string `json:"title"`
	Endpoint  string `json:"endpoint"`
	Method    string `json:"method"`
	JSONPath  string `json:"json_path"`
	Direction string `json:"direction"`
}

func ImportTranslationFromAPIConfig(db *sql.DB, configPath string) (int, error) {
	slog.Debug("ImportTranslationFromAPIConfig called", "config", configPath)

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig ReadFile: %w", err)
	}

	var config translationAPIConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig Unmarshal: %w", err)
	}

	if config.Method == "" {
		config.Method = "GET"
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(config.Method, config.Endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig new request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig read body: %w", err)
	}

	text := extractTextFromJSONPath(string(body), config.JSONPath)
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig: no text extracted from API response")
	}

	sentences := translation.SplitSentences(text)
	dirStr := translation.DetectDirection(text)
	if config.Direction != "" {
		dirStr = config.Direction
	}
	direction := translation.Direction(dirStr)

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig Begin: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content)
		 VALUES (?, 'api', ?, ?, ?)`,
		config.Title, config.Endpoint, config.Endpoint, text,
	)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig insert source: %w", err)
	}
	sourceID, _ := result.LastInsertId()

	inserted := 0
	for i, s := range sentences {
		_, err := tx.Exec(
			`INSERT INTO translation_sentences (source_id, direction, source_text, position)
			 VALUES (?, ?, ?, ?)`,
			sourceID, string(direction), s, i,
		)
		if err != nil {
			return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig insert sentence: %w", err)
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig Commit: %w", err)
	}

	slog.Debug("ImportTranslationFromAPIConfig done", "source_id", sourceID, "sentences", inserted)
	return inserted, nil
}

// extractTextFromJSONPath extracts text from JSON using dot-notation path.
func extractTextFromJSONPath(jsonStr, path string) string {
	if path == "" {
		return jsonStr
	}

	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonStr
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return fmt.Sprintf("%v", current)
		}
		current = m[part]
	}
	return fmt.Sprintf("%v", current)
}
