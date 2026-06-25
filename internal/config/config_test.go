package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Helper()

	envKeys := []string{
		"LISTEN_ADDR",
		"RELATIONAL_STORE",
		"DATABASE_URL",
		"DB_MAX_OPEN_CONNS",
		"DB_MAX_IDLE_CONNS",
		"DB_CONN_MAX_LIFETIME",
		"APP_TIMEZONE",
		"AUDIO_OBJECT_STORE",
		"MINIO_ENDPOINT",
		"MINIO_ACCESS_KEY",
		"MINIO_SECRET_KEY",
		"MINIO_BUCKET_AUDIO",
		"MINIO_USE_SSL",
		"MINIO_PUBLIC_ENDPOINT",
		"MINIO_PRESIGN_TTL_SECONDS",
		"JWT_SECRET",
		"JWT_EXPIRE_HOURS",
		"LOG_LEVEL",
		"AI_API_KEY",
		"AI_API_ENDPOINT",
		"AI_TIMEOUT_SEC",
		"DB_PATH",
		"AUDIO_STORE_PATH",
	}

	cases := []struct {
		name             string
		env              map[string]string
		wantListenAddr   string
		wantStore        string
		wantDBURL        string
		wantTZ           string
		wantAudioStore   string
		wantMinIOUseSSL  bool
		wantPresignTTL   int
		wantDBPath       string
		wantAudioPath    string
		wantMaxOpenConns int
		wantMaxIdleConns int
		wantConnLife     time.Duration
	}{
		{
			name: "postgres and minio config",
			env: map[string]string{
				"LISTEN_ADDR":               ":9090",
				"RELATIONAL_STORE":          "postgres",
				"DATABASE_URL":              "postgres://u:p@localhost:5432/app?sslmode=disable",
				"DB_MAX_OPEN_CONNS":         "12",
				"DB_MAX_IDLE_CONNS":         "6",
				"DB_CONN_MAX_LIFETIME":      "7m",
				"APP_TIMEZONE":              "Asia/Shanghai",
				"AUDIO_OBJECT_STORE":        "s3",
				"MINIO_ENDPOINT":            "localhost:9000",
				"MINIO_ACCESS_KEY":          "minioadmin",
				"MINIO_SECRET_KEY":          "minioadmin",
				"MINIO_BUCKET_AUDIO":        "japanese-learning-audio",
				"MINIO_USE_SSL":             "true",
				"MINIO_PUBLIC_ENDPOINT":     "https://cdn.example.com",
				"MINIO_PRESIGN_TTL_SECONDS": "600",
			},
			wantListenAddr:   ":9090",
			wantStore:        "postgres",
			wantDBURL:        "postgres://u:p@localhost:5432/app?sslmode=disable",
			wantTZ:           "Asia/Shanghai",
			wantAudioStore:   "s3",
			wantMinIOUseSSL:  true,
			wantPresignTTL:   600,
			wantDBPath:       "./data/app.db",
			wantAudioPath:    "./data/audio",
			wantMaxOpenConns: 12,
			wantMaxIdleConns: 6,
			wantConnLife:     7 * time.Minute,
		},
		{
			name:             "defaults",
			env:              map[string]string{},
			wantListenAddr:   ":30081",
			wantStore:        "",
			wantDBURL:        "",
			wantTZ:           time.Local.String(),
			wantAudioStore:   "",
			wantMinIOUseSSL:  false,
			wantPresignTTL:   900,
			wantDBPath:       "./data/app.db",
			wantAudioPath:    "./data/audio",
			wantMaxOpenConns: 25,
			wantMaxIdleConns: 25,
			wantConnLife:     5 * time.Minute,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range envKeys {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := Load("")
			if err != nil {
				t.Fatalf("Load(\"\") error = %v", err)
			}

			if cfg.ListenAddr != tt.wantListenAddr {
				t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, tt.wantListenAddr)
			}
			if cfg.RelationalStore != tt.wantStore {
				t.Fatalf("RelationalStore = %q, want %q", cfg.RelationalStore, tt.wantStore)
			}
			if cfg.DatabaseURL != tt.wantDBURL {
				t.Fatalf("DatabaseURL = %q, want %q", cfg.DatabaseURL, tt.wantDBURL)
			}
			if cfg.AppTimezone == nil {
				t.Fatal("AppTimezone is nil")
			}
			if cfg.AppTimezone.String() != tt.wantTZ {
				t.Fatalf("AppTimezone = %q, want %q", cfg.AppTimezone.String(), tt.wantTZ)
			}
			if cfg.AudioObjectStore != tt.wantAudioStore {
				t.Fatalf("AudioObjectStore = %q, want %q", cfg.AudioObjectStore, tt.wantAudioStore)
			}
			if cfg.MinIOUseSSL != tt.wantMinIOUseSSL {
				t.Fatalf("MinIOUseSSL = %t, want %t", cfg.MinIOUseSSL, tt.wantMinIOUseSSL)
			}
			if cfg.MinIOPresignTTLSeconds != tt.wantPresignTTL {
				t.Fatalf("MinIOPresignTTLSeconds = %d, want %d", cfg.MinIOPresignTTLSeconds, tt.wantPresignTTL)
			}
			if cfg.DBPath != tt.wantDBPath {
				t.Fatalf("DBPath = %q, want %q", cfg.DBPath, tt.wantDBPath)
			}
			if cfg.AudioStorePath != tt.wantAudioPath {
				t.Fatalf("AudioStorePath = %q, want %q", cfg.AudioStorePath, tt.wantAudioPath)
			}
			if cfg.DBMaxOpenConns != tt.wantMaxOpenConns {
				t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, tt.wantMaxOpenConns)
			}
			if cfg.DBMaxIdleConns != tt.wantMaxIdleConns {
				t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, tt.wantMaxIdleConns)
			}
			if cfg.DBConnMaxLifetime != tt.wantConnLife {
				t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, tt.wantConnLife)
			}
		})
	}
}

func TestLoad_InvalidTimezone(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "Mars/OlympusMons")

	_, err := Load("")
	if err == nil {
		t.Fatal("Load(\"\") expected error for invalid timezone")
	}
	if !strings.Contains(err.Error(), "APP_TIMEZONE") {
		t.Fatalf("Load(\"\") error = %v, want APP_TIMEZONE context", err)
	}
}
