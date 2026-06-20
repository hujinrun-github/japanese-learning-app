package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	pgschema "japanese-learning-app/internal/data/migrations/postgres"
)

const migrationLockName = "japanese-learning-app:migrations"

func (a Adapter) RunMigrations(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("postgres.RunMigrations conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext($1))`, migrationLockName); err != nil {
		return fmt.Errorf("postgres.RunMigrations lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, migrationLockName)
	}()

	return runEmbeddedMigrations(ctx, conn)
}

func runEmbeddedMigrations(ctx context.Context, conn *sql.Conn) error {
	if err := ensureSchemaMigrationsTable(ctx, conn); err != nil {
		return err
	}

	names, err := embeddedMigrationNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		filename := path.Join("schema", name)
		content, err := pgschema.SchemaFS.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("postgres.runEmbeddedMigrations read %s: %w", filename, err)
		}

		checksum := canonicalMigrationChecksum(content)
		if err := applyMigrationFile(ctx, conn, name, checksum, string(content)); err != nil {
			return err
		}
	}

	return nil
}

func ensureSchemaMigrationsTable(ctx context.Context, conn *sql.Conn) error {
	const stmt = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename    TEXT PRIMARY KEY,
	checksum    TEXT NOT NULL,
	applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`

	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("postgres.ensureSchemaMigrationsTable: %w", err)
	}
	return nil
}

func embeddedMigrationNames() ([]string, error) {
	entries, err := fs.ReadDir(pgschema.SchemaFS, "schema")
	if err != nil {
		return nil, fmt.Errorf("postgres.embeddedMigrationNames: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func applyMigrationFile(ctx context.Context, conn *sql.Conn, name, checksum, sqlText string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres.applyMigrationFile begin %s: %w", name, err)
	}

	var existingChecksum string
	row := tx.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE filename = $1`, name)
	switch err := row.Scan(&existingChecksum); {
	case err == nil:
		if !migrationChecksumMatches(existingChecksum, []byte(sqlText)) {
			_ = tx.Rollback()
			return fmt.Errorf("postgres.applyMigrationFile checksum mismatch for %s: stored=%s current=%s", name, existingChecksum, checksum)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("postgres.applyMigrationFile commit noop %s: %w", name, err)
		}
		return nil
	case errors.Is(err, sql.ErrNoRows):
		// Continue and apply migration.
	case err != nil:
		_ = tx.Rollback()
		return fmt.Errorf("postgres.applyMigrationFile read %s: %w", name, err)
	}

	if strings.TrimSpace(sqlText) != "" {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("postgres.applyMigrationFile exec %s: %w", name, err)
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)`,
		name,
		checksum,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("postgres.applyMigrationFile insert %s: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres.applyMigrationFile commit %s: %w", name, err)
	}

	return nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func canonicalMigrationChecksum(content []byte) string {
	return sha256Hex(normalizeMigrationLineEndings(content))
}

func migrationChecksumMatches(storedChecksum string, content []byte) bool {
	for _, checksum := range migrationChecksumVariants(content) {
		if storedChecksum == checksum {
			return true
		}
	}
	return false
}

func migrationChecksumVariants(content []byte) []string {
	normalized := normalizeMigrationLineEndings(content)
	crlf := []byte(strings.ReplaceAll(string(normalized), "\n", "\r\n"))
	return []string{
		canonicalMigrationChecksum(content),
		sha256Hex(content),
		sha256Hex(crlf),
	}
}

func normalizeMigrationLineEndings(content []byte) []byte {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return []byte(normalized)
}
