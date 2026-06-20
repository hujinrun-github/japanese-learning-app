package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type shadowingMaterialLesson struct {
	ID               int64          `json:"id"`
	Title            string         `json:"title"`
	JLPTLevel        string         `json:"jlpt_level"`
	ShadowingEnabled bool           `json:"shadowing_enabled"`
	ShadowingVersion int            `json:"shadowing_version"`
	ShadowingConfig  map[string]any `json:"shadowing_config"`
	VideoObjectID    *int64         `json:"video_object_id"`
	VideoURL         string         `json:"video_url"`
}

type bindShadowingLessonVideoRequest struct {
	VideoObjectID int64 `json:"video_object_id"`
}

type createShadowingMaterialDraftRequest struct {
	Title           string `json:"title"`
	JLPTLevel       string `json:"jlpt_level"`
	AudioURL        string `json:"audio_url"`
	VideoURL        string `json:"video_url"`
	MediaDurationMS int64  `json:"media_duration_ms"`
	Transcript      string `json:"transcript"`
}

type uploadedShadowingVideo struct {
	Bucket        string
	ObjectKey     string
	ContentSHA256 string
	SizeBytes     int64
	MimeType      string
}

type ShadowingVideoUploader interface {
	UploadLessonVideo(rctx context.Context, filename, contentType string, reader io.Reader) (uploadedShadowingVideo, error)
}

func (h *Handler) listShadowingMaterialLessons(w http.ResponseWriter, r *http.Request) {
	db := h.shadowingMaterialsDB()
	if db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shadowing materials database is not configured"})
		return
	}

	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	size := queryInt(r, "size", 20)
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	offset := (page - 1) * size
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	dialect := h.shadowingMaterialsSQL()
	where := ""
	args := []any{}
	if search != "" {
		if dialect == "postgres" {
			where = "WHERE title ILIKE " + sqlPlaceholder(dialect, 1)
		} else {
			where = "WHERE title LIKE " + sqlPlaceholder(dialect, 1)
		}
		args = append(args, "%"+search+"%")
	}

	countQuery := "SELECT COUNT(*) FROM lessons " + where
	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "count shadowing lessons failed: " + err.Error()})
		return
	}

	limitPlaceholder := sqlPlaceholder(dialect, len(args)+1)
	offsetPlaceholder := sqlPlaceholder(dialect, len(args)+2)
	args = append(args, size, offset)

	query := fmt.Sprintf(`
		SELECT id, title, jlpt_level, shadowing_enabled, shadowing_version, shadowing_config_json, video_object_id
		FROM lessons
		%s
		ORDER BY id DESC
		LIMIT %s OFFSET %s`, where, limitPlaceholder, offsetPlaceholder)
	rows, err := db.Query(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list shadowing lessons failed: " + err.Error()})
		return
	}
	defer rows.Close()

	items := []shadowingMaterialLesson{}
	for rows.Next() {
		item, err := scanShadowingMaterialLesson(rows, h.shadowingVideoBasePath())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan shadowing lesson failed: " + err.Error()})
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list shadowing lessons failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handler) createShadowingMaterialDraft(w http.ResponseWriter, r *http.Request) {
	var req createShadowingMaterialDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	req.JLPTLevel = strings.TrimSpace(req.JLPTLevel)
	req.AudioURL = strings.TrimSpace(req.AudioURL)
	req.VideoURL = strings.TrimSpace(req.VideoURL)
	req.Transcript = strings.TrimSpace(req.Transcript)

	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if req.JLPTLevel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jlpt_level is required"})
		return
	}
	if req.AudioURL == "" && req.VideoURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "audio_url or video_url is required"})
		return
	}
	if req.MediaDurationMS <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media_duration_ms must be greater than 0"})
		return
	}

	transcriptSentences := splitShadowingTranscript(req.Transcript)
	if len(transcriptSentences) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "transcript must contain at least one sentence"})
		return
	}

	mediaType := "audio"
	mediaURL := req.AudioURL
	if req.VideoURL != "" {
		mediaType = "video"
		mediaURL = req.VideoURL
	}

	lesson := map[string]any{
		"title":             req.Title,
		"jlpt_level":        req.JLPTLevel,
		"tags":              []string{"shadowing", "draft"},
		"audio_url":         req.AudioURL,
		"video_url":         req.VideoURL,
		"word_ids":          []int64{},
		"shadowing_enabled": true,
		"shadowing_version": 1,
		"shadowing_config":  buildShadowingDraftConfig(mediaType, mediaURL, req.AudioURL, req.VideoURL, req.MediaDurationMS),
		"sentences":         buildShadowingDraftSentences(transcriptSentences, req.MediaDurationMS),
	}

	writeJSON(w, http.StatusCreated, map[string]any{"lesson": lesson})
}

