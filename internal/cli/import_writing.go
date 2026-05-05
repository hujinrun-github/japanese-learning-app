package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// writingImport is the JSON shape expected for each element in the writing import file.
type writingImport struct {
	Type           string `json:"type"`             // "input" | "sentence"
	Prompt         string `json:"prompt"`
	ExpectedAnswer string `json:"expected_answer"`
	GrammarPointID int64  `json:"grammar_point_id"` // 0 means no association
	JLPTLevel      string `json:"jlpt_level"`
}

// ImportWritingFromFile reads a JSON array of writing questions from filePath and inserts
// them into the writing_questions table using INSERT OR IGNORE (idempotent – duplicate
// (type, prompt) combinations are silently skipped).
// It returns the number of rows actually inserted.
func ImportWritingFromFile(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportWritingFromFile called", "file", filePath)

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportWritingFromFile ReadFile: %w", err)
	}

	var items []writingImport
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("cli.ImportWritingFromFile Unmarshal: %w", err)
	}

	slog.Debug("ImportWritingFromFile parsed items", "file", filePath, "count", len(items))
	return insertWritingQuestions(db, items)
}

// ImportWritingFromJSON parses a single JSON object string and inserts it into the
// writing_questions table. Returns 1 if inserted, 0 if it was a duplicate.
func ImportWritingFromJSON(db *sql.DB, jsonStr string) (int, error) {
	slog.Debug("ImportWritingFromJSON called", "json", jsonStr)

	var item writingImport
	if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
		return 0, fmt.Errorf("cli.ImportWritingFromJSON Unmarshal: %w", err)
	}

	return insertWritingQuestions(db, []writingImport{item})
}

// insertWritingQuestions is the shared core: opens a transaction and bulk-inserts writing questions.
func insertWritingQuestions(db *sql.DB, items []writingImport) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.insertWritingQuestions Begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO writing_questions
			(type, prompt, expected_answer, grammar_point_id, jlpt_level)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertWritingQuestions Prepare: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, item := range items {
		result, execErr := stmt.Exec(
			item.Type,
			item.Prompt,
			item.ExpectedAnswer,
			item.GrammarPointID,
			item.JLPTLevel,
		)
		if execErr != nil {
			err = fmt.Errorf("cli.insertWritingQuestions Exec for %q: %w", item.Prompt, execErr)
			slog.Error("insertWritingQuestions Exec failed", "prompt", item.Prompt, "err", execErr)
			return 0, err
		}

		n, _ := result.RowsAffected()
		inserted += int(n)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertWritingQuestions Commit: %w", commitErr)
		slog.Error("insertWritingQuestions Commit failed", "err", commitErr)
		return 0, err
	}

	slog.Debug("insertWritingQuestions done", "inserted", inserted)
	return inserted, nil
}
