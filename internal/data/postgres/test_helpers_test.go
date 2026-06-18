package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"japanese-learning-app/internal/store"
)

func newPostgresRuntimeForTest(t *testing.T) *store.StoreRuntime {
	t.Helper()

	db := newMigratedPostgresTestDB(t)
	adapter := Adapter{}

	rt, err := adapter.NewStoreRuntime(db, store.StoreDeps{
		Clock:       fixedClock{now: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)},
		Logger:      testLogger{},
		AppTimezone: time.UTC,
	})
	if err != nil {
		t.Fatalf("NewStoreRuntime() error = %v", err)
	}

	return rt
}

func newPostgresUserStoreForTest(t *testing.T) *UserStore {
	t.Helper()

	return newPostgresUserStoreForDB(newMigratedPostgresTestDB(t))
}

func newPostgresWordStoreForTest(t *testing.T) *WordStore {
	t.Helper()

	return newPostgresWordStoreForDB(newMigratedPostgresTestDB(t))
}

func newPostgresGrammarStoreForTest(t *testing.T) *GrammarStore {
	t.Helper()

	return newPostgresGrammarStoreForDB(newMigratedPostgresTestDB(t))
}

func newPostgresLessonStoreForTest(t *testing.T) *LessonStore {
	t.Helper()

	return newPostgresLessonStoreForDB(newMigratedPostgresTestDB(t))
}

func newPostgresUserStoreForDB(db queryer) *UserStore {
	return NewUserStore(db, store.StoreDeps{
		Clock:       fixedClock{now: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)},
		Logger:      testLogger{},
		AppTimezone: time.UTC,
	})
}

func newPostgresWordStoreForDB(db queryer) *WordStore {
	return NewWordStore(db, store.StoreDeps{
		Clock:       fixedClock{now: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)},
		Logger:      testLogger{},
		AppTimezone: time.UTC,
	})
}

func newPostgresGrammarStoreForDB(db queryer) *GrammarStore {
	return NewGrammarStore(db, store.StoreDeps{
		Clock:       fixedClock{now: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)},
		Logger:      testLogger{},
		AppTimezone: time.UTC,
	})
}

func newPostgresLessonStoreForDB(db queryer) *LessonStore {
	return NewLessonStore(db)
}

func openPostgresTestDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}

	db, err := (Adapter{}).Open(context.Background(), store.DatabaseConfig{
		DatabaseURL:  url,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	return db
}

func newMigratedPostgresTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db := openPostgresTestDB(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx := context.Background()
	adapter := Adapter{}
	resetPostgresTestSchema(t, db)
	if err := adapter.RunMigrations(ctx, db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	return db
}

func resetPostgresTestSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
`); err != nil {
		t.Fatalf("resetPostgresTestSchema() error = %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type testLogger struct{}

func (testLogger) Debug(string, ...any) {}
func (testLogger) Info(string, ...any)  {}
func (testLogger) Warn(string, ...any)  {}
func (testLogger) Error(string, ...any) {}
