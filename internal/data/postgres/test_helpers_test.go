package postgres

import (
	"database/sql"
	"testing"
	"time"

	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
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

func openPostgresTestDB(t *testing.T) *sql.DB { return testdb.Open(t, nil) }
func newMigratedPostgresTestDB(t *testing.T) *sql.DB {
	return testdb.Open(t, (Adapter{}).RunMigrations)
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
