package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

func main() {
	// ── CLI sub-commands ──────────────────────────────────────────────────────
	// If the first argument is a known CLI command, dispatch to the CLI handler
	// and exit without starting the HTTP server.
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		slog.Error("server binary no longer dispatches CLI commands", "arg", os.Args[1])
		os.Exit(2)
	}

	// ── Configuration ─────────────────────────────────────────────────────────
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)
	mailer := newPasswordResetMailer(passwordResetMailerSettings(os.Getenv))
	mux, cleanup, err := buildPostgresServerMux(context.Background(), cfg,
		envOrDefault("STATIC_DIR", "./front/dist/assets"), envOrDefault("TEMPLATE_DIR", "./front/dist"),
		mailer, envOrDefault("APP_BASE_URL", "http://localhost:35173"))
	if err != nil {
		slog.Error("failed to build PostgreSQL server", "err", err)
		os.Exit(1)
	}
	defer cleanup()
	slog.Info("server starting", "addr", cfg.ListenAddr, "database", "postgres")
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		slog.Error("server error", "err", err)
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

type mailerSettings struct {
	provider string
	host     string
	port     string
	user     string
	pass     string
	from     string
}

func passwordResetMailerSettings(getenv func(string) string) mailerSettings {
	if apiKey := getenv("RESEND_API_KEY"); apiKey != "" {
		from := envValue(getenv, "RESEND_FROM", "")
		if from == "" {
			from = envValue(getenv, "SMTP_FROM", "noreply@japanese-learning.app")
		}
		return mailerSettings{
			provider: "resend",
			host:     "smtp.resend.com",
			port:     "587",
			user:     "resend",
			pass:     apiKey,
			from:     from,
		}
	}

	if host := getenv("SMTP_HOST"); host != "" {
		return mailerSettings{
			provider: "smtp",
			host:     host,
			port:     envValue(getenv, "SMTP_PORT", "587"),
			user:     getenv("SMTP_USER"),
			pass:     getenv("SMTP_PASS"),
			from:     envValue(getenv, "SMTP_FROM", "noreply@japanese-learning.app"),
		}
	}

	return mailerSettings{provider: "stub"}
}

func envValue(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

func newPasswordResetMailer(settings mailerSettings) user.Mailer {
	switch settings.provider {
	case "resend":
		slog.Info("using Resend SMTPMailer for password reset", "smtp_host", settings.host, "from", settings.from)
		return user.NewSMTPMailer(settings.host, settings.port, settings.user, settings.pass, settings.from)
	case "smtp":
		slog.Info("using SMTPMailer for password reset", "smtp_host", settings.host, "from", settings.from)
		return user.NewSMTPMailer(settings.host, settings.port, settings.user, settings.pass, settings.from)
	default:
		slog.Warn("mail provider not set — using StubMailer (reset URLs logged only)")
		return &user.StubMailer{}
	}
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

// wordGoalProvider adapts data.UserStore to word.DailyGoalProvider.
// dailyGoalStore is the minimal contract needed to read a user's daily goals.
// The PostgreSQL user store supplies this interface.
type dailyGoalStore interface {
	GetDailyGoals(userID int64) (map[string]int, error)
}

type wordGoalProvider struct {
	store dailyGoalStore
}

func (p *wordGoalProvider) GetWordDailyGoal(userID int64) int {
	goals, err := p.store.GetDailyGoals(userID)
	if err != nil {
		return 20
	}
	return goals["word"]
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
