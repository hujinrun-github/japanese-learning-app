package postgres

import (
	"context"
	"os"
	"testing"

	"japanese-learning-app/internal/store"
)

func TestAdapterContracts(t *testing.T) {
	var _ store.RelationalAdapter = Adapter{}
	var _ store.MigrationCapableAdapter = Adapter{}
}

func TestRunMigrationsTwice(t *testing.T) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST is required")
	}

	ctx := context.Background()
	db, err := (Adapter{}).Open(ctx, store.DatabaseConfig{
		DatabaseURL:  url,
		MaxOpenConns: 2,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := (Adapter{}).RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := (Adapter{}).RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
}
