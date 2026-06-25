package cli

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNormalizeMigrationUserLevels(t *testing.T) {
	tests := []struct {
		name          string
		goalLevel     string
		jlptLevelsRaw string
		wantGoal      string
		wantLevels    []string
	}{
		{
			name:          "valid goal and levels pass through",
			goalLevel:     "N3",
			jlptLevelsRaw: `["N3","N2"]`,
			wantGoal:      "N3",
			wantLevels:    []string{"N3", "N2"},
		},
		{
			name:          "empty goal and blank levels fall back to N5",
			goalLevel:     "",
			jlptLevelsRaw: `[""]`,
			wantGoal:      "N5",
			wantLevels:    []string{"N5"},
		},
		{
			name:          "invalid goal uses first valid jlpt level",
			goalLevel:     "beginner",
			jlptLevelsRaw: `["N4","bad"]`,
			wantGoal:      "N4",
			wantLevels:    []string{"N4"},
		},
		{
			name:          "invalid json falls back to valid goal",
			goalLevel:     "N2",
			jlptLevelsRaw: `not-json`,
			wantGoal:      "N2",
			wantLevels:    []string{"N2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGoal, gotLevels := normalizeMigrationUserLevels(tt.goalLevel, tt.jlptLevelsRaw)
			if gotGoal != tt.wantGoal {
				t.Fatalf("goal = %q, want %q", gotGoal, tt.wantGoal)
			}
			if !reflect.DeepEqual(gotLevels, tt.wantLevels) {
				t.Fatalf("levels = %#v, want %#v", gotLevels, tt.wantLevels)
			}
		})
	}
}

