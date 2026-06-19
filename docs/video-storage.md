# Video Storage

This project stores video binaries outside PostgreSQL. PostgreSQL stores only
metadata and lesson references.

## PostgreSQL Schema

- `video_objects` stores video object metadata:
  - `bucket`
  - `object_key`
  - `kind`
  - `visibility`
  - `content_sha256`
  - `size_bytes`
  - `mime_type`
  - `metadata_json`
  - `owner_user_id`
  - `deleted_at`
- `lessons.video_object_id` references `video_objects(id)`.
- `lessons.shadowing_config_json` may also contain URL-based media fields:
  - `media_type: "video"`
  - `media_url`
  - `video_url`

## Runtime Contract

Lesson APIs expose both `audio_url` and `video_url`.

- Audio lessons use `audio_url`.
- Video lessons use `video_url`.
- For video material packs, the Postgres importer copies top-level `video_url`
  into `shadowing_config_json.video_url`.
- If `shadowing_config.media_type` is `video` and no `media_url` exists, the
  importer also sets `shadowing_config_json.media_url` to the same video URL.
- If a lesson references `video_object_id`, the Postgres lesson store exposes
  `/api/v1/videos/{id}/stream` as the fallback `video_url`.

## Current Boundary

The schema and API contract are ready for video-backed shadowing lessons. The
current learner UI is still audio-first and does not yet render a video player
for shadowing lessons.
