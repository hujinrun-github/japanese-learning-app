package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"japanese-learning-app/internal/config"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/shadowing"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
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

	userSvc := user.NewUserService(runtime.App.Users, cfg.JWTSecret, mailer, appBaseURL)
	wordSvc := word.NewWordService(runtime.App.Words)
	grammarSvc := grammar.NewGrammarService(runtime.App.Grammar)
	lessonSvc := lesson.NewLessonService(runtime.App.Lessons)
	shadowingSvc := shadowing.NewService(runtime.App.Lessons, pgdata.NewShadowingStore(db))

	userH := user.NewUserHandler(userSvc)
	wordH := word.NewWordHandler(wordSvc)
	grammarH := grammar.NewGrammarHandler(grammarSvc)
	lessonH := lesson.NewLessonHandler(lessonSvc)
	shadowingH := shadowing.NewHandler(shadowingSvc)

	mux := http.NewServeMux()
	userH.RegisterPublicRoutes(mux)

	protectedMux := http.NewServeMux()
	userH.RegisterProtectedRoutes(protectedMux)
	wordH.RegisterRoutes(protectedMux)
	grammarH.RegisterRoutes(protectedMux)
	lessonH.RegisterRoutes(protectedMux)
	shadowingH.RegisterRoutes(protectedMux)

	authenticated := user.AuthMiddleware(cfg.JWTSecret, protectedMux)
	mux.Handle("/api/v1/words/", authenticated)
	mux.Handle("/api/v1/grammar", authenticated)
	mux.Handle("/api/v1/grammar/", authenticated)
	mux.Handle("/api/v1/lessons", authenticated)
	mux.Handle("/api/v1/lessons/", authenticated)
	mux.Handle("/api/v1/users/", authenticated)

	registerVideoStreamRoutes(mux, db, cfg)
	registerStaticHandlers(mux, staticDir, templateDir)
	return mux, cleanup, nil
}

func registerStaticHandlers(mux *http.ServeMux, staticDir, templateDir string) {
	mux.Handle("/audio/", http.StripPrefix("/audio/", http.FileServer(http.Dir("./data/audio"))))
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
