# PostgreSQL Storage Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate backend durable storage from SQLite/local audio files to pluggable PostgreSQL relational storage plus MinIO/S3-compatible audio object storage, with a safe SQLite copy path.

**Architecture:** Runtime code selects a relational adapter through `RELATIONAL_STORE`, with PostgreSQL as the first supported adapter. Runtime services use `internal/store.StoreRuntime`, admin uses `AdminStores`, audio bytes go through `AudioBlobStore` and `AudioService`, and SQLite copy uses a separate `MigrationTarget` only from `backend/cmd/storage-migrate`.

**Tech Stack:** Go 1.24, `database/sql`, `github.com/jackc/pgx/v5/stdlib`, PostgreSQL 16+, MinIO/S3-compatible client, existing `net/http`, Makefile commands, table-driven Go tests.

---

## Reference Spec

- `docs/superpowers/specs/2026-06-16-postgresql-storage-upgrade-design.md`
- `constitution.md`
- `AGENTS.md`

---

## File Structure

Create:

- `internal/store/contracts.go`: app/admin store interfaces, `StoreRuntime`, `AppStores`, `AdminStores`.
- `internal/store/adapter.go`: `RelationalAdapter`, adapter registry, `DatabaseConfig`, `StoreDeps`, `RelationalCapabilities`.
- `internal/store/errors.go`: `ErrNotFound`, `ErrDuplicate`, `ErrConstraint`, `ErrRetryable`, `ErrNestedTransaction`.
- `internal/store/migration.go`: `MigrationCapableAdapter`, `MigrationTarget`, `BulkLoader`, `MigrationTable`, `MigrationColumn`, `RowBatch`, `AudioObjectRow`, manifest DTOs.
- `internal/store/testsuite`: shared adapter contract tests.
- `internal/data/postgres/db.go`: PostgreSQL open/pool/bootstrap.
- `internal/data/postgres/migrations.go`: PostgreSQL schema migration runner with checksum and advisory lock.
- `internal/data/postgres/*_store.go`: PostgreSQL store implementations.
- `internal/data/postgres/audio_store.go`: PostgreSQL `audio_objects` metadata store.
- `internal/data/postgres/migration_target.go`: preserve-ID bulk loader and actual manifest queries.
- `internal/data/migrations/postgres/schema/001_init.sql`: PostgreSQL schema.
- `internal/data/seeds/postgres/001_seed_words.sql`: optional seed data.
- `internal/storage/blob_store.go`: `AudioBlobStore` and object stream/iterator contracts.
- `internal/storage/s3_blob_store.go`: MinIO/S3-compatible implementation.
- `internal/module/audio/model.go`: `AudioObject`, object kind constants, metadata DTOs.
- `internal/module/audio/service.go`: upload, link, URL derivation, cleanup coordination.
- `internal/module/audio/handler.go`: authenticated stream endpoint with Range support.
- `internal/migration/sqlitecopy`: read-only SQLite copy orchestration, remap, manifests.
- `backend/cmd/storage-migrate/main.go`: offline migration binary.
- Optional `backend/cmd/appctl/main.go`: normal maintenance CLI if commands should no longer hang off server.
- `internal/maintenance/audio_reconcile.go`: object reconciliation command implementation if shared outside storage-migrate.

Modify:

- `go.mod`, `go.sum`
- `internal/config/config.go`, `internal/config/config_test.go`
- `backend/cmd/server/main.go`
- `backend/cmd/admin/main.go`
- `internal/module/admin/*.go`
- `internal/cli/*.go`
- existing `internal/data/*_store.go` during migration into `internal/data/postgres`
- existing `internal/data/*_test.go`
- `internal/module/*/service.go`, `internal/module/*/handler.go` where audio/store contracts change
- `front/react/src/api/client.ts`
- `front/react/src/pages/word/WordReviewPage.tsx`
- `front/react/src/pages/grammar/GrammarDetailPage.tsx`
- `front/react/src/pages/speaking/SpeakingPage.tsx`
- `front/react/src/pages/lesson/LessonPage.tsx`
- `front/admin/src/**`
- `front/web/static/js/*.ts`
- `Makefile`
- `README.md`, `docs/admin-panel-guide.md`, `docs/architecture.md`

---

### Task 1: Baseline And Dependency Guardrails

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `Makefile`

- [ ] **Step 1: Capture current baseline**

Run:

```bash
git status --short
make test
make web
```

Expected:

- Record any pre-existing failures before edits.
- Do not fix unrelated dirty files in this task.

- [ ] **Step 2: Add PostgreSQL and object-store dependencies**

Run:

```bash
go get github.com/jackc/pgx/v5/stdlib
go get github.com/minio/minio-go/v7
go mod tidy
```

Expected:

- `go.mod` includes `github.com/jackc/pgx/v5` and `github.com/minio/minio-go/v7`.
- SQLite dependencies remain until the offline migration path is split, then runtime packages stop importing them.

- [ ] **Step 3: Add Makefile targets**

Modify `Makefile` with targets shaped like:

