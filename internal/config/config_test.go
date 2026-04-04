package config

import (
	"testing"
)

func TestLoad_ReturnsNilNilForNow(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") returned unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("Load(\"\") expected nil config, got %+v", cfg)
	}
}

func TestConfigFields(t *testing.T) {
	// 验证 Config struct 所有字段均可设值（编译期类型检查）
	cfg := &Config{
		ListenAddr:     ":8080",
		DBPath:         "./data/app.db",
		JWTSecret:      "supersecret",
		JWTExpireHours: 72,
		LogLevel:       "INFO",
		AIAPIKey:       "sk-test",
		AIAPIEndpoint:  "https://api.anthropic.com/v1",
		AITimeoutSec:   15,
		AudioStorePath: "./data/audio",
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ListenAddr", cfg.ListenAddr, ":8080"},
		{"DBPath", cfg.DBPath, "./data/app.db"},
		{"JWTSecret", cfg.JWTSecret, "supersecret"},
		{"LogLevel", cfg.LogLevel, "INFO"},
		{"AIAPIKey", cfg.AIAPIKey, "sk-test"},
		{"AIAPIEndpoint", cfg.AIAPIEndpoint, "https://api.anthropic.com/v1"},
		{"AudioStorePath", cfg.AudioStorePath, "./data/audio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("Config.%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}

	if cfg.JWTExpireHours != 72 {
		t.Errorf("Config.JWTExpireHours = %d, want 72", cfg.JWTExpireHours)
	}
	if cfg.AITimeoutSec != 15 {
		t.Errorf("Config.AITimeoutSec = %d, want 15", cfg.AITimeoutSec)
	}
}
