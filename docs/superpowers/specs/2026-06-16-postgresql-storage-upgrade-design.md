# PostgreSQL Storage Upgrade Design

> **Status:** Design document for migrating the backend storage layer from SQLite to PostgreSQL.

**Goal:** Upgrade the whole backend storage system to PostgreSQL and choose data structures according to data shape, query pattern, and future maintenance needs.

**Architecture:** Keep the current Go service and store boundaries, replace the SQLite-specific database layer with a PostgreSQL-native implementation, and introduce a real migration tracking system. Use relational tables for stable entities and joins, `jsonb` for flexible AI/result payloads, arrays for small scalar lists, append-only event tables for review history, and PostgreSQL search indexes for notes and content lookup.

**Tech Stack:** Go 1.24+, `database/sql`, `github.com/jackc/pgx/v5/stdlib`, PostgreSQL 16+, MinIO/S3-compatible object storage, SQL migrations, Makefile-driven build/test commands.

---

## Current Storage Context

The current project uses SQLite through `modernc.org/sqlite`. The main database entry points are:

- `internal/data/db.go`: opens SQLite, enables PRAGMA settings, and runs embedded migrations.
- `internal/data/*_store.go`: concrete SQL access for words, grammar, lessons, notes, users, sessions, speaking, writing, and translation.
- `backend/cmd/server/main.go`: opens the database with `DB_PATH`, runs migrations, and wires all stores.
- `backend/cmd/admin/main.go`: opens the same database and runs the same migrations for the admin service.
- CLI import commands under `internal/cli`: also call `data.OpenDB` and `data.RunMigrations`.

The existing SQLite migration system has known defects:

- No `schema_migrations` tracking table.
- Every startup executes every migration file again.
- `002_seed.sql` can insert duplicate words before the unique index exists.
- `003_fix_writing_questions.sql` rebuilds `writing_questions` on every startup.
- Migration idempotency relies on matching SQLite error strings.

The PostgreSQL upgrade should fix these issues as part of the storage migration rather than carrying them forward.

---

## Design Principles

1. Keep the service layer stable.
   Existing handlers and services should keep using the same store interfaces wherever practical. Most changes should stay inside `internal/data`, CLI SQL helpers, migrations, and configuration.

2. Use PostgreSQL features deliberately.
   The goal is not to put every old JSON text field into `jsonb`. Data that is queried, filtered, joined, or independently edited should become relational. Data that is flexible, AI-generated, or read as one payload can remain `jsonb`.

3. Prefer explicit schema over clever abstraction.
   Keep SQL readable. Use `CHECK` constraints for small enumerations instead of introducing custom enum types unless the value set becomes widely shared and hard to maintain.

4. Preserve import and seed validation rules.
   New word imports must still validate Chinese meanings, example sentence presence, and field completeness before import.

5. Make migration repeatable and observable.
   PostgreSQL migrations must be tracked, locked, logged, and safe to run from both server and admin processes.

6. Keep storage boundaries injectable without pretending all backends are identical.
   Relational data and audio binary data should both be wired through explicit storage adapters. PostgreSQL and MinIO are the required first production implementations, while business handlers should depend on stable store/service contracts rather than driver SDKs.

---

## Relational Database Layer

### Storage Pluggability Boundaries

The upgrade should make the relational layer pluggable at service composition boundaries. This does not mean every SQL statement must be portable. Each relational adapter owns its SQL, schema migrations, placeholders, dialect-specific features, and query tuning while exposing the same application store contracts.

Relational data:

- Add `RELATIONAL_STORE=postgres` as the selected relational adapter. PostgreSQL is the required implementation for this migration.
- Keep `DATABASE_URL` as the connection string used by the selected adapter. Adapter-specific options should use separate environment variables only when needed.
- Keep existing module service interfaces (`WordStoreInterface`, `GrammarStoreInterface`, `NoteStoreInterface`, and similar) where practical, and introduce a single composition-level `StoreRuntime` for server/admin/CLI wiring.
- Move concrete SQL implementations behind adapter packages, for example `internal/data/postgres`.
- The SQLite database is not the default runtime store after cutover, but a future `sqlite` adapter can be added for development or embedded deployments only if it passes the relational adapter contract tests. The existing SQLite source remains read-only during this migration.
- PostgreSQL-specific features are allowed inside the PostgreSQL adapter: `jsonb`, arrays, partial indexes, `pg_trgm`, identity columns, advisory migration locks, and `timestamptz`.
- A non-PostgreSQL adapter must provide equivalent application behavior, not identical DDL. For example, it may replace PostgreSQL arrays with child tables or replace `pg_trgm` with another indexed search strategy as long as contract tests and acceptance queries pass.
- Do not put dialect branches throughout handlers or module services. Dialect branching belongs inside the relational adapter.

Suggested relational adapter shape:

```go
type RelationalAdapter interface {
    Name() string
    Open(ctx context.Context, cfg DatabaseConfig) (*sql.DB, error)
    RunMigrations(ctx context.Context, db *sql.DB) error
    NewStoreRuntime(db *sql.DB, deps StoreDeps) (*StoreRuntime, error)
    TranslateError(err error) error
    Capabilities() RelationalCapabilities
}

type MigrationCapableAdapter interface {
    RelationalAdapter
    NewMigrationTarget(db *sql.DB, deps MigrationDeps) (MigrationTarget, error)
}

type StoreRuntime struct {
    App   AppStores
    Admin AdminStores

    // Transact provides tx-bound stores. It covers relational writes only;
    // object-store compensation is coordinated by AudioService.
    Transact(ctx context.Context, fn func(tx *StoreRuntime) error) error
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

type MigrationDeps struct {
    Clock          Clock
    Logger         Logger
    AppTimezone    *time.Location
    MigrationRunID string
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

type MigrationTarget interface {
    VerifySchemaOnly(ctx context.Context) error
    WithBulkLoad(ctx context.Context, fn func(loader BulkLoader) error) error
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
```

`StoreRuntime` should be the only relational object handed to server/admin/CLI composition code. Individual modules can continue to receive the narrower interface they already use.

Admin wiring must not keep depending on concrete stores such as `*data.WordStore`, `*data.GrammarStore`, or raw `*sql.DB`. Admin handlers should receive `AdminStores` plus application services such as `AudioService`. Admin interfaces should be narrow but complete for the admin surface: CRUD, pagination, search, record listing, bulk import staging, target selection for audio generation, and user deletion orchestration.

Package ownership:

- Put neutral adapter/runtime contracts in `internal/store`, for example `internal/store/contracts.go`, `internal/store/adapter.go`, and `internal/store/errors.go`.
- Keep consumer-owned narrow interfaces in the consuming package when that avoids coupling, for example module service interfaces can remain in their module/service packages.
- `internal/store` may depend only on stable domain model packages that do not import `internal/store`. If that would create an import cycle, define small DTOs in `internal/store` instead.
- `internal/store` must not import `internal/data/postgres`, `internal/module/admin` handlers, CLI packages, module services that already depend on stores, or server binaries.
- `internal/data/postgres` imports `internal/store` and implements its contracts.
- Server, admin, CLI, and migration binaries import `internal/store` plus the selected adapter registration. They should not import concrete PostgreSQL stores directly.

Transaction rules:

- `Transact` must create tx-bound `AppStores` and `AdminStores` backed by the same transaction.
- If `fn` returns an error, the adapter must roll back and return that error. If rollback also fails, return an error that preserves the callback error and rollback error for `errors.Is`/`errors.As`.
- If `fn` panics, the adapter must roll back and then re-panic with the original panic value. Rollback failures during panic handling are logged but must not replace the original panic.
- If `fn` returns nil, the adapter commits. If commit fails, return the translated commit error.
- Transaction stores must not expose raw `*sql.Tx` to handlers, CLI commands, or module services.
- Nested transactions are unsupported by default. Calling `tx.Transact(...)` from inside a tx-bound `StoreRuntime` must return `ErrNestedTransaction`, not panic. An adapter may opt into savepoints only if it documents the semantics and contract tests cover commit/rollback at each nesting level.
- Use `Transact` for review current-state plus event append, audio metadata plus target-row linking, user hard-delete metadata cleanup, import batches that must be atomic, and admin multi-row changes.

Minimum relational adapter contract:

- Owns migrations and migration locks for its backend.
- Owns SQL placeholder style, conflict/upsert syntax, identity handling, array/JSON mapping, and time handling.
- Exposes one consistent error translation layer for duplicate keys, missing rows, constraint violations, and retryable connection errors.
- Provides `Transact` and passes commit/rollback contract tests.
- Provides `MigrationTarget` only to the offline migration binary when it is used as a SQLite migration target.
- Passes the shared store behavior tests for every module.
- Passes migration/manifest validation when used as a migration target.
- Documents any unsupported capability at startup and fails fast if the application feature set requires it.

Migration bulk-load identifier rules:

- Migration orchestration must not pass raw table names, raw column names, or SQL fragments.
- `MigrationTable` and `MigrationColumn` values are adapter-defined enums exposed through `internal/store/migration.go` or adapter-specific construction helpers.
- Each adapter owns an allowlist mapping `MigrationTable`/`MigrationColumn` to quoted identifiers.
- The adapter must reject unknown table/column IDs and reject columns that are not allowed for the selected table.
- Identifier quoting is centralized inside the adapter. Migration code outside the adapter only passes data values.
- Contract tests should include an invalid table/column case to prove the loader rejects it before SQL execution.

Required capabilities for the PostgreSQL migration target:

- `Transactions`: required for review/event writes, audio metadata linking, imports, and user deletion cleanup.
- `Migrations` and `MigrationLocks`: required for concurrent server/admin startup.
- `IdentityOverride`: required for SQLite source ID preservation during migration.
- `InsertReturning` and `Upsert`: required for generated IDs and idempotent imports.
- `JSONDocuments`: required for flexible AI feedback and metadata fields.
- `ArrayFields` and `TagFiltering`: required by JLPT levels, tags, suggestions, and linked word IDs in the PostgreSQL adapter.
- `TextSearch`: required for word, grammar, and note search. A non-PostgreSQL adapter can implement this differently if behavior tests pass.
- `TimeWindowFiltering`: required for daily stats using Go-computed `[start, end)` windows.
- `AudioMetadataLinkage`: required because business rows reference `audio_objects`.

Error translation:

```go
var (
    ErrNotFound   = errors.New("not found")
    ErrDuplicate  = errors.New("duplicate")
    ErrConstraint = errors.New("constraint violation")
    ErrRetryable  = errors.New("retryable database error")
    ErrNestedTransaction = errors.New("nested transaction")
)
```

Adapters may wrap these sentinel errors in typed errors that include table, constraint, operation, and cause. Callers should use `errors.Is` and `errors.As`, not string matching. The PostgreSQL adapter should map `sql.ErrNoRows` to `ErrNotFound`, SQLSTATE `23505` to `ErrDuplicate`, SQLSTATE `23503`, `23514`, `23502`, and `23P01` to `ErrConstraint`, and SQLSTATE `40001`, `40P01`, `08000`, `08003`, `08006`, `53300`, and other documented transient connection failures to `ErrRetryable`.

Object/audio data:

- Audio binary storage must be injected through a narrow interface, for example `AudioBlobStore`.
- The first production implementation is MinIO using an S3-compatible client.
- Future S3-compatible backends such as AWS S3, Cloudflare R2, or Ceph can be added by configuration if they satisfy the same range-read, metadata, delete, list, and presign behavior.
- Local filesystem storage is not a durable production backend after this migration. It may only be used by a migration reader, a temporary debug writer, or unit-test fake.
- Business code should depend on the audio service, not directly on MinIO SDK calls. The application composition root wires `AudioBlobStore`, `AudioStore` metadata persistence, and module stores together.

Suggested interface shape:

```go
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
```

`GetOptions` must support byte ranges so the HTTP stream endpoint can implement browser audio seeking without knowing which object-store implementation is underneath.

Object-store write semantics:

- `PutIfAbsent` never overwrites an existing object.
- If the key does not exist, it uploads the object with metadata that includes at least `content_sha256`, `size_bytes`, and `content_type`.
- If the key already exists, it must `Stat` the existing object and compare the expected integrity values. If hash, size, or content type differ, return `ErrObjectConflict` and do not update metadata.
- If the key already exists with matching integrity, return a `PutResult` marked as reused/existing. This makes concurrent generation of the same deterministic audio key idempotent.
- Callers should use `errors.Is(err, ErrObjectConflict)` for integrity mismatches.
- Implementations may use backend-native conditional writes when available. If the backend cannot atomically create-if-absent, use an adapter-documented temp-key plus verify/promote pattern or accept duplicate temp uploads that reconciliation can clean; never overwrite the canonical key.

Object stream and iterator lifecycle:

- `ObjectStream` must expose an `io.ReadCloser`-compatible body. The caller owns it and must call `Close()` on every successful `Get`, including range responses.
- The stream endpoint should `defer stream.Close()` immediately after successful `Get` to avoid leaking MinIO HTTP bodies.
- `ObjectIterator` must hide backend pagination and continuation tokens from callers.
- Recommended iterator shape is `Next(ctx) bool`, `Object() ObjectInfo`, `Err() error`, and `Close() error`.
- `Next(ctx)` must stop promptly when the context is canceled. After iteration ends, callers must check `Err()` and then call `Close()`.
- Reconciliation must treat iterator errors as incomplete scans and fail the run rather than cleaning objects from a partial listing.

### Relational Store Configuration

Replace `DB_PATH` with `RELATIONAL_STORE` plus `DATABASE_URL`.

Required environment variables:

```text
RELATIONAL_STORE=postgres
DATABASE_URL=postgres://japanese_app:japanese_app@localhost:5432/japanese_learning_app?sslmode=disable
```

Recommended local value:

```text
postgres://japanese_app:japanese_app@localhost:5432/japanese_learning_app?sslmode=disable
```

Recommended server/admin startup behavior:

- Read `RELATIONAL_STORE`.
- Fail fast if `RELATIONAL_STORE` is empty or unsupported.
- Read `DATABASE_URL`.
- Fail fast if it is empty.
- Resolve the relational adapter by `RELATIONAL_STORE`.
- For `postgres`, open via `sql.Open("pgx", databaseURL)`.
- Configure connection pool from environment variables instead of hard-coding one value for server, admin, tests, and migration:
  - `DB_MAX_OPEN_CONNS`, default `25` for the main server.
  - `DB_MAX_IDLE_CONNS`, default equal to `DB_MAX_OPEN_CONNS`.
  - `DB_CONN_MAX_LIFETIME`, default `5m`.
  - Admin, migration, and test processes should use lower defaults unless configured otherwise so parallel processes do not exhaust PostgreSQL `max_connections`.
- Ping before running migrations.
- Build the `StoreRuntime` from the selected adapter and inject module-specific interfaces into services/handlers.

Recommended application time zone:

```text
APP_TIMEZONE=Asia/Shanghai
```

Use this value for product-day calculations such as daily goals and today-completed counts. Do not rely on the PostgreSQL session time zone for product-day boundaries.

### Object Storage Configuration

Use MinIO as the only durable audio object store.

Required environment variables:

```text
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET_AUDIO=japanese-learning-audio
MINIO_USE_SSL=false
```

Optional environment variables:

```text
MINIO_PUBLIC_ENDPOINT=https://audio.example.com
MINIO_PRESIGN_TTL_SECONDS=900
MINIO_REGION=us-east-1
AUDIO_IMPORT_TIMEOUT_SECONDS=10
AUDIO_IMPORT_MAX_BYTES=52428800
```

Server/admin startup should validate MinIO connectivity and bucket existence when audio features are enabled. Local development should provide a Makefile target that starts MinIO and creates the bucket before running audio-generating commands.

