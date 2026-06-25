package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestProcessLessonAudioDirScansLessonsSubdirectory(t *testing.T) {
	audioDir := t.TempDir()
	lessonsDir := filepath.Join(audioDir, "lessons")
	if err := os.Mkdir(lessonsDir, 0o755); err != nil {
		t.Fatalf("mkdir lessons dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lessonsDir, "intro.wav"), []byte("fake wav"), 0o644); err != nil {
		t.Fatalf("write lesson audio: %v", err)
	}

	stats := &audioStats{}
	err := processLessonAudioDir(
		context.Background(),
		nil,
		nil,
		audioDir,
		audioMigrateConfig{MinIOBucket: "audio", DryRun: true},
		stats,
		map[string]int64{},
	)
	if err != nil {
		t.Fatalf("processLessonAudioDir() error = %v", err)
	}
	if stats.FilesScanned != 1 {
		t.Fatalf("FilesScanned = %d, want 1", stats.FilesScanned)
	}
}

func TestUpdateLessonAudioFKsMatchesLessonsPrefix(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE lessons (
			id INTEGER PRIMARY KEY,
			audio_url TEXT NOT NULL,
			audio_object_id INTEGER
		);
		CREATE TABLE audio_objects (
			id INTEGER PRIMARY KEY,
			object_key TEXT NOT NULL
		);
		INSERT INTO lessons (id, audio_url) VALUES (1, '/audio/lessons/intro.wav');
		INSERT INTO audio_objects (id, object_key) VALUES (42, 'lessons/intro.wav');
	`)
	if err != nil {
		t.Fatalf("prepare db: %v", err)
	}

	updated, err := updateLessonAudioFKs(context.Background(), db, db)
	if err != nil {
		t.Fatalf("updateLessonAudioFKs() error = %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var audioObjectID int64
	if err := db.QueryRow(`SELECT audio_object_id FROM lessons WHERE id = 1`).Scan(&audioObjectID); err != nil {
		t.Fatalf("query updated lesson: %v", err)
	}
	if audioObjectID != 42 {
		t.Fatalf("audio_object_id = %d, want 42", audioObjectID)
	}
}

func TestUpdateTableAudioFKsFallsBackToAdditionalPrefixes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE words (
			id INTEGER PRIMARY KEY,
			audio_url TEXT NOT NULL,
			audio_object_id INTEGER
		);
		CREATE TABLE audio_objects (
			id INTEGER PRIMARY KEY,
			object_key TEXT NOT NULL
		);
		INSERT INTO words (id, audio_url) VALUES (1, 'legacy.wav');
		INSERT INTO audio_objects (id, object_key) VALUES (99, 'examples/legacy.wav');
	`)
	if err != nil {
		t.Fatalf("prepare db: %v", err)
	}

	updated, err := updateTableAudioFKs(context.Background(), db, db, "words", "audio_url", "audio_object_id", "words/", "examples/")
	if err != nil {
		t.Fatalf("updateTableAudioFKs() error = %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var audioObjectID int64
	if err := db.QueryRow(`SELECT audio_object_id FROM words WHERE id = 1`).Scan(&audioObjectID); err != nil {
		t.Fatalf("query updated word: %v", err)
	}
	if audioObjectID != 99 {
		t.Fatalf("audio_object_id = %d, want 99", audioObjectID)
	}
}

func TestUpdateExampleAudioFKsMatchesJapaneseHash(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	text := "私は毎日日本語を勉強します。"
	sum := sha256.Sum256([]byte(text))
	filename := fmt.Sprintf("%x", sum)[:16] + ".wav"

	_, err = db.Exec(`
		CREATE TABLE word_examples (
			id INTEGER PRIMARY KEY,
			japanese TEXT NOT NULL,
			audio_object_id INTEGER
		);
		CREATE TABLE audio_objects (
			id INTEGER PRIMARY KEY,
			object_key TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO word_examples (id, japanese) VALUES (1, ?)`, text); err != nil {
		t.Fatalf("insert word_example: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO audio_objects (id, object_key) VALUES (77, ?)`, "examples/"+filename); err != nil {
		t.Fatalf("insert audio_object: %v", err)
	}

	updated, err := updateExampleAudioFKs(context.Background(), db, "word_examples")
	if err != nil {
		t.Fatalf("updateExampleAudioFKs() error = %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var audioObjectID int64
	if err := db.QueryRow(`SELECT audio_object_id FROM word_examples WHERE id = 1`).Scan(&audioObjectID); err != nil {
		t.Fatalf("query updated example: %v", err)
	}
	if audioObjectID != 77 {
		t.Fatalf("audio_object_id = %d, want 77", audioObjectID)
	}
}
