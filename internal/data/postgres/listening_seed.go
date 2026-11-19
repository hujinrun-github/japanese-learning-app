package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	pgschema "japanese-learning-app/internal/data/migrations/postgres"
)

// SeedListeningQuestions safely supplements lessons imported after migrations.
// The same reviewed SQL pack is used by the migration and the repeatable CLI.
// Its exact-text guard and ON CONFLICT preserve edited sources and saved items.
func SeedListeningQuestions(ctx context.Context, db *sql.DB) (int64, error) {
	content, err := pgschema.SchemaFS.ReadFile("schema/019_listening_content.sql")
	if err != nil {
		return 0, fmt.Errorf("read reviewed listening questions: %w", err)
	}
	result, err := db.ExecContext(ctx, string(content))
	if err != nil {
		slog.Error("seed listening questions failed", "err", err)
		return 0, fmt.Errorf("seed reviewed listening questions: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count seeded listening questions: %w", err)
	}
	slog.Debug("listening questions seeded", "inserted", n)
	return n, nil
}
