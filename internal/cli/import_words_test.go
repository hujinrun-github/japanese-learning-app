package cli_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"japanese-learning-app/internal/cli"
	"japanese-learning-app/internal/data"
)

// openTestDB creates an in-memory SQLite database with migrations applied.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "cli_test_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	dbPath := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := data.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := data.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// writeTempJSON writes v as JSON to a temp file and returns the path.
func writeTempJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	f, err := os.CreateTemp("", "words_import_*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.Write(b); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// wordJSON is a minimal JSON representation for import.
type wordJSON struct {
	KanjiForm    string `json:"kanji_form"`
	Reading      string `json:"reading"`
	PartOfSpeech string `json:"part_of_speech"`
	Meaning      string `json:"meaning"`
	Examples     []any  `json:"examples"`
	JLPTLevel    string `json:"jlpt_level"`
}

func TestImportWords_Success(t *testing.T) {
	db := openTestDB(t)

	words := []wordJSON{
		{KanjiForm: "猫", Reading: "ねこ", PartOfSpeech: "名詞", Meaning: "cat", JLPTLevel: "N5", Examples: []any{}},
		{KanjiForm: "犬", Reading: "いぬ", PartOfSpeech: "名詞", Meaning: "dog", JLPTLevel: "N5", Examples: []any{}},
		{KanjiForm: "魚", Reading: "さかな", PartOfSpeech: "名詞", Meaning: "fish", JLPTLevel: "N4", Examples: []any{}},
	}
	filePath := writeTempJSON(t, words)

	n, err := cli.ImportWords(db, filePath)
	if err != nil {
		t.Fatalf("ImportWords error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 imported, got %d", n)
	}

	// Verify rows exist in DB
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE kanji_form IN ('猫','犬','魚')`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows in DB, got %d", count)
	}
}

func TestImportWords_InvalidJSON(t *testing.T) {
	db := openTestDB(t)

	f, _ := os.CreateTemp("", "bad_*.json")
	f.WriteString("not valid json{{")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	_, err := cli.ImportWords(db, f.Name())
	if err == nil {
		t.Error("expected error for invalid JSON file")
	}
}

func TestImportWords_FileNotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := cli.ImportWords(db, filepath.Join(os.TempDir(), "nonexistent_words_file.json"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestImportWords_IdempotentOnDuplicate(t *testing.T) {
	db := openTestDB(t)

	words := []wordJSON{
		{KanjiForm: "山", Reading: "やま", PartOfSpeech: "名詞", Meaning: "mountain", JLPTLevel: "N5", Examples: []any{}},
	}
	filePath := writeTempJSON(t, words)

	n1, err := cli.ImportWords(db, filePath)
	if err != nil {
		t.Fatalf("first import error: %v", err)
	}

	// Second import of same file should not error and should not double-insert.
	n2, err := cli.ImportWords(db, filePath)
	if err != nil {
		t.Fatalf("second import error: %v", err)
	}

	// Inserted on second pass should be 0 (INSERT OR IGNORE).
	if n2 != 0 {
		t.Errorf("expected 0 new rows on second import, got %d", n2)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM words WHERE kanji_form = '山'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 row, got %d (n1=%d, n2=%d)", count, n1, n2)
	}
}
