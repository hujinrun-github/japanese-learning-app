# Shadowing Material Video Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a PostgreSQL + MinIO shadowing material import workflow shared by CLI and Admin, with TDD coverage for video binding, upload limits, storage checks, and URL-only conflict behavior.

**Architecture:** Create a shared `internal/module/shadowingmaterial` service that owns lesson material import, video object validation, upload metadata insertion, and lesson video binding. CLI and Admin become adapters around that service, while the Admin page gains a reviewed JSON import action. Existing upload and bind endpoints are routed through the same media validation rules so they cannot bypass limits.

**Tech Stack:** Go 1.24, `database/sql`, PostgreSQL JSONB, MinIO Go SDK, `net/http`, React 18, TypeScript, Vite, table-driven Go tests.

---

## Preconditions

- Use a disposable PostgreSQL database for tests because existing PG tests reset the `public` schema.
- In PowerShell, set `DATABASE_URL_TEST` before running PG tests:

```powershell
$env:DATABASE_URL_TEST = "postgres://postgres:12345@192.168.1.20:19588/japanese_test?sslmode=disable"
```

- Do not point `DATABASE_URL_TEST` at the real `japanese` database.
- Keep each task TDD-shaped: write the failing test, run it, implement the smallest change, rerun tests, then commit.

## File Map

- Create `internal/module/shadowingmaterial/types.go`: request/result structs, interfaces, default constants.
- Create `internal/module/shadowingmaterial/errors.go`: typed service errors and error-code helpers.
- Create `internal/module/shadowingmaterial/lesson_import.go`: PostgreSQL lesson JSON parsing and upsert logic moved out of CLI.
- Create `internal/module/shadowingmaterial/media.go`: video URL construction, max-byte enforcement, object metadata conflict checks, MinIO uploader/verifier.
- Create `internal/module/shadowingmaterial/service.go`: orchestration for import, upload, bind.
- Create `internal/module/shadowingmaterial/service_test.go`: table-driven service tests.
- Create `internal/module/shadowingmaterial/test_helpers_test.go`: PG schema reset and test fixtures.
- Modify `internal/cli/import_lessons_postgres.go`: keep compatibility wrappers around the shared service.
- Modify `internal/cli/root.go`: add video flags and call the shared service.
- Modify `internal/cli/import_lessons_postgres_test.go`: keep old tests passing and add CLI flag tests.
- Modify `internal/module/admin/handler.go`: add shared service config fields and import route.
- Modify `internal/module/admin/shadowing_materials.go`: route import, upload, and bind through shared service.
- Modify `internal/module/admin/shadowing_materials_test.go`: add handler tests for import, upload limit, kind check, and availability check.
- Modify `backend/cmd/admin/main.go`: pass `SHADOWING_VIDEO_MAX_BYTES` and MinIO dependencies into Admin handler config.
- Modify `front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.tsx`: add reviewed JSON import workflow.
- Modify `front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.module.css`: style import controls.
- Modify `docs/video-storage.md`: document the final import/storage contract.

## Shared API Shape

Implement these shared types first so all later tasks use the same vocabulary:

```go
package shadowingmaterial

import (
	"context"
	"database/sql"
	"io"
)

const (
	DefaultVideoBucket   = "lesson-videos"
	DefaultVideoBasePath = "/api/v1/videos"
	DefaultMaxVideoBytes = int64(209715200)
	VideoKindLesson     = "lesson_shadowing"
)

type VideoUploader interface {
	UploadLessonVideo(ctx context.Context, upload VideoUpload) (UploadedVideo, error)
}

type VideoObjectVerifier interface {
	StatObject(ctx context.Context, bucket, objectKey string) error
}

type VideoUpload struct {
	Filename    string
	ContentType string
	Reader      io.Reader
	SizeBytes   int64
}

type UploadedVideo struct {
	Bucket        string
	ObjectKey     string
	ContentSHA256 string
	SizeBytes     int64
	MimeType      string
}

type ImportRequest struct {
	LessonJSON       []byte
	VideoObjectID    *int64
	VideoFile        *VideoUpload
	ClearVideoObject bool
}

type ImportResult struct {
	LessonID        int64
	Inserted        bool
	VideoObjectID   *int64
	VideoURL        string
	ShadowingConfig map[string]any
}

type BatchImportResult struct {
	Inserted int
	Lessons  []ImportResult
}

type Service struct {
	DB            *sql.DB
	VideoBasePath string
	Bucket        string
	MaxVideoBytes int64
	Uploader      VideoUploader
	Verifier      VideoObjectVerifier
}
```

## Task 1: Create Shared Package Skeleton And Error Contract

**Files:**
- Create: `internal/module/shadowingmaterial/types.go`
- Create: `internal/module/shadowingmaterial/errors.go`
- Create: `internal/module/shadowingmaterial/service_test.go`
- Create: `internal/module/shadowingmaterial/test_helpers_test.go`

- [ ] **Step 1: Write failing tests for error codes and default options**

Add this test to `internal/module/shadowingmaterial/service_test.go`:

```go
package shadowingmaterial

import (
	"errors"
	"net/http"
	"testing"
)

func TestServiceErrorStatusAndCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "video too large", err: ErrVideoTooLarge, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "ERR_VIDEO_TOO_LARGE"},
		{name: "video object conflict", err: ErrVideoObjectConflict, wantStatus: http.StatusConflict, wantCode: "ERR_VIDEO_OBJECT_CONFLICT"},
		{name: "video object unavailable", err: ErrVideoObjectUnavailable, wantStatus: http.StatusConflict, wantCode: "ERR_VIDEO_OBJECT_UNAVAILABLE"},
		{name: "video storage unavailable", err: ErrVideoStorageUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "ERR_VIDEO_STORAGE_UNAVAILABLE"},
		{name: "video storage check failed", err: ErrVideoStorageCheckFailed, wantStatus: http.StatusBadGateway, wantCode: "ERR_VIDEO_STORAGE_CHECK_FAILED"},
		{name: "bad request", err: ErrBadRequest, wantStatus: http.StatusBadRequest, wantCode: "ERR_BAD_REQUEST"},
		{name: "invalid lesson", err: ErrInvalidLesson, wantStatus: http.StatusUnprocessableEntity, wantCode: "ERR_INVALID_LESSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusCode(tt.err); got != tt.wantStatus {
				t.Fatalf("StatusCode() = %d, want %d", got, tt.wantStatus)
			}
			if got := Code(tt.err); got != tt.wantCode {
				t.Fatalf("Code() = %q, want %q", got, tt.wantCode)
			}
		})
	}
}

func TestWrapPreservesServiceError(t *testing.T) {
	err := Wrap(ErrVideoObjectConflict, "existing object metadata differs")
	if !errors.Is(err, ErrVideoObjectConflict) {
		t.Fatalf("errors.Is(err, ErrVideoObjectConflict) = false")
	}
	if StatusCode(err) != http.StatusConflict {
		t.Fatalf("StatusCode(wrapped) = %d, want %d", StatusCode(err), http.StatusConflict)
	}
	if Code(err) != "ERR_VIDEO_OBJECT_CONFLICT" {
		t.Fatalf("Code(wrapped) = %q, want ERR_VIDEO_OBJECT_CONFLICT", Code(err))
	}
}

func TestServiceDefaults(t *testing.T) {
	service := Service{}
	service.ApplyDefaults()

	if service.VideoBasePath != DefaultVideoBasePath {
		t.Fatalf("VideoBasePath = %q, want %q", service.VideoBasePath, DefaultVideoBasePath)
	}
	if service.Bucket != DefaultVideoBucket {
		t.Fatalf("Bucket = %q, want %q", service.Bucket, DefaultVideoBucket)
	}
	if service.MaxVideoBytes != DefaultMaxVideoBytes {
		t.Fatalf("MaxVideoBytes = %d, want %d", service.MaxVideoBytes, DefaultMaxVideoBytes)
	}
}
```

