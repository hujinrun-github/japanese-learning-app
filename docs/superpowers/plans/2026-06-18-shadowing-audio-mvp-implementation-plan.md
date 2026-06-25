# Shadowing Audio MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Phase 0 + Phase 1 lesson-centered shadowing MVP: audio-first lesson detail routing, safe content import/validation, authenticated shadowing progress and attempts APIs, and a learner-facing audio shadowing page.

**Architecture:** `lesson` remains the single source of content. A new `internal/module/shadowing` module owns practice/session behavior and composes the existing lesson store plus a new SQLite shadowing store. PostgreSQL receives schema-only compatibility updates in this plan; no PostgreSQL shadowing runtime store, service wiring, route registration, video playback, admin UI, AI material generation, server-side scoring, or persistent recording upload is included.

**Tech Stack:** Go 1.24, `net/http`, `database/sql`, SQLite migrations, schema-only PostgreSQL SQL, table-driven Go tests, Python 3 validation script, React 18, React Router 6, TypeScript, Vitest, existing `useAudioRecorder`, Makefile commands.

---

## Reference Inputs

- `docs/superpowers/specs/2026-06-18-shadowing-lesson-centered-design.md`
- `docs/shadowing.md`
- `constitution.md`
- `AGENTS.md`

---

## Scope Cut

This plan implements only the audio MVP:

- Phase 0: lesson detail routing, Lesson API/TS contract alignment, optional API error `details`, SQLite runtime migration, PostgreSQL schema-only update, import upsert/report/cleanup tooling, validation script, and 1 to 3 manual material packs.
- Phase 1: `/lesson/:id/shadowing`, audio-only player, current sentence logic, loop/slow/record local playback, self-score, progress restore, authenticated shadowing APIs.

This plan explicitly excludes:

- Video playback even if `video_url` exists.
- Admin `Shadowing Materials` page and jobs API.
- AI material pack generation.
- Persistent user recording upload.
- ASR or automatic scoring.
- PostgreSQL shadowing runtime store/service/router.

---

## File Structure

Create:

- `internal/module/shadowing/model.go`: request/response DTOs, progress/attempt models, practice mode constants, error codes.
- `internal/module/shadowing/current_sentence.go`: Go current-sentence pure function used by service validation tests.
- `internal/module/shadowing/current_sentence_test.go`: table-driven tests for sentence boundary behavior.
- `internal/module/shadowing/service.go`: shadowing business rules, version validation, media/content validation, aggregation orchestration.
- `internal/module/shadowing/service_test.go`: table-driven service tests with fake lesson and shadowing stores.
- `internal/module/shadowing/handler.go`: authenticated `/api/v1/lessons/{id}/shadowing*` endpoints.
- `internal/module/shadowing/handler_test.go`: handler tests for auth, stale version, disabled lesson, missing media.
- `internal/data/shadowing_store.go`: SQLite progress and attempt persistence.
- `internal/data/shadowing_store_test.go`: SQLite integration tests for progress upsert and attempt aggregation.
- `internal/data/shadowing_migration_test.go`: repeat-run migration safety tests for SQLite shadowing columns/tables.
- `internal/data/migrations/013_shadowing.sql`: SQLite shadowing runtime migration without cleanup DML and without unique lesson index.
- `internal/httputil/response_test.go`: tests for optional error `details`.
- `internal/cli/import_lessons_test.go`: importer upsert, duplicate report, cleanup, and unique-index command tests.
- `internal/cli/lesson_duplicates.go`: duplicate report, cleanup, and unique-index helpers.
- `scripts/validate_lessons_shadowing.py`: manual material pack validator.
- `scripts/testdata/shadowing_valid_lesson.json`: validator success fixture.
- `scripts/testdata/shadowing_invalid_lesson.json`: validator failure fixture.
- `front/react/src/api/shadowing.ts`: shadowing API client functions.
- `front/react/src/api/client.test.ts`: `APIError.details` parsing tests.
- `front/react/src/pages/lesson/LessonListPage.tsx`: routed lesson list page.
- `front/react/src/pages/lesson/LessonDetailPage.tsx`: routed lesson detail page and shadowing CTA.
- `front/react/src/pages/shadowing/ShadowingPage.tsx`: audio-only shadowing page.
- `front/react/src/pages/shadowing/ShadowingPage.module.css`: shadowing page styles.
- `front/react/src/util/shadowing/currentSentence.ts`: frontend current-sentence pure function.
- `front/react/src/util/shadowing/currentSentence.test.ts`: frontend boundary tests.
- `front/react/src/types/shadowing.ts`: frontend shadowing DTOs if `types/api.ts` becomes too large.
- `data/seed/lessons_shadowing_pilot.json`: manual pilot material pack import file once audio/source metadata is prepared.

Modify:

- `backend/cmd/server/main.go`: instantiate and register shadowing only for the SQLite runtime path used by the current server.
- `internal/httputil/response.go`: add optional `details` support without breaking existing `WriteError` callers.
- `internal/module/lesson/model.go`: add shadowing metadata and align canonical fields.
- `internal/data/lesson_store.go`: read and expose shadowing metadata.
- `internal/data/lesson_store_test.go`: cover new fields.
- `internal/data/adapters.go`: expose lesson store methods needed by shadowing service through a small adapter.
- `internal/data/migrations/postgres/schema/001_init.sql`: schema-only shadowing columns/tables for PostgreSQL.
- `internal/cli/import_lessons.go`: extend JSON shape and replace `INSERT OR IGNORE` with update-or-insert behavior.
- `internal/cli/root.go`: add lesson duplicate report/cleanup/index commands.
- `front/react/src/App.tsx`: add `/lesson/:id` and `/lesson/:id/shadowing`.
- `front/react/src/pages/lesson/LessonPage.tsx`: either reduce to a wrapper or replace usages with list/detail pages.
- `front/react/src/pages/lesson/LessonPage.module.css`: keep shared lesson styles or split only if needed.
- `front/react/src/types/api.ts`: align lesson and error DTOs with backend.
- `front/react/src/api/client.ts`: preserve optional `details` on API errors.
- `front/react/src/hooks/useAudioRecorder.ts`: revoke object URLs on new recording and unmount.
- `Makefile`: optionally add `validate-shadowing-pilot` using `scripts/validate_lessons_shadowing.py`.

---

### Task 1: Shared Error Details Contract

**Files:**
- Create: `internal/httputil/response_test.go`
- Create: `front/react/src/api/client.test.ts`
- Modify: `internal/httputil/response.go`
- Modify: `front/react/src/types/api.ts`
- Modify: `front/react/src/api/client.ts`

- [ ] **Step 1: Write the Go failing test for `details`**

Add `internal/httputil/response_test.go`:

```go
package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorWithDetails(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteErrorWithDetails(rec, http.StatusConflict, "ERR_SHADOWING_VERSION_STALE", "shadowing version is stale", "", map[string]any{
		"current_shadowing_version": 2,
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var got APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Code != "ERR_SHADOWING_VERSION_STALE" {
		t.Fatalf("code = %q", got.Code)
	}
	if got.Details == nil {
		t.Fatal("details = nil, want map")
	}
	if got.Details["current_shadowing_version"] != float64(2) {
		t.Fatalf("current_shadowing_version = %#v", got.Details["current_shadowing_version"])
	}
}
```

- [ ] **Step 2: Run the Go test and verify it fails**

Run:

```bash
go test ./internal/httputil -run TestWriteErrorWithDetails -v -count=1
```

Expected: fail with `undefined: WriteErrorWithDetails` or `got.Details undefined`.

- [ ] **Step 3: Implement optional details in backend error responses**

Change `internal/httputil/response.go`:

```go
type APIError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, code, message, requestID string) {
	WriteErrorWithDetails(w, status, code, message, requestID, nil)
}

func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message, requestID string, details map[string]any) {
	WriteJSON(w, status, APIError{
		Code:      code,
		Message:   message,
		RequestID: requestID,
		Details:   details,
	})
}
```

- [ ] **Step 4: Run backend error tests**

Run:

```bash
go test ./internal/httputil -v -count=1
```

Expected: pass.

- [ ] **Step 5: Write frontend failing test for `APIError.details`**

Add `front/react/src/api/client.test.ts`:

