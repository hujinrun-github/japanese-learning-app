package speaking_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestGradioTTSClient_Synthesize_Success(t *testing.T) {
	const (
		wantText         = "こんにちは"
		wantLanguage     = "Japanese"
		wantSpeaker      = "Vivian"
		wantInstructions = "標準語で、自然にはっきり発音してください。"
	)

	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gradio_api/call/v2/run_instruct":
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("expected application/json, got %s", got)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if body["text"] != wantText {
				t.Errorf("text = %q, want %q", body["text"], wantText)
			}
			if body["lang_disp"] != wantLanguage {
				t.Errorf("lang_disp = %q, want %q", body["lang_disp"], wantLanguage)
			}
			if body["spk_disp"] != wantSpeaker {
				t.Errorf("spk_disp = %q, want %q", body["spk_disp"], wantSpeaker)
			}
			if body["instruct"] != wantInstructions {
				t.Errorf("instruct = %q, want %q", body["instruct"], wantInstructions)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"event_id":"evt-1"}`)
		case "/gradio_api/call/run_instruct/evt-1":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: complete\ndata: [{\"path\":\"/tmp/audio.wav\",\"url\":\"%s/gradio_api/file=/tmp/audio.wav\",\"meta\":{\"_type\":\"gradio.FileData\"}},\"Finished\"]\n\n", serverURL)
		case "/gradio_api/file=/tmp/audio.wav":
			w.Header().Set("Content-Type", "audio/wav")
			fmt.Fprint(w, "fake-gradio-audio")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	serverURL = srv.URL

	client := speaking.NewGradioTTSClient(srv.URL, 10*time.Second,
		speaking.WithGradioLanguage(wantLanguage),
		speaking.WithGradioSpeaker(wantSpeaker),
		speaking.WithGradioInstructions(wantInstructions),
	)

	audio, err := client.Synthesize(context.Background(), wantText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(audio) != "fake-gradio-audio" {
		t.Fatalf("audio = %q, want fake-gradio-audio", string(audio))
	}
}

func TestGradioTTSClient_Synthesize_IgnoresHeartbeatAndEmptyData(t *testing.T) {
	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gradio_api/call/v2/run_instruct":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"event_id":"evt-1"}`)
		case "/gradio_api/call/run_instruct/evt-1":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: heartbeat\ndata: null\n\n")
			fmt.Fprint(w, "event: generating\ndata: []\n\n")
			fmt.Fprintf(w, "event: complete\ndata: [{\"path\":\"/tmp/audio.wav\",\"url\":\"%s/gradio_api/file=/tmp/audio.wav\",\"meta\":{\"_type\":\"gradio.FileData\"}},\"Finished\"]\n\n", serverURL)
		case "/gradio_api/file=/tmp/audio.wav":
			w.Header().Set("Content-Type", "audio/wav")
			fmt.Fprint(w, "fake-gradio-audio")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	serverURL = srv.URL

	client := speaking.NewGradioTTSClient(srv.URL, 10*time.Second)
	audio, err := client.Synthesize(context.Background(), "こんにちは")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(audio) != "fake-gradio-audio" {
		t.Fatalf("audio = %q, want fake-gradio-audio", string(audio))
	}
}

func TestGradioTTSClient_Synthesize_Errors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "missing event id",
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{}`)
			},
		},
		{
			name: "missing audio url",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/gradio_api/call/v2/run_instruct":
					fmt.Fprint(w, `{"event_id":"evt-1"}`)
				case "/gradio_api/call/run_instruct/evt-1":
					fmt.Fprint(w, "event: complete\ndata: [{\"path\":\"/tmp/audio.wav\"},\"Finished\"]\n\n")
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
			},
		},
		{
			name: "audio download error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/gradio_api/call/v2/run_instruct":
					fmt.Fprint(w, `{"event_id":"evt-1"}`)
				case "/gradio_api/call/run_instruct/evt-1":
					fmt.Fprint(w, "event: complete\ndata: [{\"url\":\"/gradio_api/file=/tmp/audio.wav\"},\"Finished\"]\n\n")
				case "/gradio_api/file=/tmp/audio.wav":
					http.Error(w, "download failed", http.StatusInternalServerError)
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			client := speaking.NewGradioTTSClient(srv.URL, 10*time.Second)
			if _, err := client.Synthesize(context.Background(), "こんにちは"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
