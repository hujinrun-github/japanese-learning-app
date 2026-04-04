package data

import (
	"database/sql"
	"os"
	"testing"
)

// testDB is set by TestMain and shared across all integration tests in this package.
var testDB *sql.DB

func TestMain(m *testing.M) {
	// Create a temp file for the SQLite database
	f, err := os.CreateTemp("", "japanese_learning_test_*.db")
	if err != nil {
		panic("failed to create temp db file: " + err.Error())
	}
	dbPath := f.Name()
	f.Close()

	// Open the database
	db, err := OpenDB(dbPath)
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}

	// Run migrations (creates all tables + seeds N5/N4 words)
	if err := RunMigrations(db); err != nil {
		panic("failed to run migrations: " + err.Error())
	}

	testDB = db

	// Run tests
	code := m.Run()

	// Cleanup
	db.Close()
	os.Remove(dbPath)

	os.Exit(code)
}