```make
postgres-migrate:
	go run ./backend/cmd/storage-migrate migrate-sqlite-to-relational --help

storage-reconcile:
	go run ./backend/cmd/storage-migrate audio-reconcile --dry-run

seed-postgres:
	go run ./backend/cmd/appctl seed-postgres
```

Expected:

- Server `go run ./backend/cmd/server/` no longer doubles as the CLI command runner after Task 10.

- [ ] **Step 4: Verify dependencies**

Run:

```bash
go test ./... -run '^$'
```

Expected:

- Packages compile or fail only where later tasks intentionally introduce missing implementations.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum Makefile
git commit -m "chore(storage): add postgres and object storage dependencies"
```

---

### Task 2: Neutral Store Contracts And Error Mapping

**Files:**
- Create: `internal/store/contracts.go`
- Create: `internal/store/adapter.go`
- Create: `internal/store/errors.go`
- Create: `internal/store/migration.go`
- Create: `internal/store/testsuite/errors_test.go`
- Create: `internal/store/testsuite/transaction_contract.go`

- [ ] **Step 1: Write error contract tests**

Create `internal/store/testsuite/errors_test.go`:

```go
package testsuite

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/store"
)

func TestSentinelErrors(t *testing.T) {
	cases := []error{
		store.ErrNotFound,
		store.ErrDuplicate,
		store.ErrConstraint,
		store.ErrRetryable,
		store.ErrNestedTransaction,
	}
	for _, err := range cases {
		if !errors.Is(err, err) {
			t.Fatalf("sentinel error does not match itself: %v", err)
		}
	}
}
```

Run:

```bash
go test ./internal/store/testsuite -run TestSentinelErrors -v
```

Expected: FAIL because `internal/store` does not exist.

- [ ] **Step 2: Add sentinel errors**

Create `internal/store/errors.go`:

```go
package store

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrDuplicate         = errors.New("duplicate")
	ErrConstraint        = errors.New("constraint violation")
	ErrRetryable         = errors.New("retryable database error")
	ErrNestedTransaction = errors.New("nested transaction")
)
```

- [ ] **Step 3: Add runtime contracts**

Create `internal/store/contracts.go`:

```go
package store

import "context"

type StoreRuntime struct {
	App   AppStores
	Admin AdminStores

	transact func(context.Context, func(*StoreRuntime) error) error
}

func (r *StoreRuntime) Transact(ctx context.Context, fn func(*StoreRuntime) error) error {
	if r.transact == nil {
		return ErrNestedTransaction
	}
	return r.transact(ctx, fn)
}

type AppStores struct {
	Users        UserStoreInterface
	Words        WordStoreInterface
	Grammar      GrammarStoreInterface
	Lessons      LessonStoreInterface
	Notes        NoteStoreInterface
	Speaking     SpeakingStoreInterface
	Writing      WritingStoreInterface
	Translation  TranslationStoreInterface
	Sessions     SessionStoreInterface
	AudioObjects AudioObjectStoreInterface
}

type AdminStores struct {
	Users       AdminUserStoreInterface
	Words       AdminWordStoreInterface
	Grammar     AdminGrammarStoreInterface
	Lessons     AdminLessonStoreInterface
	Speaking    AdminSpeakingStoreInterface
	Writing     AdminWritingStoreInterface
	Translation AdminTranslationStoreInterface
	Records     AdminRecordStoreInterface
	Imports     AdminImportStoreInterface
}
```

Add small interface declarations in the same file using the method sets consumed by services/admin. Start with empty interfaces only to break package structure, then fill method sets as each store migrates:

```go
type UserStoreInterface interface{}
type WordStoreInterface interface{}
type GrammarStoreInterface interface{}
type LessonStoreInterface interface{}
type NoteStoreInterface interface{}
type SpeakingStoreInterface interface{}
type WritingStoreInterface interface{}
type TranslationStoreInterface interface{}
type SessionStoreInterface interface{}
type AudioObjectStoreInterface interface{}

type AdminUserStoreInterface interface{}
type AdminWordStoreInterface interface{}
type AdminGrammarStoreInterface interface{}
type AdminLessonStoreInterface interface{}
type AdminSpeakingStoreInterface interface{}
type AdminWritingStoreInterface interface{}
type AdminTranslationStoreInterface interface{}
type AdminRecordStoreInterface interface{}
type AdminImportStoreInterface interface{}
```

- [ ] **Step 4: Add adapter contracts**

Create `internal/store/adapter.go`:

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

type RelationalAdapter interface {
	Name() string
	Open(ctx context.Context, cfg DatabaseConfig) (*sql.DB, error)
	RunMigrations(ctx context.Context, db *sql.DB) error
	NewStoreRuntime(db *sql.DB, deps StoreDeps) (*StoreRuntime, error)
	TranslateError(error) error
	Capabilities() RelationalCapabilities
}

type DatabaseConfig struct {
	Store           string
	DatabaseURL     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	AppTimezone     *time.Location
}

type StoreDeps struct {
	Clock       Clock
	Logger      Logger
	AppTimezone *time.Location
}

type Clock interface {
	Now() time.Time
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type RelationalCapabilities struct {
	Transactions         bool
	Migrations           bool
	MigrationLocks       bool
	IdentityOverride     bool
	InsertReturning      bool
	Upsert               bool
	JSONDocuments        bool
	ArrayFields          bool
	TagFiltering         bool
	TextSearch           bool
	TimeWindowFiltering  bool
	AudioMetadataLinkage bool
}
```

