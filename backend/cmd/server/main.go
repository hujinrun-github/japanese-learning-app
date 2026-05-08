package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"japanese-learning-app/internal/cli"
	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/module/review"
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
	listenAddr := envOrDefault("LISTEN_ADDR", ":8081")
	jwtSecret := envOrDefault("JWT_SECRET", "change-me-in-production")
	logLevel := envOrDefault("LOG_LEVEL", "INFO")
	aiAPIKey := envOrDefault("AI_API_KEY", "")
	aiEndpoint := envOrDefault("AI_API_ENDPOINT", "https://api.anthropic.com/v1/messages")
	staticDir := envOrDefault("STATIC_DIR", "./front/dist/assets")
	templateDir := envOrDefault("TEMPLATE_DIR", "./front/dist")
	// SMTP settings for password reset emails
	smtpHost := envOrDefault("SMTP_HOST", "")
	smtpPort := envOrDefault("SMTP_PORT", "587")
	smtpUser := envOrDefault("SMTP_USER", "")
	smtpPass := envOrDefault("SMTP_PASS", "")
	smtpFrom := envOrDefault("SMTP_FROM", "noreply@japanese-learning.app")
	appBaseURL := envOrDefault("APP_BASE_URL", "http://localhost:5173")

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
	noteStore     := data.NewNoteStore(db)

	// ── AI reviewer (writing) ─────────────────────────────────────────────────
	var aiReviewer writing.AIReviewer
	if aiAPIKey != "" {
		slog.Info("using ClaudeClient for AI writing review", "endpoint", aiEndpoint)
		aiReviewer = writing.NewClaudeClient(aiAPIKey)
	} else {
		slog.Warn("AI_API_KEY not set — using StubReviewer (no real AI feedback)")
		aiReviewer = &writing.StubReviewer{}
	}

	// ── Mailer (password reset) ───────────────────────────────────────────────
	var mailer user.Mailer
	if smtpHost != "" {
		slog.Info("using SMTPMailer for password reset", "smtp_host", smtpHost)
		mailer = user.NewSMTPMailer(smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom)
	} else {
		slog.Warn("SMTP_HOST not set — using StubMailer (reset URLs logged only)")
		mailer = &user.StubMailer{}
	}

	// ── Store adapters (bridge data.*Store to module.*StoreInterface) ─────────
	wordAdapter    := data.NewWordStoreAdapter(wordStore)
	lessonAdapter  := data.NewLessonStoreAdapter(lessonStore)
	userAdapter    := data.NewUserStoreAdapter(userStore)
	sessionAdapter := data.NewSessionStoreAdapter(sessionStore)
	noteAdapter    := data.NewNoteStoreAdapter(noteStore)

	// ── Services ──────────────────────────────────────────────────────────────
	wordSvc     := word.NewWordService(wordAdapter)
	grammarSvc  := grammar.NewGrammarService(grammarStore)
	lessonSvc   := lesson.NewLessonService(lessonAdapter)
	speakingSvc := speaking.NewSpeakingService(speakingStore, speaking.NewWaveformScorer())
	writingSvc  := writing.NewWritingService(writingStore, aiReviewer)
	userSvc     := user.NewUserService(userAdapter, jwtSecret, mailer, appBaseURL)
	summarySvc  := summary.NewSummaryService(sessionAdapter)
	noteSvc     := note.NewNoteService(noteAdapter)

	// ── Handlers ─────────────────────────────────────────────────────────────
	wordH     := word.NewWordHandlerWithNotes(wordSvc, &wordNoteProvider{svc: noteSvc})
	grammarH  := grammar.NewGrammarHandlerWithNotes(grammarSvc, &grammarNoteProvider{svc: noteSvc})
	lessonH   := lesson.NewLessonHandler(lessonSvc)
	speakingH := speaking.NewSpeakingHandler(speakingSvc)
	writingH  := writing.NewWritingHandler(writingSvc)
	userH     := user.NewUserHandler(userSvc)
	summaryH  := summary.NewSummaryHandler(summarySvc)
	noteH     := note.NewNoteHandler(noteSvc)
	reviewH   := review.NewReviewHandler(wordSvc, noteSvc)

	// ── Mux ───────────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Public auth routes (no middleware)
	userH.RegisterPublicRoutes(mux)

	// Protected API routes wrapped in AuthMiddleware
	protectedMux := http.NewServeMux()
	userH.RegisterProtectedRoutes(protectedMux)
	wordH.RegisterRoutes(protectedMux)
	grammarH.RegisterRoutes(protectedMux)
	lessonH.RegisterRoutes(protectedMux)
	speakingH.RegisterRoutes(protectedMux)
	writingH.RegisterRoutes(protectedMux)
	summaryH.RegisterRoutes(protectedMux)
	noteH.RegisterRoutes(protectedMux)
	reviewH.RegisterRoutes(protectedMux)

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
	mux.Handle("/api/v1/notes", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/notes/", user.AuthMiddleware(jwtSecret, protectedMux))
	mux.Handle("/api/v1/review/", user.AuthMiddleware(jwtSecret, protectedMux))

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// SPA fallback: serve static files if they exist, otherwise serve index.html
	// so React Router can handle client-side routes.
	spaFS := http.Dir(templateDir)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		f, err := spaFS.Open(path)
		if err != nil {
			// File not found — serve index.html for client-side routing
			http.ServeFile(w, r, filepath.Join(string(spaFS), "index.html"))
			return
		}
		f.Close()
		http.FileServer(spaFS).ServeHTTP(w, r)
	}))

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

// wordNoteProvider adapts note.NoteService to word.NoteDigestProvider.
type wordNoteProvider struct {
	svc *note.NoteService
}

func (p *wordNoteProvider) ListByReference(userID int64, refType string, refID int64, limit int) ([]word.NoteDigest, error) {
	digests, err := p.svc.ListByReference(userID, refType, refID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]word.NoteDigest, len(digests))
	for i, d := range digests {
		result[i] = word.NoteDigest{ID: d.ID, Title: d.Title, Type: string(d.Type)}
	}
	return result, nil
}

// grammarNoteProvider adapts note.NoteService to grammar.NoteDigestProvider.
type grammarNoteProvider struct {
	svc *note.NoteService
}

func (p *grammarNoteProvider) ListByReference(userID int64, refType string, refID int64, limit int) ([]grammar.NoteDigest, error) {
	digests, err := p.svc.ListByReference(userID, refType, refID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]grammar.NoteDigest, len(digests))
	for i, d := range digests {
		result[i] = grammar.NoteDigest{ID: d.ID, Title: d.Title, Type: string(d.Type)}
	}
	return result, nil
}
