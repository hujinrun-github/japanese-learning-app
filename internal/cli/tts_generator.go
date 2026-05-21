package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"japanese-learning-app/internal/module/speaking"
)

// ttsSentence is a lightweight struct for extracting sentences from import JSON files.
type ttsSentence struct {
	Japanese string `json:"japanese"`
}

// extractWordSentences parses a words JSON file and returns all sentences that need TTS audio.
// Collects: each word's reading + each example's japanese text.
func extractWordSentences(raw []byte) ([]string, error) {
	var items []struct {
		Reading  string        `json:"reading"`
		Examples []ttsSentence `json:"examples"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("cli.extractWordSentences Unmarshal: %w", err)
	}
	var out []string
	for _, w := range items {
		if r := strings.TrimSpace(w.Reading); r != "" {
			out = append(out, r)
		}
		for _, e := range w.Examples {
			if j := strings.TrimSpace(e.Japanese); j != "" {
				out = append(out, j)
			}
		}
	}
	return out, nil
}

// extractGrammarSentences parses a grammar JSON file and returns all example japanese sentences.
func extractGrammarSentences(raw []byte) ([]string, error) {
	var items []struct {
		Examples []ttsSentence `json:"examples"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("cli.extractGrammarSentences Unmarshal: %w", err)
	}
	var out []string
	for _, g := range items {
		for _, e := range g.Examples {
			if j := strings.TrimSpace(e.Japanese); j != "" {
				out = append(out, j)
			}
		}
	}
	return out, nil
}

// extractSpeakingSentences parses a speaking materials JSON file and returns all text fields.
func extractSpeakingSentences(raw []byte) ([]string, error) {
	var items []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("cli.extractSpeakingSentences Unmarshal: %w", err)
	}
	var out []string
	for _, m := range items {
		if t := strings.TrimSpace(m.Text); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// generateTTSFiles generates audio files for each sentence using the TTS client.
// Files are named <sha256(text)[:16]>.wav and saved to outDir. Already-existing files
// are skipped. Returns the count of newly generated files.
func generateTTSFiles(client speaking.TTSSynthesizer, outDir string, sentences []string) (int, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return 0, fmt.Errorf("cli.generateTTSFiles mkdir %s: %w", outDir, err)
	}

	ctx := context.Background()
	generated := 0

	for _, text := range sentences {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
		filename := hash + ".wav"
		path := filepath.Join(outDir, filename)

		if _, err := os.Stat(path); err == nil {
			continue
		}

		audio, err := client.Synthesize(ctx, text)
		if err != nil {
			slog.Error("TTS synthesis failed", "text", text, "err", err)
			continue
		}

		if err := os.WriteFile(path, audio, 0644); err != nil {
			slog.Error("failed to write TTS audio file", "path", path, "err", err)
			continue
		}

		slog.Debug("TTS audio generated", "text", truncateStr(text, 40), "file", filename)
		generated++
	}

	return generated, nil
}

func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
