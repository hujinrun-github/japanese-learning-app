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
// words table using INSERT ... ON CONFLICT DO UPDATE. On conflict (kanji_form, reading),
// content fields are updated but audio_url is preserved.
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
		return 0, fmt.Errorf("cli.insertWords begin: %w", err)
	}
	defer tx.Rollback()
	for _, w := range items {
		var id int64
		err := tx.QueryRow(`INSERT INTO words (kanji_form,reading,part_of_speech,meaning,jlpt_level,reading_type)
   VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT(kanji_form,reading) DO UPDATE SET
   part_of_speech=excluded.part_of_speech,meaning=excluded.meaning,jlpt_level=excluded.jlpt_level,reading_type=excluded.reading_type,updated_at=now()
   RETURNING id`, w.KanjiForm, w.Reading, w.PartOfSpeech, w.Meaning, w.JLPTLevel, w.ReadingType).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("cli.insertWords upsert: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM word_examples WHERE word_id=$1`, id); err != nil {
			return 0, fmt.Errorf("cli.insertWords clear examples: %w", err)
		}
		raw, err := json.Marshal(w.Examples)
		if err != nil {
			return 0, fmt.Errorf("encode word examples: %w", err)
		}
		var examples []struct {
			Japanese string `json:"japanese"`
			Chinese  string `json:"chinese"`
		}
		if err := json.Unmarshal(raw, &examples); err != nil {
			return 0, fmt.Errorf("decode word examples: %w", err)
		}
		for i, e := range examples {
			if _, err := tx.Exec(`INSERT INTO word_examples(word_id,position,japanese,chinese) VALUES($1,$2,$3,$4)`, id, i, e.Japanese, e.Chinese); err != nil {
				return 0, fmt.Errorf("insert word example: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cli.insertWords commit: %w", err)
	}
	return len(items), nil
}
