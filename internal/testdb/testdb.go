// Package testdb opens isolated schemas inside a dedicated PostgreSQL test
// database. It never drops or resets the public schema.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// ValidateURLs requires an explicitly named test database, separate from the
// application's database, including when connection URL options differ.
func ValidateURLs(testURL, appURL string) error {
	cfg, err := pgx.ParseConfig(testURL)
	if err != nil {
		return fmt.Errorf("parse test database URL: %w", err)
	}
	if !strings.HasSuffix(cfg.Database, "_test") && !strings.Contains(cfg.Database, "_test_") {
		return fmt.Errorf("test database name must end in _test or contain _test_: %q", cfg.Database)
	}
	if appURL != "" {
		app, err := pgx.ParseConfig(appURL)
		if err != nil {
			return fmt.Errorf("parse application database URL: %w", err)
		}
		if cfg.Database == app.Database {
			return fmt.Errorf("DATABASE_URL_TEST must name a different database from DATABASE_URL")
		}
	}
	return nil
}

// Open creates one schema per test, so packages and parallel tests can share
// DATABASE_URL_TEST safely. Pass nil to test the migrations themselves.
func Open(t *testing.T, migrate func(context.Context, *sql.DB) error) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		if os.Getenv("REQUIRE_TEST_DATABASE") == "1" {
			t.Fatal("DATABASE_URL_TEST is required")
		}
		t.Skip("DATABASE_URL_TEST is required for PostgreSQL integration tests")
	}
	if err := ValidateURLs(dsn, os.Getenv("DATABASE_URL")); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnectTimeout = 5 * time.Second
	control := stdlib.OpenDB(*cfg)
	control.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := control.PingContext(ctx); err != nil {
		control.Close()
		t.Fatalf("connect test PostgreSQL: %v", err)
	}
	// Extensions live in public, independently of the disposable test schema.
	if _, err := control.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('japanese-learning-app:test-extension'))`); err != nil {
		control.Close()
		t.Fatal(err)
	}
	_, extensionErr := control.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public`)
	_, unlockErr := control.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext('japanese-learning-app:test-extension'))`)
	if extensionErr != nil || unlockErr != nil {
		control.Close()
		t.Fatalf("prepare test extension: %v; unlock: %v", extensionErr, unlockErr)
	}
	name := fmt.Sprintf("test_%x", rand.Text())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := control.ExecContext(ctx, "CREATE SCHEMA "+identifier); err != nil {
		control.Close()
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = identifier + ",public"
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := control.ExecContext(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop isolated test schema: %v", err)
		}
		if err := control.Close(); err != nil {
			t.Errorf("close test control connection: %v", err)
		}
	})
	if migrate != nil {
		if err := migrate(ctx, db); err != nil {
			t.Fatalf("migrate test schema: %v", err)
		}
	}
	return db
}

// URL preserves the test's schema when a server opens its own connection pool.
func URL(t *testing.T, db *sql.DB) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("DATABASE_URL_TEST"))
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}
