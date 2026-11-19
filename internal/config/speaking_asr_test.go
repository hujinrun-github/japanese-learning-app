package config

import (
	"strings"
	"testing"
)

func TestLoadSpeakingASR(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		model      string
		timeout    string
		wantURL    string
		wantModel  string
		wantSecond int
		wantError  string
	}{
		{name: "disabled by default", wantModel: "whisper", wantSecond: 120},
		{name: "configured transcription endpoint", url: "http://localhost:8000/v1/audio/transcriptions", model: "kotoba-whisper", timeout: "180", wantURL: "http://localhost:8000/v1/audio/transcriptions", wantModel: "kotoba-whisper", wantSecond: 180},
		{name: "trims configuration", url: " https://asr.example/v1/audio/transcriptions ", model: " whisper-custom ", wantURL: "https://asr.example/v1/audio/transcriptions", wantModel: "whisper-custom", wantSecond: 120},
		{name: "requires absolute URL", url: "/v1/audio/transcriptions", wantError: "SPEAKING_ASR_URL"},
		{name: "rejects unsupported scheme", url: "file:///tmp/audio", wantError: "SPEAKING_ASR_URL"},
		{name: "rejects invalid timeout", timeout: "slow", wantError: "SPEAKING_ASR_TIMEOUT_SEC"},
		{name: "rejects zero timeout", timeout: "0", wantError: "SPEAKING_ASR_TIMEOUT_SEC"},
		{name: "rejects negative timeout", timeout: "-1", wantError: "SPEAKING_ASR_TIMEOUT_SEC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SPEAKING_ASR_URL", tt.url)
			t.Setenv("SPEAKING_ASR_PROVIDER", "")
			t.Setenv("SPEAKING_ASR_MODEL", tt.model)
			t.Setenv("SPEAKING_ASR_TIMEOUT_SEC", tt.timeout)
			cfg, err := Load("")
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Load() error = %v, want context %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if cfg.SpeakingASRURL != tt.wantURL || cfg.SpeakingASRModel != tt.wantModel || cfg.SpeakingASRTimeoutSec != tt.wantSecond {
				t.Fatalf("speaking ASR = (%q, %q, %d), want (%q, %q, %d)", cfg.SpeakingASRURL, cfg.SpeakingASRModel, cfg.SpeakingASRTimeoutSec, tt.wantURL, tt.wantModel, tt.wantSecond)
			}
		})
	}
}

func TestLoadSpeakingASRProvider(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		want      string
		wantError bool
	}{
		{name: "defaults to OpenAI protocol", want: "openai"},
		{name: "explicit OpenAI protocol", provider: "openai", want: "openai"},
		{name: "explicit whisper.cpp protocol", provider: "whisper_cpp", want: "whisper_cpp"},
		{name: "trims provider", provider: " whisper_cpp ", want: "whisper_cpp"},
		{name: "whitespace uses default", provider: "  ", want: "openai"},
		{name: "rejects unknown protocol", provider: "whisper", wantError: true},
		{name: "rejects unsupported capitalization", provider: "OPENAI", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SPEAKING_ASR_PROVIDER", tt.provider)
			t.Setenv("SPEAKING_ASR_URL", "")
			t.Setenv("SPEAKING_ASR_MODEL", "")
			t.Setenv("SPEAKING_ASR_TIMEOUT_SEC", "")
			cfg, err := Load("")
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), "SPEAKING_ASR_PROVIDER") {
					t.Fatalf("Load() error = %v, want SPEAKING_ASR_PROVIDER context", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if cfg.SpeakingASRProvider != tt.want {
				t.Fatalf("SpeakingASRProvider = %q, want %q", cfg.SpeakingASRProvider, tt.want)
			}
		})
	}
}
