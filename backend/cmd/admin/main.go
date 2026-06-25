package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"

	"japanese-learning-app/internal/config"
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

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if cfg.RelationalStore != "" && cfg.RelationalStore != "sqlite" && cfg.RelationalStore != "postgres" {
		slog.Error("unsupported relational store", "relational_store", cfg.RelationalStore)
		os.Exit(1)
	}

	ctx := context.Background()
	var sqliteOnlyDB *sql.DB
	var shadowingDB *sql.DB
	shadowingSQL := "sqlite"
	var cleanup []func()

	var wordStore store.AdminWordStoreInterface
	var grammarStore store.AdminGrammarStoreInterface
	var speakingStore store.AdminSpeakingStoreInterface
	var writingStore store.AdminWritingStoreInterface
	var translationStore store.AdminTranslationStoreInterface
	var userStore store.AdminUserStoreInterface

	if cfg.RelationalStore == "postgres" {
		adapter := pgdata.Adapter{}
		pgDB, err := adapter.Open(ctx, store.DatabaseConfig{
			DatabaseURL:     cfg.DatabaseURL,
			MaxOpenConns:    cfg.DBMaxOpenConns,
			MaxIdleConns:    cfg.DBMaxIdleConns,
			ConnMaxLifetime: cfg.DBConnMaxLifetime,
			AppTimezone:     cfg.AppTimezone,
		})
		if err != nil {
			slog.Error("failed to open PostgreSQL database", "err", err)
			os.Exit(1)
		}
		cleanup = append(cleanup, func() { _ = pgDB.Close() })

		if err := adapter.RunMigrations(ctx, pgDB); err != nil {
			slog.Error("failed to run PostgreSQL migrations", "err", err)
			os.Exit(1)
		}

		runtime, err := adapter.NewStoreRuntime(pgDB, store.StoreDeps{AppTimezone: cfg.AppTimezone})
		if err != nil {
			slog.Error("failed to build PostgreSQL store runtime", "err", err)
			os.Exit(1)
		}

		wordStore = runtime.Admin.Words
		grammarStore = runtime.Admin.Grammar
		speakingStore = runtime.Admin.Speaking
		writingStore = runtime.Admin.Writing
		translationStore = runtime.Admin.Translation
		userStore = runtime.Admin.Users
		shadowingDB = pgDB
		shadowingSQL = "postgres"
		slog.Info("admin using PostgreSQL relational store")
	} else {
		db, err := data.OpenDB(cfg.DBPath)
		if err != nil {
			slog.Error("failed to open database", "err", err)
			os.Exit(1)
		}
		cleanup = append(cleanup, func() { _ = db.Close() })

		if err := data.RunMigrations(db); err != nil {
			slog.Error("failed to run migrations", "err", err)
			os.Exit(1)
		}

		sqliteOnlyDB = db
		shadowingDB = db
		wordStore = data.NewWordStore(db)
		grammarStore = data.NewGrammarStore(db)
		speakingStore = data.NewSpeakingStore(db)
		writingStore = data.NewWritingStore(db)
		translationStore = data.NewTranslationStore(db)
		userStore = data.NewUserStore(db)
		slog.Info("admin using SQLite relational store", "db_path", cfg.DBPath)
	}
	defer func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}()

	shadowingDatabaseURL := os.Getenv("SHADOWING_DATABASE_URL")
	if shadowingDatabaseURL != "" {
		adapter := pgdata.Adapter{}
		pgDB, err := adapter.Open(ctx, store.DatabaseConfig{
			DatabaseURL:     shadowingDatabaseURL,
			MaxOpenConns:    cfg.DBMaxOpenConns,
			MaxIdleConns:    cfg.DBMaxIdleConns,
			ConnMaxLifetime: cfg.DBConnMaxLifetime,
			AppTimezone:     cfg.AppTimezone,
		})
		if err != nil {
			slog.Error("failed to open shadowing PostgreSQL database", "err", err)
			os.Exit(1)
		}
		cleanup = append(cleanup, func() { _ = pgDB.Close() })
		if err := adapter.RunMigrations(ctx, pgDB); err != nil {
			slog.Error("failed to run shadowing PostgreSQL migrations", "err", err)
			os.Exit(1)
		}
		shadowingDB = pgDB
		shadowingSQL = "postgres"
	}

	h := admin.NewHandler(admin.HandlerConfig{
		AdminToken:             adminToken,
		WordStore:              wordStore,
		GrammarStore:           grammarStore,
		SpeakingStore:          speakingStore,
		WritingStore:           writingStore,
		TranslationStore:       translationStore,
		UserStore:              userStore,
		DB:                     sqliteOnlyDB,
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
