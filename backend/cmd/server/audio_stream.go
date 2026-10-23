package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/httputil"
)

type audioStreamHandler struct {
	db  *sql.DB
	cfg *config.Config
}

type audioObjectRecord struct {
	bucket     string
	objectKey  string
	visibility string
	mimeType   string
}

func registerAudioStreamRoutes(mux *http.ServeMux, db *sql.DB, cfg *config.Config) {
	handler := &audioStreamHandler{db: db, cfg: cfg}
	mux.HandleFunc("GET /api/v1/audio/{id}/stream", handler.serve)
}

func (h *audioStreamHandler) serve(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid audio id", "")
		return
	}

	record, err := h.loadAudioObject(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "audio object not found", "")
		return
	}
	if err != nil {
		slog.Error("audio stream: failed to load audio object", "audio_object_id", id, "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load audio object", "")
		return
	}
	if record.visibility != "public" {
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "audio object not found", "")
		return
	}

	client, err := newAudioMinIOClient(h.cfg)
	if err != nil {
		slog.Error("audio stream: storage is not configured", "audio_object_id", id, "err", err)
		httputil.WriteError(w, http.StatusServiceUnavailable, "ERR_AUDIO_STORAGE_UNAVAILABLE", "audio storage is not configured", "")
		return
	}

	object, err := client.GetObject(r.Context(), record.bucket, record.objectKey, minio.GetObjectOptions{})
	if err != nil {
		slog.Error("audio stream: failed to open audio object", "audio_object_id", id, "err", err)
		httputil.WriteError(w, http.StatusBadGateway, "ERR_AUDIO_STREAM_FAILED", "failed to open audio object", "")
		return
	}
	defer func() {
		if err := object.Close(); err != nil {
			slog.Error("audio stream: failed to close audio object", "audio_object_id", id, "err", err)
		}
	}()

	info, err := object.Stat()
	if err != nil {
		slog.Error("audio stream: failed to stat audio object", "audio_object_id", id, "err", err)
		httputil.WriteError(w, http.StatusBadGateway, "ERR_AUDIO_STREAM_FAILED", "failed to stat audio object", "")
		return
	}

	if record.mimeType != "" {
		w.Header().Set("Content-Type", record.mimeType)
	} else if info.ContentType != "" {
		w.Header().Set("Content-Type", info.ContentType)
	}
	http.ServeContent(w, r, path.Base(record.objectKey), info.LastModified, object)
}

func (h *audioStreamHandler) loadAudioObject(r *http.Request, id int64) (audioObjectRecord, error) {
	var record audioObjectRecord
	err := h.db.QueryRowContext(
		r.Context(),
		`SELECT bucket, object_key, visibility, mime_type
		 FROM audio_objects
		 WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&record.bucket, &record.objectKey, &record.visibility, &record.mimeType)
	if err != nil {
		return audioObjectRecord{}, fmt.Errorf("audioStreamHandler.loadAudioObject id=%d: %w", id, err)
	}
	return record, nil
}

func newAudioMinIOClient(cfg *config.Config) (*minio.Client, error) {
	if cfg == nil || strings.TrimSpace(cfg.MinIOEndpoint) == "" || cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
		return nil, errors.New("missing minio audio storage config")
	}

	endpoint, secure, err := minIOEndpoint(cfg.MinIOEndpoint, cfg.MinIOUseSSL)
	if err != nil {
		return nil, fmt.Errorf("resolve minio audio endpoint: %w", err)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio audio client: %w", err)
	}
	return client, nil
}
