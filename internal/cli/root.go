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
//	import-words    --file <path> | --json <json>   Batch/single import words.
//	import-grammar  --file <path> | --json <json>   Batch/single import grammar points.
//	import-lessons  --file <path> | --json <json>   Batch/single import lessons.
//	import-speaking --file <path> | --json <json>   Batch/single import speaking materials.
//	import-writing  --file <path> | --json <json>   Batch/single import writing questions.
func Run(args []string) int {
	if len(args) < 1 {
		printUsage()
		return 1
	}

	switch args[0] {
	case "import-words":
		return runImportWords(args[1:])
	case "import-grammar":
		return runImportGrammar(args[1:])
	case "import-lessons":
		return runImportLessons(args[1:])
	case "import-speaking":
		return runImportSpeaking(args[1:])
	case "import-writing":
		return runImportWriting(args[1:])
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
	fmt.Fprintln(os.Stderr, "  import-words    --file <path> | --json <json>  import words")
	fmt.Fprintln(os.Stderr, "  import-grammar  --file <path> | --json <json>  import grammar points")
	fmt.Fprintln(os.Stderr, "  import-lessons  --file <path> | --json <json>  import lessons")
	fmt.Fprintln(os.Stderr, "  import-speaking --file <path> | --json <json>  import speaking materials")
	fmt.Fprintln(os.Stderr, "  import-writing  --file <path> | --json <json>  import writing questions")
}

func runImportWords(args []string) int {
	fs := flag.NewFlagSet("import-words", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing words to import")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")
	autoFill := fs.Bool("auto-fill", false, "use kagome morphological analyzer to fill missing reading/part_of_speech/reading_type")

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

	n, err := ImportWords(db, *filePath, *autoFill)
	if err != nil {
		slog.Error("import-words: ImportWords failed", "file", *filePath, "err", err)
		fmt.Fprintf(os.Stderr, "import-words: %v\n", err)
		return 1
	}

	fmt.Printf("import-words: inserted %d word(s) from %s\n", n, *filePath)
	return 0
}

func runImportGrammar(args []string) int {
	fs := flag.NewFlagSet("import-grammar", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing grammar points to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single grammar point")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-grammar: %v\n", err)
		return 1
	}
	if *filePath == "" && *jsonStr == "" {
		fmt.Fprintln(os.Stderr, "import-grammar: --file or --json is required")
		fs.Usage()
		return 1
	}
	if *filePath != "" && *jsonStr != "" {
		fmt.Fprintln(os.Stderr, "import-grammar: --file and --json are mutually exclusive")
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-grammar: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-grammar: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-grammar: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-grammar: run migrations: %v\n", err)
		return 1
	}

	var n int
	if *filePath != "" {
		n, err = ImportGrammarFromFile(db, *filePath)
		if err != nil {
			slog.Error("import-grammar: ImportGrammarFromFile failed", "file", *filePath, "err", err)
			fmt.Fprintf(os.Stderr, "import-grammar: %v\n", err)
			return 1
		}
		fmt.Printf("import-grammar: inserted %d grammar point(s) from %s\n", n, *filePath)
	} else {
		n, err = ImportGrammarFromJSON(db, *jsonStr)
		if err != nil {
			slog.Error("import-grammar: ImportGrammarFromJSON failed", "err", err)
			fmt.Fprintf(os.Stderr, "import-grammar: %v\n", err)
			return 1
		}
		fmt.Printf("import-grammar: inserted %d grammar point(s)\n", n)
	}
	return 0
}