func (h *Handler) uploadShadowingVideo(w http.ResponseWriter, r *http.Request) {
	db := h.shadowingMaterialsDB()
	if db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shadowing materials database is not configured"})
		return
	}
	uploader, err := h.shadowingVideoUploader()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}

	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form: " + err.Error()})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	uploaded, err := uploader.UploadLessonVideo(r.Context(), header.Filename, contentType, file)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload video failed: " + err.Error()})
		return
	}
	if uploaded.Bucket == "" {
		uploaded.Bucket = h.shadowingVideoBucket()
	}
	if uploaded.MimeType == "" {
		uploaded.MimeType = contentType
	}

	dialect := h.shadowingMaterialsSQL()
	videoObjectID, err := insertShadowingVideoObject(db, dialect, uploaded)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save video object failed: " + err.Error()})
		return
	}

	videoURL := shadowingVideoURL(h.shadowingVideoBasePath(), videoObjectID)
	response := map[string]any{
		"video_object_id": videoObjectID,
		"video_url":       videoURL,
		"bucket":          uploaded.Bucket,
		"object_key":      uploaded.ObjectKey,
	}

	lessonID, err := parseOptionalInt64(r.FormValue("lesson_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lesson_id"})
		return
	}
	if lessonID > 0 {
		config, err := bindShadowingLessonVideoObject(db, dialect, h.shadowingVideoBasePath(), lessonID, videoObjectID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": "bind video failed: " + err.Error()})
			return
		}
		response["lesson_id"] = lessonID
		response["shadowing_config"] = config
	}

	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) bindShadowingLessonVideo(w http.ResponseWriter, r *http.Request) {
	db := h.shadowingMaterialsDB()
	if db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shadowing materials database is not configured"})
		return
	}

	lessonID, err := pathID(r)
	if err != nil || lessonID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lesson id"})
		return
	}

	var req bindShadowingLessonVideoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.VideoObjectID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video_object_id is required"})
		return
	}

	dialect := h.shadowingMaterialsSQL()
	if err := ensurePublicVideoObject(db, dialect, req.VideoObjectID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": "video object not found"})
		return
	}

	config, err := bindShadowingLessonVideoObject(db, dialect, h.shadowingVideoBasePath(), lessonID, req.VideoObjectID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": "lesson not found"})
		return
	}

	videoURL := shadowingVideoURL(h.shadowingVideoBasePath(), req.VideoObjectID)
	writeJSON(w, http.StatusOK, map[string]any{
		"lesson_id":        lessonID,
		"video_object_id":  req.VideoObjectID,
		"video_url":        videoURL,
		"shadowing_config": config,
	})
}

func (h *Handler) shadowingMaterialsDB() *sql.DB {
	if h.cfg.ShadowingMaterialsDB != nil {
		return h.cfg.ShadowingMaterialsDB
	}
	return h.cfg.DB
}

func (h *Handler) shadowingMaterialsSQL() string {
	if h.cfg.ShadowingMaterialsSQL != "" {
		return h.cfg.ShadowingMaterialsSQL
	}
	return "sqlite"
}

func (h *Handler) shadowingVideoBasePath() string {
	base := strings.TrimRight(h.cfg.ShadowingVideoBasePath, "/")
	if base == "" {
		return "/api/v1/videos"
	}
	return base
}

func (h *Handler) shadowingVideoBucket() string {
	if strings.TrimSpace(h.cfg.ShadowingVideoBucket) != "" {
		return strings.TrimSpace(h.cfg.ShadowingVideoBucket)
	}
	return "lesson-videos"
}

func (h *Handler) shadowingVideoUploader() (ShadowingVideoUploader, error) {
	if h.cfg.ShadowingVideoUploader != nil {
		return h.cfg.ShadowingVideoUploader, nil
	}
	if strings.TrimSpace(h.cfg.MinIOEndpoint) == "" || h.cfg.MinIOAccessKey == "" || h.cfg.MinIOSecretKey == "" {
		return nil, fmt.Errorf("shadowing video storage is not configured")
	}
	return minIOShadowingVideoUploader{
		endpoint:  h.cfg.MinIOEndpoint,
		accessKey: h.cfg.MinIOAccessKey,
		secretKey: h.cfg.MinIOSecretKey,
		useSSL:    h.cfg.MinIOUseSSL,
		bucket:    h.shadowingVideoBucket(),
	}, nil
}

