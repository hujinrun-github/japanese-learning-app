package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/store"
)

func TestImportLessonsToPostgresFromFileUpsertsContentAndShadowingMediaURL(t *testing.T) {
	db := newMigratedPostgresLessonImportDB(t)

	firstPath := writePostgresLessonImportTempJSON(t, []map[string]any{
		postgresLessonImportJSON("PG Shadowing Import", "/audio/lessons/old.wav", "old sentence", 1),
	})
	inserted, err := ImportLessonsToPostgresFromFile(db, firstPath)
	if err != nil {
		t.Fatalf("first ImportLessonsToPostgresFromFile error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("first ImportLessonsToPostgresFromFile inserted = %d, want 1", inserted)
	}

	secondPath := writePostgresLessonImportTempJSON(t, []map[string]any{
		postgresLessonImportJSON("PG Shadowing Import", "/audio/lessons/new.wav", "new sentence", 2),
	})
	inserted, err = ImportLessonsToPostgresFromFile(db, secondPath)
	if err != nil {
		t.Fatalf("second ImportLessonsToPostgresFromFile error: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("second ImportLessonsToPostgresFromFile inserted = %d, want 0 for update", inserted)
	}

	var lessonID int64
	var count int
	var shadowingEnabled bool
	var shadowingVersion int
	var configJSON string
	err = db.QueryRow(`
		SELECT MIN(id), COUNT(*), bool_or(shadowing_enabled), MAX(shadowing_version), MAX(shadowing_config_json::text)
		FROM lessons
		WHERE title = $1 AND jlpt_level = $2`,
		"PG Shadowing Import",
		"N5",
	).Scan(&lessonID, &count, &shadowingEnabled, &shadowingVersion, &configJSON)
	if err != nil {
		t.Fatalf("query imported lesson error: %v", err)
	}
	if count != 1 {
		t.Fatalf("lesson row count = %d, want 1", count)
	}
	if !shadowingEnabled {
		t.Fatal("shadowing_enabled = false, want true")
	}
	if shadowingVersion != 2 {
		t.Fatalf("shadowing_version = %d, want 2", shadowingVersion)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		t.Fatalf("unmarshal shadowing_config_json: %v", err)
	}
	if config["media_url"] != "/audio/lessons/new.wav" {
		t.Fatalf("media_url = %#v, want updated audio URL", config["media_url"])
	}

	var sentenceCount int
	var chinese string
	if err := db.QueryRow(`
		SELECT COUNT(*), MAX(chinese)
		FROM lesson_sentences
		WHERE lesson_id = $1`,
		lessonID,
	).Scan(&sentenceCount, &chinese); err != nil {
		t.Fatalf("query lesson_sentences error: %v", err)
	}
	if sentenceCount != 1 {
		t.Fatalf("sentence count = %d, want 1", sentenceCount)
	}
	if chinese != "new sentence" {
		t.Fatalf("sentence chinese = %q, want updated value", chinese)
	}
}

func TestRunImportLessonsPostgresCommand(t *testing.T) {
	db := newMigratedPostgresLessonImportDB(t)
	url := os.Getenv("DATABASE_URL_TEST")
	filePath := writePostgresLessonImportTempJSON(t, []map[string]any{
		postgresLessonImportJSON("PG Shadowing Command", "/audio/lessons/command.wav", "command sentence", 1),
	})

	code := Run([]string{"import-lessons-postgres", "--database-url", url, "--file", filePath})
	if code != 0 {
		t.Fatalf("Run(import-lessons-postgres) exit code = %d, want 0", code)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lessons WHERE title = $1`, "PG Shadowing Command").Scan(&count); err != nil {
		t.Fatalf("query imported command lesson: %v", err)
	}
	if count != 1 {
		t.Fatalf("imported command lesson count = %d, want 1", count)
	}
}

func writePostgresLessonImportTempJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal temp JSON: %v", err)
	}
	path := filepath.Join(t.TempDir(), "lessons.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write temp JSON: %v", err)
	}
	return path
}

func postgresLessonImportJSON(title, audioURL, chinese string, version int) map[string]any {
	return map[string]any{
		"title":             title,
		"jlpt_level":        "N5",
		"tags":              []string{"shadowing"},
		"audio_url":         audioURL,
		"word_ids":          []int64{},
		"shadowing_enabled": true,
		"shadowing_version": version,
		"shadowing_config":  map[string]any{"media_type": "audio"},
		"sentences": []map[string]any{
			{
				"index":    0,
				"tokens":   []map[string]string{{"surface": "test", "reading": ""}},
				"chinese":  chinese,
				"start_ms": 0,
				"end_ms":   3000,
			},
		},
	}
}

func newMigratedPostgresLessonImportDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}

	ctx := context.Background()
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{
		DatabaseURL:  url,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("open postgres test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
`); err != nil {
		t.Fatalf("reset postgres test schema: %v", err)
	}
	if err := adapter.RunMigrations(ctx, db); err != nil {
		t.Fatalf("run postgres migrations: %v", err)
	}

	return db
}
