package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/module/shadowing"
)

// newLessonAudioObjectLoader resolves a lesson audio object id to its WAV bytes
// in MinIO. It is only used by the PostgreSQL runtime, where audio_objects rows
// point at migrated objects; SQLite seed lessons read local WAV files instead.
func newLessonAudioObjectLoader(db *sql.DB, cfg *config.Config) shadowing.ObjectLoader {
	return func(ctx context.Context, objectID int64) ([]byte, error) {
		var bucket, objectKey string
		if err := db.QueryRowContext(ctx,
			`SELECT bucket, object_key FROM audio_objects WHERE id = $1 AND deleted_at IS NULL`,
			objectID,
		).Scan(&bucket, &objectKey); err != nil {
			return nil, fmt.Errorf("newLessonAudioObjectLoader query id=%d: %w", objectID, err)
		}

		client, err := newAudioMinIOClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("newLessonAudioObjectLoader minio client: %w", err)
		}
		object, err := client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			return nil, fmt.Errorf("newLessonAudioObjectLoader GetObject: %w", err)
		}
		defer func() {
			if err := object.Close(); err != nil {
				slog.Error("newLessonAudioObjectLoader: close object failed", "err", err)
			}
		}()

		data, err := io.ReadAll(object)
		if err != nil {
			return nil, fmt.Errorf("newLessonAudioObjectLoader read: %w", err)
		}
		return data, nil
	}
}
