package cli

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	pgdata "japanese-learning-app/internal/data/postgres"
)

func runSeedListening(args []string) int {
	fs := flag.NewFlagSet("seed-listening", flag.ContinueOnError)
	databaseURL := fs.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL database URL")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "seed-listening: DATABASE_URL is required")
		return 1
	}
	db, err := openDatabase(*databaseURL)
	if err != nil {
		slog.Error("seed-listening: open database failed", "err", err)
		return 1
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("seed-listening: close database failed", "err", err)
		}
	}()
	n, err := pgdata.SeedListeningQuestions(context.Background(), db)
	if err != nil {
		slog.Error("seed-listening failed", "err", err)
		return 1
	}
	fmt.Printf("seed-listening: inserted %d reviewed question(s); unmatched/previously seeded lessons left unchanged\n", n)
	return 0
}
