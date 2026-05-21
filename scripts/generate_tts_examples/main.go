package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type ttsRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

type ttsItem struct {
	key      string // original text, used as lookup key in audio_map
	ttsInput string // text sent to TTS (word readings get "。" appended)
}

func main() {
	dbPath := flag.String("db", "./data/app.db", "path to SQLite database")
	ttsURL := flag.String("tts-url", "http://192.168.1.16:8091/v1/audio/speech", "vLLM TTS endpoint URL")
	ttsModel := flag.String("tts-model", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice", "TTS model name")
	outDir := flag.String("out", "./data/audio/examples", "output directory for audio files")
	dryRun := flag.Bool("dry-run", false, "only print sentences, don't generate audio")
	limit := flag.Int("limit", 0, "max sentences to process (0=all)")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		slog.Error("failed to create output directory", "dir", *outDir, "err", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		slog.Error("failed to open database", "path", *dbPath, "err", err)
		os.Exit(1)
	}
	defer db.Close()

	items, err := loadSentences(db)
	if err != nil {
		slog.Error("failed to load sentences", "err", err)
		os.Exit(1)
	}

	slog.Info("loaded sentences", "total", len(items))

	client := &http.Client{Timeout: 30 * time.Second}
	mapping := make(map[string]string) // original text → filename
	processed := 0
	skipped := 0

	for i, item := range items {
		if *limit > 0 && processed >= *limit {
			slog.Info("limit reached", "limit", *limit)
			break
		}

		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(item.ttsInput)))[:16]
		filename := hash + ".wav"
		filepath := filepath.Join(*outDir, filename)

		if _, err := os.Stat(filepath); err == nil {
			skipped++
			mapping[item.key] = filename
			continue
		}

		if *dryRun {
			fmt.Printf("[%d/%d] %s\n", i+1, len(items), item.ttsInput)
			continue
		}

		fmt.Printf("[%d/%d] %s → %s\n", i+1, len(items), truncate(item.ttsInput, 50), filename)

		audio, err := synthesize(client, *ttsURL, *ttsModel, item.ttsInput)
		if err != nil {
			slog.Error("TTS failed", "sentence", item.ttsInput, "err", err)
			continue
		}

		if err := os.WriteFile(filepath, audio, 0644); err != nil {
			slog.Error("failed to write audio file", "path", filepath, "err", err)
			continue
		}

		mapping[item.key] = filename
		processed++

		// small delay to avoid overwhelming the TTS server
		time.Sleep(200 * time.Millisecond)
	}

	// write mapping file
	mapPath := filepath.Join(*outDir, "audio_map.json")
	mapData, _ := json.MarshalIndent(mapping, "", "  ")
	if err := os.WriteFile(mapPath, mapData, 0644); err != nil {
		slog.Error("failed to write mapping file", "path", mapPath, "err", err)
	} else {
		slog.Info("mapping saved", "path", mapPath, "entries", len(mapping))
	}

	slog.Info("done", "processed", processed, "skipped", skipped, "total", len(items))
}

func loadSentences(db *sql.DB) ([]ttsItem, error) {
	query := `
		SELECT DISTINCT 'word' as src, reading as text FROM words WHERE reading != ''
		UNION
		SELECT 'example', json_extract(value, '$.japanese')
		FROM words, json_each(words.examples_json)
		UNION
		SELECT 'example', json_extract(value, '$.japanese')
		FROM grammar_points, json_each(grammar_points.examples_json)
		UNION
		SELECT 'speaking', text FROM speaking_materials WHERE text != ''
		ORDER BY 2
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var items []ttsItem
	for rows.Next() {
		var src string
		var text sql.NullString
		if err := rows.Scan(&src, &text); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if text.Valid && strings.TrimSpace(text.String) != "" {
			key := strings.TrimSpace(text.String)
			ttsInput := key
			if src == "word" && !hasFinalPunct(key) {
				ttsInput = key + "。"
			}
			items = append(items, ttsItem{key: key, ttsInput: ttsInput})
		}
	}
	return items, rows.Err()
}

func hasFinalPunct(s string) bool {
	if len(s) == 0 {
		return false
	}
	last := []rune(s)[len([]rune(s))-1]
	return last == '。' || last == '！' || last == '？' || last == '…' || last == '〜'
}

func synthesize(client *http.Client, url, model, text string) ([]byte, error) {
	body, err := json.Marshal(ttsRequest{
		Model: model,
		Input: text,
		Voice: "ono_anna",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	return audio, nil
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