func sqlPlaceholder(dialect string, position int) string {
	if dialect == "postgres" {
		return "$" + strconv.Itoa(position)
	}
	return "?"
}

func ensurePublicVideoObject(db *sql.DB, dialect string, videoObjectID int64) error {
	query := "SELECT id FROM video_objects WHERE id = " + sqlPlaceholder(dialect, 1) + " AND deleted_at IS NULL AND visibility = 'public'"
	var id int64
	return db.QueryRow(query, videoObjectID).Scan(&id)
}

func loadShadowingLessonConfig(db *sql.DB, dialect string, lessonID int64) (map[string]any, error) {
	query := "SELECT shadowing_config_json FROM lessons WHERE id = " + sqlPlaceholder(dialect, 1)
	var raw string
	if err := db.QueryRow(query, lessonID).Scan(&raw); err != nil {
		return nil, err
	}
	config := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	return config, nil
}

func bindShadowingLessonVideoObject(db *sql.DB, dialect, videoBasePath string, lessonID, videoObjectID int64) (map[string]any, error) {
	config, err := loadShadowingLessonConfig(db, dialect, lessonID)
	if err != nil {
		return nil, err
	}
	videoURL := shadowingVideoURL(videoBasePath, videoObjectID)
	config["media_type"] = "video"
	config["media_url"] = videoURL
	config["video_url"] = videoURL
	configRaw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	if err := updateShadowingLessonVideo(db, dialect, lessonID, videoObjectID, string(configRaw)); err != nil {
		return nil, err
	}
	return config, nil
}

func updateShadowingLessonVideo(db *sql.DB, dialect string, lessonID, videoObjectID int64, configJSON string) error {
	if dialect == "postgres" {
		_, err := db.Exec(`
			UPDATE lessons
			SET video_object_id = $1,
			    shadowing_enabled = TRUE,
			    shadowing_version = GREATEST(shadowing_version, 1),
			    shadowing_config_json = $2::jsonb,
			    updated_at = now()
			WHERE id = $3`,
			videoObjectID,
			configJSON,
			lessonID,
		)
		return err
	}

	_, err := db.Exec(`
		UPDATE lessons
		SET video_object_id = ?,
		    shadowing_enabled = 1,
		    shadowing_version = CASE WHEN shadowing_version < 1 THEN 1 ELSE shadowing_version END,
		    shadowing_config_json = ?
		WHERE id = ?`,
		videoObjectID,
		configJSON,
		lessonID,
	)
	return err
}

