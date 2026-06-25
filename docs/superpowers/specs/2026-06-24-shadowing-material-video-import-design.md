# Shadowing Material Video Import Design

Date: 2026-06-24

## Background

The project has already moved the main runtime direction to PostgreSQL plus
MinIO:

- PostgreSQL stores structured data and media metadata.
- MinIO stores media binaries.
- `video_objects` stores video object metadata.
- `lessons.video_object_id` points to `video_objects.id`.
- The learner service can stream lesson videos through
  `/api/v1/videos/{id}/stream`.

The missing piece is the import workflow. Today the project has three partial
paths that do not form a complete material production loop:

- CLI lesson import can upsert lesson JSON into PostgreSQL, but it only stores
  `video_url` in `shadowing_config_json`; it does not upload video to MinIO or
  set `lessons.video_object_id`.
- Admin Shadowing Materials can upload a video to MinIO and bind it to an
  existing lesson, but it cannot import the reviewed material pack JSON into
  PostgreSQL.
- The admin page can generate a draft material pack JSON, but the operator must
  manually move that JSON into another import path.

This design closes that gap without redesigning video storage.

## Goals

- Support PostgreSQL-backed shadowing material import from both CLI/manual flow
  and the admin page.
- Store uploaded lesson videos in MinIO and store their metadata in
  PostgreSQL `video_objects`.
- Bind imported shadowing lessons to `lessons.video_object_id` when a MinIO
  video object is used.
- Keep URL-only `video_url` import as a fallback for external or temporary
  media, without leaving stale `video_object_id` bindings behind.
- Make the admin page able to go from reviewed draft JSON to an imported lesson
  without leaving the page.
- Reuse one backend import path so CLI and admin behavior stay consistent.

## Non-Goals

- Do not add a learner-side video player in this change. The learner API can
  expose `video_url`, but the UI video playback work remains separate.
- Do not build AI/ASR/furigana automation in this change. Draft generation stays
  review-first and manually triggered.
- Do not convert the existing Words, Grammar, Speaking, or Writing bulk import
  screens to PostgreSQL in this change.
- Do not store video binaries in PostgreSQL.
- Do not make SQLite the target for new shadowing video import behavior.

## Recommended Approach

Implement a PostgreSQL-only Shadowing Material Import service and use it from
both the admin HTTP handler and the CLI.

The service owns the end-to-end flow:

1. Parse and validate one reviewed lesson material pack JSON.
2. Resolve the video source.
3. Upsert the lesson, sentences, and word links into PostgreSQL.
4. If a MinIO-backed video is present, upsert `video_objects`.
5. Set `lessons.video_object_id`.
6. Normalize `shadowing_config_json` so `media_type`, `media_url`, and
   `video_url` point to `/api/v1/videos/{id}/stream`.

This keeps the existing storage model intact and prevents the CLI and admin
page from growing separate import semantics.

Video binding options apply only to a single lesson material pack. Batch lesson
arrays remain supported only for URL-only import until a separate batch video
mapping format is designed.

## Video Source Modes

The import service supports exactly three video modes.

### Existing Video Object

Input contains `video_object_id`.

Behavior:

- Verify the object exists in `video_objects`.
- Require `kind = 'lesson_shadowing'`.
- Require `visibility = 'public'` and `deleted_at IS NULL`.
- Verify the referenced MinIO object is actually readable with
  `StatObject(bucket, object_key)` before importing or binding.
- Import or update the lesson.
- Bind the lesson to the object.
- Set `shadowing_config_json.media_type = "video"`.
- Set `shadowing_config_json.media_url` and `video_url` to
  `/api/v1/videos/{id}/stream`.

This is the safest mode when the operator already uploaded video from the admin
page.

If the database row exists but the MinIO object is missing, return
`409 ERR_VIDEO_OBJECT_UNAVAILABLE`. If the storage check fails because MinIO is
unreachable or rejects the request, return `502 ERR_VIDEO_STORAGE_CHECK_FAILED`.
If MinIO configuration is missing, return `503 ERR_VIDEO_STORAGE_UNAVAILABLE`.
The existing admin bind endpoint must use the same validation so it cannot bind
an unplayable video object.

