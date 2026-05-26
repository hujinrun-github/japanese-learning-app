package main

import (
	"log/slog"
	"net/http"
	"os"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/admin"
)

func main() {
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		slog.Error("ADMIN_TOKEN environment variable is required")
		os.Exit(1)
	}

	aiAPIKey := os.Getenv("AI_API_KEY")
	aiEndpoint := envOrDefault("AI_API_ENDPOINT", "https://api.anthropic.com/v1/messages")

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

	// Initialize stores
	wordStore := data.NewWordStore(db)
	grammarStore := data.NewGrammarStore(db)
	speakingStore := data.NewSpeakingStore(db)
	writingStore := data.NewWritingStore(db)
	translationStore := data.NewTranslationStore(db)
	userStore := data.NewUserStore(db)

	h := admin.NewHandler(admin.HandlerConfig{
		AdminToken:       adminToken,
		WordStore:        wordStore,
		GrammarStore:     grammarStore,
		SpeakingStore:    speakingStore,
		WritingStore:     writingStore,
		TranslationStore: translationStore,
		UserStore:        userStore,
		DB:               db,
		AIAPIKey:         aiAPIKey,
		AIAPIEndpoint:    aiEndpoint,
	})

	mux := h.RegisterRoutes()

	addr := envOrDefault("LISTEN_ADDR", ":8082")
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