- [ ] **Step 2: Run the failing test**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestServiceErrorStatusAndCode|TestWrapPreservesServiceError|TestServiceDefaults" -count=1
```

Expected: FAIL because `internal/module/shadowingmaterial` does not exist.

- [ ] **Step 3: Implement minimal skeleton**

Create `internal/module/shadowingmaterial/types.go` with the shared API shape from the earlier section.

Create `internal/module/shadowingmaterial/errors.go`:

```go
package shadowingmaterial

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrBadRequest                = errors.New("bad request")
	ErrInvalidLesson             = errors.New("invalid lesson")
	ErrVideoTooLarge             = errors.New("video too large")
	ErrVideoObjectConflict       = errors.New("video object conflict")
	ErrVideoObjectUnavailable    = errors.New("video object unavailable")
	ErrVideoStorageUnavailable   = errors.New("video storage unavailable")
	ErrVideoStorageCheckFailed   = errors.New("video storage check failed")
	ErrVideoUploadFailed         = errors.New("video upload failed")
	ErrUnsupportedRelationalMode = errors.New("unsupported relational mode")
)

func Wrap(kind error, message string) error {
	if message == "" {
		return kind
	}
	return fmt.Errorf("%s: %w", message, kind)
}

func Code(err error) string {
	switch {
	case errors.Is(err, ErrVideoTooLarge):
		return "ERR_VIDEO_TOO_LARGE"
	case errors.Is(err, ErrVideoObjectConflict):
		return "ERR_VIDEO_OBJECT_CONFLICT"
	case errors.Is(err, ErrVideoObjectUnavailable):
		return "ERR_VIDEO_OBJECT_UNAVAILABLE"
	case errors.Is(err, ErrVideoStorageUnavailable):
		return "ERR_VIDEO_STORAGE_UNAVAILABLE"
	case errors.Is(err, ErrVideoStorageCheckFailed):
		return "ERR_VIDEO_STORAGE_CHECK_FAILED"
	case errors.Is(err, ErrVideoUploadFailed):
		return "ERR_VIDEO_UPLOAD_FAILED"
	case errors.Is(err, ErrInvalidLesson):
		return "ERR_INVALID_LESSON"
	case errors.Is(err, ErrUnsupportedRelationalMode):
		return "ERR_UNSUPPORTED_RELATIONAL_MODE"
	case errors.Is(err, ErrBadRequest):
		return "ERR_BAD_REQUEST"
	default:
		return "ERR_INTERNAL"
	}
}

