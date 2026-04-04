package data

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // register sqlite driver
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// OpenDB 打开（或创建）SQLite 数据库，并启用 WAL 模式和外键约束。
// path 可以是磁盘文件路径，也可以是 ":memory:" 用于测试。
func OpenDB(path string) (*sql.DB, error) {
	slog.Debug("opening database", "path", path)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		slog.Error("failed to open database", "err", err, "path", path)
		return nil, fmt.Errorf("data.OpenDB: %w", err)
	}

	// 验证连接可用
	if err := db.Ping(); err != nil {
		slog.Error("failed to ping database", "err", err, "path", path)
		return nil, fmt.Errorf("data.OpenDB ping: %w", err)
	}

	// 启用 WAL 模式（提升并发读性能）
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		slog.Error("failed to set WAL mode", "err", err)
		return nil, fmt.Errorf("data.OpenDB WAL: %w", err)
	}

	// 启用外键约束
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		slog.Error("failed to enable foreign keys", "err", err)
		return nil, fmt.Errorf("data.OpenDB foreign_keys: %w", err)
	}

	slog.Debug("database opened successfully", "path", path)
	return db, nil
}

// RunMigrations 按文件名顺序读取 migrations/ 目录下所有 .sql 文件并执行。
// 每个文件作为一个事务执行，保证原子性。
func RunMigrations(db *sql.DB) error {
	slog.Info("running migrations")

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		slog.Error("failed to read migrations dir", "err", err)
		return fmt.Errorf("data.RunMigrations readdir: %w", err)
	}

	// 按文件名升序排序（001_, 002_, ...）
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		fpath := filepath.Join("migrations", name)
		content, err := migrationFS.ReadFile(fpath)
		if err != nil {
			slog.Error("failed to read migration file", "err", err, "file", name)
			return fmt.Errorf("data.RunMigrations read %s: %w", name, err)
		}

		slog.Debug("applying migration", "file", name)
		if _, err := db.Exec(string(content)); err != nil {
			slog.Error("failed to apply migration", "err", err, "file", name)
			return fmt.Errorf("data.RunMigrations exec %s: %w", name, err)
		}
		slog.Info("migration applied", "file", name)
	}

	slog.Info("all migrations completed", "count", len(names))
	return nil
}
