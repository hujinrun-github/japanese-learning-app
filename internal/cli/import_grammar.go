package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// grammarImport is the JSON shape expected for each element in the grammar import file.
type grammarImport struct {
	Name            string          `json:"name"`
	Meaning         string          `json:"meaning"`
	ConjunctionRule string          `json:"conjunction_rule"`
	UsageNote       string          `json:"usage_note"`
	Examples        any             `json:"examples"`
	QuizQuestions   any             `json:"quiz_questions"`
	JLPTLevel       string          `json:"jlpt_level"`
}

// ImportGrammarFromFile reads a JSON array of grammar points from filePath and inserts them
// into the grammar_points table using INSERT OR IGNORE (idempotent – duplicate names per
// jlpt_level are silently skipped).
// It returns the number of rows actually inserted.
func ImportGrammarFromFile(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportGrammarFromFile called", "file", filePath)

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportGrammarFromFile ReadFile: %w", err)
	}

	var items []grammarImport
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("cli.ImportGrammarFromFile Unmarshal: %w", err)
	}

	slog.Debug("ImportGrammarFromFile parsed items", "file", filePath, "count", len(items))
	return insertGrammarPoints(db, items)
}

// ImportGrammarFromJSON parses a single JSON object string and inserts it into the
// grammar_points table. Returns 1 if inserted, 0 if it was a duplicate.
func ImportGrammarFromJSON(db *sql.DB, jsonStr string) (int, error) {
	slog.Debug("ImportGrammarFromJSON called", "json", jsonStr)

	var item grammarImport
	if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
		return 0, fmt.Errorf("cli.ImportGrammarFromJSON Unmarshal: %w", err)
	}

	return insertGrammarPoints(db, []grammarImport{item})
}

// insertGrammarPoints is the shared core: opens a transaction and bulk-inserts grammar points.
func insertGrammarPoints(db *sql.DB, items []grammarImport) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.insertGrammarPoints Begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO grammar_points
			(name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertGrammarPoints Prepare: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, item := range items {
		examplesJSON, jsonErr := json.Marshal(item.Examples)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertGrammarPoints marshal examples for %q: %w", item.Name, jsonErr)
			slog.Error("insertGrammarPoints marshal examples failed", "name", item.Name, "err", jsonErr)
			return 0, err
		}

		quizJSON, jsonErr := json.Marshal(item.QuizQuestions)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertGrammarPoints marshal quiz_questions for %q: %w", item.Name, jsonErr)
			slog.Error("insertGrammarPoints marshal quiz_questions failed", "name", item.Name, "err", jsonErr)
			return 0, err
		}

		result, execErr := stmt.Exec(
			item.Name,
			item.Meaning,
			item.ConjunctionRule,
			item.UsageNote,
			string(examplesJSON),
			string(quizJSON),
			item.JLPTLevel,
		)
		if execErr != nil {
			err = fmt.Errorf("cli.insertGrammarPoints Exec for %q: %w", item.Name, execErr)
			slog.Error("insertGrammarPoints Exec failed", "name", item.Name, "err", execErr)
			return 0, err
		}

		n, _ := result.RowsAffected()
		inserted += int(n)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertGrammarPoints Commit: %w", commitErr)
		slog.Error("insertGrammarPoints Commit failed", "err", commitErr)
		return 0, err
	}

	slog.Debug("insertGrammarPoints done", "inserted", inserted)
	return inserted, nil
}
