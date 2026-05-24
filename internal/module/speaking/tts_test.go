package speaking_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
)

func TestVLLMTTSClient_Synthesize_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Write([]byte("fake-audio-data"))
	}))
	defer srv.Close()

	client := speaking.NewVLLMTTSClient(srv.URL, "test-model", 10*time.Second)
	ctx := context.Background()

	audio, err := client.Synthesize(ctx, "おはようございます")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(audio) == 0 {
		t.Error("expected non-empty audio data")
	}
}

func TestVLLMTTSClient_Synthesize_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := speaking.NewVLLMTTSClient(srv.URL, "test-model", 10*time.Second)
	ctx := context.Background()

	_, err := client.Synthesize(ctx, "test")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestVLLMTTSClient_Synthesize_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(nil)
	}))
	defer srv.Close()

	client := speaking.NewVLLMTTSClient(srv.URL, "test-model", 10*time.Second)
	ctx := context.Background()

	_, err := client.Synthesize(ctx, "test")
	if err == nil {
		t.Error("expected error for empty response")
	}
}