```ts
import { describe, expect, it, vi, afterEach } from 'vitest'
import { APIError, apiFetch } from './client'

describe('apiFetch error details', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('preserves flat backend error details', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      code: 'ERR_SHADOWING_VERSION_STALE',
      message: 'shadowing version is stale',
      request_id: '',
      details: { current_shadowing_version: 2 },
    }), { status: 409, headers: { 'Content-Type': 'application/json' } })))

    await expect(apiFetch('POST', '/api/v1/lessons/1/shadowing/progress', {}))
      .rejects.toMatchObject({
        code: 'ERR_SHADOWING_VERSION_STALE',
        status: 409,
        details: { current_shadowing_version: 2 },
      })
  })
})
```

- [ ] **Step 6: Run frontend test and verify it fails**

Run:

```bash
cd front/react
npm test -- src/api/client.test.ts
```

Expected: fail because `APIError` does not expose `details`.

- [ ] **Step 7: Implement frontend details parsing**

Change `front/react/src/types/api.ts`:

```ts
export interface APIError {
  code: string
  message: string
  details?: Record<string, unknown>
}
```

Change `front/react/src/api/client.ts`:

```ts
export class APIError extends Error {
  code: string
  status: number
  details?: Record<string, unknown>

  constructor(code: string, message: string, status: number, details?: Record<string, unknown>) {
    super(message)
    this.name = 'APIError'
    this.code = code
    this.status = status
    this.details = details
  }
}
```

Inside the non-OK branch, parse both existing shapes:

```ts
let details: Record<string, unknown> | undefined
try {
  const err = await res.json()
  if (err.error) {
    code = err.error.code ?? code
    message = err.error.message ?? message
    details = err.error.details
  } else {
    code = err.code ?? code
    message = err.message ?? message
    details = err.details
  }
} catch {
  // keep default code and message
}
throw new APIError(code, message, res.status, details)
```

- [ ] **Step 8: Run frontend API tests**

Run:

```bash
cd front/react
npm test -- src/api/client.test.ts
```

Expected: pass.

- [ ] **Step 9: Commit error contract change**

Run:

```bash
git add internal/httputil/response.go internal/httputil/response_test.go front/react/src/types/api.ts front/react/src/api/client.ts front/react/src/api/client.test.ts
git commit -m "feat(shadowing): support API error details"
```

---

### Task 2: SQLite Runtime Migration And PostgreSQL Schema-Only Update

**Files:**
- Create: `internal/data/migrations/013_shadowing.sql`
- Create: `internal/data/shadowing_migration_test.go`
- Modify: `internal/data/migrations/postgres/schema/001_init.sql`

- [ ] **Step 1: Write migration repeat-safety test first**

Add `internal/data/shadowing_migration_test.go`:

```go
package data

import (
	"database/sql"
	"testing"
)

func TestShadowingMigrationRepeatSafe(t *testing.T) {
	dbPath := t.TempDir() + "/shadowing.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations first: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations second: %v", err)
	}

	assertColumnExists(t, db, "lessons", "shadowing_enabled")
	assertColumnExists(t, db, "lessons", "video_url")
	assertColumnExists(t, db, "lessons", "shadowing_version")
	assertColumnExists(t, db, "lessons", "shadowing_config_json")
	assertTableExists(t, db, "lesson_shadowing_progress")
	assertTableExists(t, db, "lesson_shadowing_attempts")
}

func assertColumnExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return
		}
	}
	t.Fatalf("%s.%s column not found", table, column)
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err != nil {
		t.Fatalf("table %s not found: %v", table, err)
	}
}
```

- [ ] **Step 2: Run migration test and verify it fails**

Run:

```bash
go test ./internal/data -run TestShadowingMigrationRepeatSafe -v -count=1
```

Expected: fail because shadowing columns/tables do not exist.

- [ ] **Step 3: Add SQLite migration without cleanup DML or lesson unique index**

Create `internal/data/migrations/013_shadowing.sql`:

```sql
-- 013_shadowing.sql
-- Audio-first shadowing runtime schema.
-- This migration is repeat-run safe under the current duplicate-column tolerant runner.
-- It intentionally does not delete duplicate lessons and does not create a unique lesson index.

ALTER TABLE lessons ADD COLUMN shadowing_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE lessons ADD COLUMN video_url TEXT NOT NULL DEFAULT '';
ALTER TABLE lessons ADD COLUMN shadowing_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE lessons ADD COLUMN shadowing_config_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS lesson_shadowing_progress (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id              INTEGER NOT NULL,
    lesson_id            INTEGER NOT NULL,
    shadowing_version    INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    last_sentence_index  INTEGER NOT NULL DEFAULT 0 CHECK (last_sentence_index >= 0),
    last_position_ms     INTEGER NOT NULL DEFAULT 0 CHECK (last_position_ms >= 0),
    last_practice_mode   TEXT    NOT NULL DEFAULT 'normal',
    updated_at           DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, lesson_id, shadowing_version),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (lesson_id) REFERENCES lessons(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS lesson_shadowing_attempts (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id            INTEGER NOT NULL,
    lesson_id          INTEGER NOT NULL,
    shadowing_version  INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    sentence_index     INTEGER NOT NULL CHECK (sentence_index >= 0),
    practice_mode      TEXT    NOT NULL,
    playback_rate      REAL    NOT NULL DEFAULT 1.0 CHECK (playback_rate > 0),
    loop_count         INTEGER NOT NULL DEFAULT 0 CHECK (loop_count >= 0),
    self_score         INTEGER,
    recognition_text   TEXT    NOT NULL DEFAULT '',
    audio_ref          TEXT    NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    CHECK (self_score IS NULL OR (self_score BETWEEN 0 AND 100)),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (lesson_id) REFERENCES lessons(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lesson_shadowing_attempts_sentence
    ON lesson_shadowing_attempts (user_id, lesson_id, shadowing_version, sentence_index, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lesson_shadowing_attempts_history
    ON lesson_shadowing_attempts (user_id, lesson_id, shadowing_version, created_at DESC);
```

- [ ] **Step 4: Run SQLite migration test**

Run:

```bash
go test ./internal/data -run TestShadowingMigrationRepeatSafe -v -count=1
```

Expected: pass.

- [ ] **Step 5: Update PostgreSQL schema-only SQL**

Modify `internal/data/migrations/postgres/schema/001_init.sql`:

```sql
ALTER TABLE lessons
    ADD COLUMN IF NOT EXISTS shadowing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS shadowing_version INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    ADD COLUMN IF NOT EXISTS shadowing_config_json JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS lesson_shadowing_progress (
    id                   BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id            BIGINT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    shadowing_version    INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    last_sentence_index  INTEGER NOT NULL DEFAULT 0 CHECK (last_sentence_index >= 0),
    last_position_ms     BIGINT NOT NULL DEFAULT 0 CHECK (last_position_ms >= 0),
    last_practice_mode   TEXT NOT NULL DEFAULT 'normal',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, lesson_id, shadowing_version)
);

CREATE TABLE IF NOT EXISTS lesson_shadowing_attempts (
    id                 BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id          BIGINT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    shadowing_version  INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    sentence_index     INTEGER NOT NULL CHECK (sentence_index >= 0),
    practice_mode      TEXT NOT NULL,
    playback_rate      DOUBLE PRECISION NOT NULL DEFAULT 1.0 CHECK (playback_rate > 0),
    loop_count         INTEGER NOT NULL DEFAULT 0 CHECK (loop_count >= 0),
    self_score         INTEGER CHECK (self_score IS NULL OR (self_score BETWEEN 0 AND 100)),
    recognition_text   TEXT NOT NULL DEFAULT '',
    audio_ref          TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lesson_shadowing_attempts_sentence
    ON lesson_shadowing_attempts(user_id, lesson_id, shadowing_version, sentence_index, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lesson_shadowing_attempts_history
    ON lesson_shadowing_attempts(user_id, lesson_id, shadowing_version, created_at DESC);
```

Do not add `video_url` to PostgreSQL in this task. PostgreSQL continues to use `audio_object_id`, and this plan does not wire shadowing runtime routes for PostgreSQL.

- [ ] **Step 6: Run backend data tests**

Run:

```bash
go test ./internal/data -v -count=1
```

Expected: pass.

- [ ] **Step 7: Commit migration work**

Run:

```bash
git add internal/data/migrations/013_shadowing.sql internal/data/shadowing_migration_test.go internal/data/migrations/postgres/schema/001_init.sql
git commit -m "feat(shadowing): add audio mvp storage schema"
```

---

### Task 3: Lesson Contract, Import Upsert, Duplicate Safety, And Validator

