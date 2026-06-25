package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"

	"japanese-learning-app/internal/store"
)

type Adapter struct{}

func (Adapter) Name() string {
	return "postgres"
}

func (Adapter) Open(ctx context.Context, cfg store.DatabaseConfig) (*sql.DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("postgres.Open: database URL is required")
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres.Open: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres.Open ping: %w", err)
	}

	return db, nil
}

func (Adapter) Capabilities() store.RelationalCapabilities {
	return store.RelationalCapabilities{
		Transactions:         true,
		Migrations:           true,
		MigrationLocks:       true,
		IdentityOverride:     true,
		InsertReturning:      true,
		Upsert:               true,
		JSONDocuments:        true,
		ArrayFields:          true,
		TagFiltering:         true,
		TextSearch:           true,
		TimeWindowFiltering:  true,
		AudioMetadataLinkage: true,
	}
}

func (Adapter) NewStoreRuntime(db *sql.DB, deps store.StoreDeps) (*store.StoreRuntime, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres.NewStoreRuntime: db is required")
	}

	logger := loggerOrNop(deps.Logger)

	return newRuntime(db, deps, func(ctx context.Context, fn func(*store.StoreRuntime) error) (err error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return translateError(err)
		}

		txRuntime := newRuntime(tx, deps, func(context.Context, func(*store.StoreRuntime) error) error {
			return store.ErrNestedTransaction
		})

		defer func() {
			if p := recover(); p != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					logger.Error("transaction rollback after panic failed", "err", rbErr)
				}
				panic(p)
			}
		}()

		if err := fn(txRuntime); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return errors.Join(err, rbErr)
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			return translateError(err)
		}

		return nil
	}), nil
}

func (Adapter) NewMigrationTarget(db *sql.DB, deps store.MigrationDeps) (store.MigrationTarget, error) {
	_ = db
	_ = deps
	return nil, fmt.Errorf("postgres.NewMigrationTarget: not implemented")
}
