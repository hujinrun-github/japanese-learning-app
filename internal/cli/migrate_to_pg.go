package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"japanese-learning-app/internal/data"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/store"
)

// ---- CLI command ----

func runMigrateSQLiteToPG(args []string) int {
	fs := flag.NewFlagSet("migrate-sqlite-to-pg", flag.ContinueOnError)
	sqliteDB := fs.String("sqlite-db", "./data/app.db", "path to the SQLite database file")
	databaseURL := fs.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL database URL")
	audioDir := fs.String("audio-dir", "./data/audio", "path to local audio files directory")
	dryRun := fs.Bool("dry-run", false, "validate and count without writing to PostgreSQL")
	skipAudio := fs.Bool("skip-audio", false, "skip audio file migration to MinIO")
	phase := fs.String("phase", "", "migrate only a specific phase (users|content|content-children|userdata|sessions|bookkeeping|audio)")
	batchSize := fs.Int("batch-size", 100, "rows per insert batch")
	skipTables := fs.String("skip-tables", "", "comma-separated table names to skip")
	resetPG := fs.Bool("reset-pg", false, "truncate all PG tables before migration (requires --force)")
	force := fs.Bool("force", false, "confirm dangerous operations")
	minioEndpoint := fs.String("minio-endpoint", os.Getenv("MINIO_ENDPOINT"), "MinIO endpoint")
	minioBucket := fs.String("minio-bucket", defaultBucket(), "MinIO bucket for audio objects")
	minioAccessKey := fs.String("minio-access-key", os.Getenv("MINIO_ACCESS_KEY"), "MinIO access key")
	minioSecretKey := fs.String("minio-secret-key", os.Getenv("MINIO_SECRET_KEY"), "MinIO secret key")
	minioUseSSL := fs.Bool("minio-use-ssl", os.Getenv("MINIO_USE_SSL") == "true", "MinIO SSL")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: %v\n", err)
		return 1
	}
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "migrate-sqlite-to-pg: --database-url or DATABASE_URL is required")
		return 1
	}
	if *resetPG && !*force {
		fmt.Fprintln(os.Stderr, "migrate-sqlite-to-pg: --reset-pg requires --force")
		return 1
	}

	ctx := context.Background()

	// Open SQLite
	sqliteDBConn, err := data.OpenDB(*sqliteDB)
	if err != nil {
		slog.Error("migrate-sqlite-to-pg: failed to open SQLite", "db", *sqliteDB, "err", err)
		fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: open SQLite: %v\n", err)
		return 1
	}
	defer sqliteDBConn.Close()

	if err := data.RunMigrations(sqliteDBConn); err != nil {
		slog.Error("migrate-sqlite-to-pg: failed to run SQLite migrations", "err", err)
		fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: SQLite migrations: %v\n", err)
		return 1
	}

	// Open PostgreSQL
	adapter := pgdata.Adapter{}
	pgDB, err := adapter.Open(ctx, store.DatabaseConfig{
		DatabaseURL:  *databaseURL,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		slog.Error("migrate-sqlite-to-pg: failed to open PostgreSQL", "err", err)
		fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: open PostgreSQL: %v\n", err)
		return 1
	}
	defer pgDB.Close()

	if err := adapter.RunMigrations(ctx, pgDB); err != nil {
		slog.Error("migrate-sqlite-to-pg: failed to run PG migrations", "err", err)
		fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: PG migrations: %v\n", err)
		return 1
	}

	skipSet := parseSkipSet(*skipTables)

	m := &migrator{
		sqliteDB:  sqliteDBConn,
		pgDB:      pgDB,
		dryRun:    *dryRun,
		batchSize: *batchSize,
		skipSet:   skipSet,
	}

	// Reset PG tables if requested
	if *resetPG && *force {
		if err := m.truncateAll(); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: truncate: %v\n", err)
			return 1
		}
		fmt.Println("All PG tables truncated.")
	}

	stats := &migrateStats{}

	// Phase 1: Zero-dependency tables
	if *phase == "" || *phase == "users" {
		if err := m.migrateUsers(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: users phase: %v\n", err)
			return 1
		}
	}

	// Phase 2: Content root tables
	if *phase == "" || *phase == "content" {
		if err := m.migrateWords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content phase (words): %v\n", err)
			return 1
		}
		if err := m.migrateGrammarPoints(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content phase (grammar): %v\n", err)
			return 1
		}
		if err := m.migrateLessons(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content phase (lessons): %v\n", err)
			return 1
		}
		if err := m.migrateSpeakingMaterials(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content phase (speaking): %v\n", err)
			return 1
		}
		if err := m.migrateWritingQuestions(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content phase (writing): %v\n", err)
			return 1
		}
		if err := m.migrateTranslationSources(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content phase (translation_sources): %v\n", err)
			return 1
		}
	}

	// Phase 3: Content children (normalized from JSON)
	if *phase == "" || *phase == "content-children" {
		if err := m.migrateWordExamples(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content-children phase (word_examples): %v\n", err)
			return 1
		}
		if err := m.migrateGrammarExamples(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content-children phase (grammar_examples): %v\n", err)
			return 1
		}
		if err := m.migrateGrammarQuizQuestions(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content-children phase (grammar_quiz_questions): %v\n", err)
			return 1
		}
		if err := m.migrateLessonSentences(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content-children phase (lesson_sentences): %v\n", err)
			return 1
		}
		if err := m.migrateLessonWords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content-children phase (lesson_words): %v\n", err)
			return 1
		}
		if err := m.migrateTranslationSentences(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: content-children phase (translation_sentences): %v\n", err)
			return 1
		}
	}

	// Phase 4: User data tables
	if *phase == "" || *phase == "userdata" {
		if err := m.migrateWordRecords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (word_records): %v\n", err)
			return 1
		}
		if err := m.migrateGrammarRecords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (grammar_records): %v\n", err)
			return 1
		}
		if err := m.migrateSpeakingRecords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (speaking_records): %v\n", err)
			return 1
		}
		if err := m.migrateWritingRecords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (writing_records): %v\n", err)
			return 1
		}
		if err := m.migrateTranslationRecords(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (translation_records): %v\n", err)
			return 1
		}
		if err := m.migrateWordBookmarks(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (word_bookmarks): %v\n", err)
			return 1
		}
		if err := m.migrateNotes(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (notes): %v\n", err)
			return 1
		}
		if err := m.migrateNoteLinks(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: userdata phase (note_links): %v\n", err)
			return 1
		}
	}

	// Phase 5: Session & shadowing data
	if *phase == "" || *phase == "sessions" {
		if err := m.migrateStudySessions(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: sessions phase (study_sessions): %v\n", err)
			return 1
		}
		if err := m.migrateSessionSummaries(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: sessions phase (session_summaries): %v\n", err)
			return 1
		}
		if err := m.migrateLessonShadowingProgress(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: sessions phase (shadowing_progress): %v\n", err)
			return 1
		}
		if err := m.migrateLessonShadowingAttempts(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: sessions phase (shadowing_attempts): %v\n", err)
			return 1
		}
	}

	// Phase 6: Utility tables
	if *phase == "" || *phase == "bookkeeping" {
		if err := m.migratePasswordResetTokens(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: bookkeeping phase (password_reset_tokens): %v\n", err)
			return 1
		}
		if err := m.resetSequences(stats); err != nil {
			fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: reset sequences: %v\n", err)
			return 1
		}
	}

	// Phase 7: Audio migration
	if !*skipAudio && (*phase == "" || *phase == "audio") {
		if *minioEndpoint == "" {
			fmt.Println("migrate-sqlite-to-pg: skipping audio migration (no MinIO endpoint configured)")
		} else {
			audioCfg := audioMigrateConfig{
				AudioDir:      *audioDir,
				MinIOEndpoint: *minioEndpoint,
				MinIOBucket:   *minioBucket,
				AccessKey:     *minioAccessKey,
				SecretKey:     *minioSecretKey,
				UseSSL:        *minioUseSSL,
				DryRun:        *dryRun,
			}
			if err := migrateAudioFiles(ctx, sqliteDBConn, pgDB, audioCfg, stats); err != nil {
				fmt.Fprintf(os.Stderr, "migrate-sqlite-to-pg: audio phase: %v\n", err)
				return 1
			}
		}
	}

	// Print summary
	stats.print()

	// Run verification
	if !*dryRun {
		fmt.Println("\n--- Verification ---")
		v := &verifier{sqliteDB: sqliteDBConn, pgDB: pgDB}
		if errs := v.verifyAll(); len(errs) > 0 {
			fmt.Println("Verification issues found:")
			for _, e := range errs {
				fmt.Printf("  ⚠ %s\n", e)
			}
		} else {
			fmt.Println("  ✓ All verification checks passed")
		}
	}

	if *dryRun {
		fmt.Println("\n[Dry run completed. No changes were made.]")
	}
	return 0
}

func defaultBucket() string {
	if b := os.Getenv("MINIO_BUCKET_AUDIO"); b != "" {
		return b
	}
	return "audio"
}

func parseSkipSet(s string) map[string]bool {
	m := make(map[string]bool)
	if s == "" {
		return m
	}
	for _, t := range strings.Split(s, ",") {
		m[strings.TrimSpace(t)] = true
	}
	return m
}

// ---- migrator ----

