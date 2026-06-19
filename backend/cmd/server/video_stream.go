package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/httputil"
)

type videoStreamHandler struct {
	db  *sql.DB
	cfg *config.Config
}

type videoObjectRecord struct {
	bucket     string
	objectKey  string
	visibility string
	mimeType   string
}

func registerVideoStreamRoutes(mux *http.ServeMux, db *sql.DB, cfg *config.Config) {
	handler := &videoStreamHandler{db: db, cfg: cfg}
	mux.HandleFunc("GET /api/v1/videos/{id}/stream", handler.serve)
}

func (h *videoStreamHandler) serve(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid video id", "")
		return
	}

	record, err := h.loadVideoObject(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "video object not found", "")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load video object", "")
		return
	}
	if record.visibility != "public" {
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "video object not found", "")
		return
	}

	client, err := newVideoMinIOClient(h.cfg)
	if err != nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "ERR_VIDEO_STORAGE_UNAVAILABLE", "video storage is not configured", "")
		return
	}

	object, err := client.GetObject(r.Context(), record.bucket, record.objectKey, minio.GetObjectOptions{})
	if err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "ERR_VIDEO_STREAM_FAILED", "failed to open video object", "")
		return
	}
	defer object.Close()

	info, err := object.Stat()
	if err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "ERR_VIDEO_STREAM_FAILED", "failed to stat video object", "")
		return
	}

	if record.mimeType != "" {
		w.Header().Set("Content-Type", record.mimeType)
	} else if info.ContentType != "" {
		w.Header().Set("Content-Type", info.ContentType)
	}
	http.ServeContent(w, r, path.Base(record.objectKey), info.LastModified, object)
}

func (h *videoStreamHandler) loadVideoObject(r *http.Request, id int64) (videoObjectRecord, error) {
	var record videoObjectRecord
	err := h.db.QueryRowContext(
		r.Context(),
		`SELECT bucket, object_key, visibility, mime_type
		 FROM video_objects
		 WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&record.bucket, &record.objectKey, &record.visibility, &record.mimeType)
	return record, err
}

func newVideoMinIOClient(cfg *config.Config) (*minio.Client, error) {
	if cfg == nil || strings.TrimSpace(cfg.MinIOEndpoint) == "" || cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
		return nil, errors.New("missing minio video storage config")
	}

	endpoint, secure, err := minIOEndpoint(cfg.MinIOEndpoint, cfg.MinIOUseSSL)
	if err != nil {
		return nil, err
	}
	return minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: secure,
	})
}

func minIOEndpoint(raw string, fallbackSecure bool) (string, bool, error) {
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
