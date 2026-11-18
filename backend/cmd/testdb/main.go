// testdb prepares a dedicated PostgreSQL database for integration tests.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"japanese-learning-app/internal/testdb"
)

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare() error {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		return fmt.Errorf("set DATABASE_URL_TEST to a dedicated PostgreSQL database ending in _test")
	}
	if err := testdb.ValidateURLs(dsn, os.Getenv("DATABASE_URL")); err != nil {
		return err
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse test connection: %w", err)
	}
	name := cfg.Database
	cfg.Database = "postgres"
	cfg.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var exists int
	err = db.QueryRowContext(ctx, "SELECT 1 FROM pg_database WHERE datname=$1", name).Scan(&exists)
	if err == sql.ErrNoRows {
		if _, err := db.ExecContext(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			return fmt.Errorf("create test database %q: %w", name, err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect test database: %w", err)
	}
	fmt.Println("PostgreSQL test database ready:", name)
	return nil
}
