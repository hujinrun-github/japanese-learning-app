package cli

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"japanese-learning-app/internal/data"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/store"
)

// Run is the entry point for the CLI. It parses os.Args and dispatches to the
// appropriate sub-command.
// Supported sub-commands:
//
//	import-words    --file <path> | --json <json>   Batch/single import words.
//	import-grammar  --file <path> | --json <json>   Batch/single import grammar points.
//	import-lessons  --file <path> | --json <json>   Batch/single import lessons.
//	report-lesson-duplicates --db <path>             Report duplicate lessons.
//	cleanup-lesson-duplicates --db <path> [--apply]  Report or delete duplicate lesson rows.
//	create-lesson-unique-index --db <path>           Create the lesson unique index.
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
	case "import-lessons-postgres":
		return runImportLessonsPostgres(args[1:])
	case "report-lesson-duplicates":
		return runReportLessonDuplicates(args[1:])
	case "cleanup-lesson-duplicates":
		return runCleanupLessonDuplicates(args[1:])
	case "create-lesson-unique-index":
		return runCreateLessonUniqueIndex(args[1:])
	case "import-speaking":
		return runImportSpeaking(args[1:])
	case "import-writing":
		return runImportWriting(args[1:])
	case "import-translation-api":
		return runImportTranslationAPI(args[1:])
	case "generate-word-audio":
		return runGenerateWordAudio(args[1:])
	case "migrate-sqlite-to-pg":
		return runMigrateSQLiteToPG(args[1:])
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
	fmt.Fprintln(os.Stderr, "  import-lessons-postgres --database-url <url> --file <path> | --json <json>  import lessons into PostgreSQL")
	fmt.Fprintln(os.Stderr, "  report-lesson-duplicates --db <path>            report duplicate lessons")
	fmt.Fprintln(os.Stderr, "  cleanup-lesson-duplicates --db <path> [--apply] report duplicates; delete only with --apply")
	fmt.Fprintln(os.Stderr, "  create-lesson-unique-index --db <path>          create lessons(title,jlpt_level) unique index")
	fmt.Fprintln(os.Stderr, "  import-speaking --file <path> | --json <json>  import speaking materials")
	fmt.Fprintln(os.Stderr, "  import-writing            --file <path> | --json <json>  import writing questions")
	fmt.Fprintln(os.Stderr, "  import-translation-api    --file <config.json>           import translation sentences from API")
	fmt.Fprintln(os.Stderr, "  generate-word-audio --db <path> --level <N5-N1>  regenerate word audio via TTS")
	fmt.Fprintln(os.Stderr, "  migrate-sqlite-to-pg --database-url <url> [--sqlite-db <path>] [--audio-dir <dir>]")
	fmt.Fprintln(os.Stderr, "                        [--minio-endpoint <url>] [--dry-run] [--skip-audio] [--phase <name>]")
	fmt.Fprintln(os.Stderr, "                        migrate all data from SQLite to PostgreSQL")
}

