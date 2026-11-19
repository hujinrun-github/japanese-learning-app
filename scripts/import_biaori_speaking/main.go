// Command import_biaori_speaking imports a reviewed, audio-verified material pack.
// It deliberately does not run migrations: importing content must not deploy
// unrelated, unpublished schema changes from the local checkout.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/store"
)

type material struct {
	Type          string          `json:"type"`
	Title         string          `json:"title"`
	Text          string          `json:"text"`
	Level         string          `json:"jlpt_level"`
	Lines         []string        `json:"lines"`
	Bucket        string          `json:"bucket"`
	ObjectKey     string          `json:"object_key"`
	ContentSHA256 string          `json:"content_sha256"`
	SizeBytes     int64           `json:"size_bytes"`
	Metadata      json.RawMessage `json:"metadata_json"`
}

type imported struct {
	Title         string `json:"title"`
	MaterialID    int64  `json:"material_id"`
	AudioObjectID int64  `json:"audio_object_id"`
	AudioURL      string `json:"audio_url"`
}

type skipped struct {
	Title       string `json:"title"`
	DuplicateOf string `json:"duplicate_of"`
	ExistingID  int64  `json:"existing_id"`
}

type importResult struct {
	Pending  int        `json:"pending"`
	Applied  bool       `json:"applied"`
	Inserted []imported `json:"inserted"`
	Skipped  []skipped  `json:"skipped"`
}

func canonical(text string) string {
	labels := regexp.MustCompile(`^(?:[1-4][.]\s*|[^：:\n]{1,8}[：:]\s*)`)
	var out strings.Builder
	for _, line := range strings.Split(norm.NFKC.String(text), "\n") {
		line = labels.ReplaceAllString(strings.TrimSpace(line), "")
		for _, c := range line {
			if unicode.IsLetter(c) || unicode.IsNumber(c) {
				out.WriteRune(c)
			}
		}
	}
	return out.String()
}

func importPack(ctx context.Context, db *sql.DB, items []material, apply bool) (importResult, error) {
	result := importResult{Applied: apply, Inserted: []imported{}, Skipped: []skipped{}}
	for _, item := range items {
		if item.Title == "" || canonical(item.Text) == "" || item.Type != "shadow" ||
			(item.Level != "N5" && item.Level != "N4") || item.Bucket == "" || item.ObjectKey == "" ||
			len(item.ContentSHA256) != 64 || item.SizeBytes <= 44 || !json.Valid(item.Metadata) {
			return result, fmt.Errorf("invalid material or audio metadata: %q", item.Title)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin material import: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("rollback material import", "err", err)
		}
	}()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "biaori-speaking-import-v1"); err != nil {
		return result, fmt.Errorf("lock material import: %w", err)
	}
	speakingStore := postgres.NewSpeakingStore(tx, store.StoreDeps{})
	audioStore := postgres.NewAudioObjectStore(tx, store.StoreDeps{})
	existing, err := speakingStore.ListMaterials("", "")
	if err != nil {
		return result, fmt.Errorf("load existing speaking materials: %w", err)
	}
	seen := make(map[string]speaking.SpeakingMaterial, len(existing)+len(items))
	titles := make(map[string]string, len(existing)+len(items))
	for _, row := range existing {
		seen[canonical(row.Text)] = row
		titles[row.Title] = canonical(row.Text)
	}
	for _, item := range items {
		key := canonical(item.Text)
		if previous, ok := titles[item.Title]; ok && previous != key {
			return result, fmt.Errorf("existing title has different text: %q", item.Title)
		}
		if previous, ok := seen[key]; ok {
			result.Skipped = append(result.Skipped, skipped{Title: item.Title, DuplicateOf: previous.Title, ExistingID: previous.ID})
			continue
		}
		result.Pending++
		seen[key] = speaking.SpeakingMaterial{Title: item.Title, Text: item.Text}
		titles[item.Title] = key
		if !apply {
			continue
		}
		var audioID int64
		audioRow, err := audioStore.GetByKey(ctx, item.Bucket, item.ObjectKey)
		switch {
		case err == nil:
			if audioRow.ContentSHA256 != item.ContentSHA256 || audioRow.SizeBytes != item.SizeBytes || audioRow.Visibility != "public" {
				return result, fmt.Errorf("existing audio metadata differs: %q", item.ObjectKey)
			}
			audioID = audioRow.ID
		case errors.Is(err, sql.ErrNoRows):
			audioID, err = audioStore.Insert(ctx, store.AudioObjectRow{Bucket: item.Bucket, ObjectKey: item.ObjectKey,
				Kind: "example_audio", Visibility: "public", ContentSHA256: item.ContentSHA256,
				SizeBytes: item.SizeBytes, MimeType: "audio/wav", MetadataJSON: item.Metadata})
			if err != nil {
				return result, fmt.Errorf("register audio for %q: %w", item.Title, err)
			}
		default:
			return result, fmt.Errorf("look up audio for %q: %w", item.Title, err)
		}
		audioURL := fmt.Sprintf("/api/v1/audio/%d/stream", audioID)
		materialID, err := speakingStore.InsertMaterial(speaking.SpeakingMaterial{Type: item.Type, Title: item.Title,
			Text: item.Text, AudioURL: audioURL, JLPTLevel: item.Level})
		if err != nil {
			return result, fmt.Errorf("insert %q: %w", item.Title, err)
		}
		lines, err := json.Marshal(item.Lines)
		if err != nil {
			return result, fmt.Errorf("encode lines for %q: %w", item.Title, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE speaking_materials SET audio_object_id=$2,
			lines=ARRAY(SELECT jsonb_array_elements_text($3::jsonb)) WHERE id=$1`, materialID, audioID, string(lines)); err != nil {
			return result, fmt.Errorf("link audio for %q: %w", item.Title, err)
		}
		seen[key] = speaking.SpeakingMaterial{ID: materialID, Title: item.Title, Text: item.Text}
		result.Inserted = append(result.Inserted, imported{Title: item.Title, MaterialID: materialID, AudioObjectID: audioID, AudioURL: audioURL})
	}
	if !apply {
		return result, nil
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit material import: %w", err)
	}
	return result, nil
}

func run() error {
	file := flag.String("file", "", "reviewed JSON material pack with verified audio metadata")
	apply := flag.Bool("apply", false, "commit the material import; default is read-only preview")
	flag.Parse()
	if *file == "" || os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("--file and DATABASE_URL are required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read import pack: %w", err)
	}
	var items []material
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("decode import pack: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db, err := (postgres.Adapter{}).Open(ctx, store.DatabaseConfig{DatabaseURL: os.Getenv("DATABASE_URL"), MaxOpenConns: 2})
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer db.Close()
	result, err := importPack(ctx, db, items, *apply)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return fmt.Errorf("write import result: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("material import failed", "err", err)
		os.Exit(1)
	}
}
