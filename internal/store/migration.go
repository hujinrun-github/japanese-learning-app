package store

import (
	"context"
	"database/sql"
	"time"
)

type MigrationCapableAdapter interface {
	RelationalAdapter
	NewMigrationTarget(db *sql.DB, deps MigrationDeps) (MigrationTarget, error)
}

type MigrationDeps struct {
	Clock          Clock
	Logger         Logger
	AppTimezone    *time.Location
	MigrationRunID string
}

type MigrationTarget interface {
	VerifySchemaOnly(ctx context.Context) error
	WithBulkLoad(ctx context.Context, fn func(BulkLoader) error) error
	ResetIdentities(ctx context.Context, tables []MigrationTable) error
	BuildActualManifest(ctx context.Context) (*MigrationManifest, error)
}

type BulkLoader interface {
	InsertPreservingID(ctx context.Context, batch RowBatch) error
	InsertRows(ctx context.Context, batch RowBatch) error
	InsertAudioObject(ctx context.Context, row AudioObjectRow) (int64, error)
	ValidateReferences(ctx context.Context) error
}

type MigrationTable uint16
type MigrationColumn uint16

type RowBatch struct {
	Table   MigrationTable
	Columns []MigrationColumn
	Rows    [][]any
}

type AudioObjectRow struct {
	ID            int64
	Bucket        string
	ObjectKey     string
	Kind          string
	Visibility    string
	ContentSHA256 string
	SizeBytes     int64
	MimeType      string
	MetadataJSON  []byte
	OwnerUserID   *int64
	DeletedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MigrationManifest struct {
	RunID     string
	Generated time.Time
	Counts    map[string]int64
	Remaps    map[string]int64
	Errors    []string
}