func StatusCode(err error) int {
	switch {
	case errors.Is(err, ErrVideoTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrVideoObjectConflict), errors.Is(err, ErrVideoObjectUnavailable):
		return http.StatusConflict
	case errors.Is(err, ErrVideoStorageUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrVideoStorageCheckFailed), errors.Is(err, ErrVideoUploadFailed):
		return http.StatusBadGateway
	case errors.Is(err, ErrInvalidLesson):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrUnsupportedRelationalMode):
		return http.StatusNotImplemented
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
```

Add `ApplyDefaults` to `types.go`:

```go
func (s *Service) ApplyDefaults() {
	if s.VideoBasePath == "" {
		s.VideoBasePath = DefaultVideoBasePath
	}
	if s.Bucket == "" {
		s.Bucket = DefaultVideoBucket
	}
	if s.MaxVideoBytes <= 0 {
		s.MaxVideoBytes = DefaultMaxVideoBytes
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestServiceErrorStatusAndCode|TestWrapPreservesServiceError|TestServiceDefaults" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/module/shadowingmaterial/types.go internal/module/shadowingmaterial/errors.go internal/module/shadowingmaterial/service_test.go
git commit -m "feat(shadowing): add material import service contract"
```

## Task 2: Move PostgreSQL Lesson Import Core Into Shared Service

**Files:**
- Create: `internal/module/shadowingmaterial/lesson_import.go`
- Modify: `internal/cli/import_lessons_postgres.go`
- Modify: `internal/cli/import_lessons_postgres_test.go`
- Test: `internal/module/shadowingmaterial/service_test.go`
- Test: `internal/module/shadowingmaterial/test_helpers_test.go`

- [ ] **Step 1: Write failing shared importer tests**

Add this helper to `internal/module/shadowingmaterial/test_helpers_test.go`:

```go
package shadowingmaterial

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/store"
)

func newMigratedPostgresDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}

	ctx := context.Background()
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{
		DatabaseURL:  url,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("open postgres test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
`); err != nil {
		t.Fatalf("reset postgres schema: %v", err)
	}
	if err := adapter.RunMigrations(ctx, db); err != nil {
		t.Fatalf("run postgres migrations: %v", err)
	}
	return db
}

func lessonJSON(t *testing.T, title string, media map[string]any) []byte {
	t.Helper()
	value := map[string]any{
		"title":             title,
		"jlpt_level":        "N5",
		"tags":              []string{"shadowing"},
		"audio_url":         media["audio_url"],
		"video_url":         media["video_url"],
		"word_ids":          []int64{},
		"shadowing_enabled": true,
		"shadowing_version": 1,
		"shadowing_config":  media["shadowing_config"],
		"sentences": []map[string]any{
			{
				"index":    0,
				"tokens":   []map[string]string{{"surface": "テスト", "reading": "てすと"}},
				"chinese":  "测试句子",
				"start_ms": 0,
				"end_ms":   1200,
			},
		},
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal lesson JSON: %v", err)
	}
	return raw
}
```

Add these tests to `internal/module/shadowingmaterial/service_test.go`:

```go
func TestImportURLOnlyLessonInsertsAndUpdates(t *testing.T) {
	db := newMigratedPostgresDB(t)
	service := Service{DB: db}
	service.ApplyDefaults()

	first, err := service.Import(context.Background(), ImportRequest{
		LessonJSON: lessonJSON(t, "Shared Import", map[string]any{
			"audio_url":         "/audio/old.wav",
			"shadowing_config": map[string]any{"media_type": "audio"},
		}),
	})
	if err != nil {
		t.Fatalf("first Import error: %v", err)
	}
	if first.Inserted != true || first.LessonID <= 0 {
		t.Fatalf("first result = %+v, want inserted lesson id", first)
	}

	second, err := service.Import(context.Background(), ImportRequest{
		LessonJSON: lessonJSON(t, "Shared Import", map[string]any{
			"audio_url":         "/audio/new.wav",
			"shadowing_config": map[string]any{"media_type": "audio"},
		}),
	})
	if err != nil {
		t.Fatalf("second Import error: %v", err)
	}
	if second.Inserted {
		t.Fatalf("second Inserted = true, want false for upsert")
	}
	if second.LessonID != first.LessonID {
		t.Fatalf("second lesson id = %d, want %d", second.LessonID, first.LessonID)
	}

	var count int
	var configRaw string
	if err := db.QueryRow(`
		SELECT COUNT(*), MAX(shadowing_config_json::text)
		FROM lessons
		WHERE title = $1 AND jlpt_level = $2`,
		"Shared Import",
		"N5",
	).Scan(&count, &configRaw); err != nil {
		t.Fatalf("query lesson: %v", err)
	}
	if count != 1 {
		t.Fatalf("lesson count = %d, want 1", count)
	}
	if !strings.Contains(configRaw, "/audio/new.wav") {
		t.Fatalf("shadowing_config_json = %s, want updated audio URL", configRaw)
	}
}

func TestImportRejectsVideoOptionsWithBatchJSON(t *testing.T) {
	db := newMigratedPostgresDB(t)
	service := Service{DB: db}
	service.ApplyDefaults()
	id := int64(1)
	raw := append([]byte("["), lessonJSON(t, "One", map[string]any{"video_url": "/video/one.mp4", "shadowing_config": map[string]any{"media_type": "video"}})...)
	raw = append(raw, ',')
	raw = append(raw, lessonJSON(t, "Two", map[string]any{"video_url": "/video/two.mp4", "shadowing_config": map[string]any{"media_type": "video"}})...)
	raw = append(raw, ']')

	_, err := service.Import(context.Background(), ImportRequest{LessonJSON: raw, VideoObjectID: &id})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("Import error = %v, want ErrBadRequest", err)
	}
}
```

Add imports to `service_test.go`:

```go
import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestImportURLOnlyLessonInsertsAndUpdates|TestImportRejectsVideoOptionsWithBatchJSON" -count=1
```

Expected: FAIL because `Service.Import` is not implemented.

- [ ] **Step 3: Implement lesson JSON parsing and PostgreSQL upsert**

Create `internal/module/shadowingmaterial/lesson_import.go` by moving the PostgreSQL-specific import logic from `internal/cli/import_lessons_postgres.go` into the shared package.

Use these exported functions:

```go
func (s Service) Import(ctx context.Context, req ImportRequest) (ImportResult, error)
func (s Service) ImportBatch(ctx context.Context, raw []byte) (BatchImportResult, error)
func ParseLessonJSON(raw []byte) ([]LessonImport, error)
```

Implementation rules:

- Accept either a single JSON object or a JSON array.
- `Service.Import` requires exactly one lesson.
- `Service.ImportBatch` accepts arrays only for URL-only imports.
- Build `shadowing_config_json` using the existing `audio_url` and `video_url` compatibility rules.
- Do not set `video_object_id` in this task.
- Return `ImportResult{LessonID, Inserted, ShadowingConfig}` for single import.
- Return `BatchImportResult{Inserted, Lessons}` for batch import.

Keep the SQL shape from the existing CLI implementation:

```sql
SELECT id FROM lessons WHERE title = $1 AND jlpt_level = $2
```

```sql
UPDATE lessons
SET tags = COALESCE((SELECT array_agg(value) FROM jsonb_array_elements_text($2::jsonb) AS value), ARRAY[]::text[]),
    char_count = $3,
    shadowing_enabled = $4,
    shadowing_version = $5,
    shadowing_config_json = $6::jsonb,
    updated_at = now()
WHERE id = $1
```

```sql
INSERT INTO lessons (
    title, jlpt_level, tags, char_count,
    shadowing_enabled, shadowing_version, shadowing_config_json, updated_at
)
VALUES (
    $1,
    $2,
    COALESCE((SELECT array_agg(value) FROM jsonb_array_elements_text($3::jsonb) AS value), ARRAY[]::text[]),
    $4,
    $5,
    $6,
    $7::jsonb,
    now()
)
RETURNING id
```

- [ ] **Step 4: Update CLI wrappers without changing CLI behavior**

Modify `internal/cli/import_lessons_postgres.go` so existing public functions call the shared package:

```go
func ImportLessonsToPostgresFromFile(db *sql.DB, filePath string) (int, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportLessonsToPostgresFromFile ReadFile: %w", err)
	}
	service := shadowingmaterial.Service{DB: db}
	service.ApplyDefaults()
	result, err := service.ImportBatch(context.Background(), raw)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportLessonsToPostgresFromFile import: %w", err)
	}
	return result.Inserted, nil
}
```

Use the same pattern for `ImportLessonToPostgresFromJSON`.

- [ ] **Step 5: Run tests to verify pass**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestImportURLOnlyLessonInsertsAndUpdates|TestImportRejectsVideoOptionsWithBatchJSON" -count=1
go test ./internal/cli -run "TestImportLessonsToPostgresFromFileUpsertsContentAndShadowingMediaURL|TestImportLessonsToPostgresFromFilePreservesVideoMediaURL" -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/module/shadowingmaterial internal/cli/import_lessons_postgres.go internal/cli/import_lessons_postgres_test.go
git commit -m "refactor(shadowing): move postgres lesson import into shared service"
```

## Task 3: Add Video Object Binding, URL-Only Conflict, And Canonical Config Tests

**Files:**
- Modify: `internal/module/shadowingmaterial/service_test.go`
- Modify: `internal/module/shadowingmaterial/service.go`
- Modify: `internal/module/shadowingmaterial/media.go`
- Modify: `internal/module/shadowingmaterial/lesson_import.go`

- [ ] **Step 1: Write failing tests for existing video object mode**

Add this fake verifier to `internal/module/shadowingmaterial/service_test.go`:

```go
type fakeVerifier struct {
	missing map[string]bool
	failed  map[string]bool
	calls   []string
}

func (f *fakeVerifier) StatObject(_ context.Context, bucket, objectKey string) error {
	key := bucket + "/" + objectKey
	f.calls = append(f.calls, key)
	if f.missing != nil && f.missing[key] {
		return ErrVideoObjectUnavailable
	}
	if f.failed != nil && f.failed[key] {
		return ErrVideoStorageCheckFailed
	}
	return nil
}
```

Add table-driven tests:

```go
func TestImportWithExistingVideoObject(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		visibility string
		deleted    bool
		verifier   *fakeVerifier
		wantErr    error
	}{
		{name: "valid lesson shadowing object", kind: VideoKindLesson, visibility: "public", verifier: &fakeVerifier{}},
		{name: "wrong kind", kind: "avatar", visibility: "public", verifier: &fakeVerifier{}, wantErr: ErrVideoObjectUnavailable},
		{name: "private object", kind: VideoKindLesson, visibility: "private", verifier: &fakeVerifier{}, wantErr: ErrVideoObjectUnavailable},
		{name: "missing minio object", kind: VideoKindLesson, visibility: "public", verifier: &fakeVerifier{missing: map[string]bool{"lesson-videos/lessons/existing.mp4": true}}, wantErr: ErrVideoObjectUnavailable},
		{name: "storage check failed", kind: VideoKindLesson, visibility: "public", verifier: &fakeVerifier{failed: map[string]bool{"lesson-videos/lessons/existing.mp4": true}}, wantErr: ErrVideoStorageCheckFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newMigratedPostgresDB(t)
			videoObjectID := insertVideoObjectForTest(t, db, tt.kind, tt.visibility, tt.deleted)
			service := Service{DB: db, Verifier: tt.verifier}
			service.ApplyDefaults()

			result, err := service.Import(context.Background(), ImportRequest{
				LessonJSON: lessonJSON(t, "Video Object Import "+tt.name, map[string]any{
					"video_url":         "https://example.invalid/old.mp4",
					"shadowing_config": map[string]any{"media_type": "audio", "media_url": "https://example.invalid/old.mp4"},
				}),
				VideoObjectID: &videoObjectID,
			})

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Import error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Import error: %v", err)
			}
			if result.VideoObjectID == nil || *result.VideoObjectID != videoObjectID {
				t.Fatalf("VideoObjectID = %+v, want %d", result.VideoObjectID, videoObjectID)
			}
			if result.VideoURL != "/api/v1/videos/"+strconv.FormatInt(videoObjectID, 10)+"/stream" {
				t.Fatalf("VideoURL = %q, want canonical stream URL", result.VideoURL)
			}
			assertLessonVideoState(t, db, result.LessonID, videoObjectID, result.VideoURL)
		})
	}
}
```

Add helpers:

```go
func insertVideoObjectForTest(t *testing.T, db *sql.DB, kind, visibility string, deleted bool) int64 {
	t.Helper()
	deletedExpr := "NULL"
	if deleted {
		deletedExpr = "now()"
	}
	var id int64
	query := `
INSERT INTO video_objects (
	bucket, object_key, kind, visibility, content_sha256,
	size_bytes, mime_type, metadata_json, deleted_at
)
VALUES ('lesson-videos', 'lessons/existing.mp4', $1, $2, 'sha-existing', 9, 'video/mp4', '{}'::jsonb, ` + deletedExpr + `)
RETURNING id`
	if err := db.QueryRow(query, kind, visibility).Scan(&id); err != nil {
		t.Fatalf("insert video object: %v", err)
	}
	return id
}