**Files:**
- Create: `internal/cli/lesson_duplicates.go`
- Create: `internal/cli/import_lessons_test.go`
- Create: `scripts/validate_lessons_shadowing.py`
- Create: `scripts/testdata/shadowing_valid_lesson.json`
- Create: `scripts/testdata/shadowing_invalid_lesson.json`
- Modify: `internal/module/lesson/model.go`
- Modify: `internal/data/lesson_store.go`
- Modify: `internal/data/lesson_store_test.go`
- Modify: `internal/cli/import_lessons.go`
- Modify: `internal/cli/root.go`
- Modify: `Makefile`

- [ ] **Step 1: Write lesson store test for shadowing metadata**

Extend `internal/data/lesson_store_test.go` with a test that inserts a lesson with shadowing metadata and verifies `GetDetail` exposes it:

```go
func TestLessonStore_GetDetail_ShadowingMetadata(t *testing.T) {
	store := &LessonStore{db: testDB}

	res, err := testDB.Exec(`
		INSERT INTO lessons (
			title, content_furigana_json, translation_json, jlpt_level, tags_json,
			audio_url, video_url, sentence_timestamps_json, char_count, word_ids_json,
			shadowing_enabled, shadowing_version, shadowing_config_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"Shadowing Metadata Test",
		`[{"index":0,"tokens":[{"surface":"日本語","reading":"にほんご"}],"chinese":"日语","start_ms":0,"end_ms":3000}]`,
		`[]`,
		string(lesson.LevelN5),
		`["shadowing"]`,
		"/audio/lessons/shadowing-test.wav",
		"",
		`[{"index":0,"start_ms":0,"end_ms":3000}]`,
		20,
		`[1]`,
		1,
		2,
		`{"media_type":"audio","media_duration_ms":3000}`,
	)
	if err != nil {
		t.Fatalf("insert lesson: %v", err)
	}
	id, _ := res.LastInsertId()

	detail, err := store.GetDetail(id)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if !detail.ShadowingEnabled {
		t.Fatal("ShadowingEnabled = false, want true")
	}
	if detail.ShadowingVersion != 2 {
		t.Fatalf("ShadowingVersion = %d, want 2", detail.ShadowingVersion)
	}
	if detail.ShadowingConfig["media_type"] != "audio" {
		t.Fatalf("ShadowingConfig = %#v", detail.ShadowingConfig)
	}
}
```

- [ ] **Step 2: Run lesson store test and verify it fails**

Run:

```bash
go test ./internal/data -run TestLessonStore_GetDetail_ShadowingMetadata -v -count=1
```

Expected: fail because lesson models do not expose the new fields.

- [ ] **Step 3: Extend lesson backend model**

Modify `internal/module/lesson/model.go`:

```go
type LessonSummary struct {
	ID                 int64          `json:"id"`
	Title              string         `json:"title"`
	JLPTLevel          JLPTLevel      `json:"jlpt_level"`
	Tags               []string       `json:"tags"`
	CharCount          int            `json:"char_count"`
	AudioURL           string         `json:"audio_url"`
	VideoURL           string         `json:"video_url"`
	ShadowingEnabled   bool           `json:"shadowing_enabled"`
	ShadowingVersion   int            `json:"shadowing_version"`
	ShadowingConfig    map[string]any `json:"shadowing_config"`
}
```

Keep `Sentence.Chinese`, `Sentence.StartMS`, and `Sentence.EndMS` as the canonical contract.

- [ ] **Step 4: Read shadowing metadata in SQLite lesson store**

Change the `SELECT` statements in `internal/data/lesson_store.go` to include:

```sql
video_url, shadowing_enabled, shadowing_version, shadowing_config_json
```

When scanning, convert `shadowing_enabled` from integer to bool:

```go
var shadowingEnabled int
var shadowingConfigJSON string
if err := row.Scan(
	&l.ID, &l.Title, &l.JLPTLevel, &tagsJSON, &l.CharCount, &l.AudioURL,
	&l.VideoURL, &shadowingEnabled, &l.ShadowingVersion, &shadowingConfigJSON,
	&contentJSON, &wordIDsJSON,
); err != nil {
	// existing error handling
}
l.ShadowingEnabled = shadowingEnabled != 0
if shadowingConfigJSON == "" {
	shadowingConfigJSON = "{}"
}
if err := json.Unmarshal([]byte(shadowingConfigJSON), &l.ShadowingConfig); err != nil {
	return nil, fmt.Errorf("data.LessonStore.GetDetail unmarshal shadowing_config: %w", err)
}
```

Use equivalent scanning in `ListSummaries`.

- [ ] **Step 5: Run lesson store tests**

Run:

```bash
go test ./internal/data -run 'TestLessonStore_' -v -count=1
```

Expected: pass.

- [ ] **Step 6: Write importer duplicate/upsert tests**

Add `internal/cli/import_lessons_test.go`:

```go
package cli

import (
	"database/sql"
	"os"
	"testing"

	"japanese-learning-app/internal/data"
)

func openLessonImportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := data.OpenDB(t.TempDir() + "/import.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := data.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return db
}

