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
  media.
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
- Require `visibility = 'public'` and `deleted_at IS NULL`.
- Import or update the lesson.
- Bind the lesson to the object.
- Set `shadowing_config_json.media_type = "video"`.
- Set `shadowing_config_json.media_url` and `video_url` to
  `/api/v1/videos/{id}/stream`.

This is the safest mode when the operator already uploaded video from the admin
page.

### Uploaded Video File

Input contains a video file.

Behavior:

- Upload the file to MinIO.
- Use bucket `MINIO_BUCKET_VIDEO`, defaulting to `lesson-videos`.
- Use object key `lessons/shadowing/<sha256-first-16><ext>`.
- Insert or update `video_objects` by `(bucket, object_key)`.
- Import or update the lesson.
- Bind the lesson to the created video object.

The object key is content-addressed, so retrying the same file is idempotent.
If the database step fails after the MinIO upload succeeds, the orphan object is
acceptable for the first version because a retry reuses the same object key.

### URL-Only Video

Input contains only `video_url`.

Behavior:

- Import or update the lesson.
- Preserve `video_url` in `shadowing_config_json`.
- If `shadowing_config.media_type = "video"` and `media_url` is empty, set
  `media_url = video_url`.
- Leave `lessons.video_object_id` unchanged unless an explicit clearing option
  is added in a later change.

This mode is a fallback and does not satisfy MinIO-backed storage by itself.

## Backend Components

### Reusable Service

Add a small reusable package or service boundary that can be called from both
CLI and admin code. It must not live inside `internal/module/admin` if CLI also
uses it, because `admin` already imports `internal/cli`.

Suggested boundary:

- `internal/module/shadowingmaterial`

Core responsibilities:

- Validate import request.
- Upload video to MinIO when a file is provided.
- Insert or update `video_objects`.
- Call the PostgreSQL lesson upsert function.
- Bind the lesson to `video_object_id`.

The existing admin upload/bind helpers should be moved into this shared service
or wrapped by it so the admin page continues to use the same behavior.

### PostgreSQL Lesson Importer

Extend the PostgreSQL lesson importer so it can return imported lesson IDs and
accept media binding options.

Suggested API shape:

```go
type PostgresLessonImportOptions struct {
	VideoObjectID *int64
	VideoURL      string
	VideoBasePath string
}
```

The importer should keep existing JSON compatibility while adding these rules:

- Existing top-level `video_url` remains supported.
- When `VideoObjectID` is set, it wins over URL-only fields.
- When `VideoObjectID` is set, the importer updates `lessons.video_object_id`.
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
- `import_mode`: optional, defaults to `upsert`.

Validation:

- `lesson_json` is required and must be a single lesson object.
- `video_object_id` is request metadata, not trusted lesson content. If the
  JSON body includes a `video_object_id` field, the server ignores it or rejects
  it consistently; the sidecar form field is the source of truth.
- `video_file` and `video_object_id` are mutually exclusive.
- If `video_file` is present, MinIO config must be available.
- If `video_object_id` is present, the object must exist and be public.
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
- `404` for missing `video_object_id`.
- `409` if duplicate lesson rows make upsert ambiguous.
- `422` for invalid lesson content or unsupported material shape.
- `503` when MinIO configuration is required but missing.
- `502` when MinIO upload fails.

The existing endpoints remain valid:

- `POST /api/admin/shadowing/materials/videos`
- `POST /api/admin/shadowing/materials/lessons/{id}/video`
- `POST /api/admin/shadowing/materials/drafts`

The new import endpoint becomes the preferred page flow.

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
- `--video-base-path`: default `/api/v1/videos`.

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
- If neither is provided, import remains URL-only and keeps current behavior.
- The command prints imported lesson IDs and video binding results.

## Admin Page Flow

Update `Shadowing Materials` page so operators can finish the workflow in one
place.

UI changes:

- Keep the existing upload/bind panel.
- Keep the existing draft generator.
- Add an `Import reviewed JSON` action next to the draft JSON textarea.
- Add optional controls for `video_object_id` and `video_file`.
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

## Data Consistency

The import operation should be idempotent:

- Same lesson title and JLPT level update the existing lesson.
- Same video file creates the same MinIO object key.
- Same `(bucket, object_key)` upserts the same `video_objects` row.
- Re-importing the same material pack should not create duplicate lessons or
  duplicate video metadata.

Database work should run in one transaction where practical:

- Lesson upsert.
- Child sentence replacement.
- Lesson word replacement.
- `video_objects` upsert.
- `lessons.video_object_id` update.

MinIO upload cannot be part of the PostgreSQL transaction. The first version
uses deterministic object keys to make retry safe instead of attempting complex
rollback.

## Testing Strategy

Backend tests should be table-driven where possible.

CLI/importer tests:

- URL-only import preserves current behavior.
- Existing `video_object_id` binds lesson and normalizes config.
- `--video-file` uploads through a fake uploader and binds the created object.
- `--video-file` and `--video-object-id` together return a validation error.
- Re-import updates the same lesson and does not duplicate child rows.

Admin handler tests:

- Multipart import with `lesson_json + video_object_id` imports and binds.
- Multipart import with `lesson_json + video_file` inserts `video_objects` and
  binds the lesson.
- Missing MinIO config returns `503` only when upload is requested.
- Missing video object returns `404`.
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
- The imported lesson has `lessons.video_object_id` set.
- The imported lesson's `shadowing_config_json.video_url` and `media_url` point
  to `/api/v1/videos/{id}/stream`.
- CLI manual import can bind an existing video object or upload a video file.
- URL-only import still works for temporary external media.
- No new SQLite video import behavior is required.
- Existing video upload and bind endpoints continue to work.