func assertLessonVideoState(t *testing.T, db *sql.DB, lessonID, videoObjectID int64, videoURL string) {
	t.Helper()
	var gotVideoObjectID int64
	var configRaw string
	if err := db.QueryRow(`SELECT video_object_id, shadowing_config_json::text FROM lessons WHERE id = $1`, lessonID).Scan(&gotVideoObjectID, &configRaw); err != nil {
		t.Fatalf("query lesson video state: %v", err)
	}
	if gotVideoObjectID != videoObjectID {
		t.Fatalf("video_object_id = %d, want %d", gotVideoObjectID, videoObjectID)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(configRaw), &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if config["media_type"] != "video" || config["media_url"] != videoURL || config["video_url"] != videoURL {
		t.Fatalf("config = %+v, want canonical video fields %q", config, videoURL)
	}
}
```

- [ ] **Step 2: Write failing tests for URL-only conflict and clearing**

Add:

```go
func TestImportURLOnlyConflictAndClear(t *testing.T) {
	db := newMigratedPostgresDB(t)
	existingVideoID := insertVideoObjectForTest(t, db, VideoKindLesson, "public", false)
	service := Service{DB: db, Verifier: &fakeVerifier{}}
	service.ApplyDefaults()

	first, err := service.Import(context.Background(), ImportRequest{
		LessonJSON:    lessonJSON(t, "URL Clear Import", map[string]any{"video_url": "https://example.invalid/old.mp4", "shadowing_config": map[string]any{"media_type": "video"}}),
		VideoObjectID: &existingVideoID,
	})
	if err != nil {
		t.Fatalf("first Import error: %v", err)
	}

	_, err = service.Import(context.Background(), ImportRequest{
		LessonJSON: lessonJSON(t, "URL Clear Import", map[string]any{"video_url": "https://cdn.example/new.mp4", "shadowing_config": map[string]any{"media_type": "video"}}),
	})
	if !errors.Is(err, ErrVideoObjectConflict) {
		t.Fatalf("URL-only overwrite error = %v, want ErrVideoObjectConflict", err)
	}

	cleared, err := service.Import(context.Background(), ImportRequest{
		LessonJSON:       lessonJSON(t, "URL Clear Import", map[string]any{"video_url": "https://cdn.example/new.mp4", "shadowing_config": map[string]any{"media_type": "video"}}),
		ClearVideoObject: true,
	})
	if err != nil {
		t.Fatalf("clear Import error: %v", err)
	}
	if cleared.LessonID != first.LessonID {
		t.Fatalf("cleared lesson id = %d, want %d", cleared.LessonID, first.LessonID)
	}

	var videoObjectID sql.NullInt64
	var configRaw string
	if err := db.QueryRow(`SELECT video_object_id, shadowing_config_json::text FROM lessons WHERE id = $1`, first.LessonID).Scan(&videoObjectID, &configRaw); err != nil {
		t.Fatalf("query cleared lesson: %v", err)
	}
	if videoObjectID.Valid {
		t.Fatalf("video_object_id still valid = %d, want NULL", videoObjectID.Int64)
	}
	if !strings.Contains(configRaw, "https://cdn.example/new.mp4") {
		t.Fatalf("config = %s, want URL-only video", configRaw)
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestImportWithExistingVideoObject|TestImportURLOnlyConflictAndClear" -count=1
```

Expected: FAIL because media binding logic is not implemented.

- [ ] **Step 4: Implement media binding logic**

Create `internal/module/shadowingmaterial/media.go` with:

```go
func VideoURL(basePath string, id int64) string {
	base := strings.TrimRight(basePath, "/")
	if base == "" {
		base = DefaultVideoBasePath
	}
	return base + "/" + strconv.FormatInt(id, 10) + "/stream"
}
```

Add private helpers:

```go
type videoObjectRecord struct {
	ID            int64
	Bucket        string
	ObjectKey     string
	Kind          string
	Visibility    string
	ContentSHA256 string
	SizeBytes     int64
	MimeType      string
}

func (s Service) loadBindableVideoObject(ctx context.Context, tx *sql.Tx, id int64) (videoObjectRecord, error)
func (s Service) verifyVideoObjectAvailable(ctx context.Context, record videoObjectRecord) error
func canonicalizeVideoConfig(config map[string]any, videoURL string) map[string]any
func clearVideoObjectIfAllowed(ctx context.Context, tx *sql.Tx, lessonID int64, allow bool) error
```

Rules:

- `loadBindableVideoObject` selects only `kind = 'lesson_shadowing'`, `visibility = 'public'`, and `deleted_at IS NULL`.
- Missing or filtered rows return `ErrVideoObjectUnavailable`.
- `verifyVideoObjectAvailable` requires `s.Verifier` for existing object mode.
- `verifyVideoObjectAvailable` maps `ErrVideoObjectUnavailable`, `ErrVideoStorageUnavailable`, and `ErrVideoStorageCheckFailed` without losing `errors.Is`.
- `canonicalizeVideoConfig` sets `media_type`, `media_url`, and `video_url` to the canonical stream URL and preserves unrelated config keys.

Modify `Service.Import`:

- Before upsert, parse the lesson and determine whether existing lesson has `video_object_id`.
- If `VideoObjectID != nil`, load and stat the object before changing lesson data.
- If URL-only and existing lesson has `video_object_id`, return `ErrVideoObjectConflict` unless `ClearVideoObject` is true.
- In the same transaction as lesson upsert, set `video_object_id` to the selected object or `NULL` for clear.

- [ ] **Step 5: Run tests to verify pass**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestImportWithExistingVideoObject|TestImportURLOnlyConflictAndClear|TestImportURLOnlyLessonInsertsAndUpdates" -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/module/shadowingmaterial
git commit -m "feat(shadowing): bind imported lessons to verified video objects"
```

## Task 4: Add Upload Mode, Size Limit, And Metadata Conflict Checks

**Files:**
- Modify: `internal/module/shadowingmaterial/media.go`
- Modify: `internal/module/shadowingmaterial/service.go`
- Modify: `internal/module/shadowingmaterial/service_test.go`

- [ ] **Step 1: Write failing upload tests**

Add a fake uploader:

```go
type fakeUploader struct {
	uploaded UploadedVideo
	err      error
	calls    int
}

func (f *fakeUploader) UploadLessonVideo(_ context.Context, upload VideoUpload) (UploadedVideo, error) {
	f.calls++
	if f.err != nil {
		return UploadedVideo{}, f.err
	}
	return f.uploaded, nil
}
```

Add tests:

```go
func TestImportWithVideoFile(t *testing.T) {
	tests := []struct {
		name          string
		existingKind  string
		existingSHA   string
		existingSize  int64
		wantErr       error
		wantReuse     bool
	}{
		{name: "new object"},
		{name: "compatible reuse", existingKind: VideoKindLesson, existingSHA: "sha-upload", existingSize: 9, wantReuse: true},
		{name: "wrong kind conflict", existingKind: "avatar", existingSHA: "sha-upload", existingSize: 9, wantErr: ErrVideoObjectConflict},
		{name: "different sha conflict", existingKind: VideoKindLesson, existingSHA: "sha-other", existingSize: 9, wantErr: ErrVideoObjectConflict},
		{name: "different size conflict", existingKind: VideoKindLesson, existingSHA: "sha-upload", existingSize: 10, wantErr: ErrVideoObjectConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newMigratedPostgresDB(t)
			if tt.existingKind != "" {
				insertVideoObjectWithMetadataForTest(t, db, tt.existingKind, "sha-upload", tt.existingSHA, tt.existingSize)
			}
			uploader := &fakeUploader{uploaded: UploadedVideo{
				Bucket:        "lesson-videos",
				ObjectKey:     "lessons/shadowing/abc123.mp4",
				ContentSHA256: "sha-upload",
				SizeBytes:     9,
				MimeType:      "video/mp4",
			}}
			service := Service{DB: db, Uploader: uploader, Verifier: &fakeVerifier{}}
			service.ApplyDefaults()

			result, err := service.Import(context.Background(), ImportRequest{
				LessonJSON: lessonJSON(t, "Upload Import "+tt.name, map[string]any{
					"video_url":         "https://example.invalid/ignored.mp4",
					"shadowing_config": map[string]any{"media_type": "audio", "media_url": "https://example.invalid/ignored.mp4"},
				}),
				VideoFile: &VideoUpload{
					Filename:    "lesson.mp4",
					ContentType: "video/mp4",
					Reader:      strings.NewReader("video-bin"),
					SizeBytes:   9,
				},
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Import error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Import error: %v", err)
			}
			if result.VideoObjectID == nil {
				t.Fatalf("VideoObjectID = nil, want bound object")
			}
			assertLessonVideoState(t, db, result.LessonID, *result.VideoObjectID, result.VideoURL)
			if !strings.HasPrefix(result.VideoURL, "/api/v1/videos/") {
				t.Fatalf("VideoURL = %q, want canonical stream URL", result.VideoURL)
			}
		})
	}
}

func TestImportWithVideoFileRejectsOversizedUploadBeforeUploader(t *testing.T) {
	db := newMigratedPostgresDB(t)
	uploader := &fakeUploader{}
	service := Service{DB: db, Uploader: uploader, MaxVideoBytes: 5}
	service.ApplyDefaults()

	_, err := service.Import(context.Background(), ImportRequest{
		LessonJSON: lessonJSON(t, "Oversized Upload", map[string]any{"video_url": "https://example.invalid/ignored.mp4", "shadowing_config": map[string]any{"media_type": "video"}}),
		VideoFile: &VideoUpload{
			Filename:    "large.mp4",
			ContentType: "video/mp4",
			Reader:      strings.NewReader("video-bin"),
			SizeBytes:   9,
		},
	})
	if !errors.Is(err, ErrVideoTooLarge) {
		t.Fatalf("Import error = %v, want ErrVideoTooLarge", err)
	}
	if uploader.calls != 0 {
		t.Fatalf("uploader calls = %d, want 0", uploader.calls)
	}
}
```

Add helper:

```go
func insertVideoObjectWithMetadataForTest(t *testing.T, db *sql.DB, kind, uploadedSHA, existingSHA string, size int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
INSERT INTO video_objects (
	bucket, object_key, kind, visibility, content_sha256,
	size_bytes, mime_type, metadata_json
)
VALUES ('lesson-videos', 'lessons/shadowing/abc123.mp4', $1, 'public', $2, $3, 'video/mp4', '{}'::jsonb)
RETURNING id`,
		kind,
		existingSHA,
		size,
	).Scan(&id); err != nil {
		t.Fatalf("insert video object metadata: %v; uploaded sha was %s", err, uploadedSHA)
	}
	return id
}
```

- [ ] **Step 2: Run failing tests**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestImportWithVideoFile|TestImportWithVideoFileRejectsOversizedUploadBeforeUploader" -count=1
```

Expected: FAIL because upload mode is not implemented.

- [ ] **Step 3: Implement upload mode**

Implement:

```go
func (s Service) validateVideoUpload(upload *VideoUpload) error
func (s Service) insertOrReuseVideoObject(ctx context.Context, tx *sql.Tx, uploaded UploadedVideo) (int64, error)
```

Rules:

- `validateVideoUpload` returns `ErrBadRequest` when filename or reader is missing.
- If `SizeBytes > MaxVideoBytes`, return `ErrVideoTooLarge` before calling `Uploader`.
- Require `s.Uploader` when `VideoFile` is present; missing uploader returns `ErrVideoStorageUnavailable`.
- `insertOrReuseVideoObject` locks an existing row with `FOR UPDATE`.
- Existing row is reusable only when `kind`, `content_sha256`, and `size_bytes` match.
- Compatible rows should set `deleted_at = NULL`, update `mime_type`, and return the existing ID.
- New rows insert `kind = 'lesson_shadowing'`, `visibility = 'public'`, and `{}` metadata.
- Canonicalize all JSON video fields to the stream URL after the object ID is known.

- [ ] **Step 4: Implement MinIO uploader and verifier**

Add to `media.go`:

```go
type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

func NewMinIOUploader(cfg MinIOConfig) (VideoUploader, error)
func NewMinIOVerifier(cfg MinIOConfig) (VideoObjectVerifier, error)
```

Implementation rules:

- Reuse the existing endpoint parsing behavior from `adminMinIOEndpoint`.
- Uploader computes SHA-256 and object key `lessons/shadowing/<sha16><ext>`.
- Uploader uses `PutObject`.
- Verifier uses `StatObject`.
- Missing endpoint/access/secret returns `ErrVideoStorageUnavailable`.

- [ ] **Step 5: Run tests**

Run:

```powershell
go test ./internal/module/shadowingmaterial -run "TestImportWithVideoFile|TestImportWithVideoFileRejectsOversizedUploadBeforeUploader" -count=1
go test ./internal/module/shadowingmaterial -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/module/shadowingmaterial
git commit -m "feat(shadowing): import videos through minio-backed metadata"
```

## Task 5: Wire Admin Import, Upload, And Bind Endpoints To Shared Service

**Files:**
- Modify: `internal/module/admin/handler.go`
- Modify: `internal/module/admin/shadowing_materials.go`
- Modify: `internal/module/admin/shadowing_materials_test.go`
- Modify: `backend/cmd/admin/main.go`

- [ ] **Step 1: Write failing Admin handler tests**

Add to `internal/module/admin/shadowing_materials_test.go`:

```go
func TestImportShadowingMaterialWithExistingVideoObject(t *testing.T) {
	db := openShadowingPostgresTestDB(t)
	videoObjectID := insertAdminVideoObjectForTest(t, db, "lesson_shadowing", "public")
	verifier := &adminFakeVerifier{}
	handler := NewHandler(HandlerConfig{
		AdminToken:             "test-token",
		ShadowingMaterialsDB:   db,
		ShadowingMaterialsSQL:  "postgres",
		ShadowingVideoBasePath: "/api/v1/videos",
		ShadowingVideoVerifier: verifier,
	})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("lesson_json", string(adminLessonJSON(t, "Admin Import Existing Video"))); err != nil {
		t.Fatalf("write lesson_json: %v", err)
	}
	if err := writer.WriteField("video_object_id", strconv.FormatInt(videoObjectID, 10)); err != nil {
		t.Fatalf("write video_object_id: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/shadowing/materials/import", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST import: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201; body=%s", resp.StatusCode, raw)
	}

	var got struct {
		LessonID      int64  `json:"lesson_id"`
		VideoObjectID int64  `json:"video_object_id"`
		VideoURL      string `json:"video_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.LessonID <= 0 || got.VideoObjectID != videoObjectID || got.VideoURL != "/api/v1/videos/"+strconv.FormatInt(videoObjectID, 10)+"/stream" {
		t.Fatalf("response = %+v, want imported lesson with canonical video URL", got)
	}
}

