package postgres

import (
	"context"
	"fmt"
)

func countRows(ctx context.Context, db queryer, query string, args ...any) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres.countRows: %w", translateError(err))
	}
	return count, nil
}
