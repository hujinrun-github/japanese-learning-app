package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBindShadowingLessonVideoObject(t *testing.T) {
	db := openShadowingMaterialsTestDB(t)
	mustExec(t, db, `
		CREATE TABLE lessons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			jlpt_level TEXT NOT NULL,
			shadowing_enabled INTEGER NOT NULL DEFAULT 0,
			shadowing_version INTEGER NOT NULL DEFAULT 1,
			shadowing_config_json TEXT NOT NULL DEFAULT '{}',
			video_object_id INTEGER
		)`)
	mustExec(t, db, `
		CREATE TABLE video_objects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			bucket TEXT NOT NULL,
			object_key TEXT NOT NULL,
			kind TEXT NOT NULL,
			visibility TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			deleted_at TEXT
		)`)
	mustExec(t, db, `INSERT INTO lessons (title, jlpt_level, shadowing_config_json) VALUES ('Video Lesson', 'N5', '{"loop_count":3}')`)
	mustExec(t, db, `INSERT INTO video_objects (bucket, object_key, kind, visibility, mime_type) VALUES ('lesson-videos', 'lessons/video.mp4', 'lesson_shadowing', 'public', 'video/mp4')`)

	handler := NewHandler(HandlerConfig{
		AdminToken:             "test-token",
		ShadowingMaterialsDB:   db,
		ShadowingMaterialsSQL:  "sqlite",
		ShadowingVideoBasePath: "/api/v1/videos",
	})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		srv.URL+"/api/admin/shadowing/materials/lessons/1/video",
		strings.NewReader(`{"video_object_id":1}`),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST bind video: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		LessonID      int64  `json:"lesson_id"`
		VideoObjectID int64  `json:"video_object_id"`
		VideoURL      string `json:"video_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.LessonID != 1 || got.VideoObjectID != 1 || got.VideoURL != "/api/v1/videos/1/stream" {
		t.Fatalf("response = %+v, want bound video URL", got)
	}

	var enabled int
	var videoObjectID int64
	var configRaw string
	if err := db.QueryRow(`SELECT shadowing_enabled, video_object_id, shadowing_config_json FROM lessons WHERE id = 1`).Scan(&enabled, &videoObjectID, &configRaw); err != nil {
		t.Fatalf("query lesson: %v", err)
	}
	if enabled != 1 || videoObjectID != 1 {
		t.Fatalf("lesson enabled=%d video_object_id=%d, want enabled=1 video_object_id=1", enabled, videoObjectID)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(configRaw), &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if config["media_type"] != "video" || config["media_url"] != "/api/v1/videos/1/stream" || config["video_url"] != "/api/v1/videos/1/stream" {
		t.Fatalf("config = %+v, want video media fields", config)
	}
	if config["loop_count"].(float64) != 3 {
		t.Fatalf("config loop_count = %+v, want preserved", config["loop_count"])
	}
}

func TestUploadShadowingVideoCreatesObjectAndBindsLesson(t *testing.T) {
	db := openShadowingMaterialsTestDB(t)
	mustExec(t, db, `
		CREATE TABLE lessons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			jlpt_level TEXT NOT NULL,
			shadowing_enabled INTEGER NOT NULL DEFAULT 0,
			shadowing_version INTEGER NOT NULL DEFAULT 1,
			shadowing_config_json TEXT NOT NULL DEFAULT '{}',
			video_object_id INTEGER
		)`)
	mustExec(t, db, `
		CREATE TABLE video_objects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			bucket TEXT NOT NULL,
			object_key TEXT NOT NULL,
			kind TEXT NOT NULL,
			visibility TEXT NOT NULL,
			content_sha256 TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			mime_type TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			deleted_at TEXT
		)`)
	mustExec(t, db, `INSERT INTO lessons (title, jlpt_level) VALUES ('Upload Lesson', 'N5')`)

	uploader := &stubShadowingVideoUploader{
		result: uploadedShadowingVideo{
			Bucket:        "lesson-videos",
			ObjectKey:     "lessons/upload.mp4",
			ContentSHA256: "sha256-upload",
			SizeBytes:     9,
			MimeType:      "video/mp4",
		},
	}
	handler := NewHandler(HandlerConfig{
		AdminToken:             "test-token",
		ShadowingMaterialsDB:   db,
		ShadowingMaterialsSQL:  "sqlite",
		ShadowingVideoBasePath: "/api/v1/videos",
		ShadowingVideoUploader: uploader,
		ShadowingVideoBucket:   "lesson-videos",
	})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("lesson_id", "1"); err != nil {
		t.Fatalf("write lesson_id: %v", err)
	}
	part, err := writer.CreateFormFile("file", "upload.mp4")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("video-bin")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/shadowing/materials/videos", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST upload video: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, http.StatusCreated, raw)
	}

	var got struct {
		VideoObjectID int64  `json:"video_object_id"`
		VideoURL      string `json:"video_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.VideoObjectID != 1 || got.VideoURL != "/api/v1/videos/1/stream" {
		t.Fatalf("response = %+v, want created object URL", got)
	}
	if uploader.filename != "upload.mp4" || uploader.contentType != "application/octet-stream" {
		t.Fatalf("uploader filename=%q contentType=%q", uploader.filename, uploader.contentType)
	}

	var objectCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_objects WHERE bucket = 'lesson-videos' AND object_key = 'lessons/upload.mp4'`).Scan(&objectCount); err != nil {
		t.Fatalf("count video object: %v", err)
	}
	if objectCount != 1 {
		t.Fatalf("video object count = %d, want 1", objectCount)
	}

	var videoObjectID int64
	var configRaw string
	if err := db.QueryRow(`SELECT video_object_id, shadowing_config_json FROM lessons WHERE id = 1`).Scan(&videoObjectID, &configRaw); err != nil {
		t.Fatalf("query bound lesson: %v", err)
	}
	if videoObjectID != 1 {
		t.Fatalf("lesson video_object_id = %d, want 1", videoObjectID)
	}
	if !strings.Contains(configRaw, "/api/v1/videos/1/stream") {
		t.Fatalf("shadowing_config_json = %s, want video URL", configRaw)
	}
}

func TestCreateShadowingMaterialDraftFromTranscript(t *testing.T) {
	handler := NewHandler(HandlerConfig{AdminToken: "test-token"})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	body := `{
		"title":"Draft Lesson",
		"jlpt_level":"N5",
		"video_url":"/api/v1/videos/7/stream",
		"media_duration_ms":6000,
		"transcript":"\u304a\u306f\u3088\u3046\u3054\u3056\u3044\u307e\u3059\u3002\u4eca\u65e5\u306f\u3044\u3044\u5929\u6c17\u3067\u3059\u3002"
	}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/shadowing/materials/drafts", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST draft: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, http.StatusCreated, raw)
	}

	var got struct {
		Lesson map[string]any `json:"lesson"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Lesson["title"] != "Draft Lesson" || got.Lesson["video_url"] != "/api/v1/videos/7/stream" || got.Lesson["shadowing_enabled"] != true {
		t.Fatalf("lesson = %+v, want draft metadata", got.Lesson)
	}
	sentences := got.Lesson["sentences"].([]any)
	if len(sentences) != 2 {
		t.Fatalf("sentence count = %d, want 2", len(sentences))
	}
	first := sentences[0].(map[string]any)
	second := sentences[1].(map[string]any)
	if first["start_ms"] != float64(0) || first["end_ms"] != float64(3000) || second["start_ms"] != float64(3000) || second["end_ms"] != float64(6000) {
		t.Fatalf("sentences = %+v, want even timing draft", sentences)
	}
}

type stubShadowingVideoUploader struct {
	result      uploadedShadowingVideo
	filename    string
	contentType string
}

func (s *stubShadowingVideoUploader) UploadLessonVideo(_ context.Context, filename, contentType string, _ io.Reader) (uploadedShadowingVideo, error) {
	s.filename = filename
	s.contentType = contentType
	return s.result, nil
}

func openShadowingMaterialsTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