func TestUploadShadowingVideoRejectsOversizedFile(t *testing.T) {
	db := openShadowingPostgresTestDB(t)
	uploader := &adminFakeUploader{}
	handler := NewHandler(HandlerConfig{
		AdminToken:             "test-token",
		ShadowingMaterialsDB:   db,
		ShadowingMaterialsSQL:  "postgres",
		ShadowingVideoBasePath: "/api/v1/videos",
		ShadowingVideoUploader: uploader,
		ShadowingVideoMaxBytes: 5,
	})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "large.mp4")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("video-bin")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/shadowing/materials/videos", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 413; body=%s", resp.StatusCode, raw)
	}
	if uploader.calls != 0 {
		t.Fatalf("uploader calls = %d, want 0", uploader.calls)
	}
}
```

Add test helper names exactly once:

```go
func openShadowingPostgresTestDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}

	ctx := context.Background()
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{
		DatabaseURL:  url,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("open postgres test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
`); err != nil {
		t.Fatalf("reset postgres schema: %v", err)
	}
	if err := adapter.RunMigrations(ctx, db); err != nil {
		t.Fatalf("run postgres migrations: %v", err)
	}
	return db
}

func adminLessonJSON(t *testing.T, title string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"title":             title,
		"jlpt_level":        "N5",
		"tags":              []string{"shadowing"},
		"video_url":         "https://example.invalid/old.mp4",
		"word_ids":          []int64{},
		"shadowing_enabled": true,
		"shadowing_version": 1,
		"shadowing_config":  map[string]any{"media_type": "video", "media_url": "https://example.invalid/old.mp4"},
		"sentences": []map[string]any{
			{
				"index":    0,
				"tokens":   []map[string]string{{"surface": "管理", "reading": "かんり"}},
				"chinese":  "管理测试",
				"start_ms": 0,
				"end_ms":   1000,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal admin lesson JSON: %v", err)
	}
	return raw
}

func insertAdminVideoObjectForTest(t *testing.T, db *sql.DB, kind, visibility string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
INSERT INTO video_objects (
	bucket, object_key, kind, visibility, content_sha256,
	size_bytes, mime_type, metadata_json
)
VALUES ('lesson-videos', 'lessons/admin.mp4', $1, $2, 'sha-admin', 9, 'video/mp4', '{}'::jsonb)
RETURNING id`,
		kind,
		visibility,
	).Scan(&id); err != nil {
		t.Fatalf("insert admin video object: %v", err)
	}
	return id
}

