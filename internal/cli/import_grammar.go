package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// grammarImport is the JSON shape expected for each element in the grammar import file.
type grammarImport struct {
	Name            string `json:"name"`
	Meaning         string `json:"meaning"`
	ConjunctionRule string `json:"conjunction_rule"`
	UsageNote       string `json:"usage_note"`
	Examples        any    `json:"examples"`
	QuizQuestions   any    `json:"quiz_questions"`
	JLPTLevel       string `json:"jlpt_level"`
}

// ImportGrammar reads a JSON array of grammar points from r and inserts them into the
// grammar_points table using ON CONFLICT DO NOTHING (idempotent – duplicate names per
// jlpt_level are silently skipped).
// It returns the number of rows actually inserted.
func ImportGrammar(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportGrammar called")
	var items []grammarImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportGrammar decode: %w", err)
	}
	return insertGrammarPoints(db, items)
}

// ImportGrammarFromFile reads a JSON array of grammar points from filePath and inserts them
// into the grammar_points table using ON CONFLICT DO NOTHING (idempotent – duplicate names per
// jlpt_level are silently skipped).
// It returns the number of rows actually inserted.
func ImportGrammarFromFile(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportGrammarFromFile called", "file", filePath)

	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportGrammarFromFile open: %w", err)
	}
	defer f.Close()

	return ImportGrammar(db, f)
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
		return 0, fmt.Errorf("insert grammar begin: %w", err)
	}
	defer tx.Rollback()
	count := 0
	for _, item := range items {
		var id int64
		err := tx.QueryRow(`INSERT INTO grammar_points(name,meaning,conjunction_rule,usage_note,jlpt_level)
   VALUES($1,$2,$3,$4,$5) ON CONFLICT(name,jlpt_level) DO NOTHING RETURNING id`, item.Name, item.Meaning, item.ConjunctionRule, item.UsageNote, item.JLPTLevel).Scan(&id)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("insert grammar point: %w", err)
		}
		raw, err := json.Marshal(item.Examples)
		if err != nil {
			return 0, fmt.Errorf("encode grammar examples: %w", err)
		}
		var examples []struct {
			Japanese string `json:"japanese"`
			Chinese  string `json:"chinese"`
		}
		if err := json.Unmarshal(raw, &examples); err != nil {
			return 0, fmt.Errorf("decode grammar examples: %w", err)
		}
		for i, e := range examples {
			if _, err := tx.Exec(`INSERT INTO grammar_examples(grammar_point_id,position,japanese,chinese) VALUES($1,$2,$3,$4)`, id, i, e.Japanese, e.Chinese); err != nil {
				return 0, fmt.Errorf("insert grammar example: %w", err)
			}
		}
		raw, err = json.Marshal(item.QuizQuestions)
		if err != nil {
			return 0, fmt.Errorf("encode grammar quizzes: %w", err)
		}
		var questions []struct {
			Type        string   `json:"type"`
			Prompt      string   `json:"prompt"`
			Options     []string `json:"options"`
			Answer      string   `json:"answer"`
			Explanation string   `json:"explanation"`
		}
		if err := json.Unmarshal(raw, &questions); err != nil {
			return 0, fmt.Errorf("decode grammar quizzes: %w", err)
		}
		for i, q := range questions {
			if q.Options == nil {
				q.Options = []string{}
			}
			opts, err := json.Marshal(q.Options)
			if err != nil {
				return 0, fmt.Errorf("encode quiz options: %w", err)
			}
			if _, err := tx.Exec(`INSERT INTO grammar_quiz_questions(grammar_point_id,position,type,prompt,options,answer,explanation) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7)`, id, i, q.Type, q.Prompt, string(opts), q.Answer, q.Explanation); err != nil {
				return 0, fmt.Errorf("insert grammar quiz: %w", err)
			}
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("insert grammar commit: %w", err)
	}
	return count, nil
}
