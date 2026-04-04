package cli

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"japanese-learning-app/internal/data"
)

// Run is the entry point for the CLI. It parses os.Args and dispatches to the
// appropriate sub-command.
// Supported sub-commands:
//
//	import-words --file <path>   Batch-import words from a JSON file.
func Run(args []string) int {
	if len(args) < 1 {
		printUsage()
		return 1
	}

	switch args[0] {
	case "import-words":
		return runImportWords(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: japanese-learning-app <command> [options]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  import-words --file <path>   import words from a JSON file")
}

func runImportWords(args []string) int {
	fs := flag.NewFlagSet("import-words", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing words to import")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-words: %v\n", err)
		return 1
	}
	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "import-words: --file is required")
		fs.Usage()
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-words: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-words: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-words: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-words: run migrations: %v\n", err)
		return 1
	}

	n, err := ImportWords(db, *filePath)
	if err != nil {
		slog.Error("import-words: ImportWords failed", "file", *filePath, "err", err)
		fmt.Fprintf(os.Stderr, "import-words: %v\n", err)
		return 1
	}

	fmt.Printf("import-words: inserted %d word(s) from %s\n", n, *filePath)
	return 0
}