type adminFakeVerifier struct {
	err   error
	calls int
}

func (v *adminFakeVerifier) StatObject(_ context.Context, _, _ string) error {
	v.calls++
	return v.err
}

type adminFakeUploader struct {
	result shadowingmaterial.UploadedVideo
	err    error
	calls  int
}

func (u *adminFakeUploader) UploadLessonVideo(_ context.Context, _ shadowingmaterial.VideoUpload) (shadowingmaterial.UploadedVideo, error) {
	u.calls++
	if u.err != nil {
		return shadowingmaterial.UploadedVideo{}, u.err
	}
	if u.result.Bucket == "" {
		u.result = shadowingmaterial.UploadedVideo{
			Bucket:        "lesson-videos",
			ObjectKey:     "lessons/shadowing/admin.mp4",
			ContentSHA256: "sha-admin",
			SizeBytes:     9,
			MimeType:      "video/mp4",
		}
	}
	return u.result, nil
}
```

- [ ] **Step 2: Run failing Admin tests**

Run:

```powershell
go test ./internal/module/admin -run "TestImportShadowingMaterialWithExistingVideoObject|TestUploadShadowingVideoRejectsOversizedFile" -count=1
```

Expected: FAIL because route/config/service integration does not exist.

- [ ] **Step 3: Extend HandlerConfig and routes**

Modify `internal/module/admin/handler.go`:

```go
type HandlerConfig struct {
	AdminToken              string
	WordStore               store.AdminWordStoreInterface
	GrammarStore            store.AdminGrammarStoreInterface
	SpeakingStore           store.AdminSpeakingStoreInterface
	WritingStore            store.AdminWritingStoreInterface
	TranslationStore        store.AdminTranslationStoreInterface
	UserStore               store.AdminUserStoreInterface
	DB                      *sql.DB
	ShadowingMaterialsDB    *sql.DB
	ShadowingMaterialsSQL   string
	ShadowingVideoBasePath  string
	ShadowingVideoUploader  shadowingmaterial.VideoUploader
	ShadowingVideoVerifier  shadowingmaterial.VideoObjectVerifier
	ShadowingVideoBucket    string
	ShadowingVideoMaxBytes  int64
	MinIOEndpoint           string
	MinIOAccessKey          string
	MinIOSecretKey          string
	MinIOUseSSL             bool
	AIAPIKey                string
	AIAPIEndpoint           string
	AIModel                 string
}
```

Add route:

```go
mux.HandleFunc("POST /api/admin/shadowing/materials/import", h.auth(h.importShadowingMaterial))
```

- [ ] **Step 4: Implement Admin service builder and error writer**

In `shadowing_materials.go`, add:

```go
func (h *Handler) shadowingMaterialService() (shadowingmaterial.Service, error)
func writeShadowingMaterialServiceError(w http.ResponseWriter, err error)
func parseShadowingVideoMaxBytes(raw string) int64
```

Rules:

- If `ShadowingMaterialsSQL != "postgres"`, new import endpoint returns `501 ERR_UNSUPPORTED_RELATIONAL_MODE`.
- Use injected uploader/verifier in tests.
- If MinIO config is present and no injected uploader/verifier exists, create shared MinIO uploader/verifier.
- Existing upload and bind endpoints use the same service methods, not the old `insertShadowingVideoObject` and `ensurePublicVideoObject` helpers.
- Keep old helper functions only until they are no longer referenced; delete them after tests pass.

- [ ] **Step 5: Implement `importShadowingMaterial`**

Behavior:

- `ParseMultipartForm` with `service.MaxVideoBytes + 1<<20` memory cap.
- Read `lesson_json`, `video_object_id`, `clear_video_object`.
- Accept optional `video_file`.
- Reject simultaneous `video_file` and `video_object_id`.
- Convert `video_file` to `shadowingmaterial.VideoUpload` with filename, content type, reader, and header size.
- Call `service.Import`.
- Return `201` with `lesson_id`, `inserted`, `video_object_id`, `video_url`, and `shadowing_config`.

- [ ] **Step 6: Update old upload and bind endpoints**

Rules:

- `uploadShadowingVideo` calls `service.UploadVideo`.
- `bindShadowingLessonVideo` calls `service.BindVideo`.
- Oversized old upload returns 413 and does not call uploader.
- Wrong kind or missing MinIO object returns service-mapped error.

- [ ] **Step 7: Wire backend admin main config**

Modify `backend/cmd/admin/main.go`:

```go
ShadowingVideoMaxBytes: parseEnvInt64("SHADOWING_VIDEO_MAX_BYTES", shadowingmaterial.DefaultMaxVideoBytes),
```

Add helper:

```go
func parseEnvInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
```

- [ ] **Step 8: Run Admin tests**

Run:

```powershell
go test ./internal/module/admin -run "TestImportShadowingMaterialWithExistingVideoObject|TestUploadShadowingVideoRejectsOversizedFile|TestBindShadowingLessonVideoObject|TestUploadShadowingVideoCreatesObjectAndBindsLesson" -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add internal/module/admin backend/cmd/admin/main.go
git commit -m "feat(admin): import shadowing materials through shared service"
```

## Task 6: Add CLI Video Flags And Shared Service Execution

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/import_lessons_postgres_test.go`

