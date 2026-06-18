package data

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestShadowingMigrationRepeatSafe(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shadowing_migration.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB() error: %v", err)
	}
	defer db.Close()

	for i := 1; i <= 2; i++ {
		if err := RunMigrations(db); err != nil {
			t.Fatalf("RunMigrations() iteration %d error: %v", i, err)
		}
	}

	tests := []struct {
		name  string
		check func(t *testing.T, db *sql.DB)
	}{
		{
			name: "lessons shadowing columns exist",
			check: func(t *testing.T, db *sql.DB) {
				columns := []string{
					"shadowing_enabled",
					"video_url",
					"shadowing_version",
					"shadowing_config_json",
				}
				for _, column := range columns {
					if !sqliteColumnExists(t, db, "lessons", column) {
						t.Errorf("lessons column %q does not exist", column)
					}
				}
			},
		},
		{
			name: "shadowing tables exist",
			check: func(t *testing.T, db *sql.DB) {
				tables := []string{
					"lesson_shadowing_progress",
					"lesson_shadowing_attempts",
				}
				for _, table := range tables {
					if !sqliteTableExists(t, db, table) {
						t.Errorf("table %q does not exist", table)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, db)
		})
	}
}

func sqliteColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s) error: %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			t.Fatalf("scan PRAGMA table_info(%s) error: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PRAGMA table_info(%s) error: %v", table, err)
	}

	return false
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var name string
	err := db.QueryRow(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = ?`,
		table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query sqlite_master for table %q error: %v", table, err)
	}

	return true
}
