package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/user"
)

func TestPostgresLessonCatalog(t *testing.T) {
	db := newMigratedPostgresServerDB(t)
	if _, err := db.Exec(`INSERT INTO lessons (title, jlpt_level, tags)
		SELECT '日语阅读素材 ' || n, 'N2', ARRAY['阅读'] FROM generate_series(1, 522) AS n;
		INSERT INTO lessons (title, jlpt_level) VALUES ('私の一日', 'N5'), ('お互いさま', 'N3')`); err != nil {
		t.Fatalf("seed lesson catalog: %v", err)
	}
	cfg := &config.Config{
		DatabaseURL: os.Getenv("DATABASE_URL_TEST"), DBMaxOpenConns: 4, DBMaxIdleConns: 2,
		DBConnMaxLifetime: time.Minute, AppTimezone: time.UTC, JWTSecret: "lesson-catalog-test",
	}
	mux, cleanup, err := buildPostgresServerMux(context.Background(), cfg, t.TempDir(), t.TempDir(), &user.StubMailer{}, "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	token, _, err := user.SignToken(1, cfg.JWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, query     string
		total, returned int
		firstTitle      string
	}{
		{"all levels", "", 524, 20, "日语阅读素材 1"},
		{"last N2 page", "?level=N2&page=27", 522, 2, "日语阅读素材 521"},
		{"tag search", "?level=N2&q=%E9%98%85%E8%AF%BB&size=5", 522, 5, "日语阅读素材 1"},
		{"title search", "?q=%E3%81%8A%E4%BA%92%E3%81%84%E3%81%95%E3%81%BE", 1, 1, "お互いさま"},
		{"empty level", "?level=N1", 0, 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/lessons/catalog"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var response struct{ Data lesson.CatalogPage }
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			result := response.Data
			if result.Total != tt.total || len(result.Items) != tt.returned {
				t.Fatalf("total, returned = %d, %d; want %d, %d", result.Total, len(result.Items), tt.total, tt.returned)
			}
			if tt.returned > 0 && result.Items[0].Title != tt.firstTitle {
				t.Fatalf("first title = %q, want %q", result.Items[0].Title, tt.firstTitle)
			}
		})
	}
}