- [ ] **Step 1: Write failing CLI validation tests**

Add to `internal/cli/import_lessons_postgres_test.go`:

```go
func TestRunImportLessonsPostgresRejectsConflictingVideoFlags(t *testing.T) {
	filePath := writePostgresLessonImportTempJSON(t, []map[string]any{
		postgresVideoLessonImportJSON("CLI Conflict", "https://cdn.example/video.mp4"),
	})

	code := Run([]string{
		"import-lessons-postgres",
		"--database-url", "postgres://unused",
		"--file", filePath,
		"--video-file", "video.mp4",
		"--video-object-id", "1",
	})
	if code == 0 {
		t.Fatalf("Run exit code = 0, want non-zero")
	}
}

func TestRunImportLessonsPostgresRejectsClearWithObjectFlag(t *testing.T) {
	filePath := writePostgresLessonImportTempJSON(t, []map[string]any{
		postgresVideoLessonImportJSON("CLI Clear Conflict", "https://cdn.example/video.mp4"),
	})

	code := Run([]string{
		"import-lessons-postgres",
		"--database-url", "postgres://unused",
		"--file", filePath,
		"--video-object-id", "1",
		"--clear-video-object",
	})
	if code == 0 {
		t.Fatalf("Run exit code = 0, want non-zero")
	}
}
```

- [ ] **Step 2: Run failing CLI tests**

Run:

```powershell
go test ./internal/cli -run "TestRunImportLessonsPostgresRejectsConflictingVideoFlags|TestRunImportLessonsPostgresRejectsClearWithObjectFlag" -count=1
```

Expected: FAIL because flags do not exist.

- [ ] **Step 3: Implement flag parsing validation before DB open**

Refactor `runImportLessonsPostgres` into a thin wrapper so tests can inject fake media dependencies without weakening production MinIO checks:

```go
type importLessonsPostgresDeps struct {
	uploader shadowingmaterial.VideoUploader
	verifier shadowingmaterial.VideoObjectVerifier
}

func runImportLessonsPostgres(args []string) int {
	return runImportLessonsPostgresWithDeps(args, importLessonsPostgresDeps{})
}
```

In `runImportLessonsPostgresWithDeps`, add flags:

```go
videoFile := fs.String("video-file", "", "path to a video file to upload to MinIO and bind to the imported lesson")
videoObjectID := fs.Int64("video-object-id", 0, "existing video_objects.id to bind to the imported lesson")
clearVideoObject := fs.Bool("clear-video-object", false, "clear an existing lesson video_object_id during URL-only import")
videoBasePath := fs.String("video-base-path", "/api/v1/videos", "base path for generated video stream URLs")
videoMaxBytes := fs.Int64("video-max-bytes", envInt64("SHADOWING_VIDEO_MAX_BYTES", shadowingmaterial.DefaultMaxVideoBytes), "maximum allowed video upload size in bytes")
minioEndpoint := fs.String("minio-endpoint", os.Getenv("MINIO_ENDPOINT"), "MinIO endpoint for video upload and verification")
minioAccessKey := fs.String("minio-access-key", os.Getenv("MINIO_ACCESS_KEY"), "MinIO access key")
minioSecretKey := fs.String("minio-secret-key", os.Getenv("MINIO_SECRET_KEY"), "MinIO secret key")
minioBucketVideo := fs.String("minio-bucket-video", envOr("MINIO_BUCKET_VIDEO", shadowingmaterial.DefaultVideoBucket), "MinIO bucket for lesson videos")
minioUseSSL := fs.Bool("minio-use-ssl", os.Getenv("MINIO_USE_SSL") == "true", "use HTTPS for MinIO")
```

Validation before DB open:

```go
if *videoFile != "" && *videoObjectID > 0 {
	fmt.Fprintln(os.Stderr, "import-lessons-postgres: --video-file and --video-object-id are mutually exclusive")
	return 1
}
if *clearVideoObject && (*videoFile != "" || *videoObjectID > 0) {
	fmt.Fprintln(os.Stderr, "import-lessons-postgres: --clear-video-object is only valid for URL-only import")
	return 1
}
if *videoMaxBytes <= 0 {
	fmt.Fprintln(os.Stderr, "import-lessons-postgres: --video-max-bytes must be greater than 0")
	return 1
}
```

- [ ] **Step 4: Write failing CLI import tests for `--video-object-id`**

Add:

```go
func TestRunImportLessonsPostgresBindsExistingVideoObject(t *testing.T) {
	db := newMigratedPostgresLessonImportDB(t)
	videoObjectID := insertCLIVideoObject(t, db)
	url := os.Getenv("DATABASE_URL_TEST")
	filePath := writePostgresLessonImportTempJSON(t, []map[string]any{
		postgresVideoLessonImportJSON("CLI Existing Object", "https://cdn.example/ignored.mp4"),
	})

	code := runImportLessonsPostgresWithDeps([]string{
		"--database-url", url,
		"--file", filePath,
		"--video-object-id", strconv.FormatInt(videoObjectID, 10),
	}, importLessonsPostgresDeps{verifier: cliFakeVerifier{}})
	if code != 0 {
		t.Fatalf("Run exit code = %d, want 0", code)
	}

	var gotVideoObjectID int64
	var configRaw string
	if err := db.QueryRow(`SELECT video_object_id, shadowing_config_json::text FROM lessons WHERE title = $1`, "CLI Existing Object").Scan(&gotVideoObjectID, &configRaw); err != nil {
		t.Fatalf("query imported lesson: %v", err)
	}
	if gotVideoObjectID != videoObjectID {
		t.Fatalf("video_object_id = %d, want %d", gotVideoObjectID, videoObjectID)
	}
	if !strings.Contains(configRaw, "/api/v1/videos/"+strconv.FormatInt(videoObjectID, 10)+"/stream") {
		t.Fatalf("config = %s, want canonical stream URL", configRaw)
	}
}
```

Add helper:

```go
type cliFakeVerifier struct{}

func (cliFakeVerifier) StatObject(context.Context, string, string) error {
	return nil
}

func insertCLIVideoObject(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
INSERT INTO video_objects (
	bucket, object_key, kind, visibility, content_sha256,
	size_bytes, mime_type, metadata_json
)
VALUES ('lesson-videos', 'lessons/cli.mp4', 'lesson_shadowing', 'public', 'sha-cli', 7, 'video/mp4', '{}'::jsonb)
RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert CLI video object: %v", err)
	}
	return id
}
```

- [ ] **Step 5: Implement CLI service call**

Rules:

