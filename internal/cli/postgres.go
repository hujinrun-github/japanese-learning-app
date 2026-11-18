package cli

import (
	"context"
	"database/sql"
	"fmt"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/store"
)

func openDatabase(url string) (*sql.DB, error) {
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(context.Background(), store.DatabaseConfig{DatabaseURL: url, MaxOpenConns: 4, MaxIdleConns: 2})
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL database: %w", err)
	}
	if err := adapter.RunMigrations(context.Background(), db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate PostgreSQL database: %w", err)
	}
	return db, nil
}
