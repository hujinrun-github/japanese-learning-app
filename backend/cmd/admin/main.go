package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"japanese-learning-app/internal/config"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/admin"
	"japanese-learning-app/internal/module/errata"
	"japanese-learning-app/internal/store"
)

func main() {
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		slog.Error("ADMIN_TOKEN environment variable is required")
		os.Exit(1)
	}

	aiAPIKey := os.Getenv("AI_API_KEY")
	aiEndpoint := envOrDefault("AI_API_ENDPOINT", "https://api.deepseek.com/v1/chat/completions")
	aiModel := envOrDefault("AI_MODEL", "deepseek-chat")

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{DatabaseURL: cfg.DatabaseURL, MaxOpenConns: cfg.DBMaxOpenConns, MaxIdleConns: cfg.DBMaxIdleConns, ConnMaxLifetime: cfg.DBConnMaxLifetime})
	if err != nil {
		slog.Error("open PostgreSQL", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := adapter.RunMigrations(ctx, db); err != nil {
		slog.Error("migrate PostgreSQL", "err", err)
		os.Exit(1)
	}
	runtime, err := adapter.NewStoreRuntime(db, store.StoreDeps{AppTimezone: cfg.AppTimezone})
	if err != nil {
		slog.Error("initialize PostgreSQL stores", "err", err)
		os.Exit(1)
	}
	h := admin.NewHandler(admin.HandlerConfig{
		AdminToken:             adminToken,
		WordStore:              runtime.Admin.Words,
		GrammarStore:           runtime.Admin.Grammar,
		SpeakingStore:          runtime.Admin.Speaking,
		WritingStore:           runtime.Admin.Writing,
		TranslationStore:       runtime.Admin.Translation,
		PracticeLinks:          pgdata.NewPracticeLinkStore(db),
		UserStore:              runtime.Admin.Users,
		ErrataStore:            errata.NewStore(db),
		DB:                     db,
		AudioStorePath:         cfg.AudioStorePath,
		ShadowingMaterialsDB:   db,
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
	registerHealthRoute(mux)

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
