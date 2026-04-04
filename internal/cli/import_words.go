package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// wordImport is the JSON shape expected for each element in the import file.
type wordImport struct {
	KanjiForm    string `json:"kanji_form"`
	Reading      string `json:"reading"`
	PartOfSpeech string `json:"part_of_speech"`
	Meaning      string `json:"meaning"`
	Examples     any    `json:"examples"`
	JLPTLevel    string `json:"jlpt_level"`
}

// ImportWords reads a JSON array of words from filePath and inserts them into the
// words table using INSERT OR IGNORE (idempotent – duplicate (kanji_form, reading)
// pairs are silently skipped).
// It returns the number of rows actually inserted.
func ImportWords(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportWords called", "file", filePath)

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportWords ReadFile: %w", err)
	}

	var words []wordImport
	if err := json.Unmarshal(raw, &words); err != nil {
		return 0, fmt.Errorf("cli.ImportWords Unmarshal: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.ImportWords Begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO words
			(kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportWords Prepare: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, w := range words {
		examplesJSON, jsonErr := json.Marshal(w.Examples)
		if jsonErr != nil {
			err = fmt.Errorf("cli.ImportWords marshal examples: %w", jsonErr)
			return 0, err
		}

		result, execErr := stmt.Exec(
			w.KanjiForm,
			w.Reading,
			w.PartOfSpeech,
			w.Meaning,
			string(examplesJSON),
			w.JLPTLevel,
		)
		if execErr != nil {
			err = fmt.Errorf("cli.ImportWords Exec: %w", execErr)
			return 0, err
		}

		n, _ := result.RowsAffected()
		inserted += int(n)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.ImportWords Commit: %w", commitErr)
		return 0, err
	}

	slog.Debug("ImportWords done", "file", filePath, "inserted", inserted)
	return inserted, nil
}
