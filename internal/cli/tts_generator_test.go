package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type fakeTTSSynthesizer struct {
	calls []string
}

func (f *fakeTTSSynthesizer) Synthesize(_ context.Context, text string) ([]byte, error) {
	f.calls = append(f.calls, text)
	return []byte("wav:" + text), nil
}

func TestGenerateTTSFilesWithStats(t *testing.T) {
	tests := []struct {
		name            string
		force           bool
		existingText    string
		sentences       []string
		wantCalls       []string
		wantTotal       int
		wantGenerated   int
		wantExisting    int
		wantExistingWav string
	}{
		{
			name:            "skips existing file and dedupes input",
			force:           false,
			existingText:    "aru",
			sentences:       []string{"aru", " aru ", "nai"},
			wantCalls:       []string{"nai"},
			wantTotal:       2,
			wantGenerated:   1,
			wantExisting:    1,
			wantExistingWav: "old",
		},
		{
			name:            "force overwrites existing file",
			force:           true,
			existingText:    "aru",
			sentences:       []string{"aru", " aru ", "nai"},
			wantCalls:       []string{"aru", "nai"},
			wantTotal:       2,
			wantGenerated:   2,
			wantExisting:    0,
			wantExistingWav: "wav:aru",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := t.TempDir()
			if tt.existingText != "" {
				if err := os.WriteFile(filepath.Join(outDir, ttsTestFilename(tt.existingText)), []byte("old"), 0644); err != nil {
					t.Fatalf("write existing file: %v", err)
				}
			}

			client := &fakeTTSSynthesizer{}
			stats, err := GenerateTTSFilesWithStats(client, outDir, tt.sentences, tt.force)
			if err != nil {
				t.Fatalf("GenerateTTSFilesWithStats: %v", err)
			}

			if stats.Total != tt.wantTotal || stats.Generated != tt.wantGenerated || stats.Existing != tt.wantExisting {
				t.Fatalf("stats = %+v, want total=%d generated=%d existing=%d", stats, tt.wantTotal, tt.wantGenerated, tt.wantExisting)
			}
			if fmt.Sprint(client.calls) != fmt.Sprint(tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", client.calls, tt.wantCalls)
			}

			gotExisting, err := os.ReadFile(filepath.Join(outDir, ttsTestFilename(tt.existingText)))
			if err != nil {
				t.Fatalf("read existing file: %v", err)
			}
			if string(gotExisting) != tt.wantExistingWav {
				t.Fatalf("existing wav = %q, want %q", string(gotExisting), tt.wantExistingWav)
			}
		})
	}
}

func ttsTestFilename(text string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
	return hash + ".wav"
}