func TestNormalizeMigrationJLPTLevel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "valid N1", in: "N1", want: "N1"},
		{name: "valid N5", in: "N5", want: "N5"},
		{name: "empty falls back", in: "", want: "N5"},
		{name: "dash falls back", in: "-", want: "N5"},
		{name: "unknown falls back", in: "beginner", want: "N5"},
		{name: "trims valid value", in: " N4 ", want: "N4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMigrationJLPTLevel(tt.in); got != tt.want {
				t.Fatalf("normalizeMigrationJLPTLevel(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMapGrammarPointID(t *testing.T) {
	m := &migrator{
		grammarPointIDMap: map[int64]int64{
			70: 1,
		},
	}

	if got := m.mapGrammarPointID(70); got != 1 {
		t.Fatalf("mapGrammarPointID(70) = %d, want 1", got)
	}
	if got := m.mapGrammarPointID(3); got != 3 {
		t.Fatalf("mapGrammarPointID(3) = %d, want 3", got)
	}
}

func TestMapLessonID(t *testing.T) {
	m := &migrator{
		lessonIDMap: map[int64]int64{
			16: 1,
		},
	}

	if got := m.mapLessonID(16); got != 1 {
		t.Fatalf("mapLessonID(16) = %d, want 1", got)
	}
	if got := m.mapLessonID(2); got != 2 {
		t.Fatalf("mapLessonID(2) = %d, want 2", got)
	}
}

func TestHasSourceUser(t *testing.T) {
	m := &migrator{}
	if !m.hasSourceUser(2) {
		t.Fatal("hasSourceUser should allow records before users are loaded")
	}

	m.rememberUserID(8)
	if !m.hasSourceUser(8) {
		t.Fatal("hasSourceUser(8) = false, want true")
	}
	if m.hasSourceUser(2) {
		t.Fatal("hasSourceUser(2) = true, want false after users are loaded")
	}
}

func TestSkipRowWithMissingSourceUserReportsError(t *testing.T) {
	m := &migrator{}
	m.rememberUserID(8)
	stats := &migrateStats{}

	if !m.skipRowWithMissingSourceUser(stats, "word_records", 35, 2) {
		t.Fatal("skipRowWithMissingSourceUser returned false for missing user")
	}
	if len(stats.Errors) != 1 || !strings.Contains(stats.Errors[0], "word_records") || !strings.Contains(stats.Errors[0], "user_id=2") {
		t.Fatalf("stats.Errors = %#v, want missing-user report", stats.Errors)
	}
	if m.skipRowWithMissingSourceUser(stats, "word_records", 36, 8) {
		t.Fatal("skipRowWithMissingSourceUser returned true for existing user")
	}
}

func TestGrammarExampleInsertSQLUsesLinkedWordIDsParameter(t *testing.T) {
	sql := grammarExampleInsertSQL()
	if !strings.Contains(sql, "$6::bigint[]") {
		t.Fatalf("grammarExampleInsertSQL() = %q, want linked_word_ids as $6::bigint[] parameter", sql)
	}
}

func TestVerifierExpectedSQLiteCountUsesMigrationSemantics(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY);
		CREATE TABLE word_records (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL);
		CREATE TABLE grammar_records (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL);
		CREATE TABLE password_reset_tokens (token TEXT PRIMARY KEY, user_id INTEGER NOT NULL);
		CREATE TABLE grammar_points (id INTEGER PRIMARY KEY, name TEXT NOT NULL, jlpt_level TEXT NOT NULL);
		CREATE TABLE lessons (id INTEGER PRIMARY KEY, title TEXT NOT NULL, jlpt_level TEXT NOT NULL);

		INSERT INTO users (id) VALUES (8);
		INSERT INTO word_records (id, user_id) VALUES (1, 8), (2, 2);
		INSERT INTO grammar_records (id, user_id) VALUES (1, 8), (2, 7);
		INSERT INTO password_reset_tokens (token, user_id) VALUES ('keep', 8), ('drop', 2);
		INSERT INTO grammar_points (id, name, jlpt_level) VALUES (1, 'ている', 'N5'), (2, 'ている', ' N5 ');
		INSERT INTO lessons (id, title, jlpt_level) VALUES (1, 'Lesson A', 'N5'), (2, 'Lesson A', '-');
	`)
	if err != nil {
		t.Fatalf("prepare db: %v", err)
	}

	v := &verifier{sqliteDB: db}
	tests := []struct {
		table string
		want  int64
	}{
		{table: "word_records", want: 1},
		{table: "grammar_records", want: 1},
		{table: "password_reset_tokens", want: 1},
		{table: "grammar_points", want: 1},
		{table: "lessons", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			got, err := v.expectedSQLiteCount(tt.table)
			if err != nil {
				t.Fatalf("expectedSQLiteCount(%q) error = %v", tt.table, err)
			}
			if got != tt.want {
				t.Fatalf("expectedSQLiteCount(%q) = %d, want %d", tt.table, got, tt.want)
			}
		})
	}
}

func TestVerifierCanonicalJSONCountsUseDedupedParents(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE grammar_points (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			jlpt_level TEXT NOT NULL,
			examples_json TEXT NOT NULL,
			quiz_questions_json TEXT NOT NULL
		);
		CREATE TABLE lessons (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			jlpt_level TEXT NOT NULL,
			content_furigana_json TEXT NOT NULL,
			word_ids_json TEXT NOT NULL
		);

		INSERT INTO grammar_points (id, name, jlpt_level, examples_json, quiz_questions_json)
		VALUES
			(1, 'ている', 'N5', '[{"japanese":"a"},{"japanese":"b"}]', '[{"prompt":"q"}]'),
			(2, 'ている', 'N5', '[{"japanese":"a"},{"japanese":"b"}]', '[{"prompt":"q"}]');
		INSERT INTO lessons (id, title, jlpt_level, content_furigana_json, word_ids_json)
		VALUES
			(1, 'Lesson A', 'N5', '[{"index":0},{"index":1},{"index":2}]', '[1,2]'),
			(2, 'Lesson A', 'N5', '[{"index":0},{"index":1},{"index":2}]', '[1,2]');
	`)
	if err != nil {
		t.Fatalf("prepare db: %v", err)
	}

	v := &verifier{sqliteDB: db}
	tests := []struct {
		name string
		got  func() (int64, error)
		want int64
	}{
		{name: "grammar examples", got: func() (int64, error) { return v.sqliteCountCanonicalGrammarJSONArray("examples_json") }, want: 2},
		{name: "grammar quiz", got: func() (int64, error) { return v.sqliteCountCanonicalGrammarJSONArray("quiz_questions_json") }, want: 1},
		{name: "lesson sentences", got: func() (int64, error) { return v.sqliteCountCanonicalLessonJSONArray("content_furigana_json") }, want: 3},
		{name: "lesson words", got: func() (int64, error) { return v.sqliteCountCanonicalLessonJSONArray("word_ids_json") }, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.got()
			if err != nil {
				t.Fatalf("count error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMigrateLessonsAllowsMissingUpdatedAtColumn(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE lessons (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			content_furigana_json TEXT NOT NULL DEFAULT '[]',
			translation_json TEXT NOT NULL DEFAULT '[]',
			jlpt_level TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			audio_url TEXT NOT NULL DEFAULT '',
			sentence_timestamps_json TEXT NOT NULL DEFAULT '[]',
			char_count INTEGER NOT NULL DEFAULT 0,
			word_ids_json TEXT NOT NULL DEFAULT '[]',
			shadowing_enabled INTEGER NOT NULL DEFAULT 0,
			video_url TEXT NOT NULL DEFAULT '',
			shadowing_version INTEGER NOT NULL DEFAULT 1,
			shadowing_config_json TEXT NOT NULL DEFAULT '{}'
		);
		INSERT INTO lessons (
			id, title, jlpt_level, tags_json, audio_url, char_count,
			word_ids_json, content_furigana_json, shadowing_enabled,
			video_url, shadowing_version, shadowing_config_json
		) VALUES (
			1, 'Legacy lesson', 'N5', '["shadowing"]', '/audio/lesson.wav', 12,
			'[]', '[]', 1, '', 1, '{}'
		);
	`)
	if err != nil {
		t.Fatalf("prepare lessons table: %v", err)
	}

	stats := &migrateStats{}
	m := &migrator{
		sqliteDB: db,
		dryRun:   true,
	}

	if err := m.migrateLessons(stats); err != nil {
		t.Fatalf("migrateLessons() error = %v", err)
	}
	if len(stats.Phases) != 1 {
		t.Fatalf("len(stats.Phases) = %d, want 1", len(stats.Phases))
	}
	if got := stats.Phases[0].Count; got != 1 {
		t.Fatalf("migrated lessons = %d, want 1", got)
	}
}

func TestMigratePasswordResetTokensAllowsMissingCreatedAtColumn(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE password_reset_tokens (
			token TEXT NOT NULL PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			used INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO password_reset_tokens (token, user_id, expires_at, used)
		VALUES ('reset-token', 1, '2026-06-22 01:00:00', 0);
	`)
	if err != nil {
		t.Fatalf("prepare password_reset_tokens table: %v", err)
	}

	stats := &migrateStats{}
	m := &migrator{
		sqliteDB: db,
		dryRun:   true,
	}

	if err := m.migratePasswordResetTokens(stats); err != nil {
		t.Fatalf("migratePasswordResetTokens() error = %v", err)
	}
	if len(stats.Phases) != 1 {
		t.Fatalf("len(stats.Phases) = %d, want 1", len(stats.Phases))
	}
	if got := stats.Phases[0].Count; got != 1 {
		t.Fatalf("migrated password reset tokens = %d, want 1", got)
	}
}