func runImportLessons(args []string) int {
	fs := flag.NewFlagSet("import-lessons", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing lessons to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single lesson")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-lessons: %v\n", err)
		return 1
	}
	if *filePath == "" && *jsonStr == "" {
		fmt.Fprintln(os.Stderr, "import-lessons: --file or --json is required")
		fs.Usage()
		return 1
	}
	if *filePath != "" && *jsonStr != "" {
		fmt.Fprintln(os.Stderr, "import-lessons: --file and --json are mutually exclusive")
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-lessons: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-lessons: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-lessons: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-lessons: run migrations: %v\n", err)
		return 1
	}

	var n int
	if *filePath != "" {
		n, err = ImportLessonsFromFile(db, *filePath)
		if err != nil {
			slog.Error("import-lessons: ImportLessonsFromFile failed", "file", *filePath, "err", err)
			fmt.Fprintf(os.Stderr, "import-lessons: %v\n", err)
			return 1
		}
		fmt.Printf("import-lessons: inserted %d lesson(s) from %s\n", n, *filePath)
	} else {
		n, err = ImportLessonFromJSON(db, *jsonStr)
		if err != nil {
			slog.Error("import-lessons: ImportLessonFromJSON failed", "err", err)
			fmt.Fprintf(os.Stderr, "import-lessons: %v\n", err)
			return 1
		}
		fmt.Printf("import-lessons: inserted %d lesson(s)\n", n)
	}
	return 0
}

func runImportSpeaking(args []string) int {
	fs := flag.NewFlagSet("import-speaking", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing speaking materials to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single speaking material")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-speaking: %v\n", err)
		return 1
	}
	if *filePath == "" && *jsonStr == "" {
		fmt.Fprintln(os.Stderr, "import-speaking: --file or --json is required")
		fs.Usage()
		return 1
	}
	if *filePath != "" && *jsonStr != "" {
		fmt.Fprintln(os.Stderr, "import-speaking: --file and --json are mutually exclusive")
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-speaking: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-speaking: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-speaking: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-speaking: run migrations: %v\n", err)
		return 1
	}

	var n int
	if *filePath != "" {
		n, err = ImportSpeakingFromFile(db, *filePath)
		if err != nil {
			slog.Error("import-speaking: ImportSpeakingFromFile failed", "file", *filePath, "err", err)
			fmt.Fprintf(os.Stderr, "import-speaking: %v\n", err)
			return 1
		}
		fmt.Printf("import-speaking: inserted %d speaking material(s) from %s\n", n, *filePath)
	} else {
		n, err = ImportSpeakingFromJSON(db, *jsonStr)
		if err != nil {
			slog.Error("import-speaking: ImportSpeakingFromJSON failed", "err", err)
			fmt.Fprintf(os.Stderr, "import-speaking: %v\n", err)
			return 1
		}
		fmt.Printf("import-speaking: inserted %d speaking material(s)\n", n)
	}
	return 0
}

func runImportWriting(args []string) int {
	fs := flag.NewFlagSet("import-writing", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing writing questions to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single writing question")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-writing: %v\n", err)
		return 1
	}
	if *filePath == "" && *jsonStr == "" {
		fmt.Fprintln(os.Stderr, "import-writing: --file or --json is required")
		fs.Usage()
		return 1
	}
	if *filePath != "" && *jsonStr != "" {
		fmt.Fprintln(os.Stderr, "import-writing: --file and --json are mutually exclusive")
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-writing: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-writing: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-writing: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-writing: run migrations: %v\n", err)
		return 1
	}

	var n int
	if *filePath != "" {
		n, err = ImportWritingFromFile(db, *filePath)
		if err != nil {
			slog.Error("import-writing: ImportWritingFromFile failed", "file", *filePath, "err", err)
			fmt.Fprintf(os.Stderr, "import-writing: %v\n", err)
			return 1
		}
		fmt.Printf("import-writing: inserted %d writing question(s) from %s\n", n, *filePath)
	} else {
		n, err = ImportWritingFromJSON(db, *jsonStr)
		if err != nil {
			slog.Error("import-writing: ImportWritingFromJSON failed", "err", err)
			fmt.Fprintf(os.Stderr, "import-writing: %v\n", err)
			return 1
		}
		fmt.Printf("import-writing: inserted %d writing question(s)\n", n)
	}
	return 0
}
