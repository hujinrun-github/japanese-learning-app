package store

import (
	"context"
	"database/sql"
	"time"
)

type RelationalAdapter interface {
	Name() string
	Open(ctx context.Context, cfg DatabaseConfig) (*sql.DB, error)
	RunMigrations(ctx context.Context, db *sql.DB) error
	NewStoreRuntime(db *sql.DB, deps StoreDeps) (*StoreRuntime, error)
	TranslateError(error) error
	Capabilities() RelationalCapabilities
}

type DatabaseConfig struct {
	Store           string
	DatabaseURL     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	AppTimezone     *time.Location
}

type StoreDeps struct {
	Clock       Clock
	Logger      Logger
	AppTimezone *time.Location
}

type Clock interface {
	Now() time.Time
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type RelationalCapabilities struct {
	Transactions         bool
	Migrations           bool
	MigrationLocks       bool
	IdentityOverride     bool
	InsertReturning      bool
	Upsert               bool
	JSONDocuments        bool
	ArrayFields          bool
	TagFiltering         bool
	TextSearch           bool
	TimeWindowFiltering  bool
	AudioMetadataLinkage bool
}
