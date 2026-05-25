package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	ReadingType  string `json:"reading_type"`
}

// ImportWords reads a JSON array of words from r and inserts them into the
// words table using INSERT OR IGNORE (idempotent – duplicate (kanji_form, reading)
// pairs are silently skipped).
// It returns the number of rows actually inserted.
func ImportWords(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportWords called")
	var items []wordImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportWords decode: %w", err)
	}
	return insertWords(db, items)
}

// ImportWordsFromFile reads a JSON array of words from filePath and inserts them
// into the words table. When autoFill is true, missing reading/part_of_speech fields
// are filled using the kagome morphological analyzer.
func ImportWordsFromFile(db *sql.DB, filePath string, autoFill bool) (int, error) {
	slog.Debug("ImportWordsFromFile called", "file", filePath, "autoFill", autoFill)

	if !autoFill {
		f, err := os.Open(filePath)
		if err != nil {
			return 0, fmt.Errorf("cli.ImportWordsFromFile open: %w", err)
		}
		defer f.Close()
		return ImportWords(db, f)
	}

	// autoFill path: must decode, fill, then insert
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportWordsFromFile ReadFile: %w", err)
	}

	var words []wordImport
	if err := json.Unmarshal(raw, &words); err != nil {
		return 0, fmt.Errorf("cli.ImportWordsFromFile Unmarshal: %w", err)
	}

	words = AutoFillWords(words)

	inserted, err := insertWords(db, words)
	if err != nil {
		return 0, err
	}

	slog.Debug("ImportWordsFromFile done", "file", filePath, "inserted", inserted)
	return inserted, nil
}

// insertWords opens a transaction and bulk-inserts words.
func insertWords(db *sql.DB, items []wordImport) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.insertWords Begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO words
			(kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level, reading_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("cli.insertWords Prepare: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, w := range items {
		examplesJSON, jsonErr := json.Marshal(w.Examples)
		if jsonErr != nil {
			err = fmt.Errorf("cli.insertWords marshal examples: %w", jsonErr)
			return 0, err
		}

		result, execErr := stmt.Exec(
			w.KanjiForm,
			w.Reading,
			w.PartOfSpeech,
			w.Meaning,
			string(examplesJSON),
			w.JLPTLevel,
			w.ReadingType,
		)
		if execErr != nil {
			err = fmt.Errorf("cli.insertWords Exec: %w", execErr)
			return 0, err
		}

		n, _ := result.RowsAffected()
		inserted += int(n)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("cli.insertWords Commit: %w", commitErr)
		return 0, err
	}

	slog.Debug("insertWords done", "inserted", inserted)
	return inserted, nil
}