### Uploaded Video File

Input contains a video file.

Behavior:

- Upload the file to MinIO.
- Use bucket `MINIO_BUCKET_VIDEO`, defaulting to `lesson-videos`.
- Use object key `lessons/shadowing/<sha256-first-16><ext>`.
- Insert or update `video_objects` by `(bucket, object_key)` only when any
  existing row is compatible.
- Import or update the lesson.
- Bind the lesson to the created video object.

The object key is content-addressed, so retrying the same file is idempotent.
If the database step fails after the MinIO upload succeeds, the orphan object is
acceptable for the first version because a retry reuses the same object key.

Conflict rule:

- If `(bucket, object_key)` already exists, require
  `kind = 'lesson_shadowing'`.
- Require the existing `content_sha256` to equal the uploaded file hash.
- Require the existing `size_bytes` to equal the uploaded file size.
- If any check fails, return `409 ERR_VIDEO_OBJECT_CONFLICT` and do not update
  the row or bind the lesson.
- Compatible existing rows may be reused and undeleted.

When a video file is provided, the service canonicalizes all video fields. Any
top-level `video_url`, `shadowing_config.video_url`,
`shadowing_config.media_url`, or `shadowing_config.media_type` supplied in the
lesson JSON is overwritten with the server-generated stream URL and
`media_type = "video"`.

### URL-Only Video

Input contains only `video_url`.

Behavior:

- Import or update the lesson.
- Preserve `video_url` in `shadowing_config_json`.
- If `shadowing_config.media_type = "video"` and `media_url` is empty, set
  `media_url = video_url`.
- If the target lesson has an existing `video_object_id`, reject the import with
  `409 ERR_VIDEO_OBJECT_CONFLICT` unless the caller explicitly sets
  `clear_video_object = true`.
- When `clear_video_object = true`, clear `lessons.video_object_id` in the same
  transaction that writes the URL-only config.

This mode is a fallback and does not satisfy MinIO-backed storage by itself.
The explicit clearing rule prevents Admin and learner APIs from showing
different video sources for the same lesson.

## Upload Limits

The current admin upload code reads the full video into memory after parsing the
multipart request. The new import path must not add another unbounded upload
path.

First implementation requirements:

- Enforce a hard request/file size limit before reading the full body.
- Apply the same limit to the new import endpoint and the existing
  `POST /api/admin/shadowing/materials/videos` endpoint.
- Use `SHADOWING_VIDEO_MAX_BYTES` when set.
- Default to `209715200` bytes, which is 200 MiB.
- Return `413 ERR_VIDEO_TOO_LARGE` when the upload exceeds the limit.
- Do not create a MinIO object or database row after a size-limit failure.
- Keep the old upload/bind panel wired to the shared uploader and verifier so it
  cannot bypass the limit.

Implementation may still buffer files under the limit for the first version,
but the service boundary must keep uploader internals replaceable. A follow-up
can move to temp-file or streaming hash plus MinIO upload without changing the
Admin API or CLI flags.

## Backend Components

### Reusable Service

Add a small reusable package or service boundary that can be called from both
CLI and admin code. The shared import logic must not live in `internal/cli`,
because Admin should not depend on CLI command code. CLI should become argument
parsing plus a call into the shared service.

Suggested boundary:

- `internal/module/shadowingmaterial`

Core responsibilities:

- Validate import request.
- Upload video to MinIO when a file is provided.
- Insert or update `video_objects`.
- Upsert the PostgreSQL lesson data.
- Bind the lesson to `video_object_id`.

The current PostgreSQL lesson upsert core should move out of
`internal/cli/import_lessons_postgres.go` into this shared package or a sibling
shared data/import package. The CLI command then calls the shared importer.
Admin handlers call the same importer. This keeps dependency direction clean:
HTTP and CLI are adapters; the shared service owns import behavior.

### PostgreSQL Lesson Importer

Extend the PostgreSQL lesson importer so it can return imported lesson IDs and
accept media binding options.

Suggested API shape:

```go
type PostgresLessonImportOptions struct {
	VideoObjectID *int64
	VideoURL      string
	VideoBasePath string
	ClearVideoObject bool
}
```

