package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"japanese-learning-app/internal/data"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/admin"
	"japanese-learning-app/internal/store"
)

func main() {
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		slog.Error("ADMIN_TOKEN environment variable is required")
		os.Exit(1)
	}

	aiAPIKey := envOrDefault("AI_API_KEY", "sk-a235671815e6469c8c15e69f18494500")
	aiEndpoint := envOrDefault("AI_API_ENDPOINT", "https://api.deepseek.com/v1/chat/completions")
	aiModel := envOrDefault("AI_MODEL", "deepseek-chat")

	dbPath := envOrDefault("DB_PATH", "./data/app.db")
	db, err := data.OpenDB(dbPath)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}
	shadowingDB := db
	shadowingSQL := "sqlite"
	shadowingDatabaseURL := os.Getenv("SHADOWING_DATABASE_URL")
	if shadowingDatabaseURL == "" && os.Getenv("RELATIONAL_STORE") == "postgres" {
		shadowingDatabaseURL = os.Getenv("DATABASE_URL")
	}
	if shadowingDatabaseURL != "" {
		adapter := pgdata.Adapter{}
		pgDB, err := adapter.Open(context.Background(), store.DatabaseConfig{
			DatabaseURL:     shadowingDatabaseURL,
			MaxOpenConns:    4,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Hour,
		})
		if err != nil {
			slog.Error("failed to open shadowing PostgreSQL database", "err", err)
			os.Exit(1)
		}
		defer pgDB.Close()
		if err := adapter.RunMigrations(context.Background(), pgDB); err != nil {
			slog.Error("failed to run shadowing PostgreSQL migrations", "err", err)
			os.Exit(1)
		}
		shadowingDB = pgDB
		shadowingSQL = "postgres"
	}

	// Initialize stores
	wordStore := data.NewWordStore(db)
	grammarStore := data.NewGrammarStore(db)
	speakingStore := data.NewSpeakingStore(db)
	writingStore := data.NewWritingStore(db)
	translationStore := data.NewTranslationStore(db)
	userStore := data.NewUserStore(db)

	h := admin.NewHandler(admin.HandlerConfig{
		AdminToken:             adminToken,
		WordStore:              wordStore,
		GrammarStore:           grammarStore,
		SpeakingStore:          speakingStore,
		WritingStore:           writingStore,
		TranslationStore:       translationStore,
		UserStore:              userStore,
		DB:                     db,
		ShadowingMaterialsDB:   shadowingDB,
		ShadowingMaterialsSQL:  shadowingSQL,
		ShadowingVideoBasePath: envOrDefault("SHADOWING_VIDEO_BASE_PATH", "/api/v1/videos"),
		ShadowingVideoBucket:   envOrDefault("MINIO_BUCKET_VIDEO", "lesson-videos"),
		MinIOEndpoint:          os.Getenv("MINIO_ENDPOINT"),
		MinIOAccessKey:         os.Getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey:         os.Getenv("MINIO_SECRET_KEY"),
		MinIOUseSSL:            os.Getenv("MINIO_USE_SSL") == "true",
		AIAPIKey:               aiAPIKey,
		AIAPIEndpoint:          aiEndpoint,
		AIModel:                aiModel,
	})

	mux := h.RegisterRoutes()

	addr := envOrDefault("LISTEN_ADDR", ":30082")
	slog.Info("admin server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