func TestImportLessonsFromFile_UpdatesExistingLesson(t *testing.T) {
	db := openLessonImportTestDB(t)
	path := t.TempDir() + "/lessons.json"

	first := `[{"title":"Shadowing Import","jlpt_level":"N5","tags":["old"],"audio_url":"/audio/old.wav","video_url":"","shadowing_enabled":false,"shadowing_version":1,"shadowing_config":{"media_type":"audio"},"word_ids":[],"sentences":[{"index":0,"tokens":[{"surface":"古い","reading":"ふるい"}],"chinese":"旧","start_ms":0,"end_ms":1000}]}]`
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if _, err := ImportLessonsFromFile(db, path); err != nil {
		t.Fatalf("first import: %v", err)
	}

	second := `[{"title":"Shadowing Import","jlpt_level":"N5","tags":["new"],"audio_url":"/audio/new.wav","video_url":"","shadowing_enabled":true,"shadowing_version":2,"shadowing_config":{"media_type":"audio","media_duration_ms":2000},"word_ids":[1],"sentences":[{"index":0,"tokens":[{"surface":"新しい","reading":"あたらしい"}],"chinese":"新","start_ms":0,"end_ms":2000}]}]`
	if err := os.WriteFile(path, []byte(second), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if _, err := ImportLessonsFromFile(db, path); err != nil {
		t.Fatalf("second import: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lessons WHERE title='Shadowing Import' AND jlpt_level='N5'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	var audioURL string
	var enabled int
	var version int
	if err := db.QueryRow(`SELECT audio_url, shadowing_enabled, shadowing_version FROM lessons WHERE title='Shadowing Import'`).Scan(&audioURL, &enabled, &version); err != nil {
		t.Fatalf("select updated row: %v", err)
	}
	if audioURL != "/audio/new.wav" || enabled != 1 || version != 2 {
		t.Fatalf("row = audio:%q enabled:%d version:%d", audioURL, enabled, version)
	}
}
```

- [ ] **Step 7: Run importer test and verify it fails**

Run:

```bash
go test ./internal/cli -run TestImportLessonsFromFile_UpdatesExistingLesson -v -count=1
```

Expected: fail because importer still uses `INSERT OR IGNORE`.

- [ ] **Step 8: Implement import JSON extension and update-or-insert behavior**

Extend `lessonImport` in `internal/cli/import_lessons.go`:

```go
type lessonImport struct {
	Title             string         `json:"title"`
	JLPTLevel         string         `json:"jlpt_level"`
	Tags              any            `json:"tags"`
	AudioURL          string         `json:"audio_url"`
	VideoURL          string         `json:"video_url"`
	ShadowingEnabled  bool           `json:"shadowing_enabled"`
	ShadowingVersion  int            `json:"shadowing_version"`
	ShadowingConfig   map[string]any `json:"shadowing_config"`
	WordIDs           any            `json:"word_ids"`
	Sentences          []sentenceImport `json:"sentences"`
}
```

Normalize defaults before persistence:

```go
if item.ShadowingVersion == 0 {
	item.ShadowingVersion = 1
}
if item.ShadowingConfig == nil {
	item.ShadowingConfig = map[string]any{}
}
```

Replace `INSERT OR IGNORE` with a transaction helper:

```go
func upsertLesson(tx *sql.Tx, item lessonImport, payload lessonPayload) (int, error) {
	var existingID int64
	err := tx.QueryRow(`SELECT id FROM lessons WHERE title = ? AND jlpt_level = ? ORDER BY id`, item.Title, item.JLPTLevel).Scan(&existingID)
	if err == sql.ErrNoRows {
		_, err = tx.Exec(`INSERT INTO lessons
			(title, content_furigana_json, translation_json, jlpt_level, tags_json, audio_url,
			 video_url, sentence_timestamps_json, char_count, word_ids_json,
			 shadowing_enabled, shadowing_version, shadowing_config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.Title, payload.ContentJSON, payload.TranslationJSON, item.JLPTLevel, payload.TagsJSON,
			item.AudioURL, item.VideoURL, payload.TimestampsJSON, payload.CharCount, payload.WordIDsJSON,
			boolToSQLiteInt(item.ShadowingEnabled), item.ShadowingVersion, payload.ShadowingConfigJSON,
		)
		if err != nil {
			return 0, fmt.Errorf("insert lesson %q: %w", item.Title, err)
		}
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lookup lesson %q/%s: %w", item.Title, item.JLPTLevel, err)
	}

	_, err = tx.Exec(`UPDATE lessons SET
		content_furigana_json = ?,
		translation_json = ?,
		tags_json = ?,
		audio_url = ?,
		video_url = ?,
		sentence_timestamps_json = ?,
		char_count = ?,
		word_ids_json = ?,
		shadowing_enabled = ?,
		shadowing_version = ?,
		shadowing_config_json = ?
		WHERE id = ?`,
		payload.ContentJSON, payload.TranslationJSON, payload.TagsJSON, item.AudioURL, item.VideoURL,
		payload.TimestampsJSON, payload.CharCount, payload.WordIDsJSON,
		boolToSQLiteInt(item.ShadowingEnabled), item.ShadowingVersion, payload.ShadowingConfigJSON, existingID,
	)
	if err != nil {
		return 0, fmt.Errorf("update lesson %q/%s: %w", item.Title, item.JLPTLevel, err)
	}
	return 0, nil
}
```

Before calling `upsertLesson`, query duplicates for the same `(title, jlpt_level)`; if more than one row exists, return an error telling the operator to run duplicate cleanup first.

- [ ] **Step 9: Add duplicate report, cleanup, and unique-index helpers**

Create `internal/cli/lesson_duplicates.go` with three exported helpers:

```go
type LessonDuplicateGroup struct {
	Title        string
	JLPTLevel    string
	DuplicateIDs []int64
	KeptID       int64
}

func ReportLessonDuplicates(db *sql.DB) ([]LessonDuplicateGroup, error)
func CleanupLessonDuplicates(db *sql.DB) (int, error)
func CreateLessonUniqueIndex(db *sql.DB) error
```

`ReportLessonDuplicates` should group by `title, jlpt_level`, keep `MIN(id)`, and return all IDs in ascending order. `CleanupLessonDuplicates` should delete rows whose ID is not the kept ID only after collecting the report. `CreateLessonUniqueIndex` should first call `ReportLessonDuplicates`; if duplicates remain, return an error and do not run `CREATE UNIQUE INDEX`.

Use this SQL for the index command:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_lessons_title_level_unique ON lessons (title, jlpt_level)
```

- [ ] **Step 10: Register CLI commands**

Modify `internal/cli/root.go`:

```go
case "report-lesson-duplicates":
	return runReportLessonDuplicates(args[1:])
case "cleanup-lesson-duplicates":
	return runCleanupLessonDuplicates(args[1:])
case "create-lesson-unique-index":
	return runCreateLessonUniqueIndex(args[1:])
```

Each command accepts `--db ./data/app.db`. `report-lesson-duplicates` prints `title`, `jlpt_level`, `duplicate_ids`, and `kept_id`. `cleanup-lesson-duplicates` prints the same report before deleting non-kept rows and prints the deleted count. `create-lesson-unique-index` refuses to run when duplicates exist.

- [ ] **Step 11: Run importer and CLI tests**

Run:

```bash
go test ./internal/cli -v -count=1
```

Expected: pass.

- [ ] **Step 12: Add validator fixtures**

Create `scripts/testdata/shadowing_valid_lesson.json`:

```json
[
  {
    "title": "Shadowing Valid Fixture",
    "jlpt_level": "N5",
    "tags": ["shadowing"],
    "audio_url": "/audio/lessons/valid.wav",
    "video_url": "",
    "shadowing_enabled": true,
    "shadowing_version": 1,
    "shadowing_config": {
      "media_type": "audio",
      "media_duration_ms": 3000,
      "source": {
        "source_type": "owned",
        "source_name": "validator-fixture",
        "source_url": "",
        "license": "owned",
        "permission_note": "Local validation fixture",
        "subtitle_source": "manual"
      }
    },
    "word_ids": [],
    "sentences": [
      {
        "index": 0,
        "tokens": [{"surface": "日本語", "reading": "にほんご"}],
        "chinese": "日语",
        "start_ms": 0,
        "end_ms": 3000
      }
    ]
  }
]
```

Create `scripts/testdata/shadowing_invalid_lesson.json`:

```json
[
  {
    "title": "Shadowing Invalid Fixture",
    "jlpt_level": "N5",
    "tags": ["shadowing"],
    "audio_url": "",
    "video_url": "",
    "shadowing_enabled": true,
    "shadowing_version": 1,
    "shadowing_config": {
      "media_type": "audio",
      "media_duration_ms": 1000,
      "source": {
        "source_type": "owned",
        "source_name": "validator-invalid-fixture",
        "source_url": "",
        "license": "owned",
        "subtitle_source": "manual"
      }
    },
    "word_ids": [],
    "sentences": [
      {
        "index": 0,
        "tokens": [],
        "chinese": "",
        "start_ms": 500,
        "end_ms": 500
      }
    ]
  }
]
```

- [ ] **Step 13: Implement validator script**

Create `scripts/validate_lessons_shadowing.py`. It must:

- Load a JSON array from `--file`.
- Inspect only lessons where `shadowing_enabled` is true.
- Require `audio_url`.
- Require `shadowing_version >= 1`.
- Require `shadowing_config.media_duration_ms > 0`.
- Require `shadowing_config.source.source_type`, `source_name`, `license`, `permission_note`, `subtitle_source`.
- Require non-empty `sentences`.
- Require each sentence to have `tokens`, `chinese`, `start_ms`, `end_ms`.
- Require `0 <= start_ms < end_ms`.
- Require sentence timing to be non-overlapping and sorted.
- Fail if the last `end_ms` exceeds `media_duration_ms + 1000`.

Use exit code `0` on success and `1` on validation failure. Print each error as `ERROR: <title>: <message>`.

- [ ] **Step 14: Run validator fixtures**

Run:

```bash
python scripts/validate_lessons_shadowing.py --file scripts/testdata/shadowing_valid_lesson.json
python scripts/validate_lessons_shadowing.py --file scripts/testdata/shadowing_invalid_lesson.json
```

Expected: first command exits `0`; second command exits `1` and prints at least three `ERROR:` lines.

- [ ] **Step 15: Add Makefile helper**

Modify `Makefile`:

```make
validate-shadowing-pilot: ## Validate manual shadowing pilot lesson pack
	python scripts/validate_lessons_shadowing.py --file ./data/seed/lessons_shadowing_pilot.json
```

- [ ] **Step 16: Commit lesson content pipeline**

Run:

```bash
git add internal/module/lesson/model.go internal/data/lesson_store.go internal/data/lesson_store_test.go internal/cli/import_lessons.go internal/cli/import_lessons_test.go internal/cli/lesson_duplicates.go internal/cli/root.go scripts/validate_lessons_shadowing.py scripts/testdata/shadowing_valid_lesson.json scripts/testdata/shadowing_invalid_lesson.json Makefile
git commit -m "feat(shadowing): prepare lesson import pipeline"
```

---

### Task 4: Shadowing Domain Models, Store, And Service

**Files:**
- Create: `internal/module/shadowing/model.go`
- Create: `internal/module/shadowing/current_sentence.go`
- Create: `internal/module/shadowing/current_sentence_test.go`
- Create: `internal/module/shadowing/service.go`
- Create: `internal/module/shadowing/service_test.go`
- Create: `internal/data/shadowing_store.go`
- Create: `internal/data/shadowing_store_test.go`

- [ ] **Step 1: Write current-sentence tests**

Add table-driven tests in `internal/module/shadowing/current_sentence_test.go`:

```go
package shadowing

import (
	"testing"

	"japanese-learning-app/internal/module/lesson"
)

func TestCurrentSentenceIndex(t *testing.T) {
	sentences := []lesson.Sentence{
		{Index: 0, StartMS: 1000, EndMS: 2000},
		{Index: 1, StartMS: 3000, EndMS: 4000},
		{Index: 2, StartMS: 4000, EndMS: 5000},
	}

	tests := []struct {
		name string
		time int64
		want int
		ok   bool
	}{
		{name: "empty", time: 0, want: -1, ok: false},
		{name: "before first keeps first", time: 0, want: 0, ok: true},
		{name: "inside first", time: 1500, want: 0, ok: true},
		{name: "gap keeps previous", time: 2500, want: 0, ok: true},
		{name: "boundary uses next", time: 3000, want: 1, ok: true},
		{name: "after last keeps last", time: 6000, want: 2, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sentences
			if tt.name == "empty" {
				input = nil
			}
			got, ok := CurrentSentenceIndex(input, tt.time)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("CurrentSentenceIndex() = %d,%v want %d,%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
```

- [ ] **Step 2: Run current-sentence test and verify it fails**

Run:

```bash
go test ./internal/module/shadowing -run TestCurrentSentenceIndex -v -count=1
```

Expected: fail because package does not exist.

- [ ] **Step 3: Implement current-sentence function**

Create `internal/module/shadowing/current_sentence.go`:

```go
package shadowing

import "japanese-learning-app/internal/module/lesson"

func CurrentSentenceIndex(sentences []lesson.Sentence, currentTimeMS int64) (int, bool) {
	if len(sentences) == 0 {
		return -1, false
	}

	lastValid := -1
	for _, s := range sentences {
		if s.EndMS <= s.StartMS {
			continue
		}
		if currentTimeMS < s.StartMS {
			if lastValid == -1 {
				return s.Index, true
			}
			return lastValid, true
		}
		if currentTimeMS >= s.StartMS && currentTimeMS < s.EndMS {
			return s.Index, true
		}
		lastValid = s.Index
	}
	if lastValid == -1 {
		return -1, false
	}
	return lastValid, true
}
```

- [ ] **Step 4: Define shadowing models**

Create `internal/module/shadowing/model.go`:

```go
package shadowing

import "japanese-learning-app/internal/module/lesson"

type PracticeMode string

const (
	ModeNormal PracticeMode = "normal"
	ModeSlow   PracticeMode = "slow"
	ModeLoop   PracticeMode = "loop"
	ModeRecord PracticeMode = "record"
)

const (
	ErrUnauthorized       = "ERR_UNAUTHORIZED"
	ErrNotFound           = "ERR_NOT_FOUND"
	ErrDisabled           = "ERR_SHADOWING_DISABLED"
	ErrMediaMissing       = "ERR_SHADOWING_MEDIA_MISSING"
	ErrContentInvalid     = "ERR_SHADOWING_CONTENT_INVALID"
	ErrVersionStale       = "ERR_SHADOWING_VERSION_STALE"
	ErrAttemptModeInvalid = "ERR_SHADOWING_ATTEMPT_MODE_INVALID"
)

type Progress struct {
	UserID             int64        `json:"user_id"`
	LessonID           int64        `json:"lesson_id"`
	ShadowingVersion  int          `json:"shadowing_version"`
	LastSentenceIndex  int          `json:"last_sentence_index"`
	LastPositionMS     int64        `json:"last_position_ms"`
	LastPracticeMode   PracticeMode `json:"last_practice_mode"`
}

type Attempt struct {
	UserID            int64        `json:"user_id"`
	LessonID          int64        `json:"lesson_id"`
	ShadowingVersion int          `json:"shadowing_version"`
	SentenceIndex     int          `json:"sentence_index"`
	PracticeMode      PracticeMode `json:"practice_mode"`
	PlaybackRate      float64      `json:"playback_rate"`
	LoopCount         int          `json:"loop_count"`
	SelfScore         *int         `json:"self_score"`
	RecognitionText   string       `json:"recognition_text"`
	AudioRef          string       `json:"audio_ref"`
}

type SentenceAttemptSummary struct {
	SentenceIndex int  `json:"sentence_index"`
	AttemptCount  int  `json:"attempt_count"`
	BestScore     *int `json:"best_score"`
	LastScore     *int `json:"last_score"`
}

type ShadowingSession struct {
	Lesson                   *lesson.Lesson             `json:"lesson"`
	Progress                 *Progress                  `json:"progress"`
	CompletedSentenceIndexes []int                      `json:"completed_sentence_indexes"`
	CompletedSentenceCount   int                        `json:"completed_sentence_count"`
	AttemptSummary           []SentenceAttemptSummary   `json:"attempt_summary"`
}
```

- [ ] **Step 5: Write SQLite shadowing store tests**

Add `internal/data/shadowing_store_test.go` with tests for:

- `UpsertProgress` inserts then updates one row for `(user_id, lesson_id, shadowing_version)`.
- `InsertAttempt` accepts `self_score = NULL`.
- `ListAttemptSummary` counts all current-version attempts.
- `ListAttemptSummary` ignores attempts from older `shadowing_version`.
- `best_score` and `last_score` only use non-null `self_score`.

Use this expected assertion for version isolation:

```go
if summary[0].AttemptCount != 1 {
	t.Fatalf("AttemptCount = %d, want 1 current-version attempt", summary[0].AttemptCount)
}
```

- [ ] **Step 6: Implement SQLite shadowing store**

Create `internal/data/shadowing_store.go`:

```go
type ShadowingStore struct {
	db *sql.DB
}

func NewShadowingStore(db *sql.DB) *ShadowingStore {
	return &ShadowingStore{db: db}
}
```

Implement:

```go
func (s *ShadowingStore) GetProgress(userID, lessonID int64, version int) (*shadowing.Progress, error)
func (s *ShadowingStore) UpsertProgress(p shadowing.Progress) error
func (s *ShadowingStore) InsertAttempt(a shadowing.Attempt) error
func (s *ShadowingStore) ListAttemptSummary(userID, lessonID int64, version int) ([]shadowing.SentenceAttemptSummary, error)
func (s *ShadowingStore) ListCompletedSentenceIndexes(userID, lessonID int64, version int) ([]int, error)
```

Use `INSERT ... ON CONFLICT(user_id, lesson_id, shadowing_version) DO UPDATE` for progress. For summaries, query attempts ordered by sentence and created time; compute `best_score` and `last_score` in Go so nullable score semantics are explicit and easy to test.

- [ ] **Step 7: Run SQLite shadowing store tests**

Run:

```bash
go test ./internal/data -run 'TestShadowingStore_' -v -count=1
```

Expected: pass.

- [ ] **Step 8: Write service tests**

Add `internal/module/shadowing/service_test.go` covering:

- disabled lesson returns `ErrDisabled`.
- enabled lesson with empty `audio_url` returns `ErrMediaMissing`.
- empty sentence list returns `ErrContentInvalid`.
- stale version on `SaveProgress` returns `ErrVersionStale` and current version.
- `SaveAttempt` rejects `normal` and `slow`.
- `GetSession` returns completion derived from current-version attempts only.

Use fake stores in the test file rather than mocks from external libraries.

- [ ] **Step 9: Implement service**

Create `internal/module/shadowing/service.go`:

```go
type LessonReader interface {
	GetDetail(id int64) (*lesson.Lesson, error)
}

type Store interface {
	GetProgress(userID, lessonID int64, version int) (*Progress, error)
	UpsertProgress(p Progress) error
	InsertAttempt(a Attempt) error
	ListAttemptSummary(userID, lessonID int64, version int) ([]SentenceAttemptSummary, error)
	ListCompletedSentenceIndexes(userID, lessonID int64, version int) ([]int, error)
}

type Service struct {
	lessons LessonReader
	store   Store
}

func NewService(lessons LessonReader, store Store) *Service {
	return &Service{lessons: lessons, store: store}
}
```

Service rules:

- `GetSession(userID, lessonID)` loads lesson, validates `shadowing_enabled`, validates `audio_url`, validates sentences, reads progress and attempts for `lesson.ShadowingVersion`, and returns `ShadowingSession`.
- `SaveProgress(userID, lessonID, req)` validates current lesson version before upsert.
- `SaveAttempt(userID, lessonID, req)` validates current lesson version, sentence index range, and mode. Only `loop` and `record` are accepted in Phase 1.
- Wrap lower-level errors with `fmt.Errorf("shadowing.Service.<method>: %w", err)` and log failures.

- [ ] **Step 10: Run shadowing module tests**

Run:

```bash
go test ./internal/module/shadowing -v -count=1
```

Expected: pass.

- [ ] **Step 11: Commit shadowing domain and store**

Run:

```bash
git add internal/module/shadowing internal/data/shadowing_store.go internal/data/shadowing_store_test.go
git commit -m "feat(shadowing): add audio practice domain"
```

---

### Task 5: Shadowing HTTP API And Server Wiring

**Files:**
- Create: `internal/module/shadowing/handler.go`
- Create: `internal/module/shadowing/handler_test.go`
- Modify: `backend/cmd/server/main.go`
- Modify: `internal/data/adapters.go`

- [ ] **Step 1: Write handler tests**

Add `internal/module/shadowing/handler_test.go` with `httptest` coverage for:

- missing context user returns `401 ERR_UNAUTHORIZED`.
- `GET /api/v1/lessons/1/shadowing` returns session JSON.
- `POST /progress` sends stale version and receives `409 ERR_SHADOWING_VERSION_STALE` plus `details.current_shadowing_version`.
- `POST /attempts` with `normal` receives `422 ERR_SHADOWING_ATTEMPT_MODE_INVALID`.

Use a fake service interface:

```go
type fakeShadowingService struct {
	session *ShadowingSession
	err     error
}
```

Inject user ID with the real helper pattern by wrapping the handler in `user.AuthMiddleware` for one integration-style case, and use a local request context only for direct handler unit cases if needed.

- [ ] **Step 2: Run handler tests and verify they fail**

Run:

```bash
go test ./internal/module/shadowing -run 'TestHandler_' -v -count=1
```

Expected: fail because handler does not exist.

- [ ] **Step 3: Implement handler routes**

Create `internal/module/shadowing/handler.go`:

```go
type ServiceInterface interface {
	GetSession(userID, lessonID int64) (*ShadowingSession, error)
	SaveProgress(userID, lessonID int64, req ProgressRequest) (*Progress, error)
	SaveAttempt(userID, lessonID int64, req AttemptRequest) error
}

type Handler struct {
	svc ServiceInterface
}

func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/lessons/{id}/shadowing", h.handleGetSession)
	mux.HandleFunc("POST /api/v1/lessons/{id}/shadowing/progress", h.handleSaveProgress)
	mux.HandleFunc("POST /api/v1/lessons/{id}/shadowing/attempts", h.handleSaveAttempt)
}
```

Every handler must:

- Parse `lesson_id` from `r.PathValue("id")`.
- Read user ID from `user.UserIDFromContext(r.Context())`.
- Return `401 ERR_UNAUTHORIZED` when user ID is missing.
- Decode JSON request bodies with `json.NewDecoder(r.Body).Decode(&req)`.
- Map service errors to the documented status and code.
- Use `httputil.WriteErrorWithDetails` for stale version.

- [ ] **Step 4: Register runtime wiring in server**

Modify `backend/cmd/server/main.go`:

```go
shadowingStore := data.NewShadowingStore(db)
shadowingSvc := shadowing.NewService(lessonAdapter, shadowingStore)
shadowingH := shadowing.NewHandler(shadowingSvc)
```

Register routes inside the protected mux:

```go
shadowingH.RegisterRoutes(protectedMux)
```

The existing mux handle for `/api/v1/lessons/` already wraps lesson child paths with `AuthMiddleware`, so no new top-level mux prefix is needed.

- [ ] **Step 5: Run handler and server build**

Run:

```bash
go test ./internal/module/shadowing -v -count=1
go build ./backend/cmd/server/
```

Expected: pass.

- [ ] **Step 6: Commit API wiring**

Run:

```bash
git add internal/module/shadowing/handler.go internal/module/shadowing/handler_test.go backend/cmd/server/main.go internal/data/adapters.go
git commit -m "feat(shadowing): expose authenticated lesson APIs"
```

---

### Task 6: Frontend Lesson Contract And Routed Lesson Detail

**Files:**
- Create: `front/react/src/pages/lesson/LessonListPage.tsx`
- Create: `front/react/src/pages/lesson/LessonDetailPage.tsx`
- Modify: `front/react/src/App.tsx`
- Modify: `front/react/src/pages/lesson/LessonPage.tsx`
- Modify: `front/react/src/pages/lesson/LessonPage.module.css`
- Modify: `front/react/src/types/api.ts`

- [ ] **Step 1: Align frontend Lesson types**

Change `front/react/src/types/api.ts`:

```ts
export interface Sentence {
  index: number
  tokens: FuriganaToken[]
  chinese: string
  start_ms: number
  end_ms: number
}

export interface LessonSummary {
  id: number
  title: string
  jlpt_level: JLPTLevel
  tags: string[]
  char_count: number
  audio_url: string
  video_url: string
  shadowing_enabled: boolean
  shadowing_version: number
  shadowing_config: Record<string, unknown>
}

export interface Lesson extends LessonSummary {
  sentences: Sentence[]
  word_ids: number[]
}
```

- [ ] **Step 2: Create routed list page**

Move the current list behavior from `LessonPage.tsx` into `front/react/src/pages/lesson/LessonListPage.tsx`. Use `useNavigate()` instead of internal `setSelected`:

```tsx
const navigate = useNavigate()

<button
  key={lesson.id}
  className={styles.lessonItem}
  onClick={() => navigate(`/lesson/${lesson.id}`)}
>
```

Replace `lesson.sentence_count` with `lesson.char_count`.

- [ ] **Step 3: Create routed detail page**

Create `front/react/src/pages/lesson/LessonDetailPage.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import type { Lesson } from '@/types/api'
import styles from './LessonPage.module.css'

export function LessonDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const navigate = useNavigate()
  const [lesson, setLesson] = useState<Lesson | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showTranslation, setShowTranslation] = useState(false)

  useEffect(() => {
    let ignore = false
    async function load() {
      if (!id) return
      setLoading(true)
      setError('')
      try {
        const data = await apiFetch<Lesson>('GET', `/api/v1/lessons/${id}`)
        if (!ignore) setLesson(data)
      } catch (err) {
        if (!ignore) setError(err instanceof Error ? err.message : 'Failed to load')
      } finally {
        if (!ignore) setLoading(false)
      }
    }
    load()
    return () => { ignore = true }
  }, [id])

  if (loading) return <div className={styles.page}><Spinner size="lg" /></div>
  if (error) return <div className={styles.page}><p style={{ color: 'var(--color-error)' }}>{error}</p></div>
  if (!lesson) return null

  return (
    <div className={styles.page}>
      <button className={styles.backBtn} onClick={() => navigate('/lesson')}>
        {t('lesson.back')}
      </button>
      <h1 className={styles.detailTitle}>{lesson.title}</h1>
      <div className={styles.detailMeta}>
        <Badge level={lesson.jlpt_level} size="sm" />
        <span className={styles.charCount}>{t('lesson.chars', { count: lesson.char_count })}</span>
        {lesson.tags?.map((tag) => <span key={tag} className={styles.tag}>{tag}</span>)}
      </div>
      {lesson.shadowing_enabled && (
        <Link className={styles.shadowingCta} to={`/lesson/${lesson.id}/shadowing`}>
          开始影子跟读
        </Link>
      )}
      <button className={styles.translateToggle} onClick={() => setShowTranslation((v) => !v)}>
        {showTranslation ? t('lesson.translation.hide') : t('lesson.translation.show')}
      </button>
      <div className={styles.sentences}>
        {lesson.sentences?.map((sentence) => (
          <div key={sentence.index} className={styles.sentenceBlock}>
            <div className={styles.sentenceJa}>
              {sentence.tokens.map((token, i) =>
                token.reading ? <ruby key={i}>{token.surface}<rt>{token.reading}</rt></ruby> : <span key={i}>{token.surface}</span>
              )}
            </div>
            {showTranslation && <div className={styles.sentenceZh}>{sentence.chinese}</div>}
          </div>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Route list and detail**

Modify `front/react/src/App.tsx`:

```tsx
import { LessonListPage } from '@/pages/lesson/LessonListPage'
import { LessonDetailPage } from '@/pages/lesson/LessonDetailPage'
import { ShadowingPage } from '@/pages/shadowing/ShadowingPage'
```

Routes:

```tsx
<Route path="/lesson" element={<LessonListPage />} />
<Route path="/lesson/:id" element={<LessonDetailPage />} />
<Route path="/lesson/:id/shadowing" element={<ShadowingPage />} />
```

For this task, create a compile-safe `front/react/src/pages/shadowing/ShadowingPage.tsx` shell:

```tsx
import { Link, useParams } from 'react-router-dom'
import styles from './ShadowingPage.module.css'

export function ShadowingPage() {
  const { id } = useParams()

  return (
    <div className={styles.page}>
      <Link className={styles.backLink} to={id ? `/lesson/${id}` : '/lesson'}>
        返回课文
      </Link>
      <h1 className={styles.title}>影子跟读</h1>
      <p className={styles.intro}>音频跟读页面正在准备中。</p>
    </div>
  )
}
```

Create `front/react/src/pages/shadowing/ShadowingPage.module.css`:

```css
.page { padding: var(--space-4); max-width: 960px; margin: 0 auto; }
.backLink { color: var(--color-brand); font-weight: 700; text-decoration: none; }
.title { margin: var(--space-5) 0 var(--space-3); color: var(--color-text); }
.intro { color: var(--color-text-secondary); }
```

- [ ] **Step 5: Keep `LessonPage.tsx` as compatibility wrapper**

Change `front/react/src/pages/lesson/LessonPage.tsx`:

```tsx
export { LessonListPage as LessonPage } from './LessonListPage'
```

- [ ] **Step 6: Add CTA style**

Add to `LessonPage.module.css`:

```css
.shadowingCta {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin: 0 0 var(--space-5) 0;
  padding: var(--space-2) var(--space-5);
  border-radius: var(--radius-full);
  background: var(--color-brand);
  color: #fff;
  font-weight: 700;
  text-decoration: none;
}
```

- [ ] **Step 7: Build frontend**

Run:

```bash
cd front/react
npm run build
```

Expected: pass.

- [ ] **Step 8: Commit routed lesson pages**

Run:

```bash
git add front/react/src/App.tsx front/react/src/types/api.ts front/react/src/pages/lesson front/react/src/pages/shadowing
git commit -m "feat(shadowing): route lesson detail pages"
```

---

### Task 7: Frontend Shadowing API And Current Sentence Utility

**Files:**
- Create: `front/react/src/api/shadowing.ts`
- Create: `front/react/src/types/shadowing.ts`
- Create: `front/react/src/util/shadowing/currentSentence.ts`
- Create: `front/react/src/util/shadowing/currentSentence.test.ts`

- [ ] **Step 1: Add frontend shadowing DTOs**

Create `front/react/src/types/shadowing.ts`:

```ts
import type { Lesson } from './api'

export type PracticeMode = 'normal' | 'slow' | 'loop' | 'record'

export interface ShadowingProgress {
  user_id: number
  lesson_id: number
  shadowing_version: number
  last_sentence_index: number
  last_position_ms: number
  last_practice_mode: PracticeMode
}

export interface SentenceAttemptSummary {
  sentence_index: number
  attempt_count: number
  best_score: number | null
  last_score: number | null
}

export interface ShadowingSession {
  lesson: Lesson
  progress: ShadowingProgress | null
  completed_sentence_indexes: number[]
  completed_sentence_count: number
  attempt_summary: SentenceAttemptSummary[]
}

export interface SaveProgressRequest {
  shadowing_version: number
  last_sentence_index: number
  last_position_ms: number
  last_practice_mode: PracticeMode
}

export interface SaveAttemptRequest {
  shadowing_version: number
  sentence_index: number
  practice_mode: PracticeMode
  playback_rate: number
  loop_count: number
  self_score?: number | null
}
```

- [ ] **Step 2: Add API client wrappers**

Create `front/react/src/api/shadowing.ts`:

```ts
import { apiFetch } from './client'
import type { SaveAttemptRequest, SaveProgressRequest, ShadowingProgress, ShadowingSession } from '@/types/shadowing'

export function getShadowingSession(lessonId: number, signal?: AbortSignal) {
  return apiFetch<ShadowingSession>('GET', `/api/v1/lessons/${lessonId}/shadowing`, undefined, signal)
}

export function saveShadowingProgress(lessonId: number, body: SaveProgressRequest) {
  return apiFetch<ShadowingProgress>('POST', `/api/v1/lessons/${lessonId}/shadowing/progress`, body)
}

export function saveShadowingAttempt(lessonId: number, body: SaveAttemptRequest) {
  return apiFetch<void>('POST', `/api/v1/lessons/${lessonId}/shadowing/attempts`, body)
}
```

- [ ] **Step 3: Write current-sentence utility tests**

Create `front/react/src/util/shadowing/currentSentence.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { Sentence } from '@/types/api'
import { getCurrentSentenceIndex } from './currentSentence'

const sentences: Sentence[] = [
  { index: 0, tokens: [], chinese: '一', start_ms: 1000, end_ms: 2000 },
  { index: 1, tokens: [], chinese: '二', start_ms: 3000, end_ms: 4000 },
  { index: 2, tokens: [], chinese: '三', start_ms: 4000, end_ms: 5000 },
]

describe('getCurrentSentenceIndex', () => {
  it('returns null for empty list', () => {
    expect(getCurrentSentenceIndex([], 1000)).toBeNull()
  })

  it('uses first sentence before first start', () => {
    expect(getCurrentSentenceIndex(sentences, 0)).toBe(0)
  })

  it('keeps previous sentence in gaps', () => {
    expect(getCurrentSentenceIndex(sentences, 2500)).toBe(0)
  })

  it('uses next sentence at exact start boundary', () => {
    expect(getCurrentSentenceIndex(sentences, 3000)).toBe(1)
  })

  it('keeps last sentence after media ends', () => {
    expect(getCurrentSentenceIndex(sentences, 6000)).toBe(2)
  })
})
```

- [ ] **Step 4: Run utility test and verify it fails**

Run:

```bash
cd front/react
npm test -- src/util/shadowing/currentSentence.test.ts
```

Expected: fail because utility does not exist.

- [ ] **Step 5: Implement utility**

Create `front/react/src/util/shadowing/currentSentence.ts`:

```ts
import type { Sentence } from '@/types/api'

export function getCurrentSentenceIndex(sentences: Sentence[], currentTimeMs: number): number | null {
  if (sentences.length === 0) return null

  let lastValid: number | null = null
  for (const sentence of sentences) {
    if (sentence.end_ms <= sentence.start_ms) continue
    if (currentTimeMs < sentence.start_ms) {
      return lastValid ?? sentence.index
    }
    if (currentTimeMs >= sentence.start_ms && currentTimeMs < sentence.end_ms) {
      return sentence.index
    }
    lastValid = sentence.index
  }

  return lastValid
}
```

- [ ] **Step 6: Run frontend utility tests**

Run:

```bash
cd front/react
npm test -- src/util/shadowing/currentSentence.test.ts
```

Expected: pass.

- [ ] **Step 7: Commit frontend API utility**

Run:

```bash
git add front/react/src/api/shadowing.ts front/react/src/types/shadowing.ts front/react/src/util/shadowing
git commit -m "feat(shadowing): add frontend session client"
```

---

### Task 8: Audio Shadowing Page And Recorder Cleanup

**Files:**
- Modify: `front/react/src/pages/shadowing/ShadowingPage.tsx`
- Modify: `front/react/src/pages/shadowing/ShadowingPage.module.css`
- Modify: `front/react/src/hooks/useAudioRecorder.ts`

- [ ] **Step 1: Fix recorder object URL lifecycle**

Modify `front/react/src/hooks/useAudioRecorder.ts`:

```ts
import { useCallback, useEffect, useRef, useState } from 'react'
```

Add:

```ts
const audioURLRef = useRef<string | null>(null)

const revokeAudioURL = useCallback(() => {
  if (audioURLRef.current) {
    URL.revokeObjectURL(audioURLRef.current)
    audioURLRef.current = null
  }
}, [])

useEffect(() => revokeAudioURL, [revokeAudioURL])
```

At the start of `start`, call:

```ts
revokeAudioURL()
```

When creating a new URL in `stop`, store it:

```ts
const url = URL.createObjectURL(blob)
audioURLRef.current = url
```

Add `revokeAudioURL` to the dependency arrays for `start` and `stop` where used.

- [ ] **Step 2: Implement ShadowingPage data loading**

`front/react/src/pages/shadowing/ShadowingPage.tsx` should:

- Read `id` from `useParams`.
- Fetch `getShadowingSession(Number(id))`.
- Render loading, API error, and loaded states.
- If backend returns `ERR_SHADOWING_VERSION_STALE` after a write, refetch the session.
- Use only `<audio>`, not `<video>`.

Core state:

```tsx
const audioRef = useRef<HTMLAudioElement | null>(null)
const [session, setSession] = useState<ShadowingSession | null>(null)
const [currentSentenceIndex, setCurrentSentenceIndex] = useState<number | null>(null)
const [playbackRate, setPlaybackRate] = useState(1)
const [looping, setLooping] = useState(false)
const [selfScore, setSelfScore] = useState(80)
const recorder = useAudioRecorder()
```

- [ ] **Step 3: Implement audio time tracking and seek**

Use `getCurrentSentenceIndex` on `timeupdate`:

```tsx
function handleTimeUpdate() {
  const audio = audioRef.current
  if (!audio || !session) return
  const next = getCurrentSentenceIndex(session.lesson.sentences, Math.floor(audio.currentTime * 1000))
  setCurrentSentenceIndex(next)
}

function seekToSentence(index: number) {
  const audio = audioRef.current
  const sentence = session?.lesson.sentences.find((s) => s.index === index)
  if (!audio || !sentence) return
  audio.currentTime = sentence.start_ms / 1000
  setCurrentSentenceIndex(sentence.index)
}
```

On first load, if progress exists:

```tsx
audio.currentTime = session.progress.last_position_ms / 1000
setCurrentSentenceIndex(session.progress.last_sentence_index)
```

- [ ] **Step 4: Implement slow and loop controls**

Slow mode:

```tsx
function playSlow() {
  const audio = audioRef.current
  if (!audio || currentSentenceIndex === null) return
  audio.playbackRate = 0.75
  setPlaybackRate(0.75)
  seekToSentence(currentSentenceIndex)
  void audio.play()
}
```

Loop mode should loop only the current sentence. On `timeupdate`, if looping and current time is at or beyond current sentence `end_ms`, seek back to `start_ms`, increment local loop count, and call `saveShadowingAttempt` with mode `loop` after the configured loop count is reached.

- [ ] **Step 5: Implement record and self-score attempt**

Record button behavior:

- If not recording, call `recorder.start()`.
- If recording, call `recorder.stop()`.
- After stop, call `saveShadowingAttempt` with mode `record`, current sentence index, playback rate, loop count `0`, and `self_score`.
- Keep audio blob only through `recorder.audioURL` for local playback.

Payload:

```ts
await saveShadowingAttempt(session.lesson.id, {
  shadowing_version: session.lesson.shadowing_version,
  sentence_index: currentSentenceIndex,
  practice_mode: 'record',
  playback_rate: playbackRate,
  loop_count: 0,
  self_score: selfScore,
})
```

- [ ] **Step 6: Save progress on sentence changes and before unload**

Debounce manually with a `setTimeout` ref or save on meaningful transitions:

```ts
await saveShadowingProgress(session.lesson.id, {
  shadowing_version: session.lesson.shadowing_version,
  last_sentence_index: currentSentenceIndex,
  last_position_ms: Math.floor((audioRef.current?.currentTime ?? 0) * 1000),
  last_practice_mode: looping ? 'loop' : playbackRate === 0.75 ? 'slow' : 'normal',
})
```

Do not write `normal` or `slow` attempts; only progress can use those modes in Phase 1.

- [ ] **Step 7: Render audio-first UI**

The page must render:

- Back link to `/lesson/:id`.
- Lesson title and completion count.
- `<audio controls src={session.lesson.audio_url}>`.
- Current sentence card with Japanese ruby and Chinese text.
- Buttons for previous sentence, replay, slow, loop, record, play recording.
- Self-score input `0..100`.
- Subtitle list where each sentence is clickable and highlights current/completed state.

CSS can stay inside `ShadowingPage.module.css` and should use existing CSS variables. Add mobile fixed bottom controls only after desktop layout works.

- [ ] **Step 8: Run frontend build**

Run:

```bash
cd front/react
npm run build
```

Expected: pass.

- [ ] **Step 9: Commit shadowing page**

Run:

```bash
git add front/react/src/pages/shadowing front/react/src/hooks/useAudioRecorder.ts
git commit -m "feat(shadowing): add audio practice page"
```

---

### Task 9: Manual Pilot Material Pack And End-To-End Verification

**Files:**
- Create: `data/seed/lessons_shadowing_pilot.json`
- Modify: `README.md` or `docs/architecture.md` only if the project docs need a short operator note.

- [ ] **Step 1: Select 1 to 3 owned or authorized lessons**

Use existing `data/seed/lessons_*.json` as source text. For each selected lesson, prepare:

- `audio_url` under `/audio/lessons/...`.
- `shadowing_enabled: true`.
- `shadowing_version: 1`.
- `shadowing_config.media_type: "audio"`.
- `shadowing_config.media_duration_ms`.
- `shadowing_config.source` with `source_type`, `source_name`, `source_url`, `license`, `permission_note`, `subtitle_source`.
- sentence-level `start_ms` and `end_ms`.

- [ ] **Step 2: Create pilot import file**

Create `data/seed/lessons_shadowing_pilot.json` with only the selected lessons. Keep the shape compatible with `ImportLessonsFromFile`.

- [ ] **Step 3: Validate pilot material pack**

Run:

```bash
make validate-shadowing-pilot
```

Expected: exit `0`.

- [ ] **Step 4: Check duplicate lesson state before import**

Run:

```bash
go run ./backend/cmd/appctl report-lesson-duplicates --db ./data/app.db
```

Expected: either “no duplicate lessons” or a printed report. If duplicates exist, do not import until cleanup is reviewed.

- [ ] **Step 5: Clean duplicates only after reviewing the report**

Run only after confirming the report:

```bash
go run ./backend/cmd/appctl cleanup-lesson-duplicates --db ./data/app.db
go run ./backend/cmd/appctl create-lesson-unique-index --db ./data/app.db
```

Expected: cleanup prints deleted count; unique-index command succeeds only when no duplicates remain.

- [ ] **Step 6: Import pilot lessons**

Run:

```bash
go run ./backend/cmd/appctl import-lessons --db ./data/app.db --file ./data/seed/lessons_shadowing_pilot.json
```

Expected: imported or updated count prints without errors.

- [ ] **Step 7: Run full backend tests**

Run:

```bash
make test
```

Expected: pass.

- [ ] **Step 8: Run frontend tests and build**

Run:

```bash
cd front/react
npm test
npm run build
```

Expected: pass.

- [ ] **Step 9: Manual browser acceptance**

Start services:

```bash
make start-backend
make start-learner-front
```

Open:

```text
http://localhost:35173/lesson
```

Acceptance checklist:

- `/lesson` lists lessons.
- Clicking a lesson navigates to `/lesson/:id`.
- Refreshing `/lesson/:id` reloads the same lesson.
- Enabled pilot lesson shows “开始影子跟读”.
- `/lesson/:id/shadowing` loads directly after refresh.
- Audio plays.
- Current sentence highlights during playback.
- Clicking a subtitle seeks to that sentence.
- Slow playback uses `0.75x`.
- Loop records an attempt.
- Recording creates local playback.
- Self-score persists as an attempt.
- Refresh restores last sentence or position.
- Browser back from shadowing returns to lesson detail.

- [ ] **Step 10: Commit pilot material and operator note**

Run:

```bash
git add data/seed/lessons_shadowing_pilot.json README.md docs/architecture.md
git commit -m "docs(shadowing): add pilot material workflow"
```

If no README or architecture edits were needed, commit only the pilot pack:

```bash
git add data/seed/lessons_shadowing_pilot.json
git commit -m "data(shadowing): add pilot lesson pack"
```

---

## Final Verification

Run the full verification set before declaring the MVP complete:

```bash
go test ./... -v -count=1
cd front/react
npm test
npm run build
cd ../..
go build ./backend/cmd/server/
go build ./backend/cmd/appctl/
```

Expected:

- All Go tests pass.
- All Vitest tests pass.
- React build passes.
- Server and appctl binaries compile.
- Manual acceptance checklist passes for at least one pilot lesson.

---

## Self-Review Checklist

- Spec coverage: Phase 0 and Phase 1 requirements are covered by Tasks 1 through 9.
- Scope control: Phase 2/3 video, admin, AI generation, ASR, and persistent recording upload are excluded.
- PostgreSQL boundary: PostgreSQL is schema-only and has no runtime shadowing route wiring.
- Migration safety: automatic SQLite migration adds columns/tables only; duplicate cleanup and unique index are CLI/manual actions.
- Completion truth source: attempts drive completion; progress is only resumable session state.
- Version isolation: backend store/service/tests aggregate only the current `shadowing_version`.
- Frontend video boundary: `video_url` is ignored by Phase 1 UI.
- Error details: backend and frontend preserve `details.current_shadowing_version`.
- Tests: new behavior starts with failing tests where code behavior changes.