The importer should keep existing JSON compatibility while adding these rules:

- Existing top-level `video_url` remains supported.
- When `VideoObjectID` is set, it wins over URL-only fields.
- When `VideoObjectID` is set, the importer updates `lessons.video_object_id`.
- When `VideoObjectID` or a newly uploaded video file is set, the importer
  overwrites JSON-provided video fields with the canonical stream URL.
- When URL-only import targets a lesson that already has `video_object_id`, the
  importer rejects unless `ClearVideoObject` is true.
- When `ClearVideoObject` is true, the importer sets `lessons.video_object_id`
  to `NULL` while updating `shadowing_config_json`.
- The importer returns imported lesson IDs so callers can report the created or
  updated lesson.

## Admin API

Add a new endpoint:

```text
POST /api/admin/shadowing/materials/import
```

It accepts `multipart/form-data` so one endpoint can support both reviewed JSON
and optional video upload.

Fields:

- `lesson_json`: required JSON object for one lesson material pack.
- `video_file`: optional video file.
- `video_object_id`: optional existing video object ID.
- `clear_video_object`: optional boolean, default `false`.
- `import_mode`: optional, defaults to `upsert`.

Validation:

- `lesson_json` is required and must be a single lesson object.
- `video_object_id` is request metadata, not trusted lesson content. If the
  JSON body includes a `video_object_id` field, the server ignores it or rejects
  it consistently; the sidecar form field is the source of truth.
- `video_file` and `video_object_id` are mutually exclusive.
- If `video_file` is present, MinIO config must be available.
- If `video_file` is present, the upload must fit inside the configured size
  limit.
- If `video_object_id` is present, the object must exist, be public, be
  undeleted, and have `kind = 'lesson_shadowing'`.
- If `video_object_id` is present, `StatObject(bucket, object_key)` must
  succeed before the lesson is imported or bound.
- `clear_video_object` is allowed only in URL-only mode.
- If neither is present, URL-only import is allowed only when the material pack
  has `video_url` or valid audio fields.

Successful response:

```json
{
  "lesson_id": 123,
  "inserted": true,
  "video_object_id": 45,
  "video_url": "/api/v1/videos/45/stream",
  "shadowing_config": {
    "media_type": "video",
    "media_url": "/api/v1/videos/45/stream",
    "video_url": "/api/v1/videos/45/stream"
  }
}
```

Status behavior:

- `400` for malformed JSON, invalid IDs, or mutually exclusive fields.
- `404` for missing `video_object_id` or a `video_object_id` that is not a
  public undeleted `lesson_shadowing` object.
- `409` if duplicate lesson rows make upsert ambiguous or URL-only import would
  overwrite a lesson that still has `video_object_id` without
  `clear_video_object = true`.
- `409 ERR_VIDEO_OBJECT_CONFLICT` when `(bucket, object_key)` already exists
  with a different hash, size, or kind.
- `409 ERR_VIDEO_OBJECT_UNAVAILABLE` when a selected DB video object exists but
  its MinIO object is missing.
- `422` for invalid lesson content or unsupported material shape.
- `413` when uploaded video exceeds the configured size limit.
- `503` when MinIO configuration is required but missing.
- `502` when MinIO upload or object availability check fails.

The existing endpoints remain valid:

- `POST /api/admin/shadowing/materials/videos`
- `POST /api/admin/shadowing/materials/lessons/{id}/video`
- `POST /api/admin/shadowing/materials/drafts`

The new import endpoint becomes the preferred page flow.
The existing video upload and bind endpoints must share the same uploader,
object verifier, size limit, `kind` check, and MinIO availability check as the
new import endpoint.

## CLI Manual Import

Extend `import-lessons-postgres` for manual operator use.

Suggested flags:

```text
appctl import-lessons-postgres \
  --database-url "$DATABASE_URL" \
  --file data/seed/lesson.json \
  --video-file ./lesson.mp4
```

Supported video flags:

- `--video-file`: upload the file to MinIO, create `video_objects`, and bind.
- `--video-object-id`: bind an existing `video_objects.id`.
- `--clear-video-object`: allow URL-only import to clear an existing
  `lessons.video_object_id`.
