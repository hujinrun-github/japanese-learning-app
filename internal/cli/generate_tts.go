package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/speaking"
)

// TTSConfig holds TTS provider configuration shared across commands.
type TTSConfig struct {
	Provider     string // "vllm" or "sbv"
	TTSUrl       string
	TTSModel     string
	Voice        string
	Instructions string
	SBVURL       string
	SBVModel     string
	SBVSpeaker   string
	SBVStyle     string
}

// NewTTSClient creates a TTS client based on the config.
func NewTTSClient(cfg TTSConfig) speaking.TTSSynthesizer {
	switch cfg.Provider {
	case "sbv":
		return speaking.NewStyleBertVITSClient(cfg.SBVURL, 120*time.Second,
			speaking.WithSBVModel(cfg.SBVModel),
			speaking.WithSBVSpeaker(cfg.SBVSpeaker),
			speaking.WithSBVStyle(cfg.SBVStyle),
		)
	default:
		return speaking.NewVLLMTTSClient(cfg.TTSUrl, cfg.TTSModel, 30*time.Second,
			speaking.WithVoice(cfg.Voice),
			speaking.WithLanguage("Japanese"),
			speaking.WithInstructions(cfg.Instructions),
		)
	}
}

// GenerateWordAudio generates TTS audio for all words matching the given level,
// saves to outDir, and updates audio_url in the database.
func GenerateWordAudio(db *sql.DB, client speaking.TTSSynthesizer, outDir, level string, force, dryRun bool) (int, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	rows, err := queryWords(db, level)
	if err != nil {
		return 0, fmt.Errorf("query words: %w", err)
	}

	slog.Info("generate-word-audio: loaded words", "count", len(rows))
	generated := 0

	for i, r := range rows {
		if r.Reading == "" {
			continue
		}

		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Reading)))[:16]
		filename := hash + ".wav"
		path := filepath.Join(outDir, filename)

		if _, statErr := os.Stat(path); statErr == nil {
			if !force {
				if r.AudioURL != filename {
					updateWordAudio(db, r.ID, filename)
				}
				generated++
				continue
			}
			os.Remove(path)
		}

		fmt.Printf("[%d/%d] %s (%s) → %s\n", i+1, len(rows), r.KanjiForm, r.Reading, filename)

		if dryRun {
			generated++
			continue
		}

		audio, synthErr := client.Synthesize(context.Background(), r.Reading)
		if synthErr != nil {
			slog.Error("TTS failed for word", "kanji", r.KanjiForm, "reading", r.Reading, "err", synthErr)
			continue
		}

			audio = TrimWAVSilence(audio, 0.06)

		if writeErr := os.WriteFile(path, audio, 0644); writeErr != nil {
			slog.Error("failed to write audio file", "path", path, "err", writeErr)
			continue
		}

		updateWordAudio(db, r.ID, filename)
		generated++

		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("generate-word-audio: done, %d words processed\n", generated)
	return generated, nil
}

// runGenerateWordAudio regenerates TTS audio for all words in the database.
func runGenerateWordAudio(args []string) int {
	fs := flag.NewFlagSet("generate-word-audio", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")
	ttsURL := fs.String("tts-url", speaking.DefaultVLLMTTSURL(), "vLLM TTS endpoint URL")
	ttsModel := fs.String("tts-model", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice", "TTS model name")
	outDir := fs.String("out", "./data/audio/words", "output directory for audio files")
	dryRun := fs.Bool("dry-run", false, "only print what would be generated, don't generate")
	level := fs.String("level", "", "only generate for specified JLPT level (N5/N4/N3/N2/N1)")
	force := fs.Bool("force", false, "regenerate even if audio file already exists")
	voice := fs.String("voice", "ono_anna", "TTS voice name")
	instructions := fs.String("instructions", "Pronounce only the exact given word, in isolation. No extra sounds, no prefix, no suffix. Clean single-word pronunciation.", "TTS style instructions (e.g. emotion, speed, tone)")

	// style-bert-vits2 flags
	sbvBaseURL := fs.String("sbv-url", "http://127.0.0.1:7862", "style-bert-vits2 FastAPI server URL")
	sbvModel := fs.String("sbv-model", "amitaro", "style-bert-vits2 model name")
	sbvSpeaker := fs.String("sbv-speaker", "あみたろ", "style-bert-vits2 speaker name")
	sbvStyle := fs.String("sbv-style", "Neutral", "style-bert-vits2 style name")
	provider := fs.String("provider", "vllm", "TTS provider: vllm or sbv (style-bert-vits)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "generate-word-audio: %v\n", err)
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-word-audio: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		fmt.Fprintf(os.Stderr, "generate-word-audio: run migrations: %v\n", err)
		return 1
	}

	cfg := TTSConfig{
		Provider:     *provider,
		TTSUrl:       *ttsURL,
		TTSModel:     *ttsModel,
		Voice:        *voice,
		Instructions: *instructions,
		SBVURL:       *sbvBaseURL,
		SBVModel:     *sbvModel,
		SBVSpeaker:   *sbvSpeaker,
		SBVStyle:     *sbvStyle,
	}
	client := NewTTSClient(cfg)

	if _, err := GenerateWordAudio(db, client, *outDir, *level, *force, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "generate-word-audio: %v\n", err)
		return 1
	}
	return 0
}

type wordRow struct {
	ID        int64
	KanjiForm string
	Reading   string
	AudioURL  string
}

func queryWords(db *sql.DB, level string) ([]wordRow, error) {
	var query string
	var args []any
	if level != "" {
		query = "SELECT id, kanji_form, reading, audio_url FROM words WHERE jlpt_level = ? AND reading != '' ORDER BY id"
		args = append(args, level)
	} else {
		query = "SELECT id, kanji_form, reading, audio_url FROM words WHERE reading != '' ORDER BY id"
	}
	r, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var rows []wordRow
	for r.Next() {
		var wr wordRow
		if err := r.Scan(&wr.ID, &wr.KanjiForm, &wr.Reading, &wr.AudioURL); err != nil {
			return nil, err
		}
		rows = append(rows, wr)
	}
	return rows, r.Err()
}

func updateWordAudio(db *sql.DB, id int64, filename string) {
	if _, err := db.Exec("UPDATE words SET audio_url = ? WHERE id = ?", filename, id); err != nil {
		slog.Error("failed to update word audio_url", "id", id, "filename", filename, "err", err)
	}
}
