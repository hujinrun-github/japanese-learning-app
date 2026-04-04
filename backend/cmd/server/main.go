package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"japanese-learning-app/internal/cli"
	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/module/summary"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
	"japanese-learning-app/internal/module/writing"
)

func main() {
	// ── CLI sub-commands ──────────────────────────────────────────────────────
	// If the first argument is a known CLI command, dispatch to the CLI handler
	// and exit without starting the HTTP server.
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		os.Exit(cli.Run(os.Args[1:]))
	}

	// ── Configuration ─────────────────────────────────────────────────────────
	dbPath := envOrDefault("DB_PATH", "./data/app.db")
	listenAddr := envOrDefault("LISTEN_ADDR", ":8080")
	jwtSecret := envOrDefault("JWT_SECRET", "change-me-in-production")
	logLevel := envOrDefault("LOG_LEVEL", "INFO")
	aiAPIKey := envOrDefault("AI_API_KEY", "")
	aiEndpoint := envOrDefault("AI_API_ENDPOINT", "https://api.anthropic.com/v1/messages")
	staticDir := envOrDefault("STATIC_DIR", "./front/web/static")
	templateDir := envOrDefault("TEMPLATE_DIR", "./front/web/templates")

	setupLogger(logLevel)

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := data.OpenDB(dbPath)
	if err != nil {
		slog.Error("failed to open database", "db", dbPath, "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}

	// ── Stores ────────────────────────────────────────────────────────────────
	wordStore     := data.NewWordStore(db)
	grammarStore  := data.NewGrammarStore(db)
	lessonStore   := data.NewLessonStore(db)
	speakingStore := data.NewSpeakingStore(db)
	writingStore  := data.NewWritingStore(db)
	userStore     := data.NewUserStore(db)
	sessionStore  := data.NewSessionStore(db)

	// ── AI reviewer (writing) ─────────────────────────────────────────────────
	var aiReviewer writing.AIReviewer
	if aiAPIKey != "" {
		slog.Info("using ClaudeClient for AI writing review", "endpoint", aiEndpoint)
		aiReviewer = writing.NewClaudeClient(aiAPIKey)
	} else {
		slog.Warn("AI_API_KEY not set — using StubReviewer (no real AI feedback)")
		aiReviewer = &writing.StubReviewer{}
	}

	// ── Store adapters (bridge data.*Store to module.*StoreInterface) ─────────
	wordAdapter    := data.NewWordStoreAdapter(wordStore)
	lessonAdapter  := data.NewLessonStoreAdapter(lessonStore)
	userAdapter    := data.NewUserStoreAdapter(userStore)
	sessionAdapter := data.NewSessionStoreAdapter(sessionStore)

	// ── Services ──────────────────────────────────────────────────────────────
	wordSvc     := word.NewWordService(wordAdapter)
	grammarSvc  := grammar.NewGrammarService(grammarStore)
	lessonSvc   := lesson.NewLessonService(lessonAdapter)
	speakingSvc := speaking.NewSpeakingService(speakingStore, speaking.NewWaveformScorer())
	writingSvc  := writing.NewWritingService(writingStore, aiReviewer)
	userSvc     := user.NewUserService(userAdapter, jwtSecret)
	summarySvc  := summary.NewSummaryService(sessionAdapter)

	// ── Handlers ─────────────────────────────────────────────────────────────
	wordH     := word.NewWordHandler(wordSvc)
	grammarH  := grammar.NewGrammarHandler(grammarSvc)
	lessonH   := lesson.NewLessonHandler(lessonSvc)
	speakingH := speaking.NewSpeakingHandler(speakingSvc)
	writingH  := writing.NewWritingHandler(writingSvc)
	userH     := user.NewUserHandler(userSvc)
	summaryH  := summary.NewSummaryHandler(summarySvc)

	// ── Mux ───────────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Public auth routes (no middleware)
	userH.RegisterRoutes(mux)

	// Protected API routes wrapped in AuthMiddleware
	protectedMux := http.NewServeMux()
	wordH.RegisterRoutes(protectedMux)
	grammarH.RegisterRoutes(protectedMux)
	lessonH.RegisterRoutes(protectedMux)
	speakingH.RegisterRoutes(protectedMux)
	writingH.RegisterRoutes(protectedMux)
	summaryH.RegisterRoutes(protectedMux)

	mux.Handle("/api/v1/words/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/grammar", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/grammar/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/lessons", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/lessons/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/speaking/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/writing/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/summary", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/summary/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/users/", user.AuthMiddleware(jwtSecret, protectedMux))

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// HTML pages (serve template files directly; in a production app a template
	// handler would render these with data, but for Phase 4 we serve them as-is)
	mux.Handle("/", http.FileServer(http.Dir(templateDir)))

	// ── Server ────────────────────────────────────────────────────────────────
	slog.Info("server starting", "addr", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		slog.Error("server error", "err", err)
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

// envOrDefault returns the environment variable value or a fallback default.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// setupLogger configures the global slog handler based on the log level string.
func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}