func runImportWords(args []string) int {
	fs := flag.NewFlagSet("import-words", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing words to import")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")
	autoFill := fs.Bool("auto-fill", false, "use kagome morphological analyzer to fill missing reading/part_of_speech/reading_type")

	// TTS generation flags
	genAudio := fs.String("generate-audio", "", "auto-generate audio after import: vllm, sbv, gradio")
	ttsURL := fs.String("tts-url", speaking.DefaultVLLMTTSURL(), "vLLM TTS endpoint URL")
	ttsModel := fs.String("tts-model", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice", "TTS model name")
	voice := fs.String("voice", "ono_anna", "TTS voice name")
	instructions := fs.String("instructions", "標準語で、ゆっくり、はっきり発音してください。単語のあとに少し間を空けてください。", "TTS style instructions")
	sbvURL := fs.String("sbv-url", "http://127.0.0.1:7862", "style-bert-vits2 FastAPI server URL")
	sbvModel := fs.String("sbv-model", "amitaro", "style-bert-vits2 model name")
	sbvSpeaker := fs.String("sbv-speaker", "あみたろ", "style-bert-vits2 speaker name")
	sbvStyle := fs.String("sbv-style", "Neutral", "style-bert-vits2 style name")
	outDir := fs.String("out", "./data/audio/words", "output directory for audio files")

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

	n, err := ImportWordsFromFile(db, *filePath, *autoFill)
	if err != nil {
		slog.Error("import-words: ImportWordsFromFile failed", "file", *filePath, "err", err)
		fmt.Fprintf(os.Stderr, "import-words: %v\n", err)
		return 1
	}

	fmt.Printf("import-words: inserted %d word(s) from %s\n", n, *filePath)

	// Auto-generate audio if requested
	if *genAudio != "" {
		cfg := TTSConfig{
			Provider:     *genAudio,
			TTSUrl:       *ttsURL,
			TTSModel:     *ttsModel,
			Voice:        *voice,
			Instructions: *instructions,
			SBVURL:       *sbvURL,
			SBVModel:     *sbvModel,
			SBVSpeaker:   *sbvSpeaker,
			SBVStyle:     *sbvStyle,
		}
		client := NewTTSClient(cfg)
		if _, genErr := GenerateWordAudio(db, client, *outDir, "", false, false); genErr != nil {
			slog.Error("import-words: audio generation failed", "err", genErr)
			fmt.Fprintf(os.Stderr, "import-words: audio generation failed: %v\n", genErr)
		}
	}

	return 0
}

func runImportGrammar(args []string) int {
	fs := flag.NewFlagSet("import-grammar", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing grammar points to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single grammar point")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	// TTS generation flags (shared with import-words)
	genAudio := fs.String("generate-audio", "", "auto-generate audio for example sentences after import: vllm, sbv, gradio")
	ttsURL := fs.String("tts-url", speaking.DefaultVLLMTTSURL(), "vLLM TTS endpoint URL")
	ttsModel := fs.String("tts-model", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice", "TTS model name")
	voice := fs.String("voice", "ono_anna", "TTS voice name")
	instructions := fs.String("instructions", "標準語で、ゆっくり、はっきり発音してください。", "TTS style instructions")
	sbvURL := fs.String("sbv-url", "http://127.0.0.1:7862", "style-bert-vits2 FastAPI server URL")
	sbvModel := fs.String("sbv-model", "amitaro", "style-bert-vits2 model name")
	sbvSpeaker := fs.String("sbv-speaker", "あみたろ", "style-bert-vits2 speaker name")
	sbvStyle := fs.String("sbv-style", "Neutral", "style-bert-vits2 style name")

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

		// Auto-generate example audio
		if *genAudio != "" && *filePath != "" {
			cfg := TTSConfig{
				Provider:     *genAudio,
				TTSUrl:       *ttsURL,
				TTSModel:     *ttsModel,
				Voice:        *voice,
				Instructions: *instructions,
				SBVURL:       *sbvURL,
				SBVModel:     *sbvModel,
				SBVSpeaker:   *sbvSpeaker,
				SBVStyle:     *sbvStyle,
			}
			client := NewTTSClient(cfg)
			raw, _ := os.ReadFile(*filePath)
			sentences, _ := ExtractGrammarSentences(raw)
			if len(sentences) > 0 {
				if genCount, genErr := GenerateTTSFiles(client, "./data/audio/examples", sentences); genErr != nil {
					fmt.Fprintf(os.Stderr, "import-grammar: audio generation failed: %v\n", genErr)
				} else {
					fmt.Printf("import-grammar: generated %d example audio file(s)\n", genCount)
				}
			}
		}
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

func runImportLessonsPostgres(args []string) int {
	fs := flag.NewFlagSet("import-lessons-postgres", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing lessons to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single lesson")
	databaseURL := fs.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL database URL")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-lessons-postgres: %v\n", err)
		return 1
	}
	if *filePath == "" && *jsonStr == "" {
		fmt.Fprintln(os.Stderr, "import-lessons-postgres: --file or --json is required")
		fs.Usage()
		return 1
	}
	if *filePath != "" && *jsonStr != "" {
		fmt.Fprintln(os.Stderr, "import-lessons-postgres: --file and --json are mutually exclusive")
		return 1
	}
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "import-lessons-postgres: --database-url or DATABASE_URL is required")
		return 1
	}

	ctx := context.Background()
	adapter := pgdata.Adapter{}
	db, err := adapter.Open(ctx, store.DatabaseConfig{
		DatabaseURL:  *databaseURL,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		slog.Error("import-lessons-postgres: failed to open database", "err", err)
		fmt.Fprintf(os.Stderr, "import-lessons-postgres: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := adapter.RunMigrations(ctx, db); err != nil {
		slog.Error("import-lessons-postgres: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-lessons-postgres: run migrations: %v\n", err)
		return 1
	}

	var n int
	if *filePath != "" {
		n, err = ImportLessonsToPostgresFromFile(db, *filePath)
		if err != nil {
			slog.Error("import-lessons-postgres: ImportLessonsToPostgresFromFile failed", "file", *filePath, "err", err)
			fmt.Fprintf(os.Stderr, "import-lessons-postgres: %v\n", err)
			return 1
		}
		fmt.Printf("import-lessons-postgres: inserted %d lesson(s) from %s\n", n, *filePath)
	} else {
		n, err = ImportLessonToPostgresFromJSON(db, *jsonStr)
		if err != nil {
			slog.Error("import-lessons-postgres: ImportLessonToPostgresFromJSON failed", "err", err)
			fmt.Fprintf(os.Stderr, "import-lessons-postgres: %v\n", err)
			return 1
		}
		fmt.Printf("import-lessons-postgres: inserted %d lesson(s)\n", n)
	}
	return 0
}

func runReportLessonDuplicates(args []string) int {
	fs := flag.NewFlagSet("report-lesson-duplicates", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "report-lesson-duplicates: %v\n", err)
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("report-lesson-duplicates: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "report-lesson-duplicates: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("report-lesson-duplicates: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "report-lesson-duplicates: run migrations: %v\n", err)
		return 1
	}

	groups, err := ReportLessonDuplicates(db)
	if err != nil {
		slog.Error("report-lesson-duplicates: report failed", "err", err)
		fmt.Fprintf(os.Stderr, "report-lesson-duplicates: %v\n", err)
		return 1
	}
	printLessonDuplicateReport("report-lesson-duplicates", groups)
	return 0
}

func runCleanupLessonDuplicates(args []string) int {
	fs := flag.NewFlagSet("cleanup-lesson-duplicates", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")
	apply := fs.Bool("apply", false, "actually delete duplicate lesson rows after printing the report")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup-lesson-duplicates: %v\n", err)
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("cleanup-lesson-duplicates: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "cleanup-lesson-duplicates: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("cleanup-lesson-duplicates: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "cleanup-lesson-duplicates: run migrations: %v\n", err)
		return 1
	}

	groups, err := ReportLessonDuplicates(db)
	if err != nil {
		slog.Error("cleanup-lesson-duplicates: report failed", "err", err)
		fmt.Fprintf(os.Stderr, "cleanup-lesson-duplicates: %v\n", err)
		return 1
	}
	printLessonDuplicateReport("cleanup-lesson-duplicates", groups)
	if !*apply {
		fmt.Println("cleanup-lesson-duplicates: dry run only; rerun with --apply to delete duplicate lesson rows")
		return 0
	}

	deleted, err := CleanupLessonDuplicates(db)
	if err != nil {
		slog.Error("cleanup-lesson-duplicates: cleanup failed", "err", err)
		fmt.Fprintf(os.Stderr, "cleanup-lesson-duplicates: %v\n", err)
		return 1
	}
	fmt.Printf("cleanup-lesson-duplicates: deleted %d lesson row(s)\n", deleted)
	return 0
}

func runCreateLessonUniqueIndex(args []string) int {
	fs := flag.NewFlagSet("create-lesson-unique-index", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "create-lesson-unique-index: %v\n", err)
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("create-lesson-unique-index: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "create-lesson-unique-index: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("create-lesson-unique-index: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "create-lesson-unique-index: run migrations: %v\n", err)
		return 1
	}

	if err := CreateLessonUniqueIndex(db); err != nil {
		slog.Error("create-lesson-unique-index: create failed", "err", err)
		fmt.Fprintf(os.Stderr, "create-lesson-unique-index: %v\n", err)
		return 1
	}
	fmt.Println("create-lesson-unique-index: unique index ready")
	return 0
}

func printLessonDuplicateReport(prefix string, groups []LessonDuplicateGroup) {
	if len(groups) == 0 {
		fmt.Printf("%s: no duplicate lessons found\n", prefix)
		return
	}
	for _, group := range groups {
		fmt.Printf("%s: title=%q jlpt_level=%s kept_id=%d duplicate_ids=%v\n",
			prefix,
			group.Title,
			group.JLPTLevel,
			group.KeptID,
			group.DuplicateIDs,
		)
	}
}

func runImportSpeaking(args []string) int {
	fs := flag.NewFlagSet("import-speaking", flag.ContinueOnError)
	filePath := fs.String("file", "", "path to the JSON file containing speaking materials to import")
	jsonStr := fs.String("json", "", "inline JSON string for a single speaking material")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	// TTS generation flags (shared with other import commands)
	genAudio := fs.String("generate-audio", "", "auto-generate audio for speaking text after import: vllm, sbv, gradio")
	ttsURL := fs.String("tts-url", speaking.DefaultVLLMTTSURL(), "vLLM TTS endpoint URL")
	ttsModel := fs.String("tts-model", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice", "TTS model name")
	voice := fs.String("voice", "ono_anna", "TTS voice name")
	instructions := fs.String("instructions", "標準語で、ゆっくり、はっきり発音してください。", "TTS style instructions")
	sbvURL := fs.String("sbv-url", "http://127.0.0.1:7862", "style-bert-vits2 FastAPI server URL")
	sbvModel := fs.String("sbv-model", "amitaro", "style-bert-vits2 model name")
	sbvSpeaker := fs.String("sbv-speaker", "あみたろ", "style-bert-vits2 speaker name")
	sbvStyle := fs.String("sbv-style", "Neutral", "style-bert-vits2 style name")

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

		// Auto-generate speaking audio
		if *genAudio != "" && *filePath != "" {
			cfg := TTSConfig{
				Provider:     *genAudio,
				TTSUrl:       *ttsURL,
				TTSModel:     *ttsModel,
				Voice:        *voice,
				Instructions: *instructions,
				SBVURL:       *sbvURL,
				SBVModel:     *sbvModel,
				SBVSpeaker:   *sbvSpeaker,
				SBVStyle:     *sbvStyle,
			}
			client := NewTTSClient(cfg)
			raw, _ := os.ReadFile(*filePath)
			sentences, _ := ExtractSpeakingSentences(raw)
			if len(sentences) > 0 {
				if genCount, genErr := GenerateTTSFiles(client, "./data/audio/examples", sentences); genErr != nil {
					fmt.Fprintf(os.Stderr, "import-speaking: audio generation failed: %v\n", genErr)
				} else {
					fmt.Printf("import-speaking: generated %d speaking audio file(s)\n", genCount)
				}
			}
		}
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

func runImportTranslationAPI(args []string) int {
	fs := flag.NewFlagSet("import-translation-api", flag.ContinueOnError)
	configPath := fs.String("file", "", "path to JSON config file for API import")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-translation-api: %v\n", err)
		return 1
	}
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "import-translation-api: --file is required")
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-translation-api: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-translation-api: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-translation-api: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-translation-api: run migrations: %v\n", err)
		return 1
	}

	n, err := ImportTranslationFromAPIConfig(db, *configPath)
	if err != nil {
		slog.Error("import-translation-api: ImportTranslationFromAPIConfig failed", "config", *configPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-translation-api: %v\n", err)
		return 1
	}

	fmt.Printf("import-translation-api: imported %d sentence(s) from config %s\n", n, *configPath)
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
