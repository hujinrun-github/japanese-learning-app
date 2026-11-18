package postgres

import (
	"bytes"
	"context"
	"testing"

	"japanese-learning-app/internal/store"
)

func TestAdapterContracts(t *testing.T) {
	var _ store.RelationalAdapter = Adapter{}
	var _ store.MigrationCapableAdapter = Adapter{}
}

func TestRunMigrationsTwice(t *testing.T) {
	db := openPostgresTestDB(t)
	ctx := context.Background()

	if err := (Adapter{}).RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := (Adapter{}).RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationChecksumMatchesLineEndingVariants(t *testing.T) {
	lfContent := []byte("CREATE TABLE sample (\n    id BIGINT PRIMARY KEY\n);\n")
	crlfContent := bytes.ReplaceAll(lfContent, []byte("\n"), []byte("\r\n"))

	lfChecksum := sha256Hex(lfContent)
	crlfChecksum := sha256Hex(crlfContent)
	if !migrationChecksumMatches(lfChecksum, crlfContent) {
		t.Fatal("LF checksum should match CRLF-only content changes")
	}
	if !migrationChecksumMatches(crlfChecksum, lfContent) {
		t.Fatal("CRLF checksum should match LF-only content changes")
	}

	changedContent := []byte("CREATE TABLE sample (\n    id TEXT PRIMARY KEY\n);\n")
	if migrationChecksumMatches(lfChecksum, changedContent) {
		t.Fatal("checksum should reject semantic SQL content changes")
	}
}

func TestCanonicalMigrationChecksumUsesLF(t *testing.T) {
	lfContent := []byte("SELECT 1;\n")
	crlfContent := bytes.ReplaceAll(lfContent, []byte("\n"), []byte("\r\n"))

	if got, want := canonicalMigrationChecksum(crlfContent), sha256Hex(lfContent); got != want {
		t.Fatalf("canonical checksum = %s, want LF checksum %s", got, want)
	}
}
