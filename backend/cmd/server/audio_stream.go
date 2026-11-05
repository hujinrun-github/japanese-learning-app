package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
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
	mux.HandleFunc("GET /audio/{key...}", handler.serveLegacy)
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

	h.serveObject(w, r, record)
}

func (h *audioStreamHandler) serveLegacy(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "." || !fs.ValidPath(key) {
		http.NotFound(w, r)
		return
	}

	// Admin regeneration writes local files. Serve them before migrated copies.
	if h.cfg.AudioStorePath != "" {
		file, err := os.OpenInRoot(h.cfg.AudioStorePath, filepath.FromSlash(key))
		if err == nil {
			defer func() {
				if err := file.Close(); err != nil {
					slog.Error("legacy audio: failed to close local file", "key", key, "err", err)
				}
			}()
			info, err := file.Stat()
			if err != nil {
				slog.Error("legacy audio: failed to stat local file", "key", key, "err", err)
				http.Error(w, "failed to read audio file", http.StatusInternalServerError)
				return
			}
			if !info.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			http.ServeContent(w, r, path.Base(key), info.ModTime(), file)
			return
		}
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("legacy audio: failed to open local file", "key", key, "err", err)
		}
	}

	var record audioObjectRecord
	err := h.db.QueryRowContext(r.Context(), `
		SELECT bucket, object_key, visibility, mime_type
		FROM audio_objects
		WHERE object_key = $1 AND deleted_at IS NULL AND visibility = 'public'
		ORDER BY (bucket = $2) DESC, id
		LIMIT 1`, key, h.cfg.MinIOBucketAudio,
	).Scan(&record.bucket, &record.objectKey, &record.visibility, &record.mimeType)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("legacy audio: failed to look up audio object", "key", key, "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load audio object", "")
		return
	}
	h.serveObject(w, r, record)
}

func (h *audioStreamHandler) serveObject(w http.ResponseWriter, r *http.Request, record audioObjectRecord) {
	client, err := newAudioMinIOClient(h.cfg)
	if err != nil {
		slog.Error("audio stream: storage is not configured", "key", record.objectKey, "err", err)
		httputil.WriteError(w, http.StatusServiceUnavailable, "ERR_AUDIO_STORAGE_UNAVAILABLE", "audio storage is not configured", "")
		return
	}

	object, err := client.GetObject(r.Context(), record.bucket, record.objectKey, minio.GetObjectOptions{})
	if err != nil {
		slog.Error("audio stream: failed to open audio object", "key", record.objectKey, "err", err)
		httputil.WriteError(w, http.StatusBadGateway, "ERR_AUDIO_STREAM_FAILED", "failed to open audio object", "")
		return
	}
	defer func() {
		if err := object.Close(); err != nil {
			slog.Error("audio stream: failed to close audio object", "key", record.objectKey, "err", err)
		}
	}()

	info, err := object.Stat()
	if err != nil {
		slog.Error("audio stream: failed to stat audio object", "key", record.objectKey, "err", err)
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
