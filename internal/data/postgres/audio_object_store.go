package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"japanese-learning-app/internal/store"
)

// AudioObjectStore implements store.AudioObjectStoreInterface for PostgreSQL.
type AudioObjectStore struct {
	db  queryer
	now func() time.Time
}

// NewAudioObjectStore creates an AudioObjectStore backed by PostgreSQL.
func NewAudioObjectStore(db queryer, deps store.StoreDeps) *AudioObjectStore {
	return &AudioObjectStore{
		db:  db,
		now: currentTimeFunc(deps.Clock),
	}
}

// Insert creates a new audio object record and returns its ID.
func (s *AudioObjectStore) Insert(ctx context.Context, row store.AudioObjectRow) (int64, error) {
	metadataStr := string(row.MetadataJSON)
	if metadataStr == "" || metadataStr == "null" {
		metadataStr = "{}"
	}
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO audio_objects (bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type, metadata_json, owner_user_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
		 ON CONFLICT (bucket, object_key) DO UPDATE
		   SET content_sha256 = EXCLUDED.content_sha256,
		       size_bytes = EXCLUDED.size_bytes,
		       mime_type = EXCLUDED.mime_type,
		       updated_at = now()
		 RETURNING id`,
		row.Bucket,
		row.ObjectKey,
		row.Kind,
		row.Visibility,
		row.ContentSHA256,
		row.SizeBytes,
		row.MimeType,
		metadataStr,
		row.OwnerUserID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.AudioObjectStore.Insert: %w", translateError(err))
	}
	return id, nil
}

// GetByID finds an audio object by its ID.
func (s *AudioObjectStore) GetByID(ctx context.Context, id int64) (*store.AudioObjectRow, error) {
	var row store.AudioObjectRow
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type, metadata_json, owner_user_id, deleted_at, created_at, updated_at
		 FROM audio_objects WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&row.ID, &row.Bucket, &row.ObjectKey, &row.Kind, &row.Visibility, &row.ContentSHA256, &row.SizeBytes, &row.MimeType, &row.MetadataJSON, &row.OwnerUserID, &row.DeletedAt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("postgres.AudioObjectStore.GetByID %d: %w", id, sql.ErrNoRows)
		}
		return nil, fmt.Errorf("postgres.AudioObjectStore.GetByID: %w", translateError(err))
	}
	return &row, nil
}

// GetByKey finds an audio object by bucket + object_key.
func (s *AudioObjectStore) GetByKey(ctx context.Context, bucket, objectKey string) (*store.AudioObjectRow, error) {
	var row store.AudioObjectRow
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type, metadata_json, owner_user_id, deleted_at, created_at, updated_at
		 FROM audio_objects WHERE bucket = $1 AND object_key = $2 AND deleted_at IS NULL`, bucket, objectKey,
	).Scan(&row.ID, &row.Bucket, &row.ObjectKey, &row.Kind, &row.Visibility, &row.ContentSHA256, &row.SizeBytes, &row.MimeType, &row.MetadataJSON, &row.OwnerUserID, &row.DeletedAt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("postgres.AudioObjectStore.GetByKey (%s/%s): %w", bucket, objectKey, sql.ErrNoRows)
		}
		return nil, fmt.Errorf("postgres.AudioObjectStore.GetByKey: %w", translateError(err))
	}
	return &row, nil
}

// SoftDelete marks an audio object as deleted (soft delete).
func (s *AudioObjectStore) SoftDelete(ctx context.Context, id int64) error {
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE audio_objects SET deleted_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, now,
	)
	if err != nil {
		return fmt.Errorf("postgres.AudioObjectStore.SoftDelete: %w", translateError(err))
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("postgres.AudioObjectStore.SoftDelete %d: not found or already deleted", id)
	}
	return nil
}

// ListByKind returns all non-deleted audio objects of a given kind.
func (s *AudioObjectStore) ListByKind(ctx context.Context, kind string, offset, limit int) ([]store.AudioObjectRow, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audio_objects WHERE kind = $1 AND deleted_at IS NULL`, kind,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("postgres.AudioObjectStore.ListByKind count: %w", translateError(err))
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type, metadata_json, owner_user_id, deleted_at, created_at, updated_at
		 FROM audio_objects WHERE kind = $1 AND deleted_at IS NULL
		 ORDER BY id ASC LIMIT $2 OFFSET $3`, kind, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.AudioObjectStore.ListByKind query: %w", translateError(err))
	}
	defer rows.Close()

	return scanAudioObjectRows(rows, total)
}

// scanAudioObjectRows scans rows into AudioObjectRow slices.
func scanAudioObjectRows(rows *sql.Rows, total int) ([]store.AudioObjectRow, int, error) {
	items := make([]store.AudioObjectRow, 0)
	for rows.Next() {
		var item store.AudioObjectRow
		if err := rows.Scan(
			&item.ID,
			&item.Bucket,
			&item.ObjectKey,
			&item.Kind,
			&item.Visibility,
			&item.ContentSHA256,
			&item.SizeBytes,
			&item.MimeType,
			&item.MetadataJSON,
			&item.OwnerUserID,
			&item.DeletedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("postgres.scanAudioObjectRows: %w", translateError(err))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("postgres.scanAudioObjectRows rows: %w", translateError(err))
	}
	return items, total, nil
}