- `--video-base-path`: default `/api/v1/videos`.
- `--video-max-bytes`: default from `SHADOWING_VIDEO_MAX_BYTES`, falling back to
  200 MiB.

MinIO config follows the same environment and flags used by migration/admin
tools:

- `MINIO_ENDPOINT`
- `MINIO_ACCESS_KEY`
- `MINIO_SECRET_KEY`
- `MINIO_BUCKET_VIDEO`, default `lesson-videos`
- `MINIO_USE_SSL`

Rules:

- `--video-file` and `--video-object-id` are mutually exclusive.
- `--video-file` and `--video-object-id` require the input to contain exactly
  one lesson. JSON arrays with more than one lesson return a validation error.
- `--clear-video-object` is allowed only when neither `--video-file` nor
  `--video-object-id` is set.
- If URL-only import targets an existing lesson with `video_object_id` and
  `--clear-video-object` is absent, return a conflict error.
- If neither is provided, import remains URL-only for lessons without existing
  MinIO video binding.
- The command prints imported lesson IDs and video binding results.

## Admin Page Flow

Update `Shadowing Materials` page so operators can finish the workflow in one
place.

UI changes:

- Keep the existing upload/bind panel.
- Keep the existing draft generator.
- Add an `Import reviewed JSON` action next to the draft JSON textarea.
- Add optional controls for `video_object_id` and `video_file`.
- Add an explicit `Clear existing MinIO video binding` checkbox for URL-only
  replacement.
- After successful import, refresh the lesson list and show `lesson_id`,
  `video_object_id`, and `video_url`.

Recommended operator flow:

1. Upload video or select an existing `video_object_id`.
2. Generate draft JSON.
3. Review and edit the JSON.
4. Click `Import reviewed JSON`.
5. Confirm the imported lesson appears in the list with a stream URL.

The page should not auto-import immediately after generating a draft. The review
step is intentional because generated sentence timing and translations may need
human correction.

## Storage Contract

MinIO:

- Bucket: `lesson-videos` by default.
- Object key: `lessons/shadowing/<sha256-first-16><ext>`.
- Content type: use uploaded MIME type, falling back to
  `application/octet-stream`.

PostgreSQL:

- `video_objects.bucket`
- `video_objects.object_key`
- `video_objects.kind = 'lesson_shadowing'`
- `video_objects.visibility = 'public'`
- `video_objects.content_sha256`
- `video_objects.size_bytes`
- `video_objects.mime_type`
- `lessons.video_object_id`
- `lessons.shadowing_config_json`

Runtime URL:

- `/api/v1/videos/{video_object_id}/stream`

When a MinIO object is bound, this runtime URL becomes the canonical lesson
video URL. External `video_url` is only a fallback when no object is bound.

Conflict handling:

- `(bucket, object_key)` remains the identity key for MinIO-backed videos.
- The importer must not blindly overwrite a row found by `(bucket, object_key)`.
- Existing rows are reusable only when `kind`, `content_sha256`, and
  `size_bytes` match the uploaded object.
- Incompatible rows return `409 ERR_VIDEO_OBJECT_CONFLICT`.
- Existing object binding must verify the MinIO object exists before changing
  lesson data.

## Data Consistency

The import operation should be idempotent:

- Same lesson title and JLPT level update the existing lesson.
- Same video file creates the same MinIO object key.
- Same `(bucket, object_key)` reuses the same `video_objects` row only when the
  existing metadata matches the uploaded object.
- Re-importing the same material pack should not create duplicate lessons or
  duplicate video metadata.
- MinIO-backed imports always normalize JSON-provided video fields to the
  server-generated stream URL.
- URL-only imports never silently keep an old `video_object_id`.

Database work should run in one transaction where practical:

- Lesson upsert.
- Child sentence replacement.
- Lesson word replacement.
- `video_objects` upsert.
- `lessons.video_object_id` update.

MinIO upload cannot be part of the PostgreSQL transaction. The first version
uses deterministic object keys to make retry safe instead of attempting complex
rollback.

The operation order should minimize orphan objects:

