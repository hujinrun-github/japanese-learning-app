package speaking_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
)

func TestPronunciationAlignment(t *testing.T) {
	analyzer, err := speaking.NewPronunciationAnalyzer(nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, reference, recognized string
		wrong                       map[string]string
	}{
		{"same pronunciation with kana spelling", "私は駅へ行きます。", "わたしはえきへいきます", nil},
		{"katakana spelling", "カメラです。", "かめらです", nil},
		{"homophones are accepted", "橋を見ます。", "箸を見ます。", nil},
		{"one wrong word keeps later words aligned", "駅はどこですか。", "海はどこですか。", map[string]string{"駅": "mismatch"}},
		{"omitted word keeps later words aligned", "私は駅へ行きます。", "私はへ行きます。", map[string]string{"駅": "missing"}},
		{"punctuation and whitespace are not speech", "  駅は、どこですか？\n", "駅はどこですか", nil},
		{"supplementary Unicode preserves codepoint offsets", "😀 駅です。", "駅です。", nil},
		{"halfwidth katakana spelling", "カメラです。", "ｶﾒﾗです。", nil},
		{"kanji long vowel and kana", "東京へ行きます。", "とうきょうへいきます。", nil},
		{"kanji small tsu and kana", "学校へ行きます。", "がっこうへいきます。", nil},
		{"father long vowel", "お父さんです。", "おとうさんです。", nil},
		{"Arabic and kanji numeral spelling", "3です。", "三です。", nil},
		{"multi digit and kanji numeral spelling", "2026です。", "二千二十六です。", nil},
		{"fullwidth and halfwidth digits", "２０２６です。", "2026です。", nil},
		{"incorrect numbers are still marked", "3です。", "四です。", map[string]string{"3": "mismatch"}},
		{"CRLF preserves offsets", "  駅です。\r\n\r\n 東京です。", "えきです。とうきょうです。", nil},
		{"dialogue labels and item numbers are not spoken", "1. 李：私は学生です。\n小野：よろしくお願いします。", "私は学生です。よろしくお願いします。", nil},
		{"empty transcript marks speech as missing", "駅です。", "", map[string]string{"駅": "missing", "です": "missing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.Compare(tt.reference, tt.recognized)
			if result.Status != "available" {
				t.Fatalf("status = %s: %s", result.Status, result.Message)
			}
			source := []rune(tt.reference)
			seen := make(map[string]bool)
			for _, word := range result.Words {
				if word.Start < 0 || word.End > len(source) || word.End <= word.Start || string(source[word.Start:word.End]) != word.Surface {
					t.Errorf("word does not point to original codepoint range: %+v", word)
				}
				want := tt.wrong[word.Surface]
				if want == "" {
					want = "correct"
				}
				if word.Status != want {
					t.Errorf("word %s status=%s recognized=%q, want %s", word.Surface, word.Status, word.Recognized, want)
				}
				seen[word.Surface] = true
			}
			for surface := range tt.wrong {
				if !seen[surface] {
					t.Errorf("missing feedback for %q: %+v", surface, result.Words)
				}
			}
		})
	}
}

func TestPronunciationUsesStandardReadingForDisplay(t *testing.T) {
	analyzer, err := speaking.NewPronunciationAnalyzer(nil)
	if err != nil {
		t.Fatal(err)
	}
	feedback := analyzer.Compare("東京へ行きます。", "とうきょうへいきます。")
	if len(feedback.Words) == 0 || feedback.Words[0].Reading != "とうきょう" {
		t.Fatalf("standard furigana for 東京 should be とうきょう: %+v", feedback)
	}
}

func TestPronunciationAnalyzerASR(t *testing.T) {
	for _, tt := range []struct {
		name, response, wantStatus string
		status                     int
	}{
		{"recognition success", `{"text":"海はどこですか。"}`, "available", http.StatusOK},
		{"ASR service failure", `service unavailable`, "error", http.StatusServiceUnavailable},
		{"invalid JSON", `broken`, "error", http.StatusOK},
		{"empty recognition is not a perfect score", `{"text":"   "}`, "error", http.StatusOK},
		{"excessive transcript is bounded", `{"text":"` + strings.Repeat("あ", 8193) + `"}`, "error", http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Errorf("parse multipart: %v", err)
				}
				defer r.MultipartForm.RemoveAll()
				if got := r.FormValue("response_format"); got != "json" {
					t.Errorf("response_format=%q, want json", got)
				}
				if r.FormValue("language") != "ja" || r.FormValue("model") != "whisper-test" {
					t.Errorf("unexpected transcription fields: %v", r.Form)
				}
				f, header, err := r.FormFile("file")
				if err != nil {
					t.Errorf("missing file: %v", err)
				} else {
					if header.Filename != "attempt.wav" {
						t.Errorf("filename=%q, want attempt.wav", header.Filename)
					}
					if err := f.Close(); err != nil {
						t.Error(err)
					}
				}
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.response)); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			analyzer, err := speaking.NewPronunciationAnalyzer(speaking.NewVLLMScorer(srv.URL, "whisper-test", time.Second))
			if err != nil {
				t.Fatal(err)
			}
			feedback, transcript := analyzer.Analyze(context.Background(), []byte("RIFF-test-audio"), "駅はどこですか。")
			if feedback.Status != tt.wantStatus {
				t.Fatalf("status=%q, want %s: %+v", feedback.Status, tt.wantStatus, feedback)
			}
			if tt.wantStatus == "available" && (transcript != "海はどこですか。" || len(feedback.Words) == 0) {
				t.Errorf("missing recognition feedback: transcript=%q words=%+v", transcript, feedback.Words)
			}
			if tt.wantStatus == "error" && (len(feedback.Words) != 0 || feedback.Message == "") {
				t.Errorf("failed ASR must not emit word judgements: %+v", feedback)
			}
			if _, err := json.Marshal(feedback); err != nil {
				t.Fatal(err)
			}
		})
	}
	var unconfigured *speaking.PronunciationAnalyzer
	feedback, transcript := unconfigured.Analyze(context.Background(), nil, "駅です。")
	if feedback.Status != "unavailable" || len(feedback.Words) != 0 || transcript != "" {
		t.Errorf("unconfigured ASR must be explicitly unavailable: %+v %q", feedback, transcript)
	}
}

func TestPronunciationASRInFlightCancellation(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read transcription body: %v", err)
			return
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
			t.Error("ASR request context was not cancelled")
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := speaking.NewVLLMScorer(srv.URL, "whisper-test", 5*time.Second).ScoreAudio(ctx, []byte("RIFF-test-audio"), "駅です。")
		result <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("ASR request did not start")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ASR should preserve cancellation cause: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ASR client did not return after cancellation")
	}
}
