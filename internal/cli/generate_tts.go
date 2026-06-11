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
	Provider     string // "vllm", "sbv", or "gradio"
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
	case "gradio":
		return speaking.NewGradioTTSClient(cfg.TTSUrl, 5*time.Minute,
			speaking.WithGradioLanguage("Japanese"),
			speaking.WithGradioSpeaker(cfg.Voice),
			speaking.WithGradioInstructions(cfg.Instructions),
		)
	default:
		return speaking.NewVLLMTTSClient(cfg.TTSUrl, cfg.TTSModel, 30*time.Second,
			speaking.WithVoice(cfg.Voice),
			speaking.WithLanguage("Japanese"),
			speaking.WithInstructions(cfg.Instructions),
		)
	}
}

// WordAudioStats reports the result of generating word pronunciation audio.
type WordAudioStats struct {
	Total     int `json:"total"`
	Generated int `json:"generated"`
	Existing  int `json:"existing"`
	Failed    int `json:"failed"`
	DryRun    int `json:"dry_run,omitempty"`
}

func (s WordAudioStats) Processed() int {
	return s.Generated + s.Existing + s.DryRun
}

// GenerateWordAudio generates TTS audio for all words matching the given level,
// saves to outDir, and updates audio_url in the database.
func GenerateWordAudio(db *sql.DB, client speaking.TTSSynthesizer, outDir, level string, force, dryRun bool) (int, error) {
	stats, err := GenerateWordAudioWithStats(db, client, outDir, level, force, dryRun)
	if err != nil {
		return 0, err
	}
	return stats.Processed(), nil
}

// GenerateWordAudioWithStats generates word audio and returns detailed counts.
func GenerateWordAudioWithStats(db *sql.DB, client speaking.TTSSynthesizer, outDir, level string, force, dryRun bool) (WordAudioStats, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return WordAudioStats{}, fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	rows, err := queryWords(db, level)
	if err != nil {
		return WordAudioStats{}, fmt.Errorf("query words: %w", err)
	}

	return generateWordAudioRows(db, client, outDir, rows, force, dryRun)
}

// GenerateWordAudioByIDsWithStats generates word audio only for the provided word IDs.
func GenerateWordAudioByIDsWithStats(db *sql.DB, client speaking.TTSSynthesizer, outDir string, ids []int64, force, dryRun bool) (WordAudioStats, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return WordAudioStats{}, fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	rows, err := queryWordsByIDs(db, ids)
	if err != nil {
		return WordAudioStats{}, fmt.Errorf("query words by ids: %w", err)
	}

	return generateWordAudioRows(db, client, outDir, rows, force, dryRun)
}

func generateWordAudioRows(db *sql.DB, client speaking.TTSSynthesizer, outDir string, rows []wordRow, force, dryRun bool) (WordAudioStats, error) {
	slog.Info("generate-word-audio: loaded words", "count", len(rows))
	stats := WordAudioStats{Total: len(rows)}

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
				stats.Existing++
				continue
			}
			os.Remove(path)
		}

		fmt.Printf("[%d/%d] %s (%s) → %s\n", i+1, len(rows), r.KanjiForm, r.Reading, filename)

		if dryRun {
			stats.DryRun++
			continue
		}

		audio, synthErr := client.Synthesize(context.Background(), r.Reading)
		if synthErr != nil {
			slog.Error("TTS failed for word", "kanji", r.KanjiForm, "reading", r.Reading, "err", synthErr)
			stats.Failed++
			continue
		}

		audio = TrimWAVSilence(audio, 0.06)

		if writeErr := os.WriteFile(path, audio, 0644); writeErr != nil {
			slog.Error("failed to write audio file", "path", path, "err", writeErr)
			stats.Failed++
			continue
		}

		updateWordAudio(db, r.ID, filename)
		stats.Generated++

		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("generate-word-audio: done, %d words processed\n", stats.Processed())
	return stats, nil
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
	provider := fs.String("provider", "vllm", "TTS provider: vllm, sbv (style-bert-vits), or gradio")

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

func queryWordsByIDs(db *sql.DB, ids []int64) ([]wordRow, error) {
	seen := make(map[int64]bool, len(ids))
	rows := make([]wordRow, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true

		var wr wordRow
		err := db.QueryRow("SELECT id, kanji_form, reading, audio_url FROM words WHERE id = ? AND reading != ''", id).
			Scan(&wr.ID, &wr.KanjiForm, &wr.Reading, &wr.AudioURL)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, wr)
	}
	return rows, nil
}

func updateWordAudio(db *sql.DB, id int64, filename string) {
	if _, err := db.Exec("UPDATE words SET audio_url = ? WHERE id = ?", filename, id); err != nil {
		slog.Error("failed to update word audio_url", "id", id, "filename", filename, "err", err)
	}
}