1. Parse and validate lesson JSON.
2. Resolve whether the target lesson already exists and whether it has
   `video_object_id`.
3. Validate video mode, `clear_video_object`, uniqueness, size limits, and
   `video_object_id` metadata before upload.
4. Upload to MinIO only after validation passes.
5. Run the PostgreSQL transaction for lesson import, `video_objects` upsert,
   and lesson binding.

This does not eliminate every orphan-object case, but it avoids uploading when
the request is already known to be invalid.

## Testing Strategy

Backend tests should be table-driven where possible.

CLI/importer tests:

- URL-only import preserves current behavior when the target lesson has no
  existing MinIO video binding.
- URL-only import over an existing MinIO binding fails without
  `clear_video_object`.
- URL-only import with `clear_video_object` clears `lessons.video_object_id` and
  makes Admin and learner APIs expose the URL-only video.
- Existing `video_object_id` binds lesson and normalizes config.
- Existing `video_object_id` with a non-`lesson_shadowing` kind is rejected.
- Existing `video_object_id` whose MinIO object is missing is rejected before
  lesson data changes.
- Existing `video_object_id` whose MinIO stat check fails due to storage failure
  returns a storage error.
- `--video-file` uploads through a fake uploader and binds the created object.
- `--video-file` and `--video-object-id` together return a validation error.
- Oversized `--video-file` returns a validation error before upload.
- Upload reuse succeeds when `(bucket, object_key)`, hash, size, and kind match.
- Upload reuse fails when `(bucket, object_key)` exists with a different hash,
  size, or kind.
- `video_file` and `video_object_id` modes overwrite JSON-provided video fields
  with the canonical stream URL.
- Re-import updates the same lesson and does not duplicate child rows.
- CLI and Admin tests both exercise the shared import service rather than
  separate duplicate logic.

Admin handler tests:

- Multipart import with `lesson_json + video_object_id` imports and binds.
- Multipart import with `lesson_json + video_file` inserts `video_objects` and
  binds the lesson.
- Missing MinIO config returns `503` only when upload is requested.
- Missing video object returns `404`.
- Non-`lesson_shadowing` video object returns `404`.
- Oversized upload returns `413` and does not create a MinIO object or
  `video_objects` row.
- Existing `/shadowing/materials/videos` upload endpoint also returns `413` for
  oversized files.
- Existing upload and bind endpoints use the same `kind`, metadata conflict,
  and MinIO availability checks as the import endpoint.
- Invalid material JSON returns `422`.

Runtime tests:

- Imported lesson detail exposes `/api/v1/videos/{id}/stream`.
- Existing `/api/v1/videos/{id}/stream` still reads object metadata from
  PostgreSQL and streams from MinIO.

Frontend checks:

- Import button stays disabled until JSON is present.
- Successful import refreshes lesson list.
- Errors do not clear the reviewed JSON textarea.

## Acceptance Criteria

- A reviewed material pack can be imported from the admin page into PostgreSQL.
- A video file selected on the admin page is uploaded to MinIO and represented
  in PostgreSQL `video_objects`.
- MinIO-backed imports set `lessons.video_object_id`.
- URL-only replacement of an existing MinIO binding is either rejected or uses
  explicit clearing so Admin and learner APIs show the same video source.
- Binding an existing object fails when `video_objects.kind` is not
  `lesson_shadowing`.
- Binding an existing object fails before import when the MinIO object cannot be
  verified.
- Uploading a file does not overwrite an incompatible `video_objects` row with
  the same `(bucket, object_key)`.
- `video_file` and `video_object_id` imports overwrite JSON-provided video
  fields with the canonical stream URL.
- Oversized video uploads return a clear `413` response without creating media
  metadata.
- The existing `/api/admin/shadowing/materials/videos` endpoint enforces the
  same upload size limit and 413 behavior as the new import endpoint.
- MinIO-backed imports set `shadowing_config_json.video_url` and `media_url` to
  `/api/v1/videos/{id}/stream`.
- CLI manual import can bind an existing video object or upload a video file.
- CLI and Admin both call the same shared import service.
- URL-only import still works for temporary external media.
- No new SQLite video import behavior is required.
- Existing video upload and bind endpoints continue to work.
