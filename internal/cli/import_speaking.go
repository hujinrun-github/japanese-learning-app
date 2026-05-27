package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// speakingImport is the JSON shape expected for each element in the speaking import file.
type speakingImport struct {
	Type      string `json:"type"`       // "shadow" | "free"
	Title     string `json:"title"`
	Text      string `json:"text"`
	AudioURL  string `json:"audio_url"`
	JLPTLevel string `json:"jlpt_level"`
}

// ImportSpeaking reads a JSON array of speaking materials from r and inserts them
// into the speaking_materials table using INSERT OR IGNORE (idempotent – duplicate
// (type, title, jlpt_level) combinations are silently skipped).
// It returns the number of rows actually inserted.
func ImportSpeaking(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportSpeaking called")
	var items []speakingImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportSpeaking decode: %w", err)
	}
	return insertSpeakingMaterials(db, items)
}

// ImportSpeakingFromFile reads a JSON array of speaking materials from filePath and inserts
// them into the speaking_materials table using INSERT OR IGNORE (idempotent – duplicate
// (type, title, jlpt_level) combinations are silently skipped).
// It returns the number of rows actually inserted.
func ImportSpeakingFromFile(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportSpeakingFromFile called", "file", filePath)

	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportSpeakingFromFile open: %w", err)
	}
	defer f.Close()

	return ImportSpeaking(db, f)
}

// ImportSpeakingFromJSON parses a single JSON object string and inserts it into the
// speaking_materials table. Returns 1 if inserted, 0 if it was a duplicate.
func ImportSpeakingFromJSON(db *sql.DB, jsonStr string) (int, error) {
	slog.Debug("ImportSpeakingFromJSON called", "json", jsonStr)

	var item speakingImport
	if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
		return 0, fmt.Errorf("cli.ImportSpeakingFromJSON Unmarshal: %w", err)
	}

	return insertSpeakingMaterials(db, []speakingImport{item})
}

// insertSpeakingMaterials is the shared core: opens a transaction and bulk-inserts speaking materials.
func insertSpeakingMaterials(db *sql.DB, items []speakingImport) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.insertSpeakingMaterials Begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO speaking_materials
			(type, title, text, audio_url, jlpt_level)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertSpeakingMaterials Prepare: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, item := range items {
		result, execErr := stmt.Exec(
			item.Type,
			item.Title,
			item.Text,
			item.AudioURL,
			item.JLPTLevel,
		)
		if execErr != nil {
			err = fmt.Errorf("cli.insertSpeakingMaterials Exec for %q: %w", item.Title, execErr)
			slog.Error("insertSpeakingMaterials Exec failed", "title", item.Title, "err", execErr)
			return 0, err
		}

		n, _ := result.RowsAffected()
		inserted += int(n)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertSpeakingMaterials Commit: %w", commitErr)
		slog.Error("insertSpeakingMaterials Commit failed", "err", commitErr)
		return 0, err
	}

	slog.Debug("insertSpeakingMaterials done", "inserted", inserted)
	return inserted, nil
}
