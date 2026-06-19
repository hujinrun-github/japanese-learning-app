package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"japanese-learning-app/internal/cli"
	"japanese-learning-app/internal/config"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/store"
)

func TestBuildPostgresServerMuxServesShadowingSession(t *testing.T) {
	db := newMigratedPostgresServerDB(t)
	lessonPath := writeServerTempJSON(t, []map[string]any{
		{
			"title":             "PG Server Shadowing",
			"jlpt_level":        "N5",
			"tags":              []string{"shadowing"},
			"audio_url":         "/audio/lessons/server.wav",
			"word_ids":          []int64{},
			"shadowing_enabled": true,
			"shadowing_version": 1,
			"shadowing_config":  map[string]any{"media_type": "audio"},
			"sentences": []map[string]any{
				{
					"index":    0,
					"tokens":   []map[string]string{{"surface": "test", "reading": ""}},
					"chinese":  "server sentence",
					"start_ms": 0,
					"end_ms":   3000,
				},
			},
		},
	})
	if _, err := cli.ImportLessonsToPostgresFromFile(db, lessonPath); err != nil {
		t.Fatalf("ImportLessonsToPostgresFromFile error: %v", err)
	}

	var lessonID int64
	if err := db.QueryRow(`SELECT id FROM lessons WHERE title = $1`, "PG Server Shadowing").Scan(&lessonID); err != nil {
		t.Fatalf("query imported lesson id: %v", err)
	}

	templateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(templateDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	cfg := &config.Config{
		DatabaseURL:       os.Getenv("DATABASE_URL_TEST"),
		DBMaxOpenConns:    4,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: time.Minute,
		AppTimezone:       time.UTC,
		JWTSecret:         "test-secret",
	}
	mux, cleanup, err := buildPostgresServerMux(context.Background(), cfg, t.TempDir(), templateDir, &user.StubMailer{}, "http://localhost:35173")
	if err != nil {
		t.Fatalf("buildPostgresServerMux error: %v", err)
	}
	defer cleanup()

	registerBody := []byte(`{"name":"PG User","email":"pg-server@example.com","password":"password123"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(registerBody))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}

	loginBody := []byte(`{"email":"pg-server@example.com","password":"password123"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Data.Token == "" {
		t.Fatal("login token is empty")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/lessons/"+strconv.FormatInt(lessonID, 10)+"/shadowing", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shadowing status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var sessionResp struct {
		Data struct {
			Lesson struct {
				ID       int64  `json:"id"`
				AudioURL string `json:"audio_url"`
			} `json:"lesson"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("decode shadowing response: %v", err)
	}
	if sessionResp.Data.Lesson.ID != lessonID {
		t.Fatalf("session lesson id = %d, want %d", sessionResp.Data.Lesson.ID, lessonID)
	}
	if sessionResp.Data.Lesson.AudioURL != "/audio/lessons/server.wav" {
		t.Fatalf("session audio_url = %q, want imported media URL", sessionResp.Data.Lesson.AudioURL)
	}
}

func newMigratedPostgresServerDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}

	ctx := context.Background()
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{DatabaseURL: url, MaxOpenConns: 4, MaxIdleConns: 2})
	if err != nil {
		t.Fatalf("open postgres test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
`); err != nil {
		t.Fatalf("reset postgres test schema: %v", err)
	}
	if err := adapter.RunMigrations(ctx, db); err != nil {
		t.Fatalf("run postgres migrations: %v", err)
	}

	return db
}

func writeServerTempJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal temp JSON: %v", err)
	}
	path := filepath.Join(t.TempDir(), "lessons.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write temp JSON: %v", err)
	}
	return path
}