### Migration Tracking

Every relational adapter must provide tracked, locked, repeatable schema migrations. The shared application only calls `adapter.RunMigrations(ctx, db)`; it should not know the adapter's migration directory, lock primitive, or DDL syntax.

The migration tracking table can share the same logical contract across adapters:

- `version`: migration file or logical migration version.
- `checksum`: content checksum.
- `applied_at`: backend-native timestamp.

The concrete table DDL may differ by adapter.

PostgreSQL implementation:

Create a real migration table:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version        text PRIMARY KEY,
    checksum       text NOT NULL,
    applied_at     timestamptz NOT NULL DEFAULT now()
);
```

Migration behavior:

- Read embedded schema files from a PostgreSQL-specific directory, for example `internal/data/migrations/postgres/schema/*.sql`.
- Sort by file name.
- Pin the migration runner to one physical PostgreSQL connection with `db.Conn(ctx)`.
- Acquire a session-level advisory lock on that same pinned connection before applying migrations:

```sql
SELECT pg_advisory_lock(hashtext('japanese-learning-app:migrations'));
```

- For each migration:
  - Compute checksum.
  - Start a transaction on the pinned connection.
  - Check `schema_migrations` inside that transaction.
  - Skip if `version` exists with the same checksum.
  - Fail if `version` exists with a different checksum.
  - Execute the migration SQL.
  - Insert the `schema_migrations` row in the same transaction.
  - Commit the transaction.
- Release the advisory lock from the same pinned connection in a defer path:

```sql
SELECT pg_advisory_unlock(hashtext('japanese-learning-app:migrations'));
```

This avoids the `database/sql` connection-pool bug where `pg_advisory_lock`, migration execution, and `pg_advisory_unlock` could otherwise run on different physical connections.

An acceptable alternative is one outer transaction for the whole migration batch using `pg_advisory_xact_lock`, but the preferred implementation is the pinned-connection session lock plus per-migration transactions because it keeps each migration independently atomic while still serializing the full batch.

Seed behavior is intentionally separate from schema migrations:

- Server/admin startup runs schema migrations only.
- SQLite-to-PostgreSQL migration runs schema migrations only.
- Development seed data lives under a separate path such as `internal/data/seeds/postgres/*.sql` or a `seed-postgres` CLI command.
- Seed commands may run only against an empty relational content database for the selected adapter and must check that tables such as `words`, `grammar_points`, `lessons`, `speaking_materials`, and `writing_questions` are empty before inserting.
- Do not run seed migrations before copying SQLite data. The SQLite source already contains seed/content rows, and pre-seeding PostgreSQL would conflict with preserved IDs and natural unique constraints.

---

## Data Structure Choices

### 1. Stable Domain Entities: Relational Tables

Use normal relational tables for entities with stable identity, frequent filtering, and joins.

Tables:

- `users`
- `audio_objects`
- `words`
- `word_bookmarks`
- `word_examples`
- `grammar_points`
- `grammar_examples`
- `grammar_quiz_questions`
- `lessons`
- `lesson_sentences`
- `lesson_words`
- `speaking_materials`
- `writing_questions`
- `translation_sources`
- `translation_sentences`

Reasoning:

- These records have stable IDs.
- Admin pages edit them independently.
- APIs filter them by level, type, source, and search terms.
- PostgreSQL can enforce FK and uniqueness rules directly.

### 2. Flexible AI Payloads: `jsonb`

Use `jsonb` for data whose shape may evolve and is usually read/written as one payload.

Fields:

- `writing_records.ai_feedback`
- `translation_records.ai_feedback`
- `session_summaries.score_summary`
- `session_summaries.strengths`
- `session_summaries.weaknesses`
- `grammar_quiz_questions.options`
- `lesson_sentences.tokens`

Reasoning:

- AI feedback schemas may change.
- Session summaries vary by module.
- Furigana tokens are naturally nested and usually returned as a whole sentence.
- PostgreSQL `jsonb` still allows targeted inspection later if needed.

### 3. Small Scalar Lists: Arrays

Use PostgreSQL arrays when the values are simple scalars and the application often filters by membership.

Fields:

- `users.jlpt_levels text[]`
- `lessons.tags text[]`
- `notes.tags text[]`
- `grammar_examples.linked_word_ids bigint[]`
- `session_summaries.suggestions text[]`

Indexes:

```sql
CREATE INDEX idx_lessons_tags_gin ON lessons USING gin (tags);
CREATE INDEX idx_notes_tags_gin ON notes USING gin (tags);
```

Reasoning:

- These are not complex objects.
- Current SQLite code uses JSON text plus `LIKE`, which can produce false positives.
- PostgreSQL arrays support exact membership checks with GIN indexes.

### 4. Review History: Append-Only Event Tables

Do not keep long-term review history only as JSON arrays. Store current scheduling state separately from append-only review events.

Current-state tables:

- `word_records`
- `grammar_records`
- `notes`

Event tables:

- `word_review_events`
- `grammar_quiz_attempts`
- `note_review_events`

Reasoning:

- Review queues need fast current-state queries.
- History grows over time and should not rewrite one large JSON blob on every review.
- Event rows are easier to audit, aggregate, and debug.

### 5. Full Text and Fuzzy Search

Use PostgreSQL search indexes instead of SQLite FTS5 triggers.

Baseline extension: `pg_trgm`, created by the PostgreSQL bootstrap/migration role before search indexes are created.

Indexes:

```sql
CREATE INDEX idx_words_search_trgm
    ON words USING gin ((kanji_form || ' ' || reading || ' ' || meaning) gin_trgm_ops);

CREATE INDEX idx_grammar_search_trgm
    ON grammar_points USING gin ((name || ' ' || meaning) gin_trgm_ops);

CREATE INDEX idx_notes_search_trgm
    ON notes USING gin ((title || ' ' || content || ' ' || source_text) gin_trgm_ops)
    WHERE deleted_at IS NULL;
```

Reasoning:

- Japanese text often has no whitespace, so plain `to_tsvector` is less useful without a tokenizer.
- `pg_trgm` gives good fuzzy substring behavior with low operational complexity.
- If Japanese search quality becomes a core product requirement, evaluate `pgroonga` as a later upgrade.

### 6. Audio and Binary Files: MinIO Object Storage

All audio binary data must be stored in MinIO. PostgreSQL stores only object metadata and references.

Audio binary data includes:

- Word pronunciation TTS audio.
- Word example sentence TTS audio.
- Grammar example sentence TTS audio.
- Speaking material reference audio.
- Lesson reference/reading audio.
- User speaking practice recordings.
- Future generated variants from different TTS providers, voices, models, or instructions.

Do not store WAV/WebM/MP3 bytes in PostgreSQL. Do not treat `./data/audio` as durable storage after this migration; it can only be used as a temporary migration input or short-lived local cache.

PostgreSQL stores:

- One canonical `audio_objects` row per MinIO object.
- Nullable `audio_object_id` references from content/practice tables.
- Derived API response fields such as `audio_url` or `audio_ref` for backward compatibility.

MinIO bucket policy:

- Buckets are private.
- The application never exposes MinIO access keys to browsers.
- The backend serves audio through an authenticated stream endpoint or returns short-lived presigned URLs.
- `audio_objects.visibility = 'authenticated'` content audio can be served to authenticated learners and admins.
- `audio_objects.visibility = 'private'` user recordings require `owner_user_id` checks before playback or download.
- Private user recordings should be served by backend proxy streaming, not by redirecting the browser to a presigned MinIO URL.
- Content audio can use presigned URLs only when `MINIO_PUBLIC_ENDPOINT` points to a browser-reachable public endpoint; do not presign `localhost` or internal-only MinIO endpoints for browser playback.

Recommended buckets:

- `japanese-learning-audio`: all active audio objects.
- Optional later bucket `japanese-learning-audio-archive`: old generated objects retained for audit or rollback.

Object key conventions:

- `tts/words/{source_hash}/{config_hash}/{content_sha256_16}.wav`
- `tts/examples/{source_hash}/{config_hash}/{content_sha256_16}.wav`
- `tts/speaking-materials/{source_hash}/{config_hash}/{content_sha256_16}.wav`
- `lessons/{lesson_id}/{uuid}.{ext}`
- `user-recordings/{user_id}/{yyyy}/{mm}/{uuid}.{ext}`

`source_hash` is `sha256(normalized_text)`. `config_hash` is `sha256(provider + model + voice + speaker + style + instructions + postprocess_version)`. `content_sha256_16` is the first 16 hex characters of the generated object bytes. Word audio keeps the existing silence-trimming behavior, so word pronunciation uses a different `kind` and key prefix from example sentence audio even when the text is identical.

Object write consistency:

1. Synthesize or receive audio bytes.
2. Compute `content_sha256`, size, MIME type, and object key.
3. Call `AudioBlobStore.PutIfAbsent` with `ObjectIntegrity`.
4. If the object already exists, reuse it only when the existing object's hash, size, and content type match the expected values.
5. Use `StoreRuntime.Transact` to insert/update `audio_objects` and update the owning row's `audio_object_id` in one relational transaction.
6. If the database transaction fails, delete the newly uploaded MinIO object best-effort and log the orphan cleanup failure if deletion also fails.

Regeneration should create a new immutable object key and update the owning row to the new `audio_object_id`. If a TTS provider returns the exact same bytes for the same bucket/source/config, the service can reuse the existing MinIO object and `audio_objects` row by `(bucket, kind, source_hash, config_hash, content_sha256)`. If bytes differ, it creates a new row and object. Old unreferenced generated objects can be garbage-collected later. Avoid overwriting existing MinIO keys; immutable keys remove stale browser-cache problems.

Concurrent workers generating the same audio may race. The accepted outcome is one canonical object and one canonical `audio_objects` row. All workers should converge by `PutIfAbsent`, `UNIQUE(bucket, object_key)`, and reuse of matching `(bucket, kind, source_hash, config_hash, content_sha256)` rows.

Object reconciliation:

- Add an operational reconciliation command, for example `audio-reconcile`.
- List MinIO objects under managed prefixes and compare them with `audio_objects(bucket, object_key)`.
- Report objects that exist in MinIO but have no database row.
- Report database rows whose MinIO object is missing.
- Support a dry-run default and an explicit cleanup flag for orphan MinIO objects.
- Store `migration_run_id` or `operation_id` in `audio_objects.metadata` during bulk migration so failed runs can be inspected or cleaned by prefix/metadata.
- Run reconciliation after migration, after restore, and on a scheduled maintenance cadence.

---

## Proposed Schema Outline

### Content Tables

```sql
CREATE TABLE audio_objects (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bucket         text NOT NULL,
    object_key     text NOT NULL,
    kind           text NOT NULL CHECK (kind IN (
        'word_tts',
        'example_tts',
        'speaking_material_tts',
        'lesson_audio',
        'user_recording',
        'uploaded_reference'
    )),
    visibility     text NOT NULL DEFAULT 'authenticated'
        CHECK (visibility IN ('authenticated', 'private')),
    source_text    text NOT NULL DEFAULT '',
    source_hash    text NOT NULL DEFAULT '',
    config_hash    text NOT NULL DEFAULT '',
    tts_provider   text NOT NULL DEFAULT '',
    tts_model      text NOT NULL DEFAULT '',
    voice          text NOT NULL DEFAULT '',
    mime_type      text NOT NULL,
    content_sha256 text NOT NULL,
    size_bytes     bigint NOT NULL CHECK (size_bytes >= 0),
    duration_ms    integer CHECK (duration_ms IS NULL OR duration_ms >= 0),
    etag           text NOT NULL DEFAULT '',
    metadata       jsonb NOT NULL DEFAULT '{}'::jsonb,
    owner_user_id  bigint,
    created_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz,
    UNIQUE (bucket, object_key),
    CHECK (visibility <> 'private' OR owner_user_id IS NOT NULL)
);

CREATE TABLE words (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kanji_form     text NOT NULL,
    reading        text NOT NULL,
    part_of_speech text NOT NULL DEFAULT '',
    meaning        text NOT NULL,
    jlpt_level     text NOT NULL CHECK (jlpt_level IN ('N5', 'N4', 'N3', 'N2', 'N1')),
    reading_type   text NOT NULL DEFAULT '',
    audio_object_id bigint REFERENCES audio_objects(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kanji_form, reading)
);

CREATE TABLE word_examples (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    word_id       bigint NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    position      integer NOT NULL DEFAULT 0,
    japanese      text NOT NULL,
    chinese       text NOT NULL,
    furigana_html text NOT NULL DEFAULT '',
    audio_object_id bigint REFERENCES audio_objects(id) ON DELETE SET NULL,
    UNIQUE (word_id, position)
);

CREATE TABLE grammar_points (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             text NOT NULL,
    meaning          text NOT NULL,
    conjunction_rule text NOT NULL DEFAULT '',
    usage_note       text NOT NULL DEFAULT '',
    jlpt_level       text NOT NULL CHECK (jlpt_level IN ('N5', 'N4', 'N3', 'N2', 'N1')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (name, jlpt_level)
);

CREATE TABLE grammar_examples (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    grammar_point_id bigint NOT NULL REFERENCES grammar_points(id) ON DELETE CASCADE,
    position         integer NOT NULL DEFAULT 0,
    japanese         text NOT NULL,
    chinese          text NOT NULL,
    furigana_html    text NOT NULL DEFAULT '',
    linked_word_ids  bigint[] NOT NULL DEFAULT '{}',
    audio_object_id  bigint REFERENCES audio_objects(id) ON DELETE SET NULL,
    UNIQUE (grammar_point_id, position)
);

CREATE TABLE grammar_quiz_questions (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    grammar_point_id bigint NOT NULL REFERENCES grammar_points(id) ON DELETE CASCADE,
    position         integer NOT NULL DEFAULT 0,
    type             text NOT NULL CHECK (type IN ('fill_blank', 'multi_choice')),
    prompt           text NOT NULL,
    options          jsonb NOT NULL DEFAULT '[]'::jsonb,
    answer           text NOT NULL,
    explanation      text NOT NULL DEFAULT '',
    UNIQUE (grammar_point_id, position)
);
```

### Lesson Tables

```sql
CREATE TABLE lessons (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       text NOT NULL,
    jlpt_level  text NOT NULL CHECK (jlpt_level IN ('N5', 'N4', 'N3', 'N2', 'N1')),
    tags        text[] NOT NULL DEFAULT '{}',
    audio_object_id bigint REFERENCES audio_objects(id) ON DELETE SET NULL,
    char_count  integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (title, jlpt_level)
);

CREATE TABLE lesson_sentences (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    lesson_id bigint NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    position  integer NOT NULL,
    tokens    jsonb NOT NULL DEFAULT '[]'::jsonb,
    chinese   text NOT NULL DEFAULT '',
    start_ms  bigint NOT NULL DEFAULT 0,
    end_ms    bigint NOT NULL DEFAULT 0,
    UNIQUE (lesson_id, position),
    CHECK (start_ms >= 0),
    CHECK (end_ms >= start_ms)
);

CREATE TABLE lesson_words (
    lesson_id bigint NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    word_id   bigint NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    position  integer NOT NULL DEFAULT 0,
    PRIMARY KEY (lesson_id, position),
    UNIQUE (lesson_id, word_id)
);
```

### User and Goal Tables

```sql
CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          text NOT NULL DEFAULT '',
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    goal_level    text NOT NULL DEFAULT 'N5' CHECK (goal_level IN ('N5', 'N4', 'N3', 'N2', 'N1')),
    jlpt_levels   text[] NOT NULL DEFAULT ARRAY['N5'],
    streak_days   integer NOT NULL DEFAULT 0 CHECK (streak_days >= 0),
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (jlpt_levels <@ ARRAY['N5', 'N4', 'N3', 'N2', 'N1']::text[]),
    CHECK (cardinality(jlpt_levels) > 0)
);

ALTER TABLE audio_objects
    ADD CONSTRAINT fk_audio_objects_owner_user
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT;

CREATE TABLE user_daily_goals (
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module  text NOT NULL CHECK (module IN ('word', 'grammar', 'speaking', 'writing', 'lesson', 'translation')),
    goal    integer NOT NULL CHECK (goal >= 0),
    PRIMARY KEY (user_id, module)
);

CREATE TABLE password_reset_tokens (
    token      text PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used       boolean NOT NULL DEFAULT false
);
```

### Review and Practice Tables

```sql
CREATE TABLE word_records (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word_id        bigint NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    mastery_level  integer NOT NULL DEFAULT 0 CHECK (mastery_level >= 0),
    next_review_at timestamptz NOT NULL DEFAULT now(),
    ease_factor    double precision NOT NULL DEFAULT 2.5 CHECK (ease_factor > 0),
    interval       integer NOT NULL DEFAULT 0 CHECK (interval >= 0),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, word_id)
);

CREATE TABLE word_review_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word_id     bigint NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    rating      text NOT NULL CHECK (rating IN ('easy', 'normal', 'hard')),
    reviewed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE word_bookmarks (
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word_id    bigint NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, word_id)
);

CREATE TABLE grammar_records (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    grammar_point_id bigint NOT NULL REFERENCES grammar_points(id) ON DELETE CASCADE,
    status           text NOT NULL DEFAULT 'unlearned' CHECK (status IN ('unlearned', 'learning', 'mastered')),
    next_review_at   timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, grammar_point_id)
);

CREATE TABLE grammar_quiz_attempts (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    grammar_point_id bigint NOT NULL REFERENCES grammar_points(id) ON DELETE CASCADE,
    score            integer NOT NULL CHECK (score >= 0 AND score <= 100),
    attempted_at     timestamptz NOT NULL DEFAULT now()
);
```

### Notes Tables

```sql
CREATE TABLE notes (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type           text NOT NULL CHECK (type IN ('word', 'grammar', 'sentence')),
    title          text NOT NULL,
    content        text NOT NULL DEFAULT '',
    source_text    text NOT NULL DEFAULT '',
    reference_id   bigint,
    reference_type text,
    tags           text[] NOT NULL DEFAULT '{}',
    mastery_level  integer NOT NULL DEFAULT 0,
    next_review_at timestamptz,
    ease_factor    double precision NOT NULL DEFAULT 2.5 CHECK (ease_factor > 0),
    interval       integer NOT NULL DEFAULT 0 CHECK (interval >= 0),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz,
    CHECK (
        (reference_id IS NULL AND reference_type IS NULL)
        OR
        (reference_id IS NOT NULL AND reference_type IN ('word', 'grammar'))
    )
);

CREATE TABLE note_review_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_id     bigint NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    rating      text NOT NULL CHECK (rating IN ('easy', 'normal', 'hard')),
    reviewed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE note_links (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_id        bigint NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_note_id bigint NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    relation       text NOT NULL DEFAULT 'related',
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (note_id, target_note_id)
);
```

### Writing, Speaking, Translation, and Summary Tables

```sql
CREATE TABLE speaking_materials (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type       text NOT NULL CHECK (type IN ('shadow', 'free')),
    title      text NOT NULL DEFAULT '',
    text       text NOT NULL,
    lines      text NOT NULL DEFAULT '',
    audio_object_id bigint REFERENCES audio_objects(id) ON DELETE SET NULL,
    jlpt_level text NOT NULL CHECK (jlpt_level IN ('N5', 'N4', 'N3', 'N2', 'N1')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (type, title, jlpt_level)
);

CREATE TABLE speaking_records (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         text NOT NULL CHECK (type IN ('shadow', 'free')),
    material_id  bigint NOT NULL REFERENCES speaking_materials(id) ON DELETE CASCADE,
    score        integer NOT NULL DEFAULT 0 CHECK (score >= 0 AND score <= 100),
    audio_object_id bigint REFERENCES audio_objects(id) ON DELETE SET NULL,
    practiced_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE writing_questions (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type             text NOT NULL CHECK (type IN ('input', 'sentence')),
    prompt           text NOT NULL,
    expected_answer  text NOT NULL,
    grammar_point_id bigint,
    jlpt_level       text NOT NULL DEFAULT 'N5' CHECK (jlpt_level IN ('N5', 'N4', 'N3', 'N2', 'N1')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (type, prompt)
);

CREATE TABLE writing_records (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         text NOT NULL CHECK (type IN ('input', 'sentence')),
    question     text NOT NULL,
    user_answer  text NOT NULL DEFAULT '',
    ai_feedback  jsonb,
    score        integer NOT NULL DEFAULT 0 CHECK (score >= 0 AND score <= 100),
    practiced_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE translation_sources (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title        text NOT NULL,
    source_type  text NOT NULL CHECK (source_type IN ('manual', 'url', 'api')),
    source_url   text NOT NULL DEFAULT '',
    api_endpoint text NOT NULL DEFAULT '',
    raw_content  text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE translation_sentences (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id             bigint NOT NULL REFERENCES translation_sources(id) ON DELETE CASCADE,
    direction             text NOT NULL CHECK (direction IN ('cn2jp', 'jp2cn')),
    source_text           text NOT NULL,
    reference_translation text NOT NULL DEFAULT '',
    position              integer NOT NULL DEFAULT 0,
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, position)
);

CREATE TABLE translation_records (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sentence_id      bigint NOT NULL REFERENCES translation_sentences(id) ON DELETE CASCADE,
    source_text_snapshot text NOT NULL,
    direction_snapshot text NOT NULL CHECK (direction_snapshot IN ('cn2jp', 'jp2cn')),
    reference_translation_snapshot text NOT NULL DEFAULT '',
    position_snapshot integer NOT NULL DEFAULT 0,
    user_translation text NOT NULL,
    score            integer NOT NULL DEFAULT 0 CHECK (score >= 0 AND score <= 100),
    rule_score       integer NOT NULL DEFAULT 0 CHECK (rule_score >= 0 AND rule_score <= 100),
    ai_feedback      jsonb,
    practiced_at     timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION prevent_translation_sentence_prompt_rewrite()
RETURNS trigger AS $$
BEGIN
    IF (OLD.source_text IS DISTINCT FROM NEW.source_text
        OR OLD.direction IS DISTINCT FROM NEW.direction
        OR OLD.position IS DISTINCT FROM NEW.position)
       AND EXISTS (
           SELECT 1
           FROM translation_records tr
           WHERE tr.sentence_id = OLD.id
       )
    THEN
        RAISE EXCEPTION 'cannot rewrite practiced translation sentence prompt/order %', OLD.id
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_translation_sentences_immutable_prompt
BEFORE UPDATE OF source_text, direction, position ON translation_sentences
FOR EACH ROW
EXECUTE FUNCTION prevent_translation_sentence_prompt_rewrite();

CREATE TABLE study_sessions (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id       text NOT NULL UNIQUE,
    user_id          bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module           text NOT NULL CHECK (module IN ('word', 'grammar', 'lesson', 'speaking', 'writing', 'translation')),
    duration_seconds integer NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    completed_count  integer NOT NULL DEFAULT 0 CHECK (completed_count >= 0),
    started_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE session_summaries (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id    text NOT NULL UNIQUE REFERENCES study_sessions(session_id) ON DELETE CASCADE,
    user_id       bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module        text NOT NULL CHECK (module IN ('word', 'grammar', 'lesson', 'speaking', 'writing', 'translation')),
    score_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    strengths     jsonb NOT NULL DEFAULT '[]'::jsonb,
    weaknesses    jsonb NOT NULL DEFAULT '[]'::jsonb,
    suggestions   text[] NOT NULL DEFAULT '{}',
    generated_at  timestamptz NOT NULL DEFAULT now()
);
```

---

## Required Indexes

The PostgreSQL bootstrap/migration role must create required extensions before indexes that depend on them:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

The runtime application role should not require superuser privileges. If production separates migration and runtime users, only the migration owner needs permission to create `pg_trgm`.

```sql
CREATE INDEX idx_audio_objects_kind_created ON audio_objects (kind, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_audio_objects_owner ON audio_objects (owner_user_id, created_at DESC)
    WHERE owner_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_audio_objects_owner_fk ON audio_objects (owner_user_id)
    WHERE owner_user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_audio_objects_generated_unique
    ON audio_objects (bucket, kind, source_hash, config_hash, content_sha256)
    WHERE source_hash <> '' AND config_hash <> '' AND deleted_at IS NULL;

CREATE INDEX idx_words_jlpt_level ON words (jlpt_level);
CREATE INDEX idx_words_updated_at ON words (updated_at DESC);
CREATE INDEX idx_words_audio_object_fk ON words (audio_object_id)
    WHERE audio_object_id IS NOT NULL;

CREATE INDEX idx_word_records_due ON word_records (user_id, next_review_at);
CREATE INDEX idx_word_records_word_fk ON word_records (word_id);
CREATE INDEX idx_word_review_events_user_time ON word_review_events (user_id, reviewed_at DESC);
CREATE INDEX idx_word_review_events_word_time ON word_review_events (user_id, word_id, reviewed_at DESC);
CREATE INDEX idx_word_review_events_word_fk ON word_review_events (word_id);
CREATE INDEX idx_word_bookmarks_word ON word_bookmarks (word_id);
CREATE INDEX idx_word_examples_audio_object_fk ON word_examples (audio_object_id)
    WHERE audio_object_id IS NOT NULL;

CREATE INDEX idx_grammar_points_jlpt ON grammar_points (jlpt_level);
CREATE INDEX idx_grammar_records_due ON grammar_records (user_id, next_review_at);
CREATE INDEX idx_grammar_records_point_fk ON grammar_records (grammar_point_id);
CREATE INDEX idx_grammar_quiz_attempts_latest ON grammar_quiz_attempts (user_id, grammar_point_id, attempted_at DESC);
CREATE INDEX idx_grammar_quiz_attempts_point_fk ON grammar_quiz_attempts (grammar_point_id);
CREATE INDEX idx_grammar_examples_audio_object_fk ON grammar_examples (audio_object_id)
    WHERE audio_object_id IS NOT NULL;

CREATE INDEX idx_lessons_jlpt ON lessons (jlpt_level);
CREATE INDEX idx_lessons_tags_gin ON lessons USING gin (tags);
CREATE INDEX idx_lessons_audio_object_fk ON lessons (audio_object_id)
    WHERE audio_object_id IS NOT NULL;
CREATE INDEX idx_lesson_words_word_fk ON lesson_words (word_id);

CREATE INDEX idx_notes_user_fk ON notes (user_id);
CREATE INDEX idx_notes_user_id ON notes (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_notes_type ON notes (user_id, type) WHERE deleted_at IS NULL;
CREATE INDEX idx_notes_review ON notes (user_id, next_review_at)
    WHERE next_review_at IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_notes_reference ON notes (reference_type, reference_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_notes_tags_gin ON notes USING gin (tags);

CREATE INDEX idx_note_links_note_id ON note_links (note_id);
CREATE INDEX idx_note_links_target ON note_links (target_note_id);
CREATE INDEX idx_note_links_user_fk ON note_links (user_id);
CREATE INDEX idx_note_review_events_user_fk ON note_review_events (user_id);
CREATE INDEX idx_note_review_events_note_time ON note_review_events (note_id, reviewed_at DESC);

CREATE INDEX idx_speaking_materials_type_level ON speaking_materials (type, jlpt_level);
CREATE INDEX idx_speaking_materials_audio_object_fk ON speaking_materials (audio_object_id)
    WHERE audio_object_id IS NOT NULL;
CREATE INDEX idx_speaking_records_user ON speaking_records (user_id, practiced_at DESC);
CREATE INDEX idx_speaking_records_material_fk ON speaking_records (material_id);
CREATE INDEX idx_speaking_records_audio_object_fk ON speaking_records (audio_object_id)
    WHERE audio_object_id IS NOT NULL;

CREATE INDEX idx_password_reset_tokens_user_fk ON password_reset_tokens (user_id);
CREATE INDEX idx_writing_records_user ON writing_records (user_id, practiced_at DESC);
CREATE INDEX idx_translation_sentences_source ON translation_sentences (source_id, position);
CREATE INDEX idx_translation_records_user ON translation_records (user_id, practiced_at DESC);
CREATE INDEX idx_translation_records_sentence ON translation_records (sentence_id);
CREATE INDEX idx_study_sessions_user ON study_sessions (user_id, started_at DESC);
CREATE INDEX idx_session_summaries_user_generated ON session_summaries (user_id, generated_at DESC);
```

Search indexes:

```sql
CREATE INDEX idx_words_search_trgm
    ON words USING gin ((kanji_form || ' ' || reading || ' ' || meaning) gin_trgm_ops);

CREATE INDEX idx_grammar_search_trgm
    ON grammar_points USING gin ((name || ' ' || meaning) gin_trgm_ops);

CREATE INDEX idx_notes_search_trgm
    ON notes USING gin ((title || ' ' || content || ' ' || source_text) gin_trgm_ops)
    WHERE deleted_at IS NULL;
```

---

## Store Layer Changes

The store layer must be split into stable application contracts plus concrete relational adapter implementations.

Recommended package shape:

- `internal/store/contracts.go`: application-facing store interfaces, admin store interfaces, and the `StoreRuntime` bundle.
- `internal/store/adapter.go`: `RelationalAdapter`, `DatabaseConfig`, capability structs, adapter registry, and migration-target interfaces.
- `internal/store/errors.go`: sentinel errors and typed error wrappers.
- `internal/store/migration.go`: adapter-neutral migration DTOs such as `RowBatch`, `AudioObjectRow`, `MigrationManifest`, and expected/actual manifest comparison helpers.
- `internal/data/postgres/*.go`: PostgreSQL implementations of every store.
- `internal/data/postgres/migration_target.go`: PostgreSQL `MigrationTarget`/`BulkLoader` implementation used only by offline migration.
- `internal/data/postgres/migrations/*.go` or embedded migration loader helpers for PostgreSQL schema migrations.
- `internal/store/testsuite`: shared contract tests that can run against any relational adapter.
- `internal/migration/sqlitecopy`: read-only SQLite extraction, duplicate detection, remap planning, manifest generation, and copy orchestration used only by `backend/cmd/storage-migrate`.

Composition rules:

- Server/admin/CLI startup selects the adapter from `RELATIONAL_STORE`.
- Application code receives `StoreRuntime` or narrower module/admin interfaces.
- Module services and handlers should not import `internal/data/postgres`.
- Admin handlers should receive `AdminStores` and services such as `AudioService`; they should not receive concrete `*data.WordStore`, `*data.GrammarStore`, or raw `*sql.DB`.
- CLI import, seed, TTS generation, and batch audio commands should receive `StoreRuntime` plus `AudioService`; raw `*sql.DB` parameters are allowed only inside adapter packages and offline migration internals.
- No normal application path should call `data.OpenDB`, `data.RunMigrations`, `db.Exec`, or `tx.Exec` directly after the adapter layer is introduced.
- The server binary should stop dispatching general CLI commands through `internal/cli`. Use a separate CLI binary for normal maintenance commands if needed, and keep `backend/cmd/storage-migrate` as the only binary that imports SQLite copy code.
- `internal/cli` must not import the SQLite driver after cutover. SQLite imports belong only to `internal/migration/sqlitecopy` and the `storage-migrate` dependency graph.
- Store tests should be split into adapter contract tests and PostgreSQL-specific tests.
- Admin contract tests should cover CRUD, pagination, search, record listing, import staging, target selection for audio generation, and user deletion preconditions through `AdminStores`.
- A future relational backend must add its own adapter package and pass the shared contract suite before it can be treated as supported.

The PostgreSQL adapter must be updated for PostgreSQL SQL syntax and behavior.

### Admin, CLI, and Audio Write Paths

Admin, CLI, and audio generation paths must use the same adapter boundaries as runtime services.

- Admin handlers receive `AdminStores` and application services. They do not receive concrete PostgreSQL stores or raw `*sql.DB`.
- Admin single and batch audio generation calls `AudioService`, which coordinates `AudioBlobStore`, `AudioObjects`, and target-row updates through `StoreRuntime.Transact`.
- Admin import calls parse and validate input outside the transaction, then write through `AdminStores.Imports` or target-specific admin stores.
- CLI imports, seed commands, TTS generation, and batch generation create `StoreRuntime` from `RELATIONAL_STORE` and call the same service/store methods used by admin paths.
- CLI commands should not call `data.OpenDB`, `data.RunMigrations`, or pass `*sql.DB` into import helpers as the regular code path.
- The one-shot SQLite migration command is the exception: it may hold a read-only SQLite source connection, but writes still go through the selected target adapter and its transaction runner.
- Any raw SQL needed for adapter-specific bulk operations belongs in `internal/data/postgres` or another adapter package, not in `internal/cli` or `internal/module/admin`.

### PostgreSQL SQL Dialect Replacements

| SQLite Pattern | PostgreSQL Replacement |
|---|---|
| `?` placeholders | `$1`, `$2`, `$3` |
| `datetime('now')` | `now()` |
| `date(updated_at) = date('now')` | Go-computed `[start, end)` window in the configured product time zone |
| `INSERT OR IGNORE` | `INSERT ... ON CONFLICT DO NOTHING` |
| `INSERT OR REPLACE` | Use only when a real unique conflict target exists; `translation_records` must use append-only `INSERT ... RETURNING id` |
| `LastInsertId()` | `RETURNING id` |
| scan timestamp as `string` then parse | scan directly into `time.Time` |
| JSON text plus `LIKE` for tags | `tags @> ARRAY[$1]` |
| SQLite FTS5 table/triggers | `pg_trgm`/GIN search indexes |
| SQLite unique error code | PostgreSQL SQLSTATE `23505` |

### PostgreSQL Array Handling

For the PostgreSQL adapter, array-backed fields must use PostgreSQL array encoding/decoding, not JSON strings.

Fields affected:

- `users.jlpt_levels text[]`
- `lessons.tags text[]`
- `notes.tags text[]`
- `grammar_examples.linked_word_ids bigint[]`
- `session_summaries.suggestions text[]`

Go implementation guidance:

- Add one helper layer in `internal/data/postgres` for scanning and binding `text[]` and `bigint[]`.
- With `pgx/v5/stdlib`, prefer `pgtype` array helpers such as `pgtype.FlatArray[string]` and `pgtype.FlatArray[int64]`, or an equivalent local wrapper used consistently by all stores.
- Do not scan arrays into `string` and call `json.Unmarshal`.
- Do not build array literals with string concatenation.
- For single-tag filters, use typed array SQL such as `tags @> ARRAY[$1]::text[]`.
- For multi-tag filters, pass an array parameter and use `tags @> $1::text[]`.
- Add table-driven tests for empty arrays, one item, multiple items, and exact membership queries.

Other relational adapters may implement these same application fields with backend-native arrays, JSON, or child tables, but they must expose the same store behavior and pass the same filtering tests.

### Runtime Reference Validation

Some PostgreSQL fields intentionally cannot use direct FKs because they are arrays or polymorphic references. Store/admin code must validate them before write:

- `grammar_examples.linked_word_ids`: every ID must exist in `words`.
- `notes.reference_type = 'word'`: `reference_id` must exist in `words`.
- `notes.reference_type = 'grammar'`: `reference_id` must exist in `grammar_points`.
- `notes.reference_id` and `notes.reference_type` must be written as a pair.
- `writing_questions.grammar_point_id`: when non-NULL, it must exist in `grammar_points`.

Migration validation catches legacy orphans, but runtime validation prevents new dirty data after cutover.

### Translation Records

Translation practice records are append-only.

- PostgreSQL `SaveRecord` must use ordinary `INSERT ... RETURNING id`.
- Do not add a uniqueness constraint on `(user_id, sentence_id)`.
- Do not translate SQLite `INSERT OR REPLACE` here into PostgreSQL upsert.
- Submitting the same sentence twice by the same user should create two `translation_records` rows.
- When saving a record, copy the currently displayed `translation_sentences.source_text`, `direction`, `reference_translation`, and `position` into `translation_records.*_snapshot` columns in the same transaction as the insert.
- Historical review/scoring pages should render from record snapshot fields first. The `sentence_id` remains useful for joins and admin traceability, but the record is self-contained for what the learner saw.
- Add `translation` to the module enum used by `user_daily_goals`, `study_sessions`, `session_summaries`, and Go `summary.ModuleType`.
- `translation_sentences.source_text`, `translation_sentences.direction`, and `translation_sentences.position` become immutable once any `translation_records` row references that sentence.
- Re-importing the same source and position may update mutable fields such as `reference_translation` only when it does not rewrite the historical prompt behind existing practice records.
- Re-import must run in one transaction, lock the target `translation_sentences` row with `SELECT ... FOR UPDATE`, check for existing `translation_records`, then either update mutable fields, create a new source version, or reject the change.
- PostgreSQL schema should include a trigger defense that rejects updates to `source_text`, `direction`, or `position` when practice records already reference the sentence. This protects against concurrent re-imports and manual SQL mistakes.
- If source text, direction, or position changes for an already-practiced sentence, create a new `translation_source` version or reject the import with a report.

### Import Conflict Targets

Seed and CLI import commands must use explicit PostgreSQL conflict targets that match schema constraints:

| Import Target | Unique Constraint | PostgreSQL Import Behavior |
|---|---|---|
| `words` | `(kanji_form, reading)` | `ON CONFLICT (kanji_form, reading) DO UPDATE` for admin imports that enrich existing rows |
| `grammar_points` | `(name, jlpt_level)` | `ON CONFLICT (name, jlpt_level) DO NOTHING` or `DO UPDATE` when admin explicitly edits |
| `lessons` | `(title, jlpt_level)` | `ON CONFLICT (title, jlpt_level) DO NOTHING` |
| `speaking_materials` | `(type, title, jlpt_level)` | `ON CONFLICT (type, title, jlpt_level) DO NOTHING` |
| `writing_questions` | `(type, prompt)` | `ON CONFLICT (type, prompt) DO NOTHING` |
| `translation_sentences` | `(source_id, position)` | In one transaction, lock the row, update mutable fields only when no prompt/order rewrite occurs, and rely on the trigger to reject practiced `source_text`/`direction`/`position` rewrites; otherwise create a new source version or reject |

### Time Zone and Daily Statistics

Do not translate SQLite daily-stat queries directly to `updated_at::date = current_date`.
`current_date` depends on the PostgreSQL session time zone, which can differ between local development, CI, and production.

Recommended behavior:

- Add an application setting such as `APP_TIMEZONE=Asia/Shanghai`.
- Compute daily `[start, end)` boundaries in Go using that configured location.
- Pass the two timestamps into SQL as parameters.
- Query daily stats with `timestamp_column >= $1 AND timestamp_column < $2`.
- Keep all stored `timestamptz` values as absolute instants; only reporting windows use the configured product time zone.

Example:

```sql
SELECT COUNT(*)
FROM word_records
WHERE user_id = $1
  AND updated_at >= $2
  AND updated_at < $3;
```

### Insert Pattern

Current SQLite-style insert:

```go
result, err := db.Exec(`INSERT INTO words (...) VALUES (?, ?)`, a, b)
id, err := result.LastInsertId()
```

PostgreSQL insert:

```go
err := db.QueryRow(
    `INSERT INTO words (...) VALUES ($1, $2) RETURNING id`,
    a, b,
).Scan(&id)
```

### Time Handling

Remove SQLite-specific time parsing from normal store paths.

Current behavior:

- Query scans timestamps into strings.
- Code calls `parseSQLiteTime`.
- Writes use `formatSQLiteTime`.

PostgreSQL behavior:

- Query scans `timestamptz` into `time.Time`.
- Writes pass `time.Time` values directly.
- Server-generated timestamps use `now()`.

### JSON Handling

For `jsonb` fields, the Go side can keep marshaling with `encoding/json`.

Recommended scan/write shape:

- Write: marshal to `[]byte` or string and pass as a SQL parameter.
- Read: scan into `[]byte` or `json.RawMessage`, then unmarshal.
- Use SQL defaults for empty JSON objects/arrays.
- Normalize legacy empty AI feedback before writing `jsonb`:
  - `''`, whitespace-only strings, and literal `null` become SQL `NULL`.
  - Valid non-empty JSON is stored as `jsonb`.
  - Invalid non-empty JSON fails migration with the source table name and source row ID.

This rule is required because current `translation_records.ai_feedback_json` can be an empty string, which cannot be cast to PostgreSQL `jsonb`.

### MinIO Audio Storage Layer

Add a small storage boundary instead of letting CLI/admin code write files directly.

Recommended files:

- `internal/storage/blob_store.go`: `AudioBlobStore` interface, range/read option structs, object metadata structs, and compile-time implementation expectations.
- `internal/storage/s3_blob_store.go`: MinIO/S3-compatible client setup, idempotent `PutIfAbsent`, `GetObject`, ranged `GetObject`, `StatObject`, `RemoveObject`, paged `ListObjects`, and `PresignedGetObject`.
- `internal/module/audio/model.go`: `AudioObject`, object kind constants, and metadata structs.
- `internal/store/contracts.go`: `AudioObjectStoreInterface` contract.
- `internal/data/postgres/audio_store.go`: PostgreSQL persistence for `audio_objects`.
- `internal/module/audio/service.go`: object key generation, upload transaction coordination, signed/proxy URL generation, and orphan cleanup.
- `internal/module/audio/handler.go`: authenticated audio streaming endpoints.

Configuration:

```text
AUDIO_OBJECT_STORE=s3
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET_AUDIO=japanese-learning-audio
MINIO_USE_SSL=false
MINIO_PUBLIC_ENDPOINT=
MINIO_PRESIGN_TTL_SECONDS=900
AUDIO_IMPORT_TIMEOUT_SECONDS=10
AUDIO_IMPORT_MAX_BYTES=52428800
```

`AUDIO_OBJECT_STORE=s3` means an S3-compatible object store, with MinIO as the supported local and production deployment in this design. Additional values should not be added until they have the same streaming, reconciliation, backup, and test coverage.

Local development should add a Makefile target or compose service for MinIO and bucket creation.

Implementation rules:

- Only `internal/storage/s3_blob_store.go` should import the MinIO/S3 SDK.
- CLI commands, admin handlers, module services, and migration code should call `internal/module/audio/service.go`, not the object-store SDK.
- Unit tests can provide an in-memory or temp-file `AudioBlobStore` fake, but production startup should reject filesystem durable storage.
- Integration tests should still exercise the real MinIO/S3-compatible implementation because fake stores cannot prove range, metadata, presign, bucket policy, or reconciliation behavior.

Audio URL behavior:

- PostgreSQL stores `audio_object_id`, not public URLs.
- API responses may keep existing JSON fields such as `audio_url` and `audio_ref`, but those values are derived at response time.
- Recommended derived URL: `/api/v1/audio/{audio_object_id}/stream`.
- The handler verifies auth, checks object visibility/ownership, then streams from MinIO or, for content audio only, redirects to a short-lived presigned URL.
- User recordings must require `owner_user_id = current_user_id` unless the requester is an admin.
- Go response models can keep `AudioURL string json:"audio_url"` and `AudioRef string json:"audio_ref"` for compatibility, but persistence structs and SQL should use `audio_object_id`.
- External provider endpoints such as vLLM `/v1/audio/speech` and `/v1/audio/transcriptions` remain integration points only. The application stores generated or uploaded audio objects in MinIO, not provider URLs.
- Runtime joins that derive `audio_url` or `audio_ref` must include `audio_objects.deleted_at IS NULL`.
- If a business row still references a deleted audio object, API responses should return an empty audio field and the stream endpoint should return `404` or `410`; it must not return a playable URL for soft-deleted objects.

Streaming and presigned URL rules:

- `visibility = 'private'` objects, including user recordings, must use backend proxy streaming by default. Do not redirect private recordings to presigned URLs because the URL can be forwarded during its TTL.
- `visibility = 'authenticated'` content audio may use presigned URLs only when `MINIO_PUBLIC_ENDPOINT` is configured and reachable by browsers.
- If `MINIO_PUBLIC_ENDPOINT` is empty, `localhost`, a private network address, or an internal DNS name, use backend proxy streaming for all browser playback.
- The stream endpoint must support `GET` and `HEAD`.
- The stream endpoint must support HTTP `Range` requests and return correct `206 Partial Content` or `416 Range Not Satisfiable` responses.
- Required playback headers: `Content-Type`, `Content-Length` or correct range length, `Accept-Ranges: bytes`, `Content-Range` for partial responses, and stable `ETag` or `Last-Modified` where available.
- Cache policy should be explicit. Generated immutable content audio can be cached privately with a long max age. Private user recordings should use `Cache-Control: private, no-store` unless product requirements say otherwise.

All current audio producers should depend on the audio service:

- `GenerateWordAudio*` uploads word TTS to MinIO and updates `words.audio_object_id`.
- `GenerateTTSFilesWithStats` becomes target-aware; it uploads example/material TTS and updates `word_examples.audio_object_id`, `grammar_examples.audio_object_id`, or `speaking_materials.audio_object_id`.
- `admin.regenerateAudio` updates the target row's `audio_object_id` instead of writing under `./data/audio`.
- `admin.batchGenerateAudio` resolves target rows from database IDs/filters, uploads each generated object, and links each target.
- `import-words`, `import-grammar`, and `import-speaking` call the same audio service when `--generate-audio` is set.
- `scripts/generate_tts_examples` should either be removed or rewritten to use the same service; it should no longer write `audio_map.json` as the source of truth.
- Existing `--out` flags can remain temporarily for dry-run/debug output only, but production generation should ignore local durable paths and return linked `audio_object_id` values.

Frontend changes:

- Remove hash-based URL construction such as `/audio/examples/{hash}.wav`.
- Use `audio_url` returned by word/example/grammar/speaking/lesson APIs.
- Keep browser speech synthesis fallback when `audio_url` is empty or playback fails.
- Remove Vite `/audio` proxy assumptions after the backend stream endpoint is available.
- Update both React/Admin frontends and legacy `front/web/static/js` pages that set `<audio src>` directly.
- Extend API types so `WordExample`, `GrammarExample`, `SpeakingMaterial`, `LessonSummary`, and `SpeakingRecord` can receive derived audio fields from the backend.
- Change React helpers from "compute URL from text hash" to "play provided URL first, then browser TTS fallback". Example playback should accept an optional `audio_url` from the API.
- Admin audio controls should pass target identity, not only raw text. Valid targets are `word`, `word_example`, `grammar_example`, `speaking_material`, and `lesson`.
- If checked-in compiled legacy assets under `front/web/static/js/dist` are still shipped, regenerate them in the same change that updates `front/web/static/js`.

Speaking recording changes:

- Current self-rated practice can keep `speaking_records.audio_object_id = NULL`.
- When real recording upload/scoring is enabled, `POST /api/v1/speaking/practice` should accept multipart audio, upload the user recording to MinIO, set `audio_objects.kind = 'user_recording'`, set `visibility = 'private'`, set `owner_user_id`, then save `speaking_records.audio_object_id`.
- The browser's local object URL remains only a pre-submit preview and is never persisted.
- The scorer can receive the uploaded bytes directly in memory, but the request path should still persist the submitted recording to MinIO when the product needs playback/audit/history.

User deletion and private audio:

- `audio_objects` enforces `CHECK (visibility <> 'private' OR owner_user_id IS NOT NULL)`.
- The owner FK uses `ON DELETE RESTRICT`, so a hard user delete cannot silently create ownerless private objects.
- PostgreSQL design keeps current hard-delete semantics for admin/user deletion. Do not introduce `users.deleted_at` unless a separate account-deactivation feature is designed.
- Because `users.email` remains globally unique, deleting a user removes the row and allows that email to register again.
- Auth and profile queries do not need soft-delete filters under this design; if soft deletion is later added, every auth/profile/list query must filter `deleted_at IS NULL` and `email` must become a partial unique index.
- Run an account-deletion workflow before deleting the user row:
  1. Find `audio_objects` where `owner_user_id = user_id`.
  2. Delete the corresponding MinIO objects or enqueue durable deletion jobs.
  3. Delete the matching private `audio_objects` rows after object deletion succeeds.
  4. Delete or anonymize dependent practice records according to product policy.
  5. Hard-delete the user only after private audio cleanup has completed.
- Failed MinIO deletion should keep the user row and `audio_objects` rows in place so the operation can be retried; do not create ownerless private audio as a fallback.

Legacy audio import safety:

- Only `http` and `https` URLs are allowed for legacy remote audio import. Reject `file:`, `data:`, `ftp:`, and other schemes.
- Apply SSRF checks before every request and after every redirect: reject loopback, link-local, private RFC1918 ranges, carrier-grade NAT, multicast, IPv6 unique-local/link-local, and cloud metadata addresses such as `169.254.169.254`.
- Resolve DNS and validate every resolved IP. If redirects are allowed, limit them to a small fixed count and re-validate scheme, host, and resolved IP on each hop.
- Set request timeout, response-header timeout, and a hard `AUDIO_IMPORT_MAX_BYTES` limit. Abort when `Content-Length` exceeds the limit or when streaming bytes exceed it.
- Accept only known audio MIME types, for example `audio/wav`, `audio/x-wav`, `audio/mpeg`, `audio/mp4`, `audio/ogg`, and `audio/webm`. Sniff the first bytes because `Content-Type` can lie.
- Local legacy paths must be resolved with `filepath.Clean`, joined under `--audio-root`, and then checked with `filepath.Rel` or equivalent so the final path remains inside `--audio-root`.
- Resolve symlinks for both `--audio-root` and the candidate file before reading. Reject paths that escape the resolved root.
- Import should preserve rejected references in the missing/invalid-audio report without attempting unsafe fallback behavior.
- Do not store raw legacy URLs or local paths in `audio_objects.metadata`.
- Strip URL userinfo, query string, and fragment before storing any reference metadata.
- Store only a sanitized `scheme://host/path` plus `original_ref_hash = sha256(raw_ref)` when traceability is needed.
- Full raw references may appear only in a controlled local migration report, not in normal application logs, relational database rows, or MinIO object metadata.

### Audio Touchpoint Coverage

| Current Touchpoint | Current Behavior | New MinIO Behavior |
|---|---|---|
| `words.audio_url` | Stores a local filename under `/audio/words` | Replace with `words.audio_object_id`; API derives `audio_url` |
| `word.examples` playback | Frontend computes `/audio/examples/{hash}.wav` | `word_examples.audio_object_id`; API returns example `audio_url` |
| `grammar.examples` playback | Admin/learner computes or assumes example hash file | `grammar_examples.audio_object_id`; API returns example `audio_url` |
| `lessons.audio_url` | Stores direct/local/external URL | `lessons.audio_object_id`; API derives `audio_url` |
| `speaking_materials.audio_url` | Stores direct/local/external URL | `speaking_materials.audio_object_id`; API derives `audio_url` |
| `speaking_records.audio_ref` | Stores a local recording path | `speaking_records.audio_object_id`; API derives private `audio_ref` |
| Go API models | `AudioURL` / `AudioRef` look like persisted paths | Keep JSON fields, populate them from `audio_object_id` at response time |
| `backend/cmd/server` `/audio` file server | Serves `./data/audio` directly | Replace with authenticated `/api/v1/audio/{id}/stream` |
| CLI word/example TTS | Writes WAV files to `./data/audio` | Uploads to MinIO and links target rows |
| Admin single/batch TTS | Writes WAV files and returns `/audio/...` | Uploads immutable object and returns stream URL |
| React `WordReviewPage` / `GrammarDetailPage` | Calls `speakExample(text)` and computes a hash path | Pass API-provided example `audio_url`; fallback to browser TTS |
| React `SpeakingPage` | Plays generated TTS by hashing material text; self-rating submit omits audio | Play `selectedMaterial.audio_url`; multipart submit can include recorder blob |
| React `useAudioRecorder` | Returns local preview `blob:` URL | Keep preview behavior; upload the `Blob` through speaking practice submit when enabled |
| Frontend `exampleAudio.ts` and admin `audioHash.ts` | Builds hash-based audio URLs | Remove URL builders or limit them to migration/debug tooling |
| Admin Words/Grammar/Speaking pages | Batch payload sends text lists and assumes generated path | Send target IDs/types and refresh returned derived URLs |
| Browser recording preview | Uses local `blob:` object URL | Stays local until user submits; submitted audio uploads to MinIO |
| vLLM/SBV/Gradio TTS clients | Return raw audio bytes or temporary provider URLs | Keep as source clients; immediately normalize bytes into MinIO objects |
| vLLM ASR scorer | Receives uploaded bytes for transcription | Continue in-memory scoring; MinIO stores the recording when retention is required |

### Review Event Write Path

Every review operation must update the current-state table and append the event table through `StoreRuntime.Transact` in the same relational transaction.

Word review transaction:

1. Enter `StoreRuntime.Transact`.
2. Upsert `word_records` for `(user_id, word_id)`.
3. Insert one row into `word_review_events`.
4. Commit by returning nil from the transaction callback.

Grammar quiz transaction:

1. Enter `StoreRuntime.Transact`.
2. Upsert `grammar_records` for `(user_id, grammar_point_id)`.
3. Insert one row into `grammar_quiz_attempts`.
4. Commit by returning nil from the transaction callback.

Note review transaction:

1. Enter `StoreRuntime.Transact`.
2. Update the note SRS fields on `notes`.
3. Insert one row into `note_review_events`.
4. Commit by returning nil from the transaction callback.

The old JSON fields can still be assembled for API compatibility by querying recent events and mapping them into the existing Go response structs. New code must not keep appending to `review_history_json` or `quiz_history_json`; those columns should not exist in the PostgreSQL schema.

---

## Data Migration From Existing SQLite

Create a one-time migration command:

```text
go run ./backend/cmd/storage-migrate migrate-sqlite-to-relational \
  --sqlite ./data/app.db \
  --target-store postgres \
  --database "$DATABASE_URL" \
  --audio-root ./data/audio \
  --minio-endpoint "$MINIO_ENDPOINT" \
  --minio-bucket "$MINIO_BUCKET_AUDIO" \
  --migration-run-id "2026-06-16T120000Z" \
  --audio-import-timeout 10s \
  --audio-import-max-bytes 52428800
```

This command should live in a dedicated migration binary, not inside the HTTP server binary. Recommended command path:

- `backend/cmd/storage-migrate/main.go`
- `internal/migration/sqlitecopy` for command implementation, read-only SQLite extraction, duplicate/remap planning, and copy orchestration
- `internal/store/migration.go` for adapter-neutral migration DTOs and manifest comparison types
- `internal/data/postgres/migration_target.go` for PostgreSQL target bulk loading and manifest extraction

The server and admin binaries may run online schema migrations at startup, but they should not expose the one-shot SQLite data copy command. Offline migration flags such as `--sqlite`, `--audio-root`, `--allow-missing-audio`, and `--migration-run-id` belong to `storage-migrate` only. The `storage-migrate` dependency graph is the only place that may import the SQLite driver after cutover.

Rerun strategy:

- The migration command is an empty-target, one-shot cutover tool.
- It is not designed to resume from an arbitrary half-migrated relational target/MinIO state.
- Before copying data, it must verify that target content tables and `audio_objects` are empty. Schema tables such as `schema_migrations` may already exist.
- Every MinIO object created by the command must include the `migration_run_id` in metadata and/or in a managed key prefix.
- If migration fails before cutover, drop and recreate the target adapter schema, then delete or quarantine all MinIO objects for that `migration_run_id`, then rerun from SQLite.
- If migration fails after cutover, restore the relational database and MinIO from the paired backup manifest instead of rerunning the copy command over live data.
- Do not add a `--resume` mode until there is a tested checkpoint model for both target relational rows and MinIO objects.

Expected manifest:

Before writing target relational rows, the migration command must compute a source expected manifest from read-only SQLite plus the planned remap/canonicalization decisions.

The manifest should include:

- Canonical parent counts for `words`, `grammar_points`, `lessons`, `speaking_materials`, `writing_questions`, `translation_sources`, and users.
- Child table counts after JSON splitting: word examples, grammar examples, grammar quiz questions, lesson sentences, lesson words, translation sentences, note links, and daily goals.
- Append-only event counts expanded from legacy review history JSON.
- Duplicate reports and remap cardinality for words, grammar points, lessons, speaking materials, and writing questions.
- Expected counts for collapsed user records after duplicate remap merges.
- Audio import expectations: referenced audio count, imported count, allowed-missing count, rejected unsafe reference count, and MinIO object count by kind.
- Expected orphan counts after remap. All FK/soft-reference orphan counts should be zero unless an explicit allow flag says otherwise.

After migration, generate a target actual manifest from the selected relational adapter's `MigrationTarget` and MinIO, then compare it with the source expected manifest. The migration succeeds only when every manifest item matches or is explicitly allowed by a migration flag such as `--allow-missing-audio`.

Migration target API:

- `storage-migrate` must resolve the selected adapter and assert that it satisfies `MigrationCapableAdapter`.
- Runtime `StoreRuntime` is not used for preserve-ID bulk copy. It is for normal business behavior.
- The adapter-local `MigrationTarget` owns preserve-ID inserts, identity override syntax, bulk child-table inserts, sequence reset, target-empty checks, and target actual manifest queries.
- `VerifySchemaOnly` must define "empty target" as: all application/business tables are empty. This includes content tables, users, password reset tokens, user records, review events, notes, sessions, summaries, translation tables, `audio_objects`, and any join/child tables.
- `VerifySchemaOnly` may allow only adapter metadata tables such as `schema_migrations` and other explicitly documented migration bookkeeping tables to contain rows.
- If any non-metadata table contains rows, migration must abort before writing PostgreSQL rows or MinIO objects. Do not merge SQLite data into a partially used target.
- For PostgreSQL, `MigrationTarget` uses `OVERRIDING SYSTEM VALUE`, adapter-local batch insert helpers, and sequence reset SQL inside `internal/data/postgres/migration_target.go`.
- `BulkLoader` operations must run inside an adapter-owned transaction or clearly documented phase transaction. If any insert/validation step fails, the phase rolls back and the rerun strategy remains "drop/recreate target schema plus clean migration MinIO prefix".

Migration phases:

1. Open SQLite strictly read-only and the selected relational target.
2. Run target adapter schema migrations only. Do not run seed files in SQLite migration mode.
   - The target must be schema-only and content-empty before copy starts.
   - It is acceptable for `schema_migrations` to contain applied schema versions.
   - Abort if any non-metadata application table contains rows, including users, records, tokens, sessions, translation tables, child tables, join tables, or `audio_objects`.
3. Copy source IDs using the target adapter's identity override mechanism for every table where the SQLite ID is preserved. For the PostgreSQL adapter, use this shape for batch inserts:
   ```sql
   INSERT INTO words (id, kanji_form, reading, ...)
   OVERRIDING SYSTEM VALUE
   VALUES (...);
   ```
   For the PostgreSQL adapter, after each copied identity table, reset its identity sequence before normal application writes resume:
   ```sql
   SELECT setval(pg_get_serial_sequence('words', 'id'), COALESCE((SELECT MAX(id) FROM words), 0) + 1, false);
   ```
4. Build duplicate word ID mappings in memory without modifying SQLite:
   - Group source words by `(kanji_form, reading)`.
   - Compare duplicate rows after normalizing fields that can be represented differently but mean the same thing, such as JSON whitespace in `examples_json`.
   - Treat these fields as content-bearing and include them in the duplicate comparison: `part_of_speech`, `meaning`, `examples_json`, `jlpt_level`, `reading_type`, and legacy `audio_url`.
   - If duplicate rows are content-identical except for ID/timestamps, pick the smallest source `id` as the canonical word ID.
   - If duplicate rows differ in content, abort migration and write a duplicate-word report. Do not silently discard later meanings, examples, audio references, or reading metadata.
   - Record `source_word_id -> canonical_word_id` for every source word, including canonical rows that map to themselves.
   - Copy only canonical word rows into PostgreSQL.
5. Build natural-key duplicate maps for other content tables before copying them:
   - `grammar_points`: group by `(name, jlpt_level)`, compare `meaning`, `conjunction_rule`, `usage_note`, `examples_json`, and `quiz_questions_json`. Record `source_grammar_point_id -> canonical_grammar_point_id`. Remap `grammar_records.grammar_point_id`, expanded `grammar_quiz_attempts.grammar_point_id`, `writing_questions.grammar_point_id`, and `notes.reference_id` when `reference_type = 'grammar'`.
   - `lessons`: group by `(title, jlpt_level)`, compare `tags_json`, `content_furigana_json`, `translation_json`, `audio_url`, `sentence_timestamps_json`, `char_count`, and `word_ids_json` after applying the global word ID remap to `word_ids_json`. Record `source_lesson_id -> canonical_lesson_id`. Copy child `lesson_sentences` and `lesson_words` only from canonical lessons.
   - `speaking_materials`: group by `(type, title, jlpt_level)`, compare `text`, `lines`, and `audio_url`. Record `source_speaking_material_id -> canonical_speaking_material_id`. Remap `speaking_records.material_id`.
   - `writing_questions`: group by `(type, prompt)`, compare `expected_answer`, normalized `grammar_point_id` after grammar remap and `0 -> NULL`, and `jlpt_level`. Record `source_writing_question_id -> canonical_writing_question_id` if IDs are ever referenced by future records; current records store question text.
   - For every table above, content-identical duplicates keep the smallest source ID as canonical. Content differences abort migration and write a duplicate report for that table.
6. Apply the word ID remap as a global rule.
   Every source word ID must pass through the duplicate map before it is inserted into PostgreSQL. This includes:
   - `word_records.word_id`
   - `word_bookmarks.word_id`
   - expanded `word_review_events.word_id`
   - `lessons.word_ids_json -> lesson_words.word_id`
   - `grammar_examples.linked_word_ids`
   - `notes.reference_id` when `reference_type = 'word'`
   If a source word ID is not present in the map, abort migration with the source table and row ID.
7. Copy parent content tables first.
   - Copy canonical `words`.
   - Copy canonical `grammar_points`, `lessons`, `speaking_materials`, `writing_questions`, `translation_sources`, and other parent content.
   - Convert `writing_questions.grammar_point_id = 0` to SQL `NULL`; remap non-zero grammar point IDs through the grammar duplicate map and preserve them as nullable references according to the existing soft-reference behavior.
8. Split JSON examples and ordered arrays into child tables:
   - `words.examples_json` to `word_examples`
   - `grammar_points.examples_json` to `grammar_examples`, using canonical grammar points only.
   - `grammar_points.quiz_questions_json` to `grammar_quiz_questions`, using canonical grammar points only.
   - `lessons.content_furigana_json` to `lesson_sentences`
   - `lessons.word_ids_json` to `lesson_words`, preserving the JSON array order in `lesson_words.position` and remapping every word ID through the global word ID map before insert.
   - `grammar_examples.linked_word_ids`, remapping every word ID through the global word ID map before storing the array.
9. Copy users and convert:
   - `jlpt_levels` JSON string to `text[]`
   - `daily_goals_json` to `user_daily_goals`, including `translation` when present and defaulting it through the same product default-goal policy as other modules.
10. Migrate existing content audio references into MinIO and create `audio_objects`:
   - Normalize every legacy audio reference before import: trim whitespace, strip `/audio/` prefixes, resolve relative paths under `--audio-root`, and store only sanitized reference metadata plus `original_ref_hash`.
   - Apply the legacy audio import safety rules for every remote URL and local path before reading bytes.
   - `words.audio_url`: read `--audio-root/words/{filename}`, upload to `tts/words/...`, and set `words.audio_object_id`.
   - `word_examples`: compute the old `sha256(japanese)[:16].wav` path under `--audio-root/examples`; if present, upload and set `word_examples.audio_object_id`.
   - `grammar_examples`: compute the same old example hash path; if present, upload and set `grammar_examples.audio_object_id`.
   - `speaking_materials.audio_url`: if it is an HTTP(S) URL, download then upload and store only sanitized URL metadata plus `original_ref_hash`; if it is a relative/local path, read from disk then upload; set `speaking_materials.audio_object_id`.
   - `lessons.audio_url`: apply the same external/local import rule and set `lessons.audio_object_id`.
   - Do not keep external URLs as runtime playback sources after migration; all runtime playback should flow through the backend audio endpoint.
   - Missing audio that is referenced by the SQLite database should fail migration by default; provide an explicit `--allow-missing-audio` flag to leave the target `audio_object_id` NULL and print a missing-audio report.
11. Copy word-related user data with duplicate ID remapping:
   - For `word_records`, rewrite `word_id` through the duplicate map.
   - If several source records collapse into the same `(user_id, canonical_word_id)`, merge them:
     - `mastery_level`: keep the maximum value.
     - `next_review_at`: keep the earliest due time to avoid hiding due reviews.
     - `updated_at`: keep the latest timestamp.
     - `ease_factor` and `interval`: keep the values from the source row with the latest `updated_at`.
     - review events: expand and preserve events from every merged source history.
   - For `word_bookmarks`, rewrite `word_id` through the duplicate map and insert with `ON CONFLICT DO NOTHING`.
12. Copy grammar current-state tables and expand grammar quiz history JSON into `grammar_quiz_attempts` where enough data exists.
    - Remap every `grammar_point_id` through the grammar duplicate map before insert.
    - If several source records collapse into the same `(user_id, canonical_grammar_point_id)`, merge deterministically and preserve quiz history events.
13. Copy notes in one clear order:
   - Copy `notes` first. If `reference_type = 'word'`, rewrite `reference_id` through the global word ID map before insert.
   - If `reference_type = 'grammar'`, rewrite `reference_id` through the grammar duplicate map before insert.
   - Copy `note_links` after both source and target notes exist.
   - Expand note review history JSON into `note_review_events` after notes exist.
14. Copy practice records and convert AI feedback JSON text to `jsonb`:
   - Remap every `speaking_records.material_id` through the speaking material duplicate map before insert.
   - `writing_records.ai_feedback_json = 'null'` becomes SQL `NULL`.
   - `translation_records.ai_feedback_json = ''` becomes SQL `NULL`.
   - Valid non-empty feedback JSON becomes `jsonb`.
   - Invalid non-empty feedback JSON aborts migration with source row details.
15. Migrate existing user recording references:
   - `speaking_records.audio_ref`: import local files when present, create `audio_objects.kind = 'user_recording'`, set `visibility = 'private'`, set `owner_user_id`, and update `speaking_records.audio_object_id`.
   - Missing user recording files follow the same default-fail and explicit `--allow-missing-audio` behavior.
16. Copy session data and convert summary JSON fields.
17. Reset all PostgreSQL identity sequences again after the full copy and before enabling application writes.
18. Run validation queries.

Validation must be manifest-driven.

The command should write two files:

- `migration_expected_manifest.json`: computed from SQLite before writes.
- `migration_actual_manifest.json`: computed from the selected relational target plus MinIO after writes.

The comparison should fail if any normalized count, remap count, event count, audio count, or orphan count differs unexpectedly.

Supporting validation queries:

```sql
SELECT COUNT(*) FROM words;
SELECT COUNT(*) FROM word_examples;
SELECT COUNT(*) FROM grammar_points;
SELECT COUNT(*) FROM grammar_examples;
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM audio_objects WHERE deleted_at IS NULL;
SELECT COUNT(*) FROM word_records;
SELECT COUNT(*) FROM word_bookmarks;
SELECT COUNT(*) FROM notes WHERE deleted_at IS NULL;

-- Must be 0: grammar examples cannot reference missing words.
SELECT COUNT(*)
FROM grammar_examples ge
WHERE EXISTS (
    SELECT 1
    FROM unnest(ge.linked_word_ids) AS ids(word_id)
    LEFT JOIN words w ON w.id = ids.word_id
    WHERE w.id IS NULL
);

-- Must be 0: word notes cannot reference missing words.
SELECT COUNT(*)
FROM notes n
LEFT JOIN words w ON w.id = n.reference_id
WHERE n.reference_type = 'word'
  AND n.deleted_at IS NULL
  AND w.id IS NULL;

-- Must be 0: grammar notes cannot reference missing grammar points.
SELECT COUNT(*)
FROM notes n
LEFT JOIN grammar_points gp ON gp.id = n.reference_id
WHERE n.reference_type = 'grammar'
  AND n.deleted_at IS NULL
  AND gp.id IS NULL;

-- Must be 0: writing questions cannot reference missing grammar points.
SELECT COUNT(*)
FROM writing_questions wq
LEFT JOIN grammar_points gp ON gp.id = wq.grammar_point_id
WHERE wq.grammar_point_id IS NOT NULL
  AND gp.id IS NULL;
```

Schema validation should also assert that required constraints and indexes exist:

- Unique constraints for natural keys: `words`, `grammar_points`, `lessons`, `speaking_materials`, `writing_questions`, and `translation_sentences`.
- FK constraints for concrete relationships such as records, bookmarks, lesson children, note links, translation records, and session summaries.
- Query indexes listed in the Required Indexes section, including `idx_session_summaries_user_generated`.
- FK-side indexes for child tables used by `CASCADE`, `SET NULL`, and `RESTRICT` checks. Do not rely on partial query indexes that exclude deleted rows for FK enforcement paths.
- `pg_trgm` extension availability before trigram search indexes are created.
- `trg_translation_sentences_immutable_prompt` or an equivalent database-level guard exists before translation re-import is enabled.

Application validation:

- Login/register/reset password flow works.
- Word list, review queue, and bookmark flow work.
- Word, example, grammar example, speaking material, lesson, and user recording audio stream from MinIO-backed API URLs.
- Grammar list with status and quiz submission work.
- Lesson list and detail page return ordered sentences.
- Notes list, tag filter, backlinks, and search work.
- Speaking, writing, translation records can be created and listed.
- Admin CRUD pages can insert, update, delete, and search records.
- Admin handlers use `AdminStores` and services rather than concrete stores or raw `*sql.DB`.
- Admin single/batch audio generation uploads to MinIO, links target rows, and returns playable URLs.
- CLI audio generation and import-time `--generate-audio` use MinIO and do not create durable files under `./data/audio`.
- CLI import and seed commands use `StoreRuntime` and shared import services rather than passing raw `*sql.DB` through helper functions.
- CLI import commands remain idempotent.

---

## Backup, Restore, and Cutover

The selected relational database and MinIO must be treated as one logical storage system after this migration. Backing up only one side can create broken `audio_object_id` references or orphan objects.

Backup policy:

- Take relational database logical dumps or volume snapshots and MinIO bucket snapshots/versioned backups on the same release cadence.
- Record a backup manifest containing relational adapter name, database backup ID, MinIO bucket/version marker, application version, migration version, and timestamp.
- Enable MinIO bucket versioning in production if operationally available.
- Include `audio_objects` in database backups; it is the authoritative manifest for object references.

Restore policy:

1. Restore the MinIO bucket or versioned prefix first.
2. Restore the relational database from the matching backup manifest.
3. Run `audio-reconcile --dry-run`.
4. Fail the restore validation if any non-deleted `audio_objects` row points to a missing MinIO object.
5. Report MinIO objects with no database row as orphans and clean them only after manual approval or an explicit cleanup flag.

Cutover policy:

- Freeze writes and audio generation during the final SQLite-to-relational migration window.
- Keep the SQLite source and original `./data/audio` tree read-only until the relational target and MinIO validation passes.
- Write migrated objects under a migration run prefix or store `migration_run_id` metadata so a failed migration can be cleaned without scanning unrelated production objects.
- If cutover fails before traffic is switched, keep serving from SQLite/local audio and delete or quarantine objects from the failed migration run.
- If cutover fails after traffic is switched, use the backup manifest to restore both the relational database and MinIO to the same point in time, then run reconciliation before reopening traffic.

---

## Implementation Plan Outline

### Phase 1: Relational Adapter and MinIO Infrastructure

Files to change:

- `go.mod`
- `internal/data/db.go` or replacement bootstrap helpers
- Create: `internal/store/contracts.go`
- Create: `internal/store/adapter.go`
- Create: `internal/store/errors.go`
- Create: `internal/store/migration.go`
- Create: `internal/data/postgres/db.go`
- Create or move: `internal/data/postgres/*_store.go`
- Create: `internal/data/postgres/migration_target.go`
- Create: `internal/store/testsuite`
- `internal/config/config.go`
- Create: `internal/storage/blob_store.go`
- Create: `internal/storage/s3_blob_store.go`
- `backend/cmd/server/main.go`
- `backend/cmd/admin/main.go`
- Optional create: `backend/cmd/appctl/main.go` for normal non-server CLI commands.
- Create: `backend/cmd/storage-migrate/main.go`
- `internal/cli/root.go` for normal appctl-style commands only; it must not be imported by the server binary.
- `Makefile`
- `README.md`

Work:

- Add `pgx/v5/stdlib`.
- Add MinIO/S3-compatible client dependency.
- Replace `DB_PATH` with `RELATIONAL_STORE` plus `DATABASE_URL`.
- Replace `Config.DBPath` with `RelationalStore` and `DatabaseURL`.
- Add a relational adapter registry with `postgres` as the first supported adapter.
- Move PostgreSQL SQL stores into the PostgreSQL adapter package or wrap existing stores behind adapter constructors during the transition.
- Move shared contracts and adapter registry into `internal/store` so `internal/data/postgres`, admin handlers, CLI commands, and migration commands all depend inward on the same neutral package.
- Remove server runtime dependency on `internal/cli`; the server binary should only serve HTTP after parsing server flags/config.
- Ensure SQLite driver imports are absent from server/admin/appctl binaries and present only in `backend/cmd/storage-migrate` through `internal/migration/sqlitecopy`.
- Replace `Config.AudioStorePath` with MinIO configuration fields and temporary migration-only audio root flags.
- Add `AudioObjectStore` or `AudioBlobStore` configuration with `s3` as the only supported production value for this migration.
- Add `AppTimezone` or equivalent configuration for product-day calculations.
- Add `DBMaxOpenConns`, `DBMaxIdleConns`, and `DBConnMaxLifetime` configuration with process-appropriate defaults.
- Add PostgreSQL connection pool setup.
- Add MinIO configuration loading and startup validation.
- Build and inject `StoreRuntime` from the selected relational adapter.
- Wire the S3/MinIO object-store implementation through application startup rather than constructing it inside handlers or CLI commands.
- Move one-shot SQLite data migration wiring into `backend/cmd/storage-migrate`; do not hang it off the HTTP server command.
- Add migration tracking with `schema_migrations`.
- Add PostgreSQL migration directory.
- Add Makefile targets for local PostgreSQL, local MinIO, bucket creation, and migration.

### Phase 2: PostgreSQL Adapter Schema

Files to create:

- `internal/data/migrations/postgres/schema/001_init.sql`
- Optional: `internal/data/seeds/postgres/001_seed_words.sql`
- Additional content seed files only if they are required for local development or tests.

Work:

- Create the PostgreSQL schema from this design.
- Keep PostgreSQL DDL, migration locks, sequence reset helpers, and seed SQL inside the PostgreSQL adapter boundary.
- Keep schema migrations and seed data separate.
- Make server/admin startup and SQLite migration run schema migrations only.
- Put seed behavior behind an explicit `seed-postgres` command or local-development Makefile target.
- Seed only after verifying target content tables are empty.
- Move seed inserts behind idempotent `ON CONFLICT`.
- Avoid destructive startup migrations.

### Phase 3: MinIO Audio Storage Conversion

Files to create or modify:

- Create: `internal/module/audio/model.go`
- Create: `internal/module/audio/service.go`
- Create: `internal/module/audio/handler.go`
- Create: `internal/store/contracts.go` entry for `AudioObjectStoreInterface`
- Create: `internal/data/postgres/audio_store.go`
- Create: `internal/maintenance/audio_reconcile.go` or make `audio-reconcile` a `backend/cmd/storage-migrate` subcommand.
- Modify: `internal/cli/generate_tts.go`
- Modify: `internal/cli/tts_generator.go`
- Modify: `internal/cli/import_words.go`
- Modify: `internal/cli/import_grammar.go`
- Modify: `internal/cli/import_speaking.go`
- Modify: `internal/cli/import_lessons.go`
- Modify: `internal/module/admin/handler.go`
- Modify: `internal/module/admin/audio.go`
- Modify: `internal/module/admin/import.go`
- Modify: `internal/module/admin/words.go`
- Modify: `internal/module/admin/grammar.go`
- Modify: `internal/module/admin/speaking.go`
- Modify: `internal/module/admin/writing.go`
- Modify: `internal/module/admin/translation.go`
- Modify: `internal/module/admin/users.go`
- Modify: `internal/module/admin/records.go`
- Modify: `internal/module/word/model.go`
- Modify: `internal/module/word/service.go`
- Modify: `internal/module/word/handler.go`
- Modify: `internal/module/grammar/model.go` if response structs are split out during schema conversion.
- Modify: `internal/module/grammar/service.go`
- Modify: `internal/module/grammar/handler.go`
- Modify: `internal/module/lesson/model.go`
- Modify: `internal/module/lesson/service.go`
- Modify: `internal/module/lesson/handler.go`
- Modify: `internal/module/speaking/model.go`
- Modify: `internal/module/speaking/service.go`
- Modify: `internal/module/speaking/handler.go`
- Modify: `internal/module/speaking/vllm_scorer.go` only where recording persistence and scoring request flow intersect.
- Modify: `backend/cmd/server/main.go`
- Modify: `front/react/src/util/exampleAudio.ts`
- Modify: `front/react/src/types/api.ts`
- Modify: `front/react/src/hooks/useAudioRecorder.ts`
- Modify: `front/react/src/pages/word/WordReviewPage.tsx`
- Modify: `front/react/src/pages/grammar/GrammarDetailPage.tsx`
- Modify: `front/react/src/pages/speaking/SpeakingPage.tsx`
- Modify: `front/react/src/pages/lesson/LessonPage.tsx`
- Modify: `front/admin/src/components/AudioRegenButton/AudioRegenButton.tsx`
- Modify: `front/admin/src/util/audioHash.ts`
- Modify: `front/admin/src/util/ttsBatch.ts`
- Modify: `front/admin/src/pages/Words/Words.tsx`
- Modify: `front/admin/src/pages/Grammar/Grammar.tsx`
- Modify: `front/admin/src/pages/Speaking/Speaking.tsx`
- Modify: `front/web/static/js/speaking.ts`
- Modify: `front/web/static/js/lesson.ts`
- Regenerate or remove checked-in compiled legacy audio assets under `front/web/static/js/dist` if they are still deployed.
- Regenerate or remove checked-in frontend build artifacts under `front/dist` and `front/admin/dist` if those directories are deployed or committed intentionally.
- Modify: `front/react/vite.config.ts`
- Modify: `front/admin/vite.config.ts`
- Modify or remove: `scripts/generate_tts_examples/main.go`
- Modify: `README.md`
- Modify: `docs/admin-panel-guide.md`
- Modify: `docs/architecture.md`

Work:

- Replace direct `os.WriteFile("./data/audio/...")` writes with audio service uploads to MinIO.
- Replace admin audio `h.cfg.DB.Exec` calls with `AdminStores` and `AudioService` calls inside `StoreRuntime.Transact` when metadata and target rows must change atomically.
- Replace `/audio/...` static file serving with authenticated audio stream routes.
- Return derived `audio_url` / `audio_ref` values from API responses while storing only `audio_object_id`.
- Extend word and grammar example response models with optional `audio_url`.
- Make admin regeneration requests target-aware: `word`, `word_example`, `grammar_example`, `speaking_material`, or `lesson`.
- Make batch generation resolve target rows from database filters and update each target's `audio_object_id`.
- Update admin forms so direct `audio_url` editing becomes "attach/upload/link audio object" or is removed for generated content.
- Keep browser TTS fallback when MinIO-backed playback fails.
- Keep current self-rated speaking practice compatible with no uploaded recording; add multipart upload path for real recordings.
- Keep TTS configuration UI focused on provider settings; MinIO object location is backend-owned and not editable by users.
- Treat `front/react/src`, `front/admin/src`, and `front/web/static/js` as source of truth. If `front/dist`, `front/admin/dist`, or `front/web/static/js/dist` are deployed artifacts, regenerate them from source; otherwise remove or ignore them consistently.
- Before cutover, run a repository scan for stale direct audio paths and hash URL helpers, for example `rg '/audio/|audioHash|sha256'`, and review an allowlist for legitimate hashing such as MinIO object keys and non-audio code.

### Phase 4: Relational Store Adapter Conversion

Files to change:

- `internal/store/contracts.go`
- `internal/store/adapter.go`
- `internal/store/errors.go`
- `internal/store/migration.go`
- `internal/data/postgres/user_store.go`
- `internal/data/postgres/word_store.go`
- `internal/data/postgres/grammar_store.go`
- `internal/data/postgres/lesson_store.go`
- `internal/data/postgres/note_store.go`
- `internal/data/postgres/session_store.go`
- `internal/data/postgres/speaking_store.go`
- `internal/data/postgres/writing_store.go`
- `internal/data/postgres/translation_store.go`
- `internal/data/postgres/audio_store.go`
- `internal/module/summary/model.go`
- `internal/module/admin/handler.go`
- `internal/module/admin/*.go`
- `internal/store/testsuite/*_test.go`
- `internal/data/postgres/*_test.go` files that currently create `audio_url` or `audio_ref` columns.
- CLI import helpers under `internal/cli`

Work:

- Define shared module store interfaces, admin store interfaces, and `StoreRuntime` in the neutral `internal/store` package used by server, admin, CLI, migration, and tests.
- Replace admin `HandlerConfig` concrete store fields and raw `*sql.DB` with `AdminStores`, `AppStores` where needed, and services such as `AudioService`.
- Register the `postgres` relational adapter and move PostgreSQL-specific construction behind `NewStoreRuntime`.
- Replace placeholders inside the PostgreSQL adapter.
- Replace timestamp functions inside the PostgreSQL adapter.
- Replace insert ID handling with `RETURNING id` inside the PostgreSQL adapter.
- Replace SQLite conflict syntax inside the PostgreSQL adapter.
- Rewrite JSON/list access for PostgreSQL arrays, child tables, and `jsonb`.
- Rewrite review writes so current-state updates and event inserts happen in one transaction.
- Join or post-process `audio_objects` to produce response URLs without exposing MinIO object keys.
- Rewrite today-completed and daily-goal statistics to use Go-computed `[start, end)` time windows in `APP_TIMEZONE`.
- Treat `writing_questions.grammar_point_id` as nullable across store methods, Go models, admin forms, and import paths.
- Keep user deletion as hard delete, but require private audio cleanup before deleting the user row.
- Add `translation` to summary/user-goal module handling.
- Rewrite `translation_records` saves as append-only `INSERT ... RETURNING id`.
- Validate array and polymorphic soft references before admin/store writes.
- Add shared adapter contract tests for CRUD, admin CRUD/pagination/search, list filters, duplicate-key behavior, missing-row behavior, transactions, migrations, and date-window logic.
- Add transaction contract tests that prove commit on nil callback, rollback on returned error, rollback then re-panic on callback panic, no visibility of rolled-back rows, and `ErrNestedTransaction` for default nested transaction attempts.
- Add error translation tests for `ErrNotFound`, `ErrDuplicate`, `ErrConstraint`, and `ErrRetryable`.
- Remove SQLite-specific error imports.

### Phase 5: Data Migration Command

Files to create or modify:

- `backend/cmd/storage-migrate/main.go`
- `internal/migration/sqlitecopy`
- `internal/store/migration.go`
- `internal/data/postgres/migration_target.go`
- `internal/maintenance/audio_reconcile.go` if reconciliation is shared outside `storage-migrate`

Work:

- Read from SQLite in `internal/migration/sqlitecopy` and write through the selected adapter's `MigrationTarget`. The current supported target adapter is `postgres`.
- Keep raw source SQLite handles inside `internal/migration/sqlitecopy` and raw target database handles inside adapter internals. Import helpers reused by normal CLI commands should accept `StoreRuntime`/store interfaces, not `*sql.DB`.
- Keep SQLite driver imports out of `internal/cli`, server, admin, and normal CLI binaries.
- Use `MigrationTarget`/`BulkLoader` for preserve-ID copy, bulk child-table writes, sequence reset, empty-target checks, target validations, and actual manifest generation.
- Preserve source IDs for identity tables by using the target adapter's identity override mechanism. For PostgreSQL this is `OVERRIDING SYSTEM VALUE`; reset every PostgreSQL identity sequence after copy.
- Keep SQLite source read-only; handle duplicate words with a global in-memory ID remap applied to every word reference.
- Abort on duplicate `(kanji_form, reading)` rows whose content-bearing fields differ unless a future explicit merge policy is added.
- Preflight natural-key duplicates for `grammar_points`, `lessons`, `speaking_materials`, and `writing_questions`; canonicalize identical duplicates and abort on conflicting content.
- Remap `grammar_records.grammar_point_id`, `writing_questions.grammar_point_id`, grammar notes, and `speaking_records.material_id` through their canonical maps.
- Require schema-only empty relational target and clean MinIO migration run prefix before migration starts.
- Split JSON data into child tables.
- Upload existing local/external audio into MinIO and populate `audio_objects`.
- Enforce SSRF, redirect, MIME, timeout, size, and path traversal checks during audio import.
- Copy `lesson_words`, `grammar_examples.linked_word_ids`, `notes.reference_id` for word notes, `word_records`, and `word_bookmarks` only after remapping duplicate word IDs.
- Convert `writing_questions.grammar_point_id = 0` to SQL `NULL` and update Go/admin paths to treat the field as nullable.
- Normalize empty AI feedback strings before writing `jsonb`.
- Reset PostgreSQL identity sequences after copy.
- Print validation counts and fail on mismatches.
- Run `audio-reconcile --dry-run` after copy and include its summary in migration output.

### Phase 6: Tests and Verification

Testing commands should remain Makefile-first:

```text
make test
make web
```

Additional PostgreSQL-specific checks:

```text
go test ./internal/data -v -count=1
go test ./internal/store/testsuite -v -count=1
go test ./internal/data/postgres -v -count=1
go test ./internal/migration/sqlitecopy -v -count=1
go test ./internal/cli -v -count=1
go test ./backend/cmd/storage-migrate -v -count=1
go test ./internal/module/audio -v -count=1
go test ./internal/module/admin -v -count=1
go test ./internal/module/speaking -v -count=1
```

Test design:

- Prefer table-driven tests for store behavior.
- Use shared relational adapter contract tests for application store behavior.
- Use a real PostgreSQL test database for the `postgres` adapter instead of mocks.
- Keep module services testable through existing store interfaces and object-store fakes; production SQL implementations are selected through `RELATIONAL_STORE`.
- Add compile-time assertions that the PostgreSQL adapter satisfies `RelationalAdapter` and that every concrete PostgreSQL store satisfies its application-facing store interface.
- Add compile-time assertions that admin PostgreSQL stores satisfy their `Admin*StoreInterface` contracts.
- Cover `AdminStores` behavior for CRUD, pagination, search, record listing, import staging, audio target selection, and user deletion preconditions.
- Cover `StoreRuntime.Transact`: commit, rollback on returned error, rollback then re-panic on callback panic, default nested calls returning `ErrNestedTransaction`, and tx-bound stores using one connection/transaction.
- Cover error translation through `errors.Is`/`errors.As` for duplicate, not found, constraint, and retryable errors.
- Add compile-time assertions that the MinIO/S3 implementation satisfies `AudioBlobStore`.
- Cover audio service unit tests with a fake `AudioBlobStore` so upload/link/delete orchestration is tested without a network dependency.
- Use real MinIO or an S3-compatible local test container for at least one audio upload/stream integration test.
- Cover startup rejecting unsupported `RELATIONAL_STORE` values.
- Cover startup rejecting unsupported `AUDIO_OBJECT_STORE` values and filesystem durable storage in production mode.
- Cover server/admin binaries not importing the SQLite driver after cutover. The SQLite driver should appear only in the `storage-migrate` dependency graph.
- Cover `MigrationTarget` target-empty checks, preserve-ID bulk inserts, identity resets, rollback on bulk-load failure, and target actual manifest generation.
- Cover `BulkLoader` rejecting unknown `MigrationTable`/`MigrationColumn` values and rejecting columns that are not in the adapter allowlist for a table.
- Cover generated audio upsert/reuse behavior with the same `(kind, source_hash, config_hash, content_sha256)` and regeneration behavior with different content hashes.
- Cover `AudioBlobStore.PutIfAbsent`: first write creates the object, matching concurrent/existing write reuses it, mismatched hash/size/content type returns `ErrObjectConflict`, and canonical keys are never overwritten.
- Cover `ObjectStream` close behavior and `ObjectIterator` pagination/error handling, including context cancellation and reconciliation failing on partial listings.
- Cover auth rules for `visibility = 'authenticated'` content audio and `visibility = 'private'` user recordings.
- Cover private recording owner constraints and account deletion cleanup behavior.
- Cover user hard delete: private MinIO/audio objects are cleaned first, then the user row is deleted and the email can be reused.
- Cover global duplicate word ID remapping for `lesson_words`, `grammar_examples.linked_word_ids`, word notes, `word_records`, and `word_bookmarks`.
- Cover migration abort/report behavior when duplicate `(kanji_form, reading)` rows differ in content-bearing fields.
- Cover `writing_questions.grammar_point_id = 0` migrating to SQL `NULL`.
- Cover today-completed statistics around midnight using the configured application time zone.
- Cover identity preservation with `OVERRIDING SYSTEM VALUE` and sequence reset after migration.
- Cover SQLite migration refusing to run when any non-metadata target application table is not empty, including users, tokens, records, sessions, translation tables, child/join tables, and `audio_objects`.
- Cover seed command refusing to run against a non-empty content database.
- Cover natural-key duplicate canonical/remap for grammar, lessons, speaking materials, and writing questions, including abort reports for conflicting content.
- Cover `audio_objects.deleted_at` runtime behavior: derived URLs are empty and stream returns `404` or `410` for deleted objects.
- Cover notes reference constraints and migration validation for both word and grammar soft references.
- Cover runtime write validation for `grammar_examples.linked_word_ids` and note word/grammar references.
- Cover PostgreSQL array scan/bind helpers and `tags @> ARRAY[$1]::text[]` query behavior.
- Cover `session_summaries` list query with the `(user_id, generated_at DESC)` index in schema tests.
- Cover source expected manifest vs target actual manifest comparison, including remap, child row, event, audio, and orphan counts.
- Cover translation module enum support in goals, sessions, summaries, and Go `summary.ModuleType`.
- Cover translation append-only behavior: the same user submitting the same sentence twice creates two `translation_records` rows, each with source text, direction, reference translation, and position snapshots.
- Cover translation sentence re-import behavior: changing `source_text`, `direction`, or `position` is rejected when practice records already reference that sentence, both through application re-import flow and the PostgreSQL trigger.
- Cover `writing_questions.grammar_point_id` runtime validation and migration orphan checks.
- Cover `lesson_words.position` preserving the original `word_ids_json` order.
- Cover FK-side indexes in schema tests, including indexes for audio references, reverse FK checks, and cascade-heavy tables.
- Cover configurable connection pool settings and lower defaults for admin/migration/test processes.
- Cover user CHECK constraints for `goal_level`, `jlpt_levels`, and `streak_days`.
- Cover MinIO metadata sanitization: raw URLs with query tokens/userinfo are not stored in `audio_objects.metadata` or logs.
- Cover event rating check constraints and object-specific review history indexes.
- Cover basic numeric CHECK constraints for audio duration, lesson sentence timestamps, SRS fields, and session counters.
- Cover remote legacy audio import SSRF rejection and local legacy audio path traversal rejection.
- Cover audio stream `Range` behavior, including `206`, `416`, MIME headers, and `<audio>` seek playback.
- Cover presigned URL selection: content audio can presign only with a public endpoint; private recordings always proxy stream.
- Cover `audio-reconcile --dry-run` for orphan MinIO objects and missing database objects.
- Cover admin single regeneration and batch generation linking the expected target row's `audio_object_id`.
- Cover admin and CLI write paths using `StoreRuntime` and `AudioService`, with no raw DB dependency in handler/import tests.
- Cover frontend audio behavior with unit tests or browser checks: API URL first, browser TTS fallback after playback failure, and no hash-derived `/audio/...` request in normal playback.
- Cover speaking practice in both compatibility mode with no uploaded recording and multipart mode with a persisted MinIO user recording.
- Create per-test schemas or transaction rollbacks for isolation.
- Keep one migration integration test that starts with an empty database and verifies all migrations apply once.

---

## Compatibility Decisions

1. Relational storage should be selected through `RELATIONAL_STORE`.
   PostgreSQL is the first required adapter. Additional relational adapters must own their schema, migrations, SQL, and tests instead of relying on accidental SQL portability.

2. Legacy SQLite support should not remain as an implicit fallback.
   A future SQLite runtime mode is acceptable only if it is rebuilt as a first-class relational adapter and passes the shared adapter contract tests. The old SQLite database remains a read-only migration source until then.

3. The SQLite source database must remain read-only during migration.
   Duplicate words are handled by PostgreSQL-side import filtering and in-memory ID remapping, not by deleting rows from the source database.

4. Existing API response shapes should remain stable.
   Store methods can reassemble child table rows into the current Go model structs.

5. Existing local audio storage should be migrated to MinIO.
   `./data/audio` can remain only as a migration input or temporary development cache. MinIO/S3-compatible object storage is the durable source of truth for all audio binaries.

6. Audio object storage should be pluggable through an interface.
   MinIO is the required implementation for this design, but business code should only depend on the audio service and `AudioBlobStore` so another S3-compatible backend can be wired later.

7. The current `writing_questions.grammar_point_id` soft reference should remain nullable.
   PostgreSQL should not reintroduce the old hard FK behavior that caused SQLite table rebuilds. Legacy `0` means "no associated grammar point" and must migrate to SQL `NULL`.

8. Translation records should be append-only unless product behavior explicitly needs one record per sentence.
   The current SQLite `INSERT OR REPLACE` does not have a real unique constraint and should not be copied blindly. PostgreSQL should use plain `INSERT ... RETURNING id` for `translation_records`.

---

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Store SQL changes touch many files | Convert one store at a time behind relational adapter contracts with focused tests |
| Adapter abstraction leaks dialect details into handlers | Keep dialect branches inside adapter packages and inject only `StoreRuntime` or narrower interfaces into application composition |
| Adapter abstraction becomes too generic and weakens PostgreSQL schema | Let each adapter own backend-specific SQL/DDL while sharing behavior-level store contracts |
| Admin keeps using concrete stores or raw DB handles | Introduce `AdminStores`, update handler config, and cover admin behavior in adapter contract tests |
| CLI/import/audio code bypasses the adapter | Require `StoreRuntime` plus `AudioService` for normal commands and keep raw DB access inside adapters or offline migration internals |
| SQLite migration code leaks into server runtime | Keep SQLite copy code in `internal/migration/sqlitecopy`, imported only by `backend/cmd/storage-migrate`, and remove server dependency on `internal/cli` dispatch |
| Runtime stores become overloaded with migration-only bulk loading | Add adapter-local `MigrationTarget`/`BulkLoader` used only by `storage-migrate` |
| Bulk migration builds SQL from raw table or column names | Use `MigrationTable`/`MigrationColumn` enums, adapter allowlists, and centralized identifier quoting |
| Multi-row writes lose atomicity behind the adapter | Make `Transact` part of the relational contract and test commit/rollback behavior |
| Database errors regress to string matching | Translate backend errors to `ErrNotFound`, `ErrDuplicate`, `ErrConstraint`, and `ErrRetryable` and test SQLSTATE mappings |
| Existing tests assume SQLite `:memory:` | Add shared relational adapter contract tests plus a PostgreSQL test database helper and migrate tests gradually |
| JSON splitting can lose ordering | Preserve `position` columns for examples, quiz questions, and lesson sentences |
| Source SQLite cleanup can delete user references | Keep source SQLite read-only and remap duplicate IDs during copy |
| Duplicate word references remain in non-review tables | Apply the global word ID remap to every word reference before insert |
| Duplicate words have different content | Abort migration with a duplicate-word report instead of silently keeping the smallest ID |
| Seed rows conflict with copied SQLite rows | Split schema migrations from seed data; SQLite migration runs schema only against a schema-only target whose non-metadata application tables are empty |
| Natural unique constraints reject existing duplicate content rows | Preflight grammar, lesson, speaking, and writing duplicates; canonicalize identical duplicates and abort on content conflicts |
| Failed one-shot migration leaves half state | Require all non-metadata application tables to be empty, use a migration run ID, and rerun only after dropping/recreating schema plus cleaning the run prefix |
| Migration counts look right but references are wrong | Generate expected and actual manifests and compare normalized counts, remaps, child rows, events, audio, and orphan counts |
| User deletion leaves private audio behind | Keep hard-delete semantics but require private MinIO/audio object cleanup before deleting the user row |
| Today-completed stats shift at the wrong midnight | Compute `[start, end)` in Go using `APP_TIMEZONE`, not PostgreSQL `current_date` |
| `writing_questions.grammar_point_id = 0` is treated as a real ID | Convert legacy zero values to SQL `NULL` and make Go/admin paths nullable |
| Soft-deleted audio still plays through existing references | Filter runtime joins by `audio_objects.deleted_at IS NULL` and reject deleted objects in stream handlers |
| Polymorphic note references point nowhere | Enforce paired `reference_id/reference_type` checks and validate word and grammar references after migration |
| Runtime admin/store writes create new orphan arrays or soft references | Validate `linked_word_ids` and note targets before every write |
| PostgreSQL arrays are treated like JSON strings | In the PostgreSQL adapter, use one pgx/pgtype-backed helper for array scan/bind and test exact membership queries |
| Built frontend artifacts retain old `/audio/...` logic | Regenerate deployed dist directories or remove them, then scan the repository for stale direct audio paths |
| Legacy audio metadata stores signed URLs or tokens | Store only sanitized scheme/host/path plus a hash; keep raw refs only in controlled migration reports |
| Translation records accidentally become upserts | Use append-only inserts, save prompt snapshots, and test duplicate submissions create multiple rows |
| Re-importing translation sentences rewrites history | Lock the sentence row during re-import, save record snapshots, and add a PostgreSQL trigger that rejects practiced text/direction/position rewrites |
| Review history queries scan too much data | Add object-specific review-event indexes |
| Cascades or RESTRICT checks scan child tables | Add FK-side indexes for every non-trivial FK, including audio references and reverse lookup columns |
| Lesson word order is lost | Preserve `word_ids_json` order in `lesson_words.position` |
| Connection pools exhaust PostgreSQL max connections | Configure pool sizes via environment variables and use lower defaults for admin/migration/tests |
| Invalid user state enters PostgreSQL | Add CHECK constraints for `goal_level`, `jlpt_levels`, and `streak_days` |
| Invalid numeric data enters PostgreSQL | Add schema CHECK constraints for non-negative durations, intervals, and timestamp ranges |
| Empty legacy AI feedback cannot cast to `jsonb` | Normalize empty strings and literal `null` to SQL `NULL` |
| Remote audio import can be abused for SSRF | Allow only safe schemes, reject private/metadata IPs before each request/redirect, enforce timeout and size limits |
| Local audio import can escape `--audio-root` | Clean and resolve paths/symlinks, then reject any final path outside the resolved root |
| MinIO upload succeeds but DB transaction fails or process crashes | Use idempotent `PutIfAbsent`, delete best-effort on known failures, and run scheduled `audio-reconcile` to find or clean orphan objects |
| Concurrent audio generators overwrite or disagree on deterministic keys | `PutIfAbsent` verifies existing object hash/size/type and returns `ErrObjectConflict` on mismatches |
| Object streams or listings leak resources or run partial cleanup | Require stream/iterator `Close`, iterator `Err` checks, context-aware pagination, and fail reconciliation on partial listing |
| Existing audio file referenced by SQLite is missing | Fail migration by default; require explicit `--allow-missing-audio` to continue with NULL audio links |
| User recordings are private data | Require owner IDs for private objects, use proxy streaming, and define account deletion cleanup |
| Presigned URLs expose internal endpoints or shareable private recordings | Only presign content audio through a browser-reachable public endpoint; proxy private recordings |
| Browser audio seeking breaks after replacing static file server | Implement `Range` support and required playback headers in the stream endpoint |
| Relational database and MinIO backups drift apart | Backup and restore them through a shared manifest and run reconciliation after restore |
| Frontend assumes deterministic `/audio/...` paths | Return backend-generated `audio_url` fields and keep browser TTS fallback |
| Review history stops growing after migration | Write current-state update and event append in the same transaction |
| Search behavior changes | Start with `pg_trgm` substring search and add acceptance tests for Japanese queries |
| IDs may change during migration | Preserve source IDs with the target adapter's identity override mechanism; for PostgreSQL use `OVERRIDING SYSTEM VALUE`, then reset every identity sequence |
| Admin and server race on migrations | Use `schema_migrations` plus a pinned-connection advisory lock |
| Seed data can duplicate | Put unique constraints in the initial schema before any seed insert |

---

## Acceptance Criteria

The storage upgrade is complete when:

- Runtime relational storage is selected through `RELATIONAL_STORE`, with `postgres` as the first supported adapter.
- Server, admin, and CLI read `RELATIONAL_STORE` plus `DATABASE_URL`; unsupported relational adapters fail fast at startup.
- Application handlers and module services depend on store interfaces or `StoreRuntime`, not concrete PostgreSQL store structs.
- Admin handlers depend on `AdminStores` and services such as `AudioService`, not concrete `*data.*Store` values or raw `*sql.DB`.
- The PostgreSQL adapter satisfies `RelationalAdapter`, owns its migrations and SQL, and passes the shared relational adapter contract tests.
- The PostgreSQL adapter provides `StoreRuntime.Transact`, and contract tests cover commit, rollback on error, rollback then re-panic on callback panic, `ErrNestedTransaction`, and tx-bound store visibility.
- The PostgreSQL adapter exposes `MigrationTarget` only to `backend/cmd/storage-migrate`; runtime stores do not perform preserve-ID bulk copy or sequence reset work.
- `MigrationTarget`/`BulkLoader` uses `MigrationTable` and `MigrationColumn` allowlists, rejects unknown identifiers, and never accepts raw table or column names from migration orchestration.
- Database errors are translated into `ErrNotFound`, `ErrDuplicate`, `ErrConstraint`, and `ErrRetryable`; callers use `errors.Is`/`errors.As`, not string matching.
- The application no longer imports SQLite drivers in runtime paths. SQLite imports are limited to `backend/cmd/storage-migrate` through `internal/migration/sqlitecopy`. Any future SQLite support must be a first-class adapter with contract tests.
- MinIO configuration is required for audio-producing and audio-playing workflows.
- Migrations run once, are tracked in `schema_migrations`, and are safe under concurrent server/admin startup.
- All existing store behavior passes tests through the relational adapter contract suite and the PostgreSQL adapter integration suite.
- Seed/import commands are idempotent through explicit PostgreSQL unique constraints and `ON CONFLICT` targets.
- Existing SQLite data can be migrated into the selected relational target with validation counts while the SQLite source remains read-only.
- The one-shot SQLite data migration command lives in `backend/cmd/storage-migrate`, not the HTTP server binary.
- The one-shot SQLite copy implementation lives outside `internal/cli`, and server/admin binaries do not import it transitively.
- Migration produces and compares `migration_expected_manifest.json` and `migration_actual_manifest.json`; mismatches fail the migration unless explicitly allowed.
- SQLite migration runs only schema migrations, refuses any non-empty non-metadata relational application table, and records a `migration_run_id`.
- A failed SQLite migration is rerun only after recreating the target adapter schema and cleaning/quarantining the MinIO objects for that run.
- Seed data is loaded only by an explicit empty-database seed command, never by the SQLite migration path.
- Existing local/external audio references are uploaded to MinIO or reported by the migration command.
- Legacy remote audio import rejects SSRF targets, unsafe redirects, oversized files, and non-audio content.
- Legacy local audio import rejects path traversal and symlink escapes outside `--audio-root`.
- The relational target contains `audio_objects` metadata and business tables reference audio through `audio_object_id`, not durable local paths.
- Audio writes use `PutIfAbsent`; existing deterministic keys are reused only when hash, size, and content type match, and mismatches return `ErrObjectConflict`.
- Object streams and iterators have explicit close/error contracts; reconciliation fails on partial listings.
- Runtime audio URL derivation ignores soft-deleted `audio_objects`; stream requests for deleted objects return `404` or `410`.
- MinIO metadata stores only sanitized legacy refs or hashes; raw signed URLs/tokens/userinfo are excluded from DB rows and normal logs.
- Notes enforce paired `reference_id/reference_type` constraints and migration validation checks both word and grammar references.
- Admin/store writes validate `grammar_examples.linked_word_ids` and note word/grammar references before writing.
- CLI import, seed, TTS generation, and batch audio paths use `StoreRuntime` and `AudioService`; raw DB handles are limited to adapter internals and offline migration internals.
- PostgreSQL array fields are read/written through adapter-local helpers and array membership queries are covered by tests; other adapters must pass equivalent filtering tests.
- Private `audio_objects` cannot exist without an owner and user deletion cannot leave ownerless private recordings.
- Audio stream endpoints support `Range` requests and return playback-safe headers.
- Presigned URLs are used only for content audio with a public MinIO endpoint; private recordings are proxied.
- Backup/restore runbooks include matching relational database and MinIO snapshots plus reconciliation.
- `audio-reconcile --dry-run` reports missing objects and orphan MinIO objects.
- Duplicate source word IDs are remapped for every word reference, including `lesson_words`, `grammar_examples.linked_word_ids`, word notes, `word_records`, and `word_bookmarks`.
- Duplicate source words with different content produce a migration report and abort unless a future explicit merge policy is added.
- Duplicate source grammar points, lessons, speaking materials, and writing questions are canonicalized only when content-identical; content conflicts report and abort.
- Canonical maps are applied to `grammar_records`, `writing_questions.grammar_point_id`, grammar notes, and `speaking_records.material_id`.
- User deletion remains a hard-delete workflow and cannot proceed until private MinIO/audio objects are cleaned.
- Today-completed statistics use Go-computed `[start, end)` windows in `APP_TIMEZONE`.
- Legacy `writing_questions.grammar_point_id = 0` migrates to SQL `NULL` and all Go/admin paths treat it as nullable.
- `translation_sentences` enforces `UNIQUE(source_id, position)`.
- `session_summaries` has an index on `(user_id, generated_at DESC)`.
- `translation` is included in module enums for goals, sessions, summaries, and Go `summary.ModuleType`.
- `translation_records` are append-only; repeated submissions for the same user and sentence create separate rows with prompt, direction, reference translation, and position snapshots.
- Re-importing translation content cannot mutate `source_text`, `direction`, or `position` for sentences that already have practice records; changed prompts/order require a new source version or fail the import.
- PostgreSQL protects practiced `translation_sentences.source_text`, `direction`, and `position` with a trigger or equivalent database-level guard in addition to application-level transaction checks.
- `word_review_events` and `note_review_events` constrain rating to `easy`, `normal`, or `hard` and have object-specific history indexes.
- FK-side indexes exist for cascade/SET NULL/RESTRICT paths, including audio reference columns, reverse record references, password reset tokens, note links, and speaking material records.
- `lesson_words.position` preserves the order from legacy `word_ids_json`.
- Database pool settings are configurable through environment variables and are not hard-coded to `25/25` for every process.
- `users.goal_level`, `users.jlpt_levels`, and `users.streak_days` are protected by CHECK constraints.
- Numeric CHECK constraints reject negative audio durations, invalid lesson timestamp ranges, non-positive SRS ease factors, negative intervals, and negative session counters.
- `pg_trgm` is created by the migration/bootstrap role, not required from the runtime application role.
- Source IDs are preserved with `OVERRIDING SYSTEM VALUE` and identity sequences are reset before application writes resume.
- Cutover verification includes a repository scan for stale direct audio paths or hash URL helpers, with an explicit allowlist for legitimate hashing.
- Review, quiz, and note event histories continue growing after migration through append-only event tables.
- Word pronunciation, word example, grammar example, speaking material, lesson audio, and user speaking recordings can be streamed from MinIO-backed URLs.
- Word, bookmark, grammar, lesson, note, speaking, writing, translation, session, and admin workflows all work through `RELATIONAL_STORE=postgres`.
- `make test` and `make web` pass.