- Read JSON into `raw []byte`.
- Build `shadowingmaterial.Service`.
- For `--video-object-id`, use the injected verifier when provided by tests.
- For `--video-object-id` in production, create a MinIO verifier from CLI/env MinIO config and fail with `ERR_VIDEO_STORAGE_UNAVAILABLE` when config is missing.
- For `--video-file`, open the file and set `VideoFile`.
- For `--video-file`, use the injected uploader when provided by tests.
- For `--video-file` in production, create a MinIO uploader from CLI/env MinIO config and fail with `ERR_VIDEO_STORAGE_UNAVAILABLE` when config is missing.
- Call `service.Import` when video options or clear flag are present.
- Call `service.ImportBatch` for URL-only imports without video options.
- Print imported lesson IDs and video URL when present.

- [ ] **Step 6: Run CLI tests**

Run:

```powershell
go test ./internal/cli -run "TestRunImportLessonsPostgresCommand|TestRunImportLessonsPostgresBindsExistingVideoObject|TestRunImportLessonsPostgresRejectsConflictingVideoFlags|TestRunImportLessonsPostgresRejectsClearWithObjectFlag" -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/cli/root.go internal/cli/import_lessons_postgres.go internal/cli/import_lessons_postgres_test.go
git commit -m "feat(cli): add shadowing video import flags"
```

## Task 7: Add Admin Page Reviewed JSON Import Flow

**Files:**
- Modify: `front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.tsx`
- Modify: `front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.module.css`

- [ ] **Step 1: Type-check current page before changes**

Run:

```powershell
cd front/admin
npm run build
```

Expected: PASS before editing. If it fails before this task, record the pre-existing error and stop.

- [ ] **Step 2: Add import state and request function**

Modify `ShadowingMaterials.tsx` to add state:

```tsx
const [importing, setImporting] = useState(false)
const [importVideoFile, setImportVideoFile] = useState<File | null>(null)
const [clearVideoObject, setClearVideoObject] = useState(false)
```

Add response type:

```tsx
interface ShadowingMaterialImportResponse {
  lesson_id: number
  inserted: boolean
  video_object_id?: number
  video_url: string
}
```

Add function:

```tsx
async function importDraft() {
  if (!draftJSON.trim()) {
    setError('Generate or paste reviewed JSON before importing')
    return
  }

  setImporting(true)
  setError('')
  setNotice('')
  try {
    JSON.parse(draftJSON)
    const fd = new FormData()
    fd.append('lesson_json', draftJSON)
    if (importVideoFile) {
      fd.append('video_file', importVideoFile)
    } else if (videoObjectId.trim()) {
      fd.append('video_object_id', videoObjectId.trim())
    } else if (clearVideoObject) {
      fd.append('clear_video_object', 'true')
    }
    const data = await adminFetch<ShadowingMaterialImportResponse>('POST', '/shadowing/materials/import', fd)
    setNotice(`Imported lesson #${data.lesson_id}${data.video_url ? `: ${data.video_url}` : ''}`)
    if (data.video_object_id) setVideoObjectId(String(data.video_object_id))
    if (data.video_url) setDraftVideoUrl(data.video_url)
    setImportVideoFile(null)
    await fetchItems()
  } catch (err) {
    setError(err instanceof Error ? err.message : 'Import failed')
  } finally {
    setImporting(false)
  }
}
```

- [ ] **Step 3: Add controls near draft JSON output**

Render under the draft JSON textarea:

```tsx
{draftJSON && (
  <div className={styles.importPanel}>
    <div className={styles.formRow}>
      <input type="file" accept="video/*" onChange={e => setImportVideoFile(e.target.files?.[0] ?? null)} />
      <label className={styles.checkboxLabel}>
        <input type="checkbox" checked={clearVideoObject} onChange={e => setClearVideoObject(e.target.checked)} disabled={Boolean(importVideoFile || videoObjectId.trim())} />
        Clear existing MinIO video binding for URL-only import
      </label>
      <button className="adm-btn" onClick={importDraft} disabled={importing || !draftJSON.trim()}>
        {importing ? 'Importing...' : 'Import reviewed JSON'}
      </button>
    </div>
    <p className={styles.hint}>Import uses video_object_id when set, otherwise the selected video file, otherwise URL-only JSON.</p>
  </div>
)}
```

- [ ] **Step 4: Add CSS**

Add to `ShadowingMaterials.module.css`:

```css
.importPanel {
  display: grid;
  gap: 10px;
  padding: 12px;
  border: 1px dashed #c9bfae;
  border-radius: 10px;
  background: #fffdf8;
}

.checkboxLabel {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #4a443d;
  font-size: 13px;
}

.checkboxLabel input {
  width: auto;
}
```

- [ ] **Step 5: Run frontend build**

Run:

```powershell
cd front/admin
npm run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.tsx front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.module.css
git commit -m "feat(admin): import reviewed shadowing material drafts"
```

## Task 8: Update Docs And Run Full Verification

**Files:**
- Modify: `docs/video-storage.md`

- [ ] **Step 1: Update docs**

Add this section to `docs/video-storage.md`:

```markdown
## Admin Material Import

Shadowing material import is PostgreSQL-only.

- `POST /api/admin/shadowing/materials/import` imports one reviewed lesson material pack.
- A `video_file` upload stores the binary in MinIO and metadata in `video_objects`.
- A `video_object_id` import binds an existing public `lesson_shadowing` object after MinIO availability is verified.
- URL-only import keeps external `video_url`; if the lesson already has `video_object_id`, the caller must explicitly clear it.
- The existing `/api/admin/shadowing/materials/videos` endpoint uses the same upload size limit and video object validation as the import endpoint.
- Default video bucket is `lesson-videos`.
- Default upload limit is 200 MiB and can be overridden with `SHADOWING_VIDEO_MAX_BYTES`.
```

- [ ] **Step 2: Run targeted backend tests**

Run:

```powershell
go test ./internal/module/shadowingmaterial -count=1
go test ./internal/module/admin -run "Shadowing" -count=1
go test ./internal/cli -run "ImportLessonsPostgres" -count=1
```

Expected: PASS.

- [ ] **Step 3: Run broader backend tests**

Run:

```powershell
go test ./internal/module/admin ./internal/module/shadowingmaterial ./internal/cli ./backend/cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 4: Run frontend build**

Run:

```powershell
cd front/admin
npm run build
```

Expected: PASS.

- [ ] **Step 5: Run Makefile test if time allows**

Run:

```powershell
make test
```

Expected: PASS. If unrelated pre-existing tests fail, capture the exact failing package and error in the final handoff.

- [ ] **Step 6: Commit docs and final polish**

```powershell
git add docs/video-storage.md
git commit -m "docs(shadowing): document material video import"
```

## Final Review Checklist

- [ ] `internal/module/shadowingmaterial` owns import behavior.
- [ ] `internal/cli` no longer owns PostgreSQL lesson import internals.
- [ ] Admin import, upload, and bind use the same service rules.
- [ ] URL-only import over old `video_object_id` returns conflict unless explicitly cleared.
- [ ] Existing video object binding checks `kind`, visibility, deleted state, and MinIO availability.
- [ ] Upload mode rejects incompatible `(bucket, object_key)` rows.
- [ ] Upload size limit applies to new import endpoint and existing video upload endpoint.
- [ ] MinIO-backed imports overwrite JSON-provided video fields with canonical stream URLs.
- [ ] Admin page can import reviewed draft JSON and preserves JSON on error.
- [ ] CLI supports `--video-file`, `--video-object-id`, `--clear-video-object`, `--video-base-path`, and `--video-max-bytes`.
- [ ] Targeted Go tests pass.
- [ ] Admin frontend build passes.