- [ ] **Step 5: Add migration contracts**

Create `internal/store/migration.go`:

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

type MigrationCapableAdapter interface {
	RelationalAdapter
	NewMigrationTarget(db *sql.DB, deps MigrationDeps) (MigrationTarget, error)
}

type MigrationDeps struct {
	Clock          Clock
	Logger         Logger
	AppTimezone    *time.Location
	MigrationRunID string
}

type MigrationTarget interface {
	VerifySchemaOnly(ctx context.Context) error
	WithBulkLoad(ctx context.Context, fn func(BulkLoader) error) error
	ResetIdentities(ctx context.Context, tables []MigrationTable) error
	BuildActualManifest(ctx context.Context) (*MigrationManifest, error)
}

type BulkLoader interface {
	InsertPreservingID(ctx context.Context, batch RowBatch) error
	InsertRows(ctx context.Context, batch RowBatch) error
	InsertAudioObject(ctx context.Context, row AudioObjectRow) (int64, error)
	ValidateReferences(ctx context.Context) error
}

type MigrationTable uint16
type MigrationColumn uint16

type RowBatch struct {
	Table   MigrationTable
	Columns []MigrationColumn
	Rows    [][]any
}

type AudioObjectRow struct {
	Bucket        string
	ObjectKey     string
	Kind          string
	Visibility    string
	ContentSHA256 string
	SizeBytes     int64
	MimeType      string
	MetadataJSON  []byte
	OwnerUserID   *int64
}

type MigrationManifest struct {
	RunID      string
	Generated time.Time
	Counts     map[string]int64
	Remaps     map[string]int64
	Errors     []string
}
```

- [ ] **Step 6: Verify contracts compile**

Run:

```bash
go test ./internal/store/... -v -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store
git commit -m "feat(storage): add relational store contracts"
```

---

### Task 3: Configuration And Runtime Composition

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/cmd/admin/main.go`
- Optional create: `backend/cmd/appctl/main.go`

- [ ] **Step 1: Write config tests**

Add table-driven tests in `internal/config/config_test.go` covering:

```go
{
	name: "postgres and minio config",
	env: map[string]string{
		"RELATIONAL_STORE": "postgres",
		"DATABASE_URL": "postgres://u:p@localhost:5432/app?sslmode=disable",
		"MINIO_ENDPOINT": "localhost:9000",
		"MINIO_ACCESS_KEY": "minioadmin",
		"MINIO_SECRET_KEY": "minioadmin",
		"MINIO_BUCKET_AUDIO": "japanese-learning-audio",
		"APP_TIMEZONE": "Asia/Shanghai",
	},
	wantStore: "postgres",
	wantTZ: "Asia/Shanghai",
}
```

Run:

```bash
go test ./internal/config -run TestLoad -v
```

Expected: FAIL until config fields exist.

- [ ] **Step 2: Extend config**

Modify `internal/config/config.go`:

```go
type Config struct {
	ListenAddr string

	RelationalStore string
	DatabaseURL     string
	DBMaxOpenConns  int
	DBMaxIdleConns  int
	DBConnMaxLifetime time.Duration
	AppTimezoneName string
	AppTimezone     *time.Location

	AudioObjectStore string
	MinIOEndpoint    string
	MinIOAccessKey   string
	MinIOSecretKey   string
	MinIOBucketAudio string
	MinIOUseSSL      bool
	MinIOPublicEndpoint string
	MinIOPresignTTLSeconds int

	JWTSecret      string
	JWTExpireHours int
	LogLevel       string
	AIAPIKey       string
	AIAPIEndpoint  string
	AITimeoutSec   int
}
```

Keep `DBPath` and `AudioStorePath` only behind migration-only compatibility flags if a later task needs to read existing data.

- [ ] **Step 3: Stop server from dispatching CLI commands**

Modify `backend/cmd/server/main.go`:

```go
func main() {
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		slog.Error("server binary no longer dispatches CLI commands", "arg", os.Args[1])
		os.Exit(2)
	}

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	_ = cfg
}
```

Then wire adapter/runtime in Task 5 after PostgreSQL adapter exists.

- [ ] **Step 4: Add appctl skeleton for normal CLI**

Create `backend/cmd/appctl/main.go`:

```go
package main

import (
	"os"

	"japanese-learning-app/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
```

`internal/cli` must not import SQLite after Task 11.

- [ ] **Step 5: Verify server no longer imports internal/cli**

Run:

```bash
rg '"japanese-learning-app/internal/cli"' backend/cmd/server
```

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/config backend/cmd/server backend/cmd/appctl
git commit -m "refactor(server): separate runtime from cli commands"
```

---

### Task 4: PostgreSQL Adapter Bootstrap And Migrations

**Files:**
- Create: `internal/data/postgres/db.go`
- Create: `internal/data/postgres/migrations.go`
- Create: `internal/data/postgres/errors.go`
- Create: `internal/data/migrations/postgres/schema/001_init.sql`
- Create: `internal/data/seeds/postgres/001_seed_words.sql`
- Modify: `internal/data/postgres/*_test.go`

- [ ] **Step 1: Write adapter compile test**

Create `internal/data/postgres/adapter_test.go`:

```go
package postgres

import (
	"testing"

	"japanese-learning-app/internal/store"
)

func TestAdapterContracts(t *testing.T) {
	var _ store.RelationalAdapter = Adapter{}
	var _ store.MigrationCapableAdapter = Adapter{}
}
```

Run:

```bash
go test ./internal/data/postgres -run TestAdapterContracts -v
```

Expected: FAIL because `Adapter` is not defined.

- [ ] **Step 2: Implement adapter skeleton**

Create `internal/data/postgres/db.go`:

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"japanese-learning-app/internal/store"
)

type Adapter struct{}

func (Adapter) Name() string { return "postgres" }

func (Adapter) Open(ctx context.Context, cfg store.DatabaseConfig) (*sql.DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("postgres.Open: database URL is required")
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres.Open: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres.Open ping: %w", err)
	}
	return db, nil
}
```

- [ ] **Step 3: Implement capabilities and errors**

Create `internal/data/postgres/errors.go`:

```go
package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgconn"
	"japanese-learning-app/internal/store"
)

func (Adapter) TranslateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return errors.Join(store.ErrDuplicate, err)
		case "23503", "23514", "23502", "23P01":
			return errors.Join(store.ErrConstraint, err)
		case "40001", "40P01", "08000", "08003", "08006", "53300":
			return errors.Join(store.ErrRetryable, err)
		}
	}
	return err
}
```

Add `Capabilities()` in `db.go`:

```go
func (Adapter) Capabilities() store.RelationalCapabilities {
	return store.RelationalCapabilities{
		Transactions: true, Migrations: true, MigrationLocks: true,
		IdentityOverride: true, InsertReturning: true, Upsert: true,
		JSONDocuments: true, ArrayFields: true, TagFiltering: true,
		TextSearch: true, TimeWindowFiltering: true, AudioMetadataLinkage: true,
	}
}
```

- [ ] **Step 4: Add migration runner test**

Create a PostgreSQL integration test that uses `DATABASE_URL_TEST`:

```go
func TestRunMigrationsTwice(t *testing.T) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}
	ctx := context.Background()
	db, err := (Adapter{}).Open(ctx, store.DatabaseConfig{DatabaseURL: url, MaxOpenConns: 2, MaxIdleConns: 1})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := (Adapter{}).RunMigrations(ctx, db); err != nil { t.Fatal(err) }
	if err := (Adapter{}).RunMigrations(ctx, db); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 5: Implement migration tracking and lock**

Create `internal/data/postgres/migrations.go` with:

```go
func (a Adapter) RunMigrations(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("postgres.RunMigrations conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('japanese-learning-app:migrations'))`); err != nil {
		return fmt.Errorf("postgres.RunMigrations lock: %w", err)
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext('japanese-learning-app:migrations'))`)

	return runEmbeddedMigrations(ctx, conn)
}
```

`runEmbeddedMigrations` must:

- create `schema_migrations`;
- compute SHA-256 checksum per file;
- run each migration in one transaction;
- insert `schema_migrations` in the same transaction as the migration SQL;
- fail on checksum mismatch.

- [ ] **Step 6: Create PostgreSQL schema migration**

Create `internal/data/migrations/postgres/schema/001_init.sql` from the approved design. Include:

- `audio_objects`
- content tables and child tables
- `word_bookmarks`
- append-only event tables
- notes and links
- translation snapshots
- `trg_translation_sentences_immutable_prompt`
- CHECK constraints
- natural-key unique constraints
- FK-side indexes
- `pg_trgm` extension and search indexes

- [ ] **Step 7: Verify PostgreSQL adapter**

Run:

```bash
go test ./internal/data/postgres -v -count=1
```

Expected:

- Unit tests pass.
- Integration tests skip when `DATABASE_URL_TEST` is unset.

- [ ] **Step 8: Commit**

```bash
git add internal/data/postgres internal/data/migrations/postgres
git commit -m "feat(storage): add postgres adapter migrations"
```

---

### Task 5: StoreRuntime Transactions And PostgreSQL Store Skeletons

**Files:**
- Modify: `internal/data/postgres/db.go`
- Create: `internal/data/postgres/tx.go`
- Move/modify: existing `internal/data/*_store.go` into `internal/data/postgres/*_store.go`
- Create/modify: `internal/store/testsuite/transaction_test.go`

- [ ] **Step 1: Write transaction contract tests**

Create tests that call `runtime.Transact` and verify:

```go
func TestTransactRollbackOnError(t *testing.T) {
	rt := newPostgresRuntimeForTest(t)
	err := rt.Transact(context.Background(), func(tx *store.StoreRuntime) error {
		_, err := tx.App.Users.Create("rollback@example.com", "hash", "N5")
		if err != nil { return err }
		return errors.New("force rollback")
	})
	if err == nil { t.Fatal("expected error") }
	if _, err := rt.App.Users.GetByEmail("rollback@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected rollback not found, got %v", err)
	}
}
```

Expected: FAIL until stores and transaction runtime exist.

- [ ] **Step 2: Implement transaction runtime**

Create `internal/data/postgres/tx.go`:

```go
func newRuntime(db queryer, deps store.StoreDeps, transact func(context.Context, func(*store.StoreRuntime) error) error) *store.StoreRuntime {
	return &store.StoreRuntime{
		App: store.AppStores{
			Users: NewUserStore(db),
			Words: NewWordStore(db),
		},
		Admin: store.AdminStores{
			Users: NewAdminUserStore(db),
			Words: NewAdminWordStore(db),
		},
		transact: transact,
	}
}
```

Define `queryer` with `ExecContext`, `QueryContext`, and `QueryRowContext`. Fill every store as it is migrated.

- [ ] **Step 3: Implement `NewStoreRuntime`**

Add:

```go
func (a Adapter) NewStoreRuntime(db *sql.DB, deps store.StoreDeps) (*store.StoreRuntime, error) {
	var rt *store.StoreRuntime
	rt = newRuntime(db, deps, func(ctx context.Context, fn func(*store.StoreRuntime) error) (err error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return a.TranslateError(err)
		}
		txRuntime := newRuntime(tx, deps, func(context.Context, func(*store.StoreRuntime) error) error {
			return store.ErrNestedTransaction
		})
		defer func() {
			if p := recover(); p != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					deps.Logger.Error("transaction rollback after panic failed", "err", rbErr)
				}
				panic(p)
			}
		}()
		if err := fn(txRuntime); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return errors.Join(err, rbErr)
			}
			return err
		}
		return a.TranslateError(tx.Commit())
	})
	return rt, nil
}
```

- [ ] **Step 4: Migrate stores one group at a time**

Order:

1. users and password reset tokens
2. words, word examples, bookmarks, records, review events
3. grammar, quiz questions, records, attempts
4. lessons and lesson words
5. notes, note links, note review events
6. speaking materials and records
7. writing questions and records
8. translation sources, sentences, records
9. study sessions and summaries
10. audio object metadata

For each group:

- copy the current SQLite store test to PostgreSQL package;
- update SQL placeholders to `$1`;
- replace `LastInsertId` with `RETURNING id`;
- replace JSON string scans with arrays/child-table scans;
- translate errors through `Adapter.TranslateError`;
- run the package test before moving to the next group.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/data/postgres -v -count=1
go test ./internal/store/testsuite -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/data/postgres internal/store/testsuite
git commit -m "feat(storage): implement postgres store runtime"
```

---

### Task 6: MinIO/S3 Audio Object Storage

**Files:**
- Create: `internal/storage/blob_store.go`
- Create: `internal/storage/s3_blob_store.go`
- Create: `internal/module/audio/model.go`
- Create: `internal/module/audio/service.go`
- Create: `internal/module/audio/handler.go`
- Create: `internal/data/postgres/audio_store.go`
- Test: `internal/storage/*_test.go`
- Test: `internal/module/audio/*_test.go`

- [ ] **Step 1: Write object store contract tests**

Create tests for:

- `PutIfAbsent` creates object;
- second `PutIfAbsent` with same hash/size/type returns `Reused`;
- second `PutIfAbsent` with different hash returns `ErrObjectConflict`;
- `Get` caller can close stream;
- `List` surfaces `Err()`.

- [ ] **Step 2: Define `AudioBlobStore`**

Create `internal/storage/blob_store.go`:

```go
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrObjectConflict = errors.New("object integrity conflict")

type AudioBlobStore interface {
	PutIfAbsent(ctx context.Context, key string, body io.Reader, size int64, contentType string, metadata map[string]string, integrity ObjectIntegrity) (*PutResult, error)
	Get(ctx context.Context, key string, opts GetOptions) (*ObjectStream, error)
	Stat(ctx context.Context, key string) (*ObjectInfo, error)
	Remove(ctx context.Context, key string) error
	PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	List(ctx context.Context, prefix string) (ObjectIterator, error)
}

type ObjectIntegrity struct {
	ContentSHA256 string
	SizeBytes     int64
	ContentType   string
}

type PutResult struct {
	Key     string
	ETag    string
	Created bool
	Reused  bool
}

type ObjectStream struct {
	Body io.ReadCloser
	Info ObjectInfo
}

func (s *ObjectStream) Close() error {
	if s == nil || s.Body == nil {
		return nil
	}
	return s.Body.Close()
}
```

- [ ] **Step 3: Implement S3/MinIO store**

Use MinIO SDK in `internal/storage/s3_blob_store.go`. `PutIfAbsent` flow:

1. `StatObject`.
2. If found, compare metadata hash/size/content-type.
3. If match, return `PutResult{Reused: true}`.
4. If mismatch, return `ErrObjectConflict`.
5. If missing, upload with metadata.
6. On concurrent create conflict, re-stat and compare.

- [ ] **Step 4: Implement audio service**

`internal/module/audio/service.go` responsibilities:

- object key generation;
- `PutIfAbsent`;
- `StoreRuntime.Transact` for `audio_objects` insert/update and target-row link;
- derived stream URL generation;
- private/public visibility checks;
- delete and reconciliation hooks.

- [ ] **Step 5: Implement stream handler**

`internal/module/audio/handler.go` must support:

- `GET`;
- `HEAD`;
- `Range`;
- `206`;
- `416`;
- `Content-Type`;
- `Content-Length`;
- `Content-Range`;
- `Accept-Ranges: bytes`;
- private no-store cache policy.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/storage ./internal/module/audio ./internal/data/postgres -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/storage internal/module/audio internal/data/postgres/audio_store.go
git commit -m "feat(audio): add minio object storage service"
```

---

### Task 7: Server, Admin, CLI Wiring

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/cmd/admin/main.go`
- Modify: `backend/cmd/appctl/main.go`
- Modify: `internal/module/admin/handler.go`
- Modify: `internal/module/admin/*.go`
- Modify: `internal/cli/*.go`

- [ ] **Step 1: Wire runtime in server**

Server startup flow:

```go
cfg, err := config.Load("")
adapter := postgres.Adapter{}
db, err := adapter.Open(ctx, store.DatabaseConfig{Store: cfg.RelationalStore, DatabaseURL: cfg.DatabaseURL})
err = adapter.RunMigrations(ctx, db)
runtime, err := adapter.NewStoreRuntime(db, deps)
```

Inject narrow stores into module services and handlers.

- [ ] **Step 2: Wire runtime in admin**

Replace current `admin.HandlerConfig` fields:

```go
type HandlerConfig struct {
	AdminToken string
	Stores     store.AdminStores
	AppStores  store.AppStores
	Audio      *audio.Service
	AIAPIKey      string
	AIAPIEndpoint string
	AIModel       string
}
```

Remove `DB *sql.DB` and concrete `*data.*Store` fields.

- [ ] **Step 3: Move normal CLI to appctl**

`backend/cmd/appctl` calls `internal/cli.Run`. `internal/cli` creates `StoreRuntime` and `AudioService`, then calls import/generation services. It must not import SQLite.

- [ ] **Step 4: Update import helpers**

Change helpers from:

```go
func ImportWordsFromFile(db *sql.DB, filePath string, autoFill bool) (int, error)
```

to:

```go
func ImportWordsFromFile(ctx context.Context, stores store.AdminStores, filePath string, autoFill bool) (int, error)
```

Repeat for grammar, lessons, speaking, writing, and translation importers.

- [ ] **Step 5: Verify no forbidden imports**

Run:

```bash
rg '"japanese-learning-app/internal/cli"' backend/cmd/server backend/cmd/admin
rg 'modernc.org/sqlite|mattn/go-sqlite3' backend internal/cli internal/data/postgres internal/store
```

Expected:

- First command: no output.
- Second command: no output except files under `internal/migration/sqlitecopy` after Task 8.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./backend/cmd/server ./backend/cmd/admin ./backend/cmd/appctl ./internal/cli ./internal/module/admin -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add backend internal/cli internal/module/admin
git commit -m "refactor(storage): wire runtime stores into server admin and cli"
```

---

### Task 8: Offline SQLite Copy And MigrationTarget

**Files:**
- Create: `backend/cmd/storage-migrate/main.go`
- Create: `internal/migration/sqlitecopy/*.go`
- Create: `internal/data/postgres/migration_target.go`
- Modify: `internal/store/migration.go`
- Test: `internal/migration/sqlitecopy/*_test.go`
- Test: `internal/data/postgres/migration_target_test.go`

- [ ] **Step 1: Write MigrationTarget tests**

Tests:

- `VerifySchemaOnly` passes with only `schema_migrations`;
- fails when `users` has a row;
- fails when `audio_objects` has a row;
- rejects invalid `MigrationTable`;
- rejects invalid `MigrationColumn`;
- resets identity sequence after preserve-ID insert.

- [ ] **Step 2: Implement PostgreSQL MigrationTarget**

`internal/data/postgres/migration_target.go` maps enum values:

```go
var migrationTableNames = map[store.MigrationTable]string{
	store.MigrationTableWords: "words",
	store.MigrationTableWordExamples: "word_examples",
}
```

Every insert builds quoted identifiers only from allowlists:

```go
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
```

No table or column name comes from JSON, CLI flags, or source SQLite rows.

- [ ] **Step 3: Implement sqlitecopy preflight**

`internal/migration/sqlitecopy` opens source as:

```go
db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_query_only=1")
```

Then:

- verifies read-only mode;
- computes duplicate maps;
- builds `migration_expected_manifest.json`;
- refuses conflicting duplicates;
- never deletes from SQLite.

- [ ] **Step 4: Implement copy phases**

Use the approved phase order:

1. schema-only check;
2. parent canonical rows;
3. child rows;
4. users and goals;
5. review events;
6. notes;
7. practice records;
8. audio imports;
9. identity reset;
10. actual manifest;
11. reconciliation dry run.

- [ ] **Step 5: Implement storage-migrate command**

`backend/cmd/storage-migrate/main.go` parses:

```text
migrate-sqlite-to-relational
--sqlite
--target-store
--database
--audio-root
--minio-endpoint
--minio-bucket
--migration-run-id
--audio-import-timeout
--audio-import-max-bytes
--allow-missing-audio
```

It resolves adapter, asserts `MigrationCapableAdapter`, creates `MigrationTarget`, and calls `sqlitecopy.Run`.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/migration/sqlitecopy ./internal/data/postgres ./backend/cmd/storage-migrate -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/storage-migrate internal/migration/sqlitecopy internal/data/postgres/migration_target.go internal/store/migration.go
git commit -m "feat(migration): add sqlite to postgres copy command"
```

---

### Task 9: Review Events, Translation Snapshots, And Daily Windows

**Files:**
- Modify: `internal/data/postgres/word_store.go`
- Modify: `internal/data/postgres/grammar_store.go`
- Modify: `internal/data/postgres/note_store.go`
- Modify: `internal/data/postgres/translation_store.go`
- Modify: `internal/data/postgres/user_store.go`
- Modify: `internal/module/summary/model.go`

- [ ] **Step 1: Add review event tests**

Tests:

- word review updates `word_records` and appends `word_review_events` in one transaction;
- grammar quiz updates `grammar_records` and appends `grammar_quiz_attempts`;
- note review updates note SRS fields and appends `note_review_events`;
- forced error rolls back both current state and event row.

- [ ] **Step 2: Implement event write paths**

Use:

```go
return runtime.Transact(ctx, func(tx *store.StoreRuntime) error {
	if err := tx.App.Words.UpsertRecord(ctx, record); err != nil {
		return err
	}
	return tx.App.Words.AppendReviewEvent(ctx, event)
})
```

- [ ] **Step 3: Add translation snapshot tests**

Tests:

- `SaveRecord` stores `source_text_snapshot`;
- stores `direction_snapshot`;
- stores `reference_translation_snapshot`;
- stores `position_snapshot`;
- trigger rejects practiced source text change;
- trigger rejects practiced direction change;
- trigger rejects practiced position change.

- [ ] **Step 4: Implement translation save**

In one transaction:

```sql
SELECT source_text, direction, reference_translation, position
FROM translation_sentences
WHERE id = $1
FOR SHARE;
```

Then insert `translation_records` with snapshot columns.

- [ ] **Step 5: Rewrite daily windows**

Replace SQL `date('now')` logic with Go-computed windows:

```go
start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
end := start.AddDate(0, 0, 1)
```

Queries use `updated_at >= $1 AND updated_at < $2`.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/data/postgres ./internal/module/translation ./internal/module/summary -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/data/postgres internal/module/summary internal/module/translation
git commit -m "feat(storage): add event writes and translation snapshots"
```

---

### Task 10: Audio Touchpoints And Frontend Playback

**Files:**
- Modify: `internal/module/word/*`
- Modify: `internal/module/grammar/*`
- Modify: `internal/module/lesson/*`
- Modify: `internal/module/speaking/*`
- Modify: `internal/module/admin/audio.go`
- Modify: `front/react/src/api/client.ts`
- Modify: `front/react/src/pages/word/WordReviewPage.tsx`
- Modify: `front/react/src/pages/grammar/GrammarDetailPage.tsx`
- Modify: `front/react/src/pages/speaking/SpeakingPage.tsx`
- Modify: `front/react/src/pages/lesson/LessonPage.tsx`
- Modify: `front/admin/src/**`
- Modify: `front/web/static/js/*.ts`

- [ ] **Step 1: Add API response tests**

Backend handler tests verify:

- word examples include `audio_url` when linked;
- grammar examples include `audio_url`;
- speaking material includes `audio_url`;
- lesson summary/detail includes derived `audio_url`;
- private speaking record returns private stream URL only to owner/admin.

- [ ] **Step 2: Update handlers/services**

Handlers derive audio URLs from `AudioService` and `audio_object_id`. Do not expose MinIO object keys.

- [ ] **Step 3: Update admin audio**

Admin audio requests send target identity:

```json
{"target_type":"word_example","target_id":123,"provider":"vllm"}
```

Admin calls `AudioService.GenerateAndAttach`.

- [ ] **Step 4: Update frontend**

Replace hash-based playback with API-provided URL:

```ts
export async function playExample(audioUrl: string | undefined, text: string) {
  if (audioUrl) {
    try {
      const audio = new Audio(audioUrl)
      await audio.play()
      return
    } catch {
      window.speechSynthesis.speak(new SpeechSynthesisUtterance(text))
      return
    }
  }
  window.speechSynthesis.speak(new SpeechSynthesisUtterance(text))
}
```

- [ ] **Step 5: Scan stale audio paths**

Run:

```bash
rg '/audio/|audioHash|sha256' front internal scripts
```

Expected:

- Any remaining matches are allowlisted for object key hashing, tests, or migration reports.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/module/word ./internal/module/grammar ./internal/module/lesson ./internal/module/speaking ./internal/module/admin -v -count=1
make web
```

- [ ] **Step 7: Commit**

```bash
git add internal/module front scripts
git commit -m "feat(audio): serve generated audio from object storage"
```

---

### Task 11: Reconciliation, Backup, Docs, And Cutover

**Files:**
- Create: `internal/maintenance/audio_reconcile.go`
- Modify: `backend/cmd/storage-migrate/main.go`
- Modify: `README.md`
- Modify: `docs/admin-panel-guide.md`
- Modify: `docs/architecture.md`
- Modify: `Makefile`

- [ ] **Step 1: Implement audio reconcile**

Command behavior:

- list MinIO managed prefixes;
- compare to `audio_objects(bucket, object_key)`;
- report missing DB rows;
- report missing MinIO objects;
- default dry-run;
- cleanup only with explicit flag;
- fail on iterator error.

- [ ] **Step 2: Add backup/restore docs**

Document:

- relational database backup ID;
- MinIO bucket/version marker;
- app version;
- migration version;
- restore MinIO first, database second;
- run `audio-reconcile --dry-run`.

- [ ] **Step 3: Add deployment docs**

Document env vars:

```text
RELATIONAL_STORE=postgres
DATABASE_URL=...
APP_TIMEZONE=Asia/Shanghai
AUDIO_OBJECT_STORE=s3
MINIO_ENDPOINT=...
MINIO_ACCESS_KEY=...
MINIO_SECRET_KEY=...
MINIO_BUCKET_AUDIO=...
```

- [ ] **Step 4: Add cutover checklist**

Checklist:

1. Freeze writes.
2. Backup SQLite and local audio tree.
3. Backup current deployment.
4. Run schema migrations.
5. Run `storage-migrate migrate-sqlite-to-relational`.
6. Compare manifests.
7. Run `audio-reconcile --dry-run`.
8. Start server/admin with PostgreSQL and MinIO.
9. Run smoke tests.
10. Keep SQLite and local audio read-only until validation window ends.

- [ ] **Step 5: Verify docs and commands**

Run:

```bash
go test ./internal/maintenance ./backend/cmd/storage-migrate -v -count=1
rg 'DB_PATH|AudioStorePath|/audio/' README.md docs internal front
```

Expected:

- `DB_PATH` and `AudioStorePath` remain only in migration or legacy-context documentation.
- `/audio/` remains only in legacy/context sections or tests.

- [ ] **Step 6: Commit**

```bash
git add internal/maintenance backend/cmd/storage-migrate README.md docs Makefile
git commit -m "docs(storage): add postgres minio cutover runbook"
```

---

### Task 12: Final Verification

**Files:**
- All changed files

- [ ] **Step 1: Run full backend tests**

```bash
make test
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

```bash
make web
```

Expected: PASS.

- [ ] **Step 3: Run targeted scans**

```bash
rg 'modernc.org/sqlite|mattn/go-sqlite3' backend internal | rg -v 'internal/migration/sqlitecopy|go.mod|go.sum'
rg 'DB_PATH|AudioStorePath' backend internal | rg -v 'sqlitecopy|migration|legacy'
rg '/audio/|audioHash' front internal scripts
```

Expected:

- First command has no output.
- Second command has no runtime-path output.
- Third command has only allowlisted legacy/test/migration mentions.

- [ ] **Step 4: Run migration smoke test**

Use a copy of `data/app.db` and a clean PostgreSQL test database:

```bash
go run ./backend/cmd/storage-migrate migrate-sqlite-to-relational \
  --sqlite ./data/app.db \
  --target-store postgres \
  --database "$DATABASE_URL_TEST" \
  --audio-root ./data/audio \
  --minio-endpoint "$MINIO_ENDPOINT" \
  --minio-bucket "$MINIO_BUCKET_AUDIO" \
  --migration-run-id "smoke-$(date +%Y%m%d%H%M%S)" \
  --audio-import-timeout 10s \
  --audio-import-max-bytes 52428800
```

Expected:

- source expected manifest written;
- target actual manifest written;
- manifest comparison passes or fails only for explicitly allowed missing audio;
- `audio-reconcile --dry-run` reports no missing referenced objects.

- [ ] **Step 5: Run workflow smoke tests**

Manually verify:

- login/register;
- word list/review/bookmark;
- grammar list/detail/quiz;
- lesson list/detail audio playback;
- note CRUD/search/backlinks;
- speaking material playback and practice submit;
- writing submit;
- translation submit and history snapshot display;
- admin CRUD and batch audio.

- [ ] **Step 6: Commit final fixes**

```bash
git add .
git commit -m "feat(storage): migrate runtime storage to postgres and minio"
```

---

## Execution Notes

- Prefer one task per commit.
- Keep unrelated dirty files untouched.
- Use table-driven tests for new unit tests.
- Use real PostgreSQL and real MinIO/S3-compatible containers for at least one integration path.
- Do not run destructive cleanup against the original SQLite database or original `./data/audio` tree.
- Do not expose MinIO credentials or raw object keys to frontend responses.

---

## Self-Review

- Spec coverage: relational adapter, admin stores, transaction runner, migration target, SQLite isolation, MinIO, audio stream, schema, migration, translation snapshots, tests, backup/cutover are each mapped to tasks.
- Placeholder scan: no planning placeholders or intentionally vague implementation steps are present.
- Type consistency: `StoreRuntime`, `MigrationTarget`, `BulkLoader`, `AudioBlobStore`, `PutIfAbsent`, `ErrNestedTransaction`, and snapshot field names match the approved design.
