package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	pgschema "japanese-learning-app/internal/data/migrations/postgres"
)

// SeedListeningQuestions safely supplements lessons imported after migrations.
// The CLI runs the same reviewed content SQL as the migrations.
func SeedListeningQuestions(ctx context.Context, db *sql.DB) (int64, error) {
	var inserted int64
	err := withQueryerTx(ctx, db, func(tx queryer) error {
		for _, name := range []string{"019_listening_content.sql", "024_listening_my_day.sql"} {
			content, err := pgschema.SchemaFS.ReadFile("schema/" + name)
			if err != nil {
				return fmt.Errorf("read listening questions in %s: %w", name, err)
			}
			result, err := tx.ExecContext(ctx, string(content))
			if err != nil {
				return fmt.Errorf("insert listening questions from %s: %w", name, err)
			}
			n, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("count seeded listening questions from %s: %w", name, err)
			}
			inserted += n
		}
		return nil
	})
	if err != nil {
		slog.Error("seed listening questions failed", "err", err)
		return 0, fmt.Errorf("seed reviewed listening questions: %w", err)
	}
	slog.Debug("listening questions seeded", "inserted", inserted)
	return inserted, nil
}