type migrator struct {
	sqliteDB          *sql.DB
	pgDB              *sql.DB
	dryRun            bool
	batchSize         int
	skipSet           map[string]bool
	sourceUserIDs     map[int64]bool
	grammarPointIDMap map[int64]int64
	lessonIDMap       map[int64]int64
}

type migrateStats struct {
	Phases []phaseStats
	Errors []string
	Skips  []string
	Audio  *audioStats
}

type phaseStats struct {
	Table string
	Count int64
}

type audioStats struct {
	FilesScanned int
	UploadedOK   int
	UploadedFail int
	FKsUpdated   int
}

func (s *migrateStats) add(table string, count int64) {
	s.Phases = append(s.Phases, phaseStats{Table: table, Count: count})
}

func (s *migrateStats) addError(msg string) {
	s.Errors = append(s.Errors, msg)
}

func (s *migrateStats) addSkip(table string) {
	s.Skips = append(s.Skips, table)
}

func (s *migrateStats) print() {
	fmt.Println("\n--- Migration Summary ---")
	for _, p := range s.Phases {
		fmt.Printf("  [%s] %d rows\n", p.Table, p.Count)
	}
	if len(s.Skips) > 0 {
		fmt.Printf("\nSkipped tables: %s\n", strings.Join(s.Skips, ", "))
	}
	if s.Audio != nil {
		fmt.Printf("\nAudio: scanned=%d uploaded=%d failed=%d fks_updated=%d\n",
			s.Audio.FilesScanned, s.Audio.UploadedOK, s.Audio.UploadedFail, s.Audio.FKsUpdated)
	}
	if len(s.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(s.Errors))
		for _, e := range s.Errors {
			fmt.Printf("  ⚠ %s\n", e)
		}
	}
}

func (m *migrator) skip(table string) bool {
	return m.skipSet[table]
}

func (m *migrator) rememberUserID(id int64) {
	if m.sourceUserIDs == nil {
		m.sourceUserIDs = make(map[int64]bool)
	}
	m.sourceUserIDs[id] = true
}

func (m *migrator) hasSourceUser(id int64) bool {
	if m.sourceUserIDs == nil {
		return true
	}
	return m.sourceUserIDs[id]
}

func (m *migrator) skipRowWithMissingSourceUser(stats *migrateStats, table string, rowID any, userID int64) bool {
	if m.hasSourceUser(userID) {
		return false
	}
	stats.addError(fmt.Sprintf("%s: skipped row id=%v with missing source user_id=%d", table, rowID, userID))
	return true
}

func (m *migrator) rememberGrammarPointID(sourceID, targetID int64) {
	if m.grammarPointIDMap == nil {
		m.grammarPointIDMap = make(map[int64]int64)
	}
	m.grammarPointIDMap[sourceID] = targetID
}

func (m *migrator) mapGrammarPointID(sourceID int64) int64 {
	if m.grammarPointIDMap == nil {
		return sourceID
	}
	if targetID, ok := m.grammarPointIDMap[sourceID]; ok {
		return targetID
	}
	return sourceID
}

func (m *migrator) rememberLessonID(sourceID, targetID int64) {
	if m.lessonIDMap == nil {
		m.lessonIDMap = make(map[int64]int64)
	}
	m.lessonIDMap[sourceID] = targetID
}

func (m *migrator) mapLessonID(sourceID int64) int64 {
	if m.lessonIDMap == nil {
		return sourceID
	}
	if targetID, ok := m.lessonIDMap[sourceID]; ok {
		return targetID
	}
	return sourceID
}

func sqliteColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			pk           int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// execPG runs a query on PostgreSQL. In dry-run mode it only logs.
func (m *migrator) execPG(query string, args ...any) (sql.Result, error) {
	if m.dryRun {
		slog.Debug("dry-run: would execute", "query", query[:min(len(query), 100)])
		return nil, nil
	}
	return m.pgDB.Exec(query, args...)
}

func (m *migrator) queryPG(query string, args ...any) (*sql.Rows, error) {
	return m.pgDB.Query(query, args...)
}

func (m *migrator) queryPGContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.pgDB.QueryContext(ctx, query, args...)
}

// ---- truncate ----

