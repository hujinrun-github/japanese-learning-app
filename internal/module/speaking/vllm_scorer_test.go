package speaking_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
)

func TestVLLMScorer_ScoreAudio_Success(t *testing.T) {
	// mock vLLM server returning recognized text
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// verify multipart content
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"text":"おはようございます。きょうもよろしくおねがいします。"}`))
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-asr-model", 10*time.Second)
	ctx := context.Background()
	refText := "おはようございます。今日もよろしくお願いします。"

	score, recText, err := scorer.ScoreAudio(ctx, []byte("fake-audio-data"), refText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recText == "" {
		t.Error("expected non-empty recognized text")
	}
	if score < 0 || score > 100 {
		t.Errorf("score must be 0-100, got %d", score)
	}
	// recognized text is pure kana vs reference with kanji, CER should indicate some differences
	t.Logf("score=%d recognized=%q", score, recText)
}

func TestVLLMScorer_ScoreAudio_PerfectMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"おはよう"}`))
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-asr-model", 10*time.Second)
	ctx := context.Background()

	score, recText, err := scorer.ScoreAudio(ctx, []byte("audio"), "おはよう")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recText != "おはよう" {
		t.Errorf("expected recognized text おはよう, got %s", recText)
	}
	if score != 100 {
		t.Errorf("expected perfect score 100, got %d", score)
	}
}

func TestVLLMScorer_ScoreAudio_CompleteMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"まったくちがう"}`))
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-asr-model", 10*time.Second)
	ctx := context.Background()

	score, _, err := scorer.ScoreAudio(ctx, []byte("audio"), "おはよう")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// completely different strings should get low score
	if score > 50 {
		t.Errorf("complete mismatch should score <= 50, got %d", score)
	}
}

func TestVLLMScorer_ScoreAudio_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-asr-model", 10*time.Second)
	ctx := context.Background()

	_, _, err := scorer.ScoreAudio(ctx, []byte("audio"), "ref")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestVLLMScorer_ScoreAudio_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(nil)
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-asr-model", 10*time.Second)
	ctx := context.Background()

	_, _, err := scorer.ScoreAudio(ctx, []byte("audio"), "ref")
	if err == nil {
		t.Error("expected error for empty response")
	}
}

func TestVLLMScorer_ScoreAudio_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// hang until context cancels
		<-r.Context().Done()
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-asr-model", 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := scorer.ScoreAudio(ctx, []byte("audio"), "ref")
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestVLLMScorer_ScoreAudio_InvalidURL(t *testing.T) {
	scorer := speaking.NewVLLMScorer("http://127.0.0.1:0", "test-model", 1*time.Second)
	ctx := context.Background()

	_, _, err := scorer.ScoreAudio(ctx, []byte("audio"), "ref")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestVLLMScorer_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	scorer := speaking.NewVLLMScorer(srv.URL, "test-model", 100*time.Millisecond)
	ctx := context.Background()

	_, _, err := scorer.ScoreAudio(ctx, []byte("audio"), "ref")
	if err == nil {
		t.Error("expected timeout error")
	}
}