func insertShadowingVideoObject(db *sql.DB, dialect string, uploaded uploadedShadowingVideo) (int64, error) {
	if dialect == "postgres" {
		var id int64
		err := db.QueryRow(`
			INSERT INTO video_objects (
				bucket, object_key, kind, visibility, content_sha256,
				size_bytes, mime_type, metadata_json, updated_at
			)
			VALUES ($1, $2, 'lesson_shadowing', 'public', $3, $4, $5, '{}'::jsonb, now())
			ON CONFLICT (bucket, object_key) DO UPDATE SET
				content_sha256 = EXCLUDED.content_sha256,
				size_bytes = EXCLUDED.size_bytes,
				mime_type = EXCLUDED.mime_type,
				deleted_at = NULL,
				updated_at = now()
			RETURNING id`,
			uploaded.Bucket,
			uploaded.ObjectKey,
			uploaded.ContentSHA256,
			uploaded.SizeBytes,
			uploaded.MimeType,
		).Scan(&id)
		return id, err
	}

	result, err := db.Exec(`
		INSERT INTO video_objects (
			bucket, object_key, kind, visibility, content_sha256,
			size_bytes, mime_type, metadata_json
		)
		VALUES (?, ?, 'lesson_shadowing', 'public', ?, ?, ?, '{}')`,
		uploaded.Bucket,
		uploaded.ObjectKey,
		uploaded.ContentSHA256,
		uploaded.SizeBytes,
		uploaded.MimeType,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func scanShadowingMaterialLesson(rows *sql.Rows, videoBasePath string) (shadowingMaterialLesson, error) {
	var item shadowingMaterialLesson
	var enabled bool
	var configRaw string
	var videoObjectID sql.NullInt64
	if err := rows.Scan(
		&item.ID,
		&item.Title,
		&item.JLPTLevel,
		&enabled,
		&item.ShadowingVersion,
		&configRaw,
		&videoObjectID,
	); err != nil {
		return item, err
	}
	item.ShadowingEnabled = enabled
	item.ShadowingConfig = map[string]any{}
	if strings.TrimSpace(configRaw) != "" {
		if err := json.Unmarshal([]byte(configRaw), &item.ShadowingConfig); err != nil {
			return item, err
		}
	}
	if videoObjectID.Valid {
		value := videoObjectID.Int64
		item.VideoObjectID = &value
		item.VideoURL = shadowingVideoURL(videoBasePath, value)
	} else if videoURL, ok := item.ShadowingConfig["video_url"].(string); ok {
		item.VideoURL = videoURL
	}
	return item, nil
}

func shadowingVideoURL(basePath string, videoObjectID int64) string {
	return strings.TrimRight(basePath, "/") + "/" + strconv.FormatInt(videoObjectID, 10) + "/stream"
}

func parseOptionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func splitShadowingTranscript(transcript string) []string {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return nil
	}

	var sentences []string
	var builder strings.Builder
	for _, r := range transcript {
		if r == '\r' || r == '\n' || r == '\t' {
			r = ' '
		}
		builder.WriteRune(r)
		switch r {
		case '\u3002', '\uff01', '\uff1f', '!', '?':
			if sentence := strings.TrimSpace(builder.String()); sentence != "" {
				sentences = append(sentences, sentence)
			}
			builder.Reset()
		}
	}
	if sentence := strings.TrimSpace(builder.String()); sentence != "" {
		sentences = append(sentences, sentence)
	}
	return sentences
}

func buildShadowingDraftConfig(mediaType, mediaURL, audioURL, videoURL string, durationMS int64) map[string]any {
	config := map[string]any{
		"media_type":        mediaType,
		"media_url":         mediaURL,
		"media_duration_ms": durationMS,
		"source": map[string]any{
			"kind":       "admin_transcript_draft",
			"transcript": "provided",
		},
	}
	if audioURL != "" {
		config["audio_url"] = audioURL
	}
	if videoURL != "" {
		config["video_url"] = videoURL
	}
	return config
}

func buildShadowingDraftSentences(parts []string, durationMS int64) []map[string]any {
	if len(parts) == 0 {
		return nil
	}

	sentences := make([]map[string]any, 0, len(parts))
	count := int64(len(parts))
	for i, japanese := range parts {
		startMS := durationMS * int64(i) / count
		endMS := durationMS * int64(i+1) / count
		sentences = append(sentences, map[string]any{
			"japanese": japanese,
			"chinese":  "",
			"start_ms": startMS,
			"end_ms":   endMS,
			"tokens": []map[string]any{
				{
					"surface": japanese,
					"reading": "",
				},
			},
		})
	}
	return sentences
}

type minIOShadowingVideoUploader struct {
	endpoint  string
	accessKey string
	secretKey string
	useSSL    bool
	bucket    string
}

func (u minIOShadowingVideoUploader) UploadLessonVideo(ctx context.Context, filename, contentType string, reader io.Reader) (uploadedShadowingVideo, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return uploadedShadowingVideo{}, err
	}
	sum := sha256.Sum256(data)
	sha := fmt.Sprintf("%x", sum[:])
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".mp4"
	}
	objectKey := "lessons/shadowing/" + sha[:16] + ext

	endpoint, secure, err := adminMinIOEndpoint(u.endpoint, u.useSSL)
	if err != nil {
		return uploadedShadowingVideo{}, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(u.accessKey, u.secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return uploadedShadowingVideo{}, err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = client.PutObject(ctx, u.bucket, objectKey, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return uploadedShadowingVideo{}, err
	}
	return uploadedShadowingVideo{
		Bucket:        u.bucket,
		ObjectKey:     objectKey,
		ContentSHA256: sha,
		SizeBytes:     int64(len(data)),
		MimeType:      contentType,
	}, nil
}

func adminMinIOEndpoint(raw string, fallbackSecure bool) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", false, err
		}
		return parsed.Host, parsed.Scheme == "https", nil
	}
	return raw, fallbackSecure, nil
}