func (m *migrator) truncateAll() error {
	tables := []string{
		"password_reset_tokens", "lesson_shadowing_attempts", "lesson_shadowing_progress",
		"session_summaries", "study_sessions",
		"note_links", "note_review_events", "notes",
		"word_bookmarks", "word_review_events", "word_records",
		"grammar_quiz_attempts", "grammar_records",
		"speaking_records", "writing_records", "translation_records",
		"lesson_words", "lesson_sentences",
		"word_examples", "grammar_examples", "grammar_quiz_questions",
		"translation_sentences", "translation_sources",
		"writing_questions", "speaking_materials", "lessons",
		"grammar_points", "words", "users",
		"audio_objects", "video_objects",
	}
	for _, t := range tables {
		if _, err := m.pgDB.Exec("DELETE FROM " + t); err != nil {
			// Ignore "relation does not exist" errors for optional tables
			if strings.Contains(err.Error(), "does not exist") {
				continue
			}
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}
	return nil
}

// ---- Phase 1: users ----

func (m *migrator) migrateUsers(stats *migrateStats) error {
	if m.skip("users") {
		stats.addSkip("users")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, email, name, password_hash, goal_level, jlpt_levels, streak_days, daily_goals_json, created_at FROM users`)
	if err != nil {
		return fmt.Errorf("query SQLite users: %w", err)
	}
	defer rows.Close()

	type userRow struct {
		id             int64
		email          string
		name           string
		passwordHash   string
		goalLevel      string
		jlptLevels     string
		streakDays     int
		dailyGoalsJSON string
		createdAt      string
	}

	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.id, &u.email, &u.name, &u.passwordHash, &u.goalLevel, &u.jlptLevels, &u.streakDays, &u.dailyGoalsJSON, &u.createdAt); err != nil {
			return fmt.Errorf("scan SQLite user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows users: %w", err)
	}

	m.sourceUserIDs = make(map[int64]bool, len(users))
	if len(users) == 0 {
		stats.add("users", 0)
		return nil
	}

	// jlpt_levels: TEXT JSON array (e.g. '["N5","N4"]') -> PostgreSQL TEXT[] literal
	// daily_goals_json: TEXT JSON -> JSONB
	// created_at: SQLite DATETIME -> TIMESTAMPTZ

	for _, u := range users {
		parsedTime, err := parseTimestamp(u.createdAt)
		if err != nil {
			slog.Warn("migrateUsers: parsing created_at, using now", "id", u.id, "raw", u.createdAt, "err", err)
			parsedTime = time.Now()
		}
		goalLevel, jlptLevels := normalizeMigrationUserLevels(u.goalLevel, u.jlptLevels)
		jlptLevelsJSON, err := json.Marshal(jlptLevels)
		if err != nil {
			return fmt.Errorf("marshal normalized user jlpt_levels %d: %w", u.id, err)
		}
		jlptArr := parseJSONStringArray(string(jlptLevelsJSON))
		if _, err := m.execPG(
			`INSERT INTO users (id, email, name, password_hash, goal_level, jlpt_levels, streak_days, daily_goals_json, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO UPDATE SET email=EXCLUDED.email, name=EXCLUDED.name, jlpt_levels=EXCLUDED.jlpt_levels`,
			u.id, u.email, u.name, u.passwordHash, goalLevel, jlptArr, u.streakDays, u.dailyGoalsJSON, parsedTime,
		); err != nil {
			return fmt.Errorf("insert user %d: %w", u.id, err)
		}
		m.rememberUserID(u.id)
	}

	fmt.Printf("[1/26] users: %d rows migrated\n", len(users))
	stats.add("users", int64(len(users)))
	return nil
}

// ---- Phase 2: words ----

func (m *migrator) migrateWords(stats *migrateStats) error {
	if m.skip("words") {
		stats.addSkip("words")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level, reading_type, audio_url, updated_at FROM words`)
	if err != nil {
		return fmt.Errorf("query SQLite words: %w", err)
	}
	defer rows.Close()

	type wordRow struct {
		id           int64
		kanjiForm    string
		reading      string
		partOfSpeech string
		meaning      string
		examplesJSON string
		jlptLevel    string
		readingType  string
		audioURL     string
		updatedAt    sql.NullString
	}

	var words []wordRow
	for rows.Next() {
		var w wordRow
		if err := rows.Scan(&w.id, &w.kanjiForm, &w.reading, &w.partOfSpeech, &w.meaning, &w.examplesJSON, &w.jlptLevel, &w.readingType, &w.audioURL, &w.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite word: %w", err)
		}
		words = append(words, w)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows words: %w", err)
	}

	for _, w := range words {
		updatedAt := time.Now()
		if w.updatedAt.Valid && w.updatedAt.String != "" {
			if t, err := parseTimestamp(w.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		if _, err := m.execPG(
			`INSERT INTO words (id, kanji_form, reading, part_of_speech, meaning, jlpt_level, reading_type, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (id) DO UPDATE SET kanji_form=EXCLUDED.kanji_form, reading=EXCLUDED.reading`,
			w.id, w.kanjiForm, w.reading, w.partOfSpeech, w.meaning, normalizeMigrationJLPTLevel(w.jlptLevel), w.readingType, updatedAt,
		); err != nil {
			return fmt.Errorf("insert word %d: %w", w.id, err)
		}
	}

	fmt.Printf("[2/26] words: %d rows migrated\n", len(words))
	stats.add("words", int64(len(words)))
	return nil
}

// ---- Phase 2: grammar_points ----

func (m *migrator) migrateGrammarPoints(stats *migrateStats) error {
	if m.skip("grammar_points") {
		stats.addSkip("grammar_points")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level, updated_at FROM grammar_points`)
	if err != nil {
		return fmt.Errorf("query SQLite grammar_points: %w", err)
	}
	defer rows.Close()

	type gpRow struct {
		id                int64
		name              string
		meaning           string
		conjunctionRule   string
		usageNote         string
		examplesJSON      string
		quizQuestionsJSON string
		jlptLevel         string
		updatedAt         sql.NullString
	}

	var points []gpRow
	for rows.Next() {
		var g gpRow
		if err := rows.Scan(&g.id, &g.name, &g.meaning, &g.conjunctionRule, &g.usageNote, &g.examplesJSON, &g.quizQuestionsJSON, &g.jlptLevel, &g.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite grammar_point: %w", err)
		}
		points = append(points, g)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows grammar_points: %w", err)
	}

	for _, g := range points {
		updatedAt := time.Now()
		if g.updatedAt.Valid && g.updatedAt.String != "" {
			if t, err := parseTimestamp(g.updatedAt.String); err == nil {
				updatedAt = t
			}
		}

		jlptLevel := normalizeMigrationJLPTLevel(g.jlptLevel)
		if m.dryRun {
			m.rememberGrammarPointID(g.id, g.id)
			continue
		}

		var targetID int64
		if err := m.pgDB.QueryRow(
			`INSERT INTO grammar_points (id, name, meaning, conjunction_rule, usage_note, jlpt_level, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (name, jlpt_level) DO UPDATE SET
			     meaning = EXCLUDED.meaning,
			     conjunction_rule = EXCLUDED.conjunction_rule,
			     usage_note = EXCLUDED.usage_note,
			     updated_at = EXCLUDED.updated_at
			 RETURNING id`,
			g.id, g.name, g.meaning, g.conjunctionRule, g.usageNote, jlptLevel, updatedAt,
		).Scan(&targetID); err != nil {
			return fmt.Errorf("insert grammar_point %d: %w", g.id, err)
		}
		m.rememberGrammarPointID(g.id, targetID)
	}

	fmt.Printf("[3/26] grammar_points: %d rows migrated\n", len(points))
	stats.add("grammar_points", int64(len(points)))
	return nil
}

// ---- Phase 2: lessons ----

func (m *migrator) migrateLessons(stats *migrateStats) error {
	if m.skip("lessons") {
		stats.addSkip("lessons")
		return nil
	}

	updatedAtExpr := "NULL AS updated_at"
	hasUpdatedAt, err := sqliteColumnExists(m.sqliteDB, "lessons", "updated_at")
	if err != nil {
		return fmt.Errorf("inspect SQLite lessons columns: %w", err)
	}
	if hasUpdatedAt {
		updatedAtExpr = "updated_at"
	}

	rows, err := m.sqliteDB.Query(fmt.Sprintf(`SELECT id, title, jlpt_level, tags_json, audio_url, char_count,
		shadowing_enabled, video_url, shadowing_version, shadowing_config_json,
		word_ids_json, content_furigana_json, %s
		FROM lessons`, updatedAtExpr))
	if err != nil {
		return fmt.Errorf("query SQLite lessons: %w", err)
	}
	defer rows.Close()

	type lessonRow struct {
		id                  int64
		title               string
		jlptLevel           string
		tagsJSON            string
		audioURL            string
		charCount           int
		shadowingEnabled    int
		videoURL            string
		shadowingVersion    int
		shadowingConfigJSON string
		wordIDsJSON         string
		contentFuriganaJSON string
		updatedAt           sql.NullString
	}

	var lessons []lessonRow
	for rows.Next() {
		var l lessonRow
		if err := rows.Scan(&l.id, &l.title, &l.jlptLevel, &l.tagsJSON, &l.audioURL, &l.charCount,
			&l.shadowingEnabled, &l.videoURL, &l.shadowingVersion, &l.shadowingConfigJSON,
			&l.wordIDsJSON, &l.contentFuriganaJSON, &l.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite lesson: %w", err)
		}
		lessons = append(lessons, l)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows lessons: %w", err)
	}

	for _, l := range lessons {
		updatedAt := time.Now()
		if l.updatedAt.Valid && l.updatedAt.String != "" {
			if t, err := parseTimestamp(l.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		// tags_json: JSON array -> TEXT[]
		tagsArr := parseJSONStringArray(l.tagsJSON)
		// shadowing_enabled: INTEGER 0/1 -> BOOLEAN
		shadowEnabled := l.shadowingEnabled != 0
		// shadowing_version: fallback to 1
		shadowVer := l.shadowingVersion
		if shadowVer < 1 {
			shadowVer = 1
		}
		shadowConfig := l.shadowingConfigJSON
		if shadowConfig == "" {
			shadowConfig = "{}"
		}

		jlptLevel := normalizeMigrationJLPTLevel(l.jlptLevel)
		if m.dryRun {
			m.rememberLessonID(l.id, l.id)
			continue
		}

		var targetID int64
		if err := m.pgDB.QueryRow(
			`INSERT INTO lessons (id, title, jlpt_level, tags, char_count,
			 shadowing_enabled, shadowing_version, shadowing_config_json, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (title, jlpt_level) DO UPDATE SET
			     tags = EXCLUDED.tags,
			     char_count = EXCLUDED.char_count,
			     shadowing_enabled = EXCLUDED.shadowing_enabled,
			     shadowing_version = EXCLUDED.shadowing_version,
			     shadowing_config_json = EXCLUDED.shadowing_config_json,
			     updated_at = EXCLUDED.updated_at
			 RETURNING id`,
			l.id, l.title, jlptLevel, tagsArr, l.charCount,
			shadowEnabled, shadowVer, shadowConfig, updatedAt,
		).Scan(&targetID); err != nil {
			return fmt.Errorf("insert lesson %d: %w", l.id, err)
		}
		m.rememberLessonID(l.id, targetID)
	}

	fmt.Printf("[4/26] lessons: %d rows migrated\n", len(lessons))
	stats.add("lessons", int64(len(lessons)))
	return nil
}

// ---- Phase 2: speaking_materials ----

func (m *migrator) migrateSpeakingMaterials(stats *migrateStats) error {
	if m.skip("speaking_materials") {
		stats.addSkip("speaking_materials")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, type, title, text, lines, audio_url, jlpt_level, updated_at FROM speaking_materials`)
	if err != nil {
		return fmt.Errorf("query SQLite speaking_materials: %w", err)
	}
	defer rows.Close()

	type smRow struct {
		id        int64
		smType    string
		title     string
		text      string
		lines     sql.NullString
		audioURL  string
		jlptLevel string
		updatedAt sql.NullString
	}

	var materials []smRow
	for rows.Next() {
		var s smRow
		if err := rows.Scan(&s.id, &s.smType, &s.title, &s.text, &s.lines, &s.audioURL, &s.jlptLevel, &s.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite speaking_material: %w", err)
		}
		materials = append(materials, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows speaking_materials: %w", err)
	}

	for _, s := range materials {
		updatedAt := time.Now()
		if s.updatedAt.Valid && s.updatedAt.String != "" {
			if t, err := parseTimestamp(s.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		// lines: TEXT (JSON array or empty string) -> TEXT[]
		lineArr := parseJSONStringArrayOrEmpty(s.lines.String)

		if _, err := m.execPG(
			`INSERT INTO speaking_materials (id, type, title, text, lines, audio_url_legacy, jlpt_level, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, jlpt_level=EXCLUDED.jlpt_level`,
			s.id, s.smType, s.title, s.text, lineArr, s.audioURL, normalizeMigrationJLPTLevel(s.jlptLevel), updatedAt,
		); err != nil {
			return fmt.Errorf("insert speaking_material %d: %w", s.id, err)
		}
	}

	fmt.Printf("[5/26] speaking_materials: %d rows migrated\n", len(materials))
	stats.add("speaking_materials", int64(len(materials)))
	return nil
}

// ---- Phase 2: writing_questions ----

func (m *migrator) migrateWritingQuestions(stats *migrateStats) error {
	if m.skip("writing_questions") {
		stats.addSkip("writing_questions")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, type, prompt, expected_answer, grammar_point_id, jlpt_level, updated_at FROM writing_questions`)
	if err != nil {
		return fmt.Errorf("query SQLite writing_questions: %w", err)
	}
	defer rows.Close()

	type wqRow struct {
		id             int64
		qType          string
		prompt         string
		expectedAnswer string
		grammarPointID int64
		jlptLevel      string
		updatedAt      sql.NullString
	}

	var questions []wqRow
	for rows.Next() {
		var q wqRow
		if err := rows.Scan(&q.id, &q.qType, &q.prompt, &q.expectedAnswer, &q.grammarPointID, &q.jlptLevel, &q.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite writing_question: %w", err)
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows writing_questions: %w", err)
	}

	for _, q := range questions {
		updatedAt := time.Now()
		if q.updatedAt.Valid && q.updatedAt.String != "" {
			if t, err := parseTimestamp(q.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		// grammar_point_id: 0 -> NULL
		var gpID *int64
		if q.grammarPointID != 0 {
			mappedID := m.mapGrammarPointID(q.grammarPointID)
			gpID = &mappedID
		}

		if _, err := m.execPG(
			`INSERT INTO writing_questions (id, type, prompt, expected_answer, grammar_point_id, jlpt_level, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET prompt=EXCLUDED.prompt, jlpt_level=EXCLUDED.jlpt_level`,
			q.id, q.qType, q.prompt, q.expectedAnswer, gpID, normalizeMigrationJLPTLevel(q.jlptLevel), updatedAt,
		); err != nil {
			return fmt.Errorf("insert writing_question %d: %w", q.id, err)
		}
	}

	fmt.Printf("[6/26] writing_questions: %d rows migrated\n", len(questions))
	stats.add("writing_questions", int64(len(questions)))
	return nil
}

// ---- Phase 2: translation_sources ----

func (m *migrator) migrateTranslationSources(stats *migrateStats) error {
	if m.skip("translation_sources") {
		stats.addSkip("translation_sources")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, title, source_type, source_url, api_endpoint, raw_content, created_at FROM translation_sources`)
	if err != nil {
		return fmt.Errorf("query SQLite translation_sources: %w", err)
	}
	defer rows.Close()

	type tsRow struct {
		id          int64
		title       string
		sourceType  string
		sourceURL   string
		apiEndpoint string
		rawContent  string
		createdAt   string
	}

	var sources []tsRow
	for rows.Next() {
		var s tsRow
		if err := rows.Scan(&s.id, &s.title, &s.sourceType, &s.sourceURL, &s.apiEndpoint, &s.rawContent, &s.createdAt); err != nil {
			return fmt.Errorf("scan SQLite translation_source: %w", err)
		}
		sources = append(sources, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows translation_sources: %w", err)
	}

	for _, s := range sources {
		createdAt, err := parseTimestamp(s.createdAt)
		if err != nil {
			createdAt = time.Now()
		}
		if _, err := m.execPG(
			`INSERT INTO translation_sources (id, title, source_type, source_url, api_endpoint, raw_content, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title`,
			s.id, s.title, s.sourceType, s.sourceURL, s.apiEndpoint, s.rawContent, createdAt,
		); err != nil {
			return fmt.Errorf("insert translation_source %d: %w", s.id, err)
		}
	}

	fmt.Printf("[7/26] translation_sources: %d rows migrated\n", len(sources))
	stats.add("translation_sources", int64(len(sources)))
	return nil
}

// ---- Phase 3: word_examples ----

func (m *migrator) migrateWordExamples(stats *migrateStats) error {
	if m.skip("word_examples") {
		stats.addSkip("word_examples")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, examples_json FROM words WHERE examples_json != '' AND examples_json != '[]'`)
	if err != nil {
		return fmt.Errorf("query SQLite word_examples: %w", err)
	}
	defer rows.Close()

	type wordExampleJSON struct {
		Japanese     string `json:"japanese"`
		Chinese      string `json:"chinese"`
		FuriganaHTML string `json:"furigana_html"`
	}

	var total int
	for rows.Next() {
		var wordID int64
		var examplesJSON string
		if err := rows.Scan(&wordID, &examplesJSON); err != nil {
			return fmt.Errorf("scan word examples: %w", err)
		}

		var examples []wordExampleJSON
		if err := json.Unmarshal([]byte(examplesJSON), &examples); err != nil {
			slog.Warn("migrateWordExamples: skipping malformed JSON", "word_id", wordID, "err", err)
			stats.addError(fmt.Sprintf("word_examples: word_id=%d bad JSON: %v", wordID, err))
			continue
		}

		for i, ex := range examples {
			if _, err := m.execPG(
				`INSERT INTO word_examples (word_id, position, japanese, chinese, furigana_html)
				 VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (word_id, position) DO NOTHING`,
				wordID, i, ex.Japanese, ex.Chinese, ex.FuriganaHTML,
			); err != nil {
				return fmt.Errorf("insert word_example (word_id=%d pos=%d): %w", wordID, i, err)
			}
			total++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows word_examples: %w", err)
	}

	fmt.Printf("[8/26] word_examples: %d rows migrated\n", total)
	stats.add("word_examples", int64(total))
	return nil
}

// ---- Phase 3: grammar_examples ----

func grammarExampleInsertSQL() string {
	return `INSERT INTO grammar_examples (grammar_point_id, position, japanese, chinese, furigana_html, linked_word_ids)
		VALUES ($1, $2, $3, $4, $5, $6::bigint[])
		ON CONFLICT (grammar_point_id, position) DO NOTHING`
}

func (m *migrator) migrateGrammarExamples(stats *migrateStats) error {
	if m.skip("grammar_examples") {
		stats.addSkip("grammar_examples")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, examples_json FROM grammar_points WHERE examples_json != '' AND examples_json != '[]'`)
	if err != nil {
		return fmt.Errorf("query SQLite grammar_examples: %w", err)
	}
	defer rows.Close()

	type grammarExampleJSON struct {
		Japanese      string  `json:"japanese"`
		Chinese       string  `json:"chinese"`
		FuriganaHTML  string  `json:"furigana_html"`
		LinkedWordIDs []int64 `json:"linked_word_ids"`
	}

	var total int
	for rows.Next() {
		var gpID int64
		var examplesJSON string
		if err := rows.Scan(&gpID, &examplesJSON); err != nil {
			return fmt.Errorf("scan grammar examples: %w", err)
		}

		var examples []grammarExampleJSON
		if err := json.Unmarshal([]byte(examplesJSON), &examples); err != nil {
			slog.Warn("migrateGrammarExamples: skipping malformed JSON", "grammar_point_id", gpID, "err", err)
			stats.addError(fmt.Sprintf("grammar_examples: grammar_point_id=%d bad JSON: %v", gpID, err))
			continue
		}

		for i, ex := range examples {
			targetGPID := m.mapGrammarPointID(gpID)
			linked := "{}"
			if len(ex.LinkedWordIDs) > 0 {
				linked = int64ArrayToPGLiteral(ex.LinkedWordIDs)
			}
			if _, err := m.execPG(
				grammarExampleInsertSQL(),
				targetGPID, i, ex.Japanese, ex.Chinese, ex.FuriganaHTML, linked,
			); err != nil {
				return fmt.Errorf("insert grammar_example (gp=%d pos=%d): %w", gpID, i, err)
			}
			total++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows grammar_examples: %w", err)
	}

	fmt.Printf("[9/26] grammar_examples: %d rows migrated\n", total)
	stats.add("grammar_examples", int64(total))
	return nil
}

// ---- Phase 3: grammar_quiz_questions ----

func (m *migrator) migrateGrammarQuizQuestions(stats *migrateStats) error {
	if m.skip("grammar_quiz_questions") {
		stats.addSkip("grammar_quiz_questions")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, quiz_questions_json FROM grammar_points WHERE quiz_questions_json != '' AND quiz_questions_json != '[]'`)
	if err != nil {
		return fmt.Errorf("query SQLite grammar_quiz_questions: %w", err)
	}
	defer rows.Close()

	type quizQJSON struct {
		Type        string   `json:"type"`
		Prompt      string   `json:"prompt"`
		Options     []string `json:"options"`
		Answer      string   `json:"answer"`
		Explanation string   `json:"explanation"`
	}

	var total int
	for rows.Next() {
		var gpID int64
		var quizJSON string
		if err := rows.Scan(&gpID, &quizJSON); err != nil {
			return fmt.Errorf("scan grammar quiz: %w", err)
		}

		var questions []quizQJSON
		if err := json.Unmarshal([]byte(quizJSON), &questions); err != nil {
			slog.Warn("migrateGrammarQuizQuestions: skipping malformed JSON", "grammar_point_id", gpID, "err", err)
			stats.addError(fmt.Sprintf("grammar_quiz_questions: gp=%d bad JSON: %v", gpID, err))
			continue
		}

		for i, q := range questions {
			targetGPID := m.mapGrammarPointID(gpID)
			opts, _ := json.Marshal(q.Options)
			if _, err := m.execPG(
				`INSERT INTO grammar_quiz_questions (grammar_point_id, position, type, prompt, options, answer, explanation)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)
				 ON CONFLICT (grammar_point_id, position) DO NOTHING`,
				targetGPID, i, q.Type, q.Prompt, string(opts), q.Answer, q.Explanation,
			); err != nil {
				return fmt.Errorf("insert grammar_quiz_question (gp=%d pos=%d): %w", gpID, i, err)
			}
			total++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows grammar_quiz_questions: %w", err)
	}

	fmt.Printf("[10/26] grammar_quiz_questions: %d rows migrated\n", total)
	stats.add("grammar_quiz_questions", int64(total))
	return nil
}

// ---- Phase 3: lesson_sentences ----

func (m *migrator) migrateLessonSentences(stats *migrateStats) error {
	if m.skip("lesson_sentences") {
		stats.addSkip("lesson_sentences")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, content_furigana_json FROM lessons WHERE content_furigana_json != '' AND content_furigana_json != '[]'`)
	if err != nil {
		return fmt.Errorf("query SQLite lesson_sentences: %w", err)
	}
	defer rows.Close()

	type sentenceJSON struct {
		Index   int             `json:"index"`
		Tokens  json.RawMessage `json:"tokens"`
		Chinese string          `json:"chinese"`
		StartMS int64           `json:"start_ms"`
		EndMS   int64           `json:"end_ms"`
	}

	var total int
	for rows.Next() {
		var lessonID int64
		var furiganaJSON string
		if err := rows.Scan(&lessonID, &furiganaJSON); err != nil {
			return fmt.Errorf("scan lesson sentences: %w", err)
		}

		var sentences []sentenceJSON
		if err := json.Unmarshal([]byte(furiganaJSON), &sentences); err != nil {
			slog.Warn("migrateLessonSentences: skipping malformed JSON", "lesson_id", lessonID, "err", err)
			stats.addError(fmt.Sprintf("lesson_sentences: lesson_id=%d bad JSON: %v", lessonID, err))
			continue
		}

		for _, s := range sentences {
			targetLessonID := m.mapLessonID(lessonID)
			// Tokens may be an empty array, keep as JSONB
			tokensStr := string(s.Tokens)
			if tokensStr == "" || tokensStr == "null" {
				tokensStr = "[]"
			}
			if _, err := m.execPG(
				`INSERT INTO lesson_sentences (lesson_id, position, tokens, chinese, start_ms, end_ms)
				 VALUES ($1, $2, $3, $4, $5, $6)
				 ON CONFLICT (lesson_id, position) DO NOTHING`,
				targetLessonID, s.Index, tokensStr, s.Chinese, s.StartMS, s.EndMS,
			); err != nil {
				return fmt.Errorf("insert lesson_sentence (lesson=%d pos=%d): %w", lessonID, s.Index, err)
			}
			total++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows lesson_sentences: %w", err)
	}

	fmt.Printf("[11/26] lesson_sentences: %d rows migrated\n", total)
	stats.add("lesson_sentences", int64(total))
	return nil
}

// ---- Phase 3: lesson_words ----

func (m *migrator) migrateLessonWords(stats *migrateStats) error {
	if m.skip("lesson_words") {
		stats.addSkip("lesson_words")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, word_ids_json FROM lessons WHERE word_ids_json != '' AND word_ids_json != '[]'`)
	if err != nil {
		return fmt.Errorf("query SQLite lesson_words: %w", err)
	}
	defer rows.Close()

	var total int
	for rows.Next() {
		var lessonID int64
		var wordIDsJSON string
		if err := rows.Scan(&lessonID, &wordIDsJSON); err != nil {
			return fmt.Errorf("scan lesson words: %w", err)
		}

		var wordIDs []int64
		if err := json.Unmarshal([]byte(wordIDsJSON), &wordIDs); err != nil {
			slog.Warn("migrateLessonWords: skipping malformed JSON", "lesson_id", lessonID, "err", err)
			stats.addError(fmt.Sprintf("lesson_words: lesson_id=%d bad JSON: %v", lessonID, err))
			continue
		}

		for i, wid := range wordIDs {
			targetLessonID := m.mapLessonID(lessonID)
			if _, err := m.execPG(
				`INSERT INTO lesson_words (lesson_id, word_id, position)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (lesson_id, position) DO NOTHING`,
				targetLessonID, wid, i,
			); err != nil {
				return fmt.Errorf("insert lesson_word (lesson=%d pos=%d): %w", lessonID, i, err)
			}
			total++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows lesson_words: %w", err)
	}

	fmt.Printf("[12/26] lesson_words: %d rows migrated\n", total)
	stats.add("lesson_words", int64(total))
	return nil
}

// ---- Phase 3: translation_sentences ----

func (m *migrator) migrateTranslationSentences(stats *migrateStats) error {
	if m.skip("translation_sentences") {
		stats.addSkip("translation_sentences")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, source_id, direction, source_text, reference_translation, position, updated_at FROM translation_sentences`)
	if err != nil {
		return fmt.Errorf("query SQLite translation_sentences: %w", err)
	}
	defer rows.Close()

	type tsRow struct {
		id         int64
		sourceID   int64
		direction  string
		sourceText string
		refTrans   string
		position   int
		updatedAt  sql.NullString
	}

	var sentences []tsRow
	for rows.Next() {
		var s tsRow
		if err := rows.Scan(&s.id, &s.sourceID, &s.direction, &s.sourceText, &s.refTrans, &s.position, &s.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite translation_sentence: %w", err)
		}
		sentences = append(sentences, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows translation_sentences: %w", err)
	}

	for _, s := range sentences {
		updatedAt := time.Now()
		if s.updatedAt.Valid && s.updatedAt.String != "" {
			if t, err := parseTimestamp(s.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		if _, err := m.execPG(
			`INSERT INTO translation_sentences (id, source_id, direction, source_text, reference_translation, position, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET source_text=EXCLUDED.source_text, position=EXCLUDED.position`,
			s.id, s.sourceID, s.direction, s.sourceText, s.refTrans, s.position, updatedAt,
		); err != nil {
			return fmt.Errorf("insert translation_sentence %d: %w", s.id, err)
		}
	}

	fmt.Printf("[13/26] translation_sentences: %d rows migrated\n", len(sentences))
	stats.add("translation_sentences", int64(len(sentences)))
	return nil
}

// ---- Phase 4: word_records ----

func (m *migrator) migrateWordRecords(stats *migrateStats) error {
	if m.skip("word_records") {
		stats.addSkip("word_records")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at FROM word_records`)
	if err != nil {
		return fmt.Errorf("query SQLite word_records: %w", err)
	}
	defer rows.Close()

	type wrRow struct {
		id            int64
		userID        int64
		wordID        int64
		masteryLevel  int
		nextReviewAt  string
		easeFactor    float64
		interval      int
		reviewHistory string
		updatedAt     sql.NullString
	}

	var records []wrRow
	for rows.Next() {
		var r wrRow
		if err := rows.Scan(&r.id, &r.userID, &r.wordID, &r.masteryLevel, &r.nextReviewAt, &r.easeFactor, &r.interval, &r.reviewHistory, &r.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite word_record: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows word_records: %w", err)
	}

	migrated := 0
	for _, r := range records {
		if m.skipRowWithMissingSourceUser(stats, "word_records", r.id, r.userID) {
			continue
		}
		nextReview, err := parseTimestamp(r.nextReviewAt)
		if err != nil {
			nextReview = time.Now()
		}
		updatedAt := time.Now()
		if r.updatedAt.Valid && r.updatedAt.String != "" {
			if t, err := parseTimestamp(r.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		reviewHist := r.reviewHistory
		if reviewHist == "" {
			reviewHist = "[]"
		}

		if _, err := m.execPG(
			`INSERT INTO word_records (id, user_id, word_id, mastery_level, next_review_at, ease_factor, interval_days, review_history_json, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO UPDATE SET mastery_level=EXCLUDED.mastery_level, next_review_at=EXCLUDED.next_review_at`,
			r.id, r.userID, r.wordID, r.masteryLevel, nextReview, r.easeFactor, r.interval, reviewHist, updatedAt,
		); err != nil {
			return fmt.Errorf("insert word_record %d: %w", r.id, err)
		}
		migrated++
	}

	fmt.Printf("[14/26] word_records: %d rows migrated\n", migrated)
	stats.add("word_records", int64(migrated))
	return nil
}

// ---- Phase 4: grammar_records ----

func (m *migrator) migrateGrammarRecords(stats *migrateStats) error {
	if m.skip("grammar_records") {
		stats.addSkip("grammar_records")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, grammar_point_id, status, next_review_at, quiz_history_json, updated_at FROM grammar_records`)
	if err != nil {
		return fmt.Errorf("query SQLite grammar_records: %w", err)
	}
	defer rows.Close()

	type grRow struct {
		id             int64
		userID         int64
		grammarPointID int64
		status         string
		nextReviewAt   string
		quizHistory    string
		updatedAt      sql.NullString
	}

	var records []grRow
	for rows.Next() {
		var r grRow
		if err := rows.Scan(&r.id, &r.userID, &r.grammarPointID, &r.status, &r.nextReviewAt, &r.quizHistory, &r.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite grammar_record: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows grammar_records: %w", err)
	}

	migrated := 0
	for _, r := range records {
		if m.skipRowWithMissingSourceUser(stats, "grammar_records", r.id, r.userID) {
			continue
		}
		nextReview, err := parseTimestamp(r.nextReviewAt)
		if err != nil {
			nextReview = time.Now()
		}
		updatedAt := time.Now()
		if r.updatedAt.Valid && r.updatedAt.String != "" {
			if t, err := parseTimestamp(r.updatedAt.String); err == nil {
				updatedAt = t
			}
		}
		quizHist := r.quizHistory
		if quizHist == "" {
			quizHist = "[]"
		}

		if _, err := m.execPG(
			`INSERT INTO grammar_records (id, user_id, grammar_point_id, status, next_review_at, quiz_history_json, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, next_review_at=EXCLUDED.next_review_at`,
			r.id, r.userID, m.mapGrammarPointID(r.grammarPointID), r.status, nextReview, quizHist, updatedAt,
		); err != nil {
			return fmt.Errorf("insert grammar_record %d: %w", r.id, err)
		}
		migrated++
	}

	fmt.Printf("[15/26] grammar_records: %d rows migrated\n", migrated)
	stats.add("grammar_records", int64(migrated))
	return nil
}

// ---- Phase 4: speaking_records ----

func (m *migrator) migrateSpeakingRecords(stats *migrateStats) error {
	if m.skip("speaking_records") {
		stats.addSkip("speaking_records")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, type, material_id, score, audio_ref, practiced_at FROM speaking_records`)
	if err != nil {
		return fmt.Errorf("query SQLite speaking_records: %w", err)
	}
	defer rows.Close()

	type srRow struct {
		id          int64
		userID      int64
		srType      string
		materialID  int64
		score       int
		audioRef    string
		practicedAt string
	}

	var records []srRow
	for rows.Next() {
		var r srRow
		if err := rows.Scan(&r.id, &r.userID, &r.srType, &r.materialID, &r.score, &r.audioRef, &r.practicedAt); err != nil {
			return fmt.Errorf("scan SQLite speaking_record: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows speaking_records: %w", err)
	}

	for _, r := range records {
		practicedAt, err := parseTimestamp(r.practicedAt)
		if err != nil {
			practicedAt = time.Now()
		}

		if _, err := m.execPG(
			`INSERT INTO speaking_records (id, user_id, type, material_id, score, audio_ref_legacy, practiced_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET score=EXCLUDED.score`,
			r.id, r.userID, r.srType, r.materialID, r.score, r.audioRef, practicedAt,
		); err != nil {
			return fmt.Errorf("insert speaking_record %d: %w", r.id, err)
		}
	}

	fmt.Printf("[16/26] speaking_records: %d rows migrated\n", len(records))
	stats.add("speaking_records", int64(len(records)))
	return nil
}

// ---- Phase 4: writing_records ----

func (m *migrator) migrateWritingRecords(stats *migrateStats) error {
	if m.skip("writing_records") {
		stats.addSkip("writing_records")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, type, question, user_answer, ai_feedback_json, score, practiced_at FROM writing_records`)
	if err != nil {
		return fmt.Errorf("query SQLite writing_records: %w", err)
	}
	defer rows.Close()

	type wrRow struct {
		id             int64
		userID         int64
		wrType         string
		question       string
		userAnswer     string
		aiFeedbackJSON string
		score          int
		practicedAt    string
	}

	var records []wrRow
	for rows.Next() {
		var r wrRow
		if err := rows.Scan(&r.id, &r.userID, &r.wrType, &r.question, &r.userAnswer, &r.aiFeedbackJSON, &r.score, &r.practicedAt); err != nil {
			return fmt.Errorf("scan SQLite writing_record: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows writing_records: %w", err)
	}

	for _, r := range records {
		practicedAt, err := parseTimestamp(r.practicedAt)
		if err != nil {
			practicedAt = time.Now()
		}
		// ai_feedback_json: SQLite uses "null" string for null -> keep as JSONB null
		feedback := r.aiFeedbackJSON
		if feedback == "null" || feedback == "" {
			feedback = "null"
		}

		if _, err := m.execPG(
			`INSERT INTO writing_records (id, user_id, type, question, user_answer, ai_feedback_json, score, practiced_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (id) DO UPDATE SET score=EXCLUDED.score`,
			r.id, r.userID, r.wrType, r.question, r.userAnswer, feedback, r.score, practicedAt,
		); err != nil {
			return fmt.Errorf("insert writing_record %d: %w", r.id, err)
		}
	}

	fmt.Printf("[17/26] writing_records: %d rows migrated\n", len(records))
	stats.add("writing_records", int64(len(records)))
	return nil
}

// ---- Phase 4: translation_records ----

func (m *migrator) migrateTranslationRecords(stats *migrateStats) error {
	if m.skip("translation_records") {
		stats.addSkip("translation_records")
		return nil
	}

	// Need to JOIN with translation_sentences to populate snapshot columns
	rows, err := m.sqliteDB.Query(`
		SELECT tr.id, tr.user_id, tr.sentence_id, tr.user_translation, tr.score, tr.rule_score,
		       tr.ai_feedback_json, tr.practiced_at,
		       COALESCE(ts.source_text, ''), COALESCE(ts.direction, ''), COALESCE(ts.reference_translation, ''), COALESCE(ts.position, 0)
		FROM translation_records tr
		LEFT JOIN translation_sentences ts ON ts.id = tr.sentence_id`)
	if err != nil {
		return fmt.Errorf("query SQLite translation_records: %w", err)
	}
	defer rows.Close()

	type trRow struct {
		id          int64
		userID      int64
		sentenceID  int64
		userTrans   string
		score       int
		ruleScore   int
		aiFeedback  string
		practicedAt string
		sourceText  string
		direction   string
		refTrans    string
		position    int
	}

	var records []trRow
	for rows.Next() {
		var r trRow
		if err := rows.Scan(&r.id, &r.userID, &r.sentenceID, &r.userTrans, &r.score, &r.ruleScore, &r.aiFeedback, &r.practicedAt, &r.sourceText, &r.direction, &r.refTrans, &r.position); err != nil {
			return fmt.Errorf("scan SQLite translation_record: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows translation_records: %w", err)
	}

	for _, r := range records {
		practicedAt, err := parseTimestamp(r.practicedAt)
		if err != nil {
			practicedAt = time.Now()
		}
		feedback := r.aiFeedback
		if feedback == "" || feedback == "null" {
			feedback = "{}"
		}

		if _, err := m.execPG(
			`INSERT INTO translation_records (id, user_id, sentence_id, user_translation, score, rule_score,
			 ai_feedback_json, source_text_snapshot, direction_snapshot, reference_translation_snapshot, position_snapshot, practiced_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 ON CONFLICT (id) DO UPDATE SET score=EXCLUDED.score`,
			r.id, r.userID, r.sentenceID, r.userTrans, r.score, r.ruleScore,
			feedback, r.sourceText, r.direction, r.refTrans, r.position, practicedAt,
		); err != nil {
			return fmt.Errorf("insert translation_record %d: %w", r.id, err)
		}
	}

	fmt.Printf("[18/26] translation_records: %d rows migrated\n", len(records))
	stats.add("translation_records", int64(len(records)))
	return nil
}

// ---- Phase 4: word_bookmarks ----

func (m *migrator) migrateWordBookmarks(stats *migrateStats) error {
	if m.skip("word_bookmarks") {
		stats.addSkip("word_bookmarks")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT user_id, word_id, created_at FROM word_bookmarks`)
	if err != nil {
		return fmt.Errorf("query SQLite word_bookmarks: %w", err)
	}
	defer rows.Close()

	type wbRow struct {
		userID    int64
		wordID    int64
		createdAt string
	}

	var bookmarks []wbRow
	for rows.Next() {
		var b wbRow
		if err := rows.Scan(&b.userID, &b.wordID, &b.createdAt); err != nil {
			return fmt.Errorf("scan SQLite word_bookmark: %w", err)
		}
		bookmarks = append(bookmarks, b)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows word_bookmarks: %w", err)
	}

	for _, b := range bookmarks {
		createdAt, err := parseTimestamp(b.createdAt)
		if err != nil {
			createdAt = time.Now()
		}
		if _, err := m.execPG(
			`INSERT INTO word_bookmarks (user_id, word_id, created_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, word_id) DO NOTHING`,
			b.userID, b.wordID, createdAt,
		); err != nil {
			return fmt.Errorf("insert word_bookmark (user=%d word=%d): %w", b.userID, b.wordID, err)
		}
	}

	fmt.Printf("[19/26] word_bookmarks: %d rows migrated\n", len(bookmarks))
	stats.add("word_bookmarks", int64(len(bookmarks)))
	return nil
}

// ---- Phase 4: notes ----

func (m *migrator) migrateNotes(stats *migrateStats) error {
	if m.skip("notes") {
		stats.addSkip("notes")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		tags_json, mastery_level, next_review_at, ease_factor, interval, review_history_json, created_at, updated_at, deleted_at
		FROM notes`)
	if err != nil {
		return fmt.Errorf("query SQLite notes: %w", err)
	}
	defer rows.Close()

	type noteRow struct {
		id            int64
		userID        int64
		noteType      string
		title         string
		content       string
		sourceText    string
		referenceID   sql.NullInt64
		referenceType sql.NullString
		tagsJSON      string
		masteryLevel  int
		nextReviewAt  sql.NullString
		easeFactor    float64
		interval      int
		reviewHistory string
		createdAt     string
		updatedAt     string
		deletedAt     sql.NullString
	}

	var notes []noteRow
	for rows.Next() {
		var n noteRow
		if err := rows.Scan(&n.id, &n.userID, &n.noteType, &n.title, &n.content, &n.sourceText, &n.referenceID, &n.referenceType,
			&n.tagsJSON, &n.masteryLevel, &n.nextReviewAt, &n.easeFactor, &n.interval, &n.reviewHistory, &n.createdAt, &n.updatedAt, &n.deletedAt); err != nil {
			return fmt.Errorf("scan SQLite note: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows notes: %w", err)
	}

	for _, n := range notes {
		createdAt, err := parseTimestamp(n.createdAt)
		if err != nil {
			createdAt = time.Now()
		}
		updatedAt, err := parseTimestamp(n.updatedAt)
		if err != nil {
			updatedAt = time.Now()
		}
		tagsArr := parseJSONStringArray(n.tagsJSON)

		var nextReview *time.Time
		if n.nextReviewAt.Valid && n.nextReviewAt.String != "" {
			if t, err := parseTimestamp(n.nextReviewAt.String); err == nil {
				nextReview = &t
			}
		}

		var deletedAt *time.Time
		if n.deletedAt.Valid && n.deletedAt.String != "" {
			if t, err := parseTimestamp(n.deletedAt.String); err == nil {
				deletedAt = &t
			}
		}

		var refID *int64
		if n.referenceID.Valid {
			refID = &n.referenceID.Int64
		}
		var refType *string
		if n.referenceType.Valid {
			refType = &n.referenceType.String
		}

		reviewHist := n.reviewHistory
		if reviewHist == "" {
			reviewHist = "[]"
		}

		if _, err := m.execPG(
			`INSERT INTO notes (id, user_id, type, title, content, source_text, reference_id, reference_type,
			 tags, mastery_level, next_review_at, ease_factor, interval_days, review_history_json, created_at, updated_at, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			 ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, content=EXCLUDED.content`,
			n.id, n.userID, n.noteType, n.title, n.content, n.sourceText, refID, refType,
			tagsArr, n.masteryLevel, nextReview, n.easeFactor, n.interval, reviewHist, createdAt, updatedAt, deletedAt,
		); err != nil {
			return fmt.Errorf("insert note %d: %w", n.id, err)
		}
	}

	fmt.Printf("[20/26] notes: %d rows migrated\n", len(notes))
	stats.add("notes", int64(len(notes)))
	return nil
}

// ---- Phase 4: note_links ----

func (m *migrator) migrateNoteLinks(stats *migrateStats) error {
	if m.skip("note_links") {
		stats.addSkip("note_links")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, note_id, target_note_id, relation, created_at FROM note_links`)
	if err != nil {
		return fmt.Errorf("query SQLite note_links: %w", err)
	}
	defer rows.Close()

	type nlRow struct {
		id           int64
		userID       int64
		noteID       int64
		targetNoteID int64
		relation     string
		createdAt    string
	}

	var links []nlRow
	for rows.Next() {
		var l nlRow
		if err := rows.Scan(&l.id, &l.userID, &l.noteID, &l.targetNoteID, &l.relation, &l.createdAt); err != nil {
			return fmt.Errorf("scan SQLite note_link: %w", err)
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows note_links: %w", err)
	}

	for _, l := range links {
		createdAt, err := parseTimestamp(l.createdAt)
		if err != nil {
			createdAt = time.Now()
		}
		if _, err := m.execPG(
			`INSERT INTO note_links (id, user_id, note_id, target_note_id, relation, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO UPDATE SET relation=EXCLUDED.relation`,
			l.id, l.userID, l.noteID, l.targetNoteID, l.relation, createdAt,
		); err != nil {
			return fmt.Errorf("insert note_link %d: %w", l.id, err)
		}
	}

	fmt.Printf("[21/26] note_links: %d rows migrated\n", len(links))
	stats.add("note_links", int64(len(links)))
	return nil
}

// ---- Phase 5: study_sessions ----

func (m *migrator) migrateStudySessions(stats *migrateStats) error {
	if m.skip("study_sessions") {
		stats.addSkip("study_sessions")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, session_id, user_id, module, duration_seconds, completed_count, started_at FROM study_sessions`)
	if err != nil {
		return fmt.Errorf("query SQLite study_sessions: %w", err)
	}
	defer rows.Close()

	type ssRow struct {
		id              int64
		sessionID       string
		userID          int64
		module          string
		durationSeconds int
		completedCount  int
		startedAt       string
	}

	var sessions []ssRow
	for rows.Next() {
		var s ssRow
		if err := rows.Scan(&s.id, &s.sessionID, &s.userID, &s.module, &s.durationSeconds, &s.completedCount, &s.startedAt); err != nil {
			return fmt.Errorf("scan SQLite study_session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows study_sessions: %w", err)
	}

	for _, s := range sessions {
		startedAt, err := parseTimestamp(s.startedAt)
		if err != nil {
			startedAt = time.Now()
		}
		if _, err := m.execPG(
			`INSERT INTO study_sessions (id, session_id, user_id, module, duration_seconds, completed_count, started_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET duration_seconds=EXCLUDED.duration_seconds`,
			s.id, s.sessionID, s.userID, s.module, s.durationSeconds, s.completedCount, startedAt,
		); err != nil {
			return fmt.Errorf("insert study_session %d: %w", s.id, err)
		}
	}

	fmt.Printf("[22/26] study_sessions: %d rows migrated\n", len(sessions))
	stats.add("study_sessions", int64(len(sessions)))
	return nil
}

// ---- Phase 5: session_summaries ----

func (m *migrator) migrateSessionSummaries(stats *migrateStats) error {
	if m.skip("session_summaries") {
		stats.addSkip("session_summaries")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, session_id, user_id, module, score_summary_json, strengths_json, weaknesses_json, suggestions_json, generated_at FROM session_summaries`)
	if err != nil {
		return fmt.Errorf("query SQLite session_summaries: %w", err)
	}
	defer rows.Close()

	type ssRow struct {
		id           int64
		sessionID    string
		userID       int64
		module       string
		scoreSummary string
		strengths    string
		weaknesses   string
		suggestions  string
		generatedAt  string
	}

	var summaries []ssRow
	for rows.Next() {
		var s ssRow
		if err := rows.Scan(&s.id, &s.sessionID, &s.userID, &s.module, &s.scoreSummary, &s.strengths, &s.weaknesses, &s.suggestions, &s.generatedAt); err != nil {
			return fmt.Errorf("scan SQLite session_summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows session_summaries: %w", err)
	}

	for _, s := range summaries {
		generatedAt, err := parseTimestamp(s.generatedAt)
		if err != nil {
			generatedAt = time.Now()
		}
		if _, err := m.execPG(
			`INSERT INTO session_summaries (id, session_id, user_id, module, score_summary_json, strengths_json, weaknesses_json, suggestions_json, generated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO UPDATE SET score_summary_json=EXCLUDED.score_summary_json`,
			s.id, s.sessionID, s.userID, s.module, s.scoreSummary, s.strengths, s.weaknesses, s.suggestions, generatedAt,
		); err != nil {
			return fmt.Errorf("insert session_summary %d: %w", s.id, err)
		}
	}

	fmt.Printf("[23/26] session_summaries: %d rows migrated\n", len(summaries))
	stats.add("session_summaries", int64(len(summaries)))
	return nil
}

// ---- Phase 5: lesson_shadowing_progress ----

func (m *migrator) migrateLessonShadowingProgress(stats *migrateStats) error {
	if m.skip("lesson_shadowing_progress") {
		stats.addSkip("lesson_shadowing_progress")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, lesson_id, shadowing_version, last_sentence_index, last_position_ms, last_practice_mode, updated_at FROM lesson_shadowing_progress`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			stats.add("lesson_shadowing_progress", 0)
			return nil
		}
		return fmt.Errorf("query SQLite lesson_shadowing_progress: %w", err)
	}
	defer rows.Close()

	type lspRow struct {
		id                int64
		userID            int64
		lessonID          int64
		shadowingVersion  int
		lastSentenceIndex int
		lastPositionMS    int64
		lastPracticeMode  string
		updatedAt         string
	}

	var progress []lspRow
	for rows.Next() {
		var p lspRow
		if err := rows.Scan(&p.id, &p.userID, &p.lessonID, &p.shadowingVersion, &p.lastSentenceIndex, &p.lastPositionMS, &p.lastPracticeMode, &p.updatedAt); err != nil {
			return fmt.Errorf("scan SQLite lesson_shadowing_progress: %w", err)
		}
		progress = append(progress, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows lesson_shadowing_progress: %w", err)
	}

	for _, p := range progress {
		updatedAt, err := parseTimestamp(p.updatedAt)
		if err != nil {
			updatedAt = time.Now()
		}
		sv := p.shadowingVersion
		if sv < 1 {
			sv = 1
		}
		if _, err := m.execPG(
			`INSERT INTO lesson_shadowing_progress (id, user_id, lesson_id, shadowing_version, last_sentence_index, last_position_ms, last_practice_mode, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (id) DO UPDATE SET last_sentence_index=EXCLUDED.last_sentence_index, updated_at=EXCLUDED.updated_at`,
			p.id, p.userID, m.mapLessonID(p.lessonID), sv, p.lastSentenceIndex, p.lastPositionMS, p.lastPracticeMode, updatedAt,
		); err != nil {
			return fmt.Errorf("insert lesson_shadowing_progress %d: %w", p.id, err)
		}
	}

	fmt.Printf("[24/26] lesson_shadowing_progress: %d rows migrated\n", len(progress))
	stats.add("lesson_shadowing_progress", int64(len(progress)))
	return nil
}

// ---- Phase 5: lesson_shadowing_attempts ----

func (m *migrator) migrateLessonShadowingAttempts(stats *migrateStats) error {
	if m.skip("lesson_shadowing_attempts") {
		stats.addSkip("lesson_shadowing_attempts")
		return nil
	}

	rows, err := m.sqliteDB.Query(`SELECT id, user_id, lesson_id, shadowing_version, sentence_index, practice_mode, playback_rate, loop_count, self_score, recognition_text, audio_ref, created_at FROM lesson_shadowing_attempts`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			stats.add("lesson_shadowing_attempts", 0)
			return nil
		}
		return fmt.Errorf("query SQLite lesson_shadowing_attempts: %w", err)
	}
	defer rows.Close()

	type lsaRow struct {
		id               int64
		userID           int64
		lessonID         int64
		shadowingVersion int
		sentenceIndex    int
		practiceMode     string
		playbackRate     float64
		loopCount        int
		selfScore        sql.NullInt64
		recognitionText  string
		audioRef         string
		createdAt        string
	}

	var attempts []lsaRow
	for rows.Next() {
		var a lsaRow
		if err := rows.Scan(&a.id, &a.userID, &a.lessonID, &a.shadowingVersion, &a.sentenceIndex, &a.practiceMode, &a.playbackRate, &a.loopCount, &a.selfScore, &a.recognitionText, &a.audioRef, &a.createdAt); err != nil {
			return fmt.Errorf("scan SQLite lesson_shadowing_attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows lesson_shadowing_attempts: %w", err)
	}

	for _, a := range attempts {
		createdAt, err := parseTimestamp(a.createdAt)
		if err != nil {
			createdAt = time.Now()
		}
		sv := a.shadowingVersion
		if sv < 1 {
			sv = 1
		}
		var selfScore *int
		if a.selfScore.Valid {
			v := int(a.selfScore.Int64)
			selfScore = &v
		}
		if _, err := m.execPG(
			`INSERT INTO lesson_shadowing_attempts (id, user_id, lesson_id, shadowing_version, sentence_index, practice_mode, playback_rate, loop_count, self_score, recognition_text, audio_ref, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 ON CONFLICT (id) DO NOTHING`,
			a.id, a.userID, m.mapLessonID(a.lessonID), sv, a.sentenceIndex, a.practiceMode, a.playbackRate, a.loopCount, selfScore, a.recognitionText, a.audioRef, createdAt,
		); err != nil {
			return fmt.Errorf("insert lesson_shadowing_attempt %d: %w", a.id, err)
		}
	}

	fmt.Printf("[25/26] lesson_shadowing_attempts: %d rows migrated\n", len(attempts))
	stats.add("lesson_shadowing_attempts", int64(len(attempts)))
	return nil
}

// ---- Phase 6: password_reset_tokens ----

func (m *migrator) migratePasswordResetTokens(stats *migrateStats) error {
	if m.skip("password_reset_tokens") {
		stats.addSkip("password_reset_tokens")
		return nil
	}

	createdAtExpr := "NULL AS created_at"
	hasCreatedAt, err := sqliteColumnExists(m.sqliteDB, "password_reset_tokens", "created_at")
	if err != nil {
		return fmt.Errorf("inspect SQLite password_reset_tokens columns: %w", err)
	}
	if hasCreatedAt {
		createdAtExpr = "created_at"
	}

	rows, err := m.sqliteDB.Query(fmt.Sprintf(`SELECT token, user_id, expires_at, used, %s FROM password_reset_tokens`, createdAtExpr))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			stats.add("password_reset_tokens", 0)
			return nil
		}
		return fmt.Errorf("query SQLite password_reset_tokens: %w", err)
	}
	defer rows.Close()

	type prtRow struct {
		token     string
		userID    int64
		expiresAt string
		used      int
		createdAt sql.NullString
	}

	var tokens []prtRow
	for rows.Next() {
		var t prtRow
		if err := rows.Scan(&t.token, &t.userID, &t.expiresAt, &t.used, &t.createdAt); err != nil {
			return fmt.Errorf("scan SQLite password_reset_token: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows password_reset_tokens: %w", err)
	}

	migrated := 0
	for _, t := range tokens {
		if m.skipRowWithMissingSourceUser(stats, "password_reset_tokens", t.token, t.userID) {
			continue
		}
		expiresAt, err := parseTimestamp(t.expiresAt)
		if err != nil {
			expiresAt = time.Now()
		}
		createdAt := time.Now()
		if t.createdAt.Valid && t.createdAt.String != "" {
			if parsed, err := parseTimestamp(t.createdAt.String); err == nil {
				createdAt = parsed
			}
		}
		// used: INTEGER 0/1 -> BOOLEAN
		used := t.used != 0
		if _, err := m.execPG(
			`INSERT INTO password_reset_tokens (token, user_id, expires_at, used, created_at)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (token) DO UPDATE SET used=EXCLUDED.used`,
			t.token, t.userID, expiresAt, used, createdAt,
		); err != nil {
			return fmt.Errorf("insert password_reset_token %s: %w", t.token, err)
		}
		migrated++
	}

	fmt.Printf("[26/26] password_reset_tokens: %d rows migrated\n", migrated)
	stats.add("password_reset_tokens", int64(migrated))
	return nil
}

// ---- Sequence reset ----

func (m *migrator) resetSequences(stats *migrateStats) error {
	if m.dryRun {
		fmt.Println("dry-run: skipping sequence reset")
		return nil
	}

	seqTables := []string{
		"users", "words", "word_examples", "word_records", "word_review_events",
		"grammar_points", "grammar_examples", "grammar_quiz_questions", "grammar_records", "grammar_quiz_attempts",
		"lessons", "lesson_sentences",
		"speaking_materials", "speaking_records",
		"writing_questions", "writing_records",
		"translation_sources", "translation_sentences", "translation_records",
		"notes", "note_links", "note_review_events",
		"study_sessions", "session_summaries",
		"lesson_shadowing_progress", "lesson_shadowing_attempts",
		"audio_objects", "video_objects",
	}

	for _, t := range seqTables {
		seqName := t + "_id_seq"
		_, err := m.pgDB.Exec(fmt.Sprintf("SELECT setval('%s', COALESCE((SELECT MAX(id) FROM %s), 1), true)", seqName, t))
		if err != nil {
			// Ignore if sequence doesn't exist
			if strings.Contains(err.Error(), "does not exist") {
				continue
			}
			slog.Warn("resetSequences: failed to reset", "table", t, "err", err)
			stats.addError(fmt.Sprintf("sequence %s: %v", seqName, err))
		}
	}

	fmt.Println("Sequences reset for all tables.")
	return nil
}

// ---- Helper functions ----

// parseTimestamp tries to parse a SQLite datetime string into time.Time.
// It first attempts the SQLite-specific parser, then falls back to common Go layouts.
func parseTimestamp(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	// Use the existing SQLite parser from the data package
	return dataParseSQLiteTime(s)
}

// dataParseSQLiteTime wraps the internal parseSQLiteTime to make it accessible.
func dataParseSQLiteTime(s string) (time.Time, error) {
	return parseSQLiteTimeInternal(s)
}

// parseSQLiteTimeInternal is a copy of data.parseSQLiteTime to avoid import cycle.
var sqliteTimeFormats = []string{
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseSQLiteTimeInternal(s string) (time.Time, error) {
	for _, layout := range sqliteTimeFormats {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized datetime format: %q", s)
}

// parseJSONStringArray parses a JSON string array like '["N5","N4"]' into a PostgreSQL TEXT[] literal.
func parseJSONStringArray(s string) string {
	if s == "" || s == "[]" {
		return "{}"
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		// If it's already a PG array literal, return as-is
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			return s
		}
		return "{}"
	}
	if len(arr) == 0 {
		return "{}"
	}
	// Build PG array literal: {elem1,elem2,...}
	// Escape double quotes and backslashes in elements
	escaped := make([]string, len(arr))
	for i, v := range arr {
		escaped[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`) + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

// parseJSONStringArrayOrEmpty is like parseJSONStringArray but accepts sql.NullString.
// Returns "{}" (PG empty array) for empty/null inputs.
func parseJSONStringArrayOrEmpty(s string) string {
	if s == "" || s == "null" {
		return "{}"
	}
	return parseJSONStringArray(s)
}

func normalizeMigrationUserLevels(goalLevel, jlptLevelsRaw string) (string, []string) {
	levels := parseMigrationJLPTLevels(jlptLevelsRaw)
	goal := strings.TrimSpace(goalLevel)
	goalValid := isMigrationJLPTLevel(goal)

	if len(levels) == 0 {
		if goalValid {
			return goal, []string{goal}
		}
		return "N5", []string{"N5"}
	}
	if goalValid {
		return goal, levels
	}
	return levels[0], levels
}

func normalizeMigrationJLPTLevel(level string) string {
	level = strings.TrimSpace(level)
	if isMigrationJLPTLevel(level) {
		return level
	}
	return "N5"
}

func parseMigrationJLPTLevels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		if isMigrationJLPTLevel(raw) {
			parsed = []string{raw}
		}
	}

	levels := make([]string, 0, len(parsed))
	seen := make(map[string]bool, len(parsed))
	for _, level := range parsed {
		level = strings.TrimSpace(level)
		if !isMigrationJLPTLevel(level) || seen[level] {
			continue
		}
		seen[level] = true
		levels = append(levels, level)
	}
	return levels
}

func isMigrationJLPTLevel(level string) bool {
	switch level {
	case "N5", "N4", "N3", "N2", "N1":
		return true
	default:
		return false
	}
}

// int64ArrayToPGLiteral converts []int64 to a PostgreSQL BIGINT[] literal like '{1,2,3}'.
func int64ArrayToPGLiteral(arr []int64) string {
	parts := make([]string, len(arr))
	for i, v := range arr {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
