package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr string

	RelationalStore   string
	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	AppTimezoneName   string
	AppTimezone       *time.Location

	AudioObjectStore        string
	MinIOEndpoint           string
	MinIOAccessKey          string
	MinIOSecretKey          string
	MinIOBucketAudio        string
	MinIOUseSSL             bool
	MinIOPublicEndpoint     string
	MinIOPresignTTLSeconds  int

	JWTSecret      string
	JWTExpireHours int
	LogLevel       string
	AIAPIKey       string
	AIAPIEndpoint  string
	AITimeoutSec   int

	// Compatibility fields kept while SQLite-backed runtime still exists.
	DBPath         string
	AudioStorePath string
}

func Load(path string) (*Config, error) {
	_ = path

	maxOpenConns, err := loadInt("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		return nil, err
	}

	maxIdleDefault := maxOpenConns
	maxIdleConns, err := loadInt("DB_MAX_IDLE_CONNS", maxIdleDefault)
	if err != nil {
		return nil, err
	}

	connMaxLifetime, err := loadDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute)
	if err != nil {
		return nil, err
	}

	appTimezone, appTimezoneName, err := loadLocation("APP_TIMEZONE")
	if err != nil {
		return nil, err
	}

	minioUseSSL, err := loadBool("MINIO_USE_SSL", false)
	if err != nil {
		return nil, err
	}

	minioPresignTTLSeconds, err := loadInt("MINIO_PRESIGN_TTL_SECONDS", 900)
	if err != nil {
		return nil, err
	}

	jwtExpireHours, err := loadInt("JWT_EXPIRE_HOURS", 72)
	if err != nil {
		return nil, err
	}

	aiTimeoutSec, err := loadInt("AI_TIMEOUT_SEC", 15)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ListenAddr: envOrDefault("LISTEN_ADDR", ":30081"),

		RelationalStore:   os.Getenv("RELATIONAL_STORE"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		DBMaxOpenConns:    maxOpenConns,
		DBMaxIdleConns:    maxIdleConns,
		DBConnMaxLifetime: connMaxLifetime,
		AppTimezoneName:   appTimezoneName,
		AppTimezone:       appTimezone,

		AudioObjectStore:       os.Getenv("AUDIO_OBJECT_STORE"),
		MinIOEndpoint:          os.Getenv("MINIO_ENDPOINT"),
		MinIOAccessKey:         os.Getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey:         os.Getenv("MINIO_SECRET_KEY"),
		MinIOBucketAudio:       os.Getenv("MINIO_BUCKET_AUDIO"),
		MinIOUseSSL:            minioUseSSL,
		MinIOPublicEndpoint:    os.Getenv("MINIO_PUBLIC_ENDPOINT"),
		MinIOPresignTTLSeconds: minioPresignTTLSeconds,

		JWTSecret:      envOrDefault("JWT_SECRET", "change-me-in-production"),
		JWTExpireHours: jwtExpireHours,
		LogLevel:       envOrDefault("LOG_LEVEL", "INFO"),
		AIAPIKey:       os.Getenv("AI_API_KEY"),
		AIAPIEndpoint:  envOrDefault("AI_API_ENDPOINT", "https://api.anthropic.com/v1/messages"),
		AITimeoutSec:   aiTimeoutSec,

		DBPath:         envOrDefault("DB_PATH", "./data/app.db"),
		AudioStorePath: envOrDefault("AUDIO_STORE_PATH", "./data/audio"),
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("load %s: %w", key, err)
	}
	return value, nil
}

func loadBool(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("load %s: %w", key, err)
	}
	return value, nil
}

func loadDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("load %s: %w", key, err)
	}
	return value, nil
}

func loadLocation(key string) (*time.Location, string, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return time.Local, time.Local.String(), nil
	}

	location, err := time.LoadLocation(raw)
	if err != nil {
		return nil, "", fmt.Errorf("load %s: %w", key, err)
	}
	return location, raw, nil
}
