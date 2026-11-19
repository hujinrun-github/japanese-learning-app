package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/data"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/errata"
	"japanese-learning-app/internal/module/flowspace"
	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/listening"
	"japanese-learning-app/internal/module/mistake"
	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/module/notesync"
	"japanese-learning-app/internal/module/plan"
	"japanese-learning-app/internal/module/review"
	"japanese-learning-app/internal/module/shadowing"
	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/module/studyplan"
	"japanese-learning-app/internal/module/summary"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
	"japanese-learning-app/internal/module/writing"
	"japanese-learning-app/internal/store"
)

func buildPostgresServerMux(ctx context.Context, cfg *config.Config, staticDir, templateDir string, mailer user.Mailer, appBaseURL string) (*http.ServeMux, func(), error) {
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{
		DatabaseURL:     cfg.DatabaseURL,
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
		AppTimezone:     cfg.AppTimezone,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("buildPostgresServerMux open database: %w", err)
	}
	cleanup := func() { _ = db.Close() }

	if err := adapter.RunMigrations(ctx, db); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("buildPostgresServerMux run migrations: %w", err)
	}

	runtime, err := adapter.NewStoreRuntime(db, store.StoreDeps{AppTimezone: cfg.AppTimezone})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("buildPostgresServerMux store runtime: %w", err)
	}
	flowspaceOptions, err := flowspace.OptionsFromEnv()
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("buildPostgresServerMux FlowSpace configuration: %w", err)
	}
	flowspaceService, err := flowspace.NewService(db, flowspaceOptions)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("buildPostgresServerMux FlowSpace integration: %w", err)
	}
	workerContext, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); flowspaceService.Run(workerContext) }()
	closeDatabase := cleanup
	cleanup = func() { stopWorker(); <-workerDone; closeDatabase() }

	// ── AI reviewers ─────────────────────────────────────────────────────────
	aiAPIKey := cfg.AIAPIKey
	aiEndpoint := cfg.AIAPIEndpoint

	var aiReviewer writing.AIReviewer
	if aiAPIKey != "" {
		slog.Info("using ClaudeClient for AI writing review", "endpoint", aiEndpoint)
		aiReviewer = writing.NewClaudeClient(aiAPIKey, aiEndpoint, time.Duration(cfg.AITimeoutSec)*time.Second)
	} else {
		slog.Warn("AI_API_KEY not set — topic writing will be saved without AI feedback")
	}

	var translationReviewer translation.TranslationReviewer
	if aiAPIKey != "" {
		translationReviewer = translation.NewClaudeTranslationReviewer(aiAPIKey, aiEndpoint, time.Duration(cfg.AITimeoutSec)*time.Second)
	} else {
		slog.Warn("AI_API_KEY not set — using StubReviewer for translation (no real AI feedback)")
		translationReviewer = &translation.StubReviewer{}
	}

	userSvc := user.NewUserService(runtime.App.Users, cfg.JWTSecret, time.Duration(cfg.JWTExpireHours)*time.Hour, mailer, appBaseURL)
	wordSvc := word.NewWordService(runtime.App.Words)
	grammarSvc := grammar.NewGrammarService(runtime.App.Grammar)
	lessonSvc := lesson.NewLessonService(runtime.App.Lessons)
	speakingSvc := speaking.NewSpeakingService(runtime.App.Speaking, speaking.NewWaveformScorer())
	writingSvc := writing.NewWritingService(runtime.App.Writing, aiReviewer)
	translationSvc := translation.NewTranslationService(runtime.App.Translation, translationReviewer)
	noteSvc := note.NewNoteService(runtime.App.Notes)
	noteSyncSvc := notesync.NewService(runtime.App.NoteSync, runtime.App.Notes, notesync.NewEnvClientFactory(envOrDefault("NOTION_API_BASE_URL", "")))
	summarySvc := summary.NewSummaryService(runtime.App.Sessions)
	shadowingSvc := shadowing.NewService(runtime.App.Lessons, pgdata.NewShadowingStore(db))
	shadowingRefAudio := shadowing.NewReferenceAudioReader(cfg.AudioStorePath, newLessonAudioObjectLoader(db, cfg))
	evaluator := shadowing.NewEvaluator(2)
	shadowingEvaluationSvc := shadowing.NewEvaluationService(runtime.App.Lessons, shadowingRefAudio, evaluator)
	planSvc := plan.NewPlanService(userSvc, cfg.AppTimezone)

	userH := user.NewUserHandler(userSvc)
	wordH := word.NewWordHandlerWithNotes(wordSvc, &wordNoteProvider{svc: noteSvc})
	wordH.SetDailyGoalProvider(&wordGoalProvider{store: runtime.App.Users})
	wordH.SetStreakRecorder(userSvc)
	grammarH := grammar.NewGrammarHandlerWithNotes(grammarSvc, &grammarNoteProvider{svc: noteSvc})
	grammarH.SetStreakRecorder(userSvc)
	lessonH := lesson.NewLessonHandler(lessonSvc)
	listeningH := listening.NewHandler(pgdata.NewListeningStore(db, func(ctx context.Context, tx *sql.Tx, uid int64, now time.Time) error {
		return studyplan.CompleteListeningTask(ctx, tx, uid, now)
	}))
	speakingH := speaking.NewSpeakingHandler(speakingSvc)
	speakingH.SetStreakRecorder(userSvc)
	speakingEvaluationH := speaking.NewEvaluationHandler(speakingSvc, cfg.AudioStorePath, newLessonAudioObjectLoader(db, cfg), evaluator)
	writingH := writing.NewWritingHandler(writingSvc)
	writingH.SetStreakRecorder(userSvc)
	translationH := translation.NewTranslationHandler(translationSvc)
	noteH := note.NewNoteHandler(noteSvc)
	noteSyncH := notesync.NewHandler(noteSyncSvc)
	summaryH := summary.NewSummaryHandler(summarySvc)
	reviewH := review.NewReviewHandler(wordSvc, noteSvc)
	shadowingH := shadowing.NewHandler(shadowingSvc)
	shadowingEvaluationH := shadowing.NewEvaluationHandler(shadowingEvaluationSvc)
	planH := plan.NewPlanHandler(planSvc)

	mux := http.NewServeMux()
	registerHealthRoute(mux)
	userH.RegisterPublicRoutes(mux)

	protectedMux := http.NewServeMux()
	studyplan.NewHandler(db).RegisterRoutes(protectedMux, mux)
	flowspace.NewHandler(flowspaceService).RegisterRoutes(protectedMux)
	userH.RegisterProtectedRoutes(protectedMux)
	wordH.RegisterRoutes(protectedMux)
	grammarH.RegisterRoutes(protectedMux)
	lessonH.RegisterRoutes(protectedMux)
	listeningH.RegisterRoutes(protectedMux)
	lesson.RegisterCatalogRoute(protectedMux, data.NewLessonCatalog(db))
	lesson.NewAnnotationHandler(db, lessonSvc, runtime.Admin.Words, runtime.Admin.Grammar).RegisterRoutes(protectedMux)
	speakingH.RegisterRoutes(protectedMux)
	speakingEvaluationH.RegisterRoutes(protectedMux)
	writingH.RegisterRoutes(protectedMux)
	translationH.RegisterRoutes(protectedMux)
	noteH.RegisterRoutes(protectedMux)
	noteSyncH.RegisterRoutes(protectedMux)
	summaryH.RegisterRoutes(protectedMux)
	reviewH.RegisterRoutes(protectedMux)
	shadowingH.RegisterRoutes(protectedMux)
	shadowingEvaluationH.RegisterRoutes(protectedMux)
	planH.RegisterRoutes(protectedMux)
	errata.RegisterRoutes(protectedMux, errata.NewStore(db))
	mistake.RegisterRoutes(protectedMux, mistake.NewStore(db))

	authenticated := user.AuthMiddleware(cfg.JWTSecret, protectedMux)
	mux.Handle("/api/v1/study-plan", authenticated)
	mux.Handle("/api/v1/study-plan/", authenticated)
	mux.Handle("/api/v1/integrations/", authenticated)
	mux.Handle("/api/v1/words/", authenticated)
	mux.Handle("/api/v1/grammar", authenticated)
	mux.Handle("/api/v1/grammar/", authenticated)
	mux.Handle("/api/v1/lessons", authenticated)
	mux.Handle("/api/v1/lessons/", authenticated)
	mux.Handle("/api/v1/listening/", authenticated)
	mux.Handle("/api/v1/speaking/", authenticated)
	mux.Handle("/api/v1/writing/", authenticated)
	mux.Handle("/api/v1/translation/", authenticated)
	mux.Handle("/api/v1/notes", authenticated)
	mux.Handle("/api/v1/notes/", authenticated)
	mux.Handle("/api/v1/note-sync/", authenticated)
	mux.Handle("/api/v1/summary", authenticated)
	mux.Handle("/api/v1/summary/", authenticated)
	mux.Handle("/api/v1/review/", authenticated)
	mux.Handle("/api/v1/users/", authenticated)
	mux.Handle("/api/v1/daily-plan", authenticated)
	mux.Handle("/api/v1/errata", authenticated)
	mux.Handle("/api/v1/errata/", authenticated)
	mux.Handle("/api/v1/mistakes", authenticated)
	mux.Handle("/api/v1/mistakes/", authenticated)

	registerVideoStreamRoutes(mux, db, cfg)
	registerAudioStreamRoutes(mux, db, cfg)
	registerStaticHandlers(mux, staticDir, templateDir)
	return mux, cleanup, nil
}

func registerStaticHandlers(mux *http.ServeMux, staticDir, templateDir string) {
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	spaFS := http.Dir(templateDir)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		f, err := spaFS.Open(path)
		if err != nil {
			http.ServeFile(w, r, filepath.Join(string(spaFS), "index.html"))
			return
		}
		f.Close()
		http.FileServer(spaFS).ServeHTTP(w, r)
	}))
}
