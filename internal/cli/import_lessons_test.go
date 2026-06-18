package cli_test

import (
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"

	"japanese-learning-app/internal/cli"
	"japanese-learning-app/internal/data"
)

func lessonImportJSON(title, level, audioURL string, shadowingEnabled bool, shadowingVersion int) map[string]any {
	return map[string]any{
		"title":             title,
		"jlpt_level":        level,
		"tags":              []string{"shadowing"},
		"audio_url":         audioURL,
		"video_url":         "https://example.com/video.mp4",
		"word_ids":          []int64{1},
		"shadowing_enabled": shadowingEnabled,
		"shadowing_version": shadowingVersion,
		"shadowing_config":  map[string]any{"media_type": "audio"},
		"sentences":         []map[string]any{lessonSentenceJSON()},
	}
}

func lessonSentenceJSON() map[string]any {
	return map[string]any{
		"index":    0,
		"tokens":   []map[string]string{{"surface": "日本語", "reading": "にほんご"}},
		"chinese":  "日语",
		"start_ms": 0,
		"end_ms":   3000,
	}
}

func insertRawLesson(t *testing.T, db *sql.DB, title, level string) int64 {
	t.Helper()

	res, err := db.Exec(`
		INSERT INTO lessons (
			title,
			content_furigana_json,
			translation_json,
			jlpt_level,
			tags_json,
			audio_url,
			sentence_timestamps_json,
			char_count,
			word_ids_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title,
		`[{"index":0,"tokens":[{"surface":"日本語","reading":"にほんご"}],"chinese":"日语","start_ms":0,"end_ms":3000}]`,
		`["日语"]`,
		level,
		`["shadowing"]`,
		"https://example.com/audio.mp3",
		`[{"index":0,"start_ms":0,"end_ms":3000}]`,
		3,
		`[1]`,
	)
	if err != nil {
		t.Fatalf("insert raw lesson error: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId error: %v", err)
	}
	return id
}

func tempDBPath(t *testing.T) string {
	t.Helper()

	f, err := os.CreateTemp("", "cli_lesson_command_*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		t.Fatalf("close temp db file: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func seedDuplicateLessonsAtPath(t *testing.T, dbPath string) {
	t.Helper()

	db, err := data.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open setup db: %v", err)
	}
	defer db.Close()
	if err := data.RunMigrations(db); err != nil {
		t.Fatalf("run setup migrations: %v", err)
	}
	insertRawLesson(t, db, "Command Duplicate", "N5")
	insertRawLesson(t, db, "Command Duplicate", "N5")
}

func insertUser(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()

	res, err := db.Exec(
		`INSERT INTO users (email, password_hash, goal_level) VALUES (?, ?, ?)`,
		email,
		"test-hash",
		"N5",
	)
	if err != nil {
		t.Fatalf("insert user error: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("user LastInsertId error: %v", err)
	}
	return id
}

func lessonCountByTitleAtPath(t *testing.T, dbPath, title string) int {
	t.Helper()

	db, err := data.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open count db: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lessons WHERE title = ?`, title).Scan(&count); err != nil {
		t.Fatalf("count lessons by title error: %v", err)
	}
	return count
}

func TestImportLessonsFromFile_UpdatesExistingLesson(t *testing.T) {
	db := openTestDB(t)

	firstPath := writeTempJSON(t, []map[string]any{
		lessonImportJSON("Shadowing Update", "N5", "https://example.com/old.mp3", false, 1),
	})
	n, err := cli.ImportLessonsFromFile(db, firstPath)
	if err != nil {
		t.Fatalf("first ImportLessonsFromFile error: %v", err)
	}
	if n != 1 {
		t.Fatalf("first ImportLessonsFromFile inserted = %d, want 1", n)
	}

	secondPath := writeTempJSON(t, []map[string]any{
		lessonImportJSON("Shadowing Update", "N5", "https://example.com/new.mp3", true, 2),
	})
	n, err = cli.ImportLessonsFromFile(db, secondPath)
	if err != nil {
		t.Fatalf("second ImportLessonsFromFile error: %v", err)
	}
	if n != 0 {
		t.Fatalf("second ImportLessonsFromFile inserted = %d, want 0 for update", n)
	}

	var count int
	var audioURL string
	var videoURL string
	var shadowingEnabled int
	var shadowingVersion int
	var shadowingConfigJSON string
	err = db.QueryRow(`
		SELECT COUNT(*), MAX(audio_url), MAX(video_url), MAX(shadowing_enabled), MAX(shadowing_version), MAX(shadowing_config_json)
		FROM lessons
		WHERE title = ? AND jlpt_level = ?`,
		"Shadowing Update",
		"N5",
	).Scan(&count, &audioURL, &videoURL, &shadowingEnabled, &shadowingVersion, &shadowingConfigJSON)
	if err != nil {
		t.Fatalf("query imported lesson error: %v", err)
	}
	if count != 1 {
		t.Fatalf("lesson row count = %d, want 1", count)
	}
	if audioURL != "https://example.com/new.mp3" {
		t.Fatalf("audio_url = %q, want updated value", audioURL)
	}
	if videoURL != "https://example.com/video.mp4" {
		t.Fatalf("video_url = %q, want imported value", videoURL)
	}
	if shadowingEnabled != 1 {
		t.Fatalf("shadowing_enabled = %d, want 1", shadowingEnabled)
	}
	if shadowingVersion != 2 {
		t.Fatalf("shadowing_version = %d, want 2", shadowingVersion)
	}
	if shadowingConfigJSON != `{"media_type":"audio"}` {
		t.Fatalf("shadowing_config_json = %q, want media_type config", shadowingConfigJSON)
	}
}

func TestReportLessonDuplicates_GroupsAndKeepsMinID(t *testing.T) {
	db := openTestDB(t)
	keptID := insertRawLesson(t, db, "Duplicate Lesson", "N5")
	duplicateID := insertRawLesson(t, db, "Duplicate Lesson", "N5")
	insertRawLesson(t, db, "Duplicate Lesson", "N4")

	groups, err := cli.ReportLessonDuplicates(db)
	if err != nil {
		t.Fatalf("ReportLessonDuplicates error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("duplicate groups len = %d, want 1: %#v", len(groups), groups)
	}

	group := groups[0]
	if group.Title != "Duplicate Lesson" {
		t.Fatalf("group title = %q, want Duplicate Lesson", group.Title)
	}
	if group.JLPTLevel != "N5" {
		t.Fatalf("group level = %q, want N5", group.JLPTLevel)
	}
	if group.KeptID != keptID {
		t.Fatalf("KeptID = %d, want %d", group.KeptID, keptID)
	}
	if !reflect.DeepEqual(group.DuplicateIDs, []int64{keptID, duplicateID}) {
		t.Fatalf("DuplicateIDs = %#v, want [%d %d]", group.DuplicateIDs, keptID, duplicateID)
	}
}

func TestCleanupLessonDuplicates_RemovesNonKeptRows(t *testing.T) {
	db := openTestDB(t)
	keptID := insertRawLesson(t, db, "Cleanup Lesson", "N5")
	removedID := insertRawLesson(t, db, "Cleanup Lesson", "N5")

	deleted, err := cli.CleanupLessonDuplicates(db)
	if err != nil {
		t.Fatalf("CleanupLessonDuplicates error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	groups, err := cli.ReportLessonDuplicates(db)
	if err != nil {
		t.Fatalf("ReportLessonDuplicates after cleanup error: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("duplicate groups after cleanup = %#v, want none", groups)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lessons WHERE id = ?`, keptID).Scan(&count); err != nil {
		t.Fatalf("query kept row error: %v", err)
	}
	if count != 1 {
		t.Fatalf("kept row count = %d, want 1", count)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM lessons WHERE id = ?`, removedID).Scan(&count); err != nil {
		t.Fatalf("query removed row error: %v", err)
	}
	if count != 0 {
		t.Fatalf("removed row count = %d, want 0", count)
	}
}

func TestCleanupLessonDuplicates_PreservesShadowingData(t *testing.T) {
	db := openTestDB(t)
	keptID := insertRawLesson(t, db, "Cleanup Shadowing Data", "N5")
	removedID := insertRawLesson(t, db, "Cleanup Shadowing Data", "N5")
	userWithConflict := insertUser(t, db, "shadowing-conflict@example.com")
	userWithoutConflict := insertUser(t, db, "shadowing-no-conflict@example.com")

	if _, err := db.Exec(`
		INSERT INTO lesson_shadowing_attempts (
			user_id,
			lesson_id,
			shadowing_version,
			sentence_index,
			practice_mode,
			playback_rate,
			loop_count,
			self_score,
			recognition_text,
			audio_ref
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userWithConflict,
		removedID,
		1,
		0,
		"normal",
		1.0,
		2,
		88,
		"おはようございます",
		"attempt-1.wav",
	); err != nil {
		t.Fatalf("insert shadowing attempt error: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO lesson_shadowing_progress (
			user_id,
			lesson_id,
			shadowing_version,
			last_sentence_index,
			last_position_ms,
			last_practice_mode,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userWithConflict,
		keptID,
		1,
		1,
		1000,
		"old",
		"2026-01-01 00:00:00",
	); err != nil {
		t.Fatalf("insert kept progress error: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO lesson_shadowing_progress (
			user_id,
			lesson_id,
			shadowing_version,
			last_sentence_index,
			last_position_ms,
			last_practice_mode,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userWithConflict,
		removedID,
		1,
		3,
		4200,
		"loop",
		"2026-02-01 00:00:00",
	); err != nil {
		t.Fatalf("insert duplicate progress with conflict error: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO lesson_shadowing_progress (
			user_id,
			lesson_id,
			shadowing_version,
			last_sentence_index,
			last_position_ms,
			last_practice_mode,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userWithoutConflict,
		removedID,
		2,
		4,
		5300,
		"slow",
		"2026-03-01 00:00:00",
	); err != nil {
		t.Fatalf("insert duplicate progress without conflict error: %v", err)
	}

	deleted, err := cli.CleanupLessonDuplicates(db)
	if err != nil {
		t.Fatalf("CleanupLessonDuplicates error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	var duplicateAttemptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lesson_shadowing_attempts WHERE lesson_id = ?`, removedID).Scan(&duplicateAttemptCount); err != nil {
		t.Fatalf("query duplicate attempt count error: %v", err)
	}
	if duplicateAttemptCount != 0 {
		t.Fatalf("duplicate attempt count = %d, want 0", duplicateAttemptCount)
	}
	var keptAttemptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lesson_shadowing_attempts WHERE lesson_id = ?`, keptID).Scan(&keptAttemptCount); err != nil {
		t.Fatalf("query kept attempt count error: %v", err)
	}
	if keptAttemptCount != 1 {
		t.Fatalf("kept attempt count = %d, want 1", keptAttemptCount)
	}

	var sentenceIndex int
	var positionMS int
	var practiceMode string
	var updatedAt string
	if err := db.QueryRow(`
		SELECT last_sentence_index, last_position_ms, last_practice_mode, updated_at
		FROM lesson_shadowing_progress
		WHERE user_id = ? AND lesson_id = ? AND shadowing_version = ?`,
		userWithConflict,
		keptID,
		1,
	).Scan(&sentenceIndex, &positionMS, &practiceMode, &updatedAt); err != nil {
		t.Fatalf("query merged progress error: %v", err)
	}
	if sentenceIndex != 3 || positionMS != 4200 || practiceMode != "loop" || !strings.HasPrefix(updatedAt, "2026-02-01") {
		t.Fatalf("merged progress = (%d, %d, %q, %q), want duplicate newer row", sentenceIndex, positionMS, practiceMode, updatedAt)
	}

	var movedProgressCount int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM lesson_shadowing_progress
		WHERE user_id = ? AND lesson_id = ? AND shadowing_version = ?`,
		userWithoutConflict,
		keptID,
		2,
	).Scan(&movedProgressCount); err != nil {
		t.Fatalf("query moved progress count error: %v", err)
	}
	if movedProgressCount != 1 {
		t.Fatalf("moved progress count = %d, want 1", movedProgressCount)
	}
	var duplicateProgressCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lesson_shadowing_progress WHERE lesson_id = ?`, removedID).Scan(&duplicateProgressCount); err != nil {
		t.Fatalf("query duplicate progress count error: %v", err)
	}
	if duplicateProgressCount != 0 {
		t.Fatalf("duplicate progress count = %d, want 0", duplicateProgressCount)
	}
}

func TestCreateLessonUniqueIndex(t *testing.T) {
	t.Run("rejects duplicates", func(t *testing.T) {
		db := openTestDB(t)
		insertRawLesson(t, db, "Index Duplicate", "N5")
		insertRawLesson(t, db, "Index Duplicate", "N5")

		err := cli.CreateLessonUniqueIndex(db)
		if err == nil {
			t.Fatal("CreateLessonUniqueIndex error = nil, want duplicate error")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("CreateLessonUniqueIndex error = %v, want duplicate message", err)
		}
	})

	t.Run("creates index without duplicates", func(t *testing.T) {
		db := openTestDB(t)
		insertRawLesson(t, db, "Index Unique", "N5")
		insertRawLesson(t, db, "Index Unique", "N4")

		if err := cli.CreateLessonUniqueIndex(db); err != nil {
			t.Fatalf("CreateLessonUniqueIndex error: %v", err)
		}

		var name string
		err := db.QueryRow(`
			SELECT name
			FROM sqlite_master
			WHERE type = 'index' AND name = ?`,
			"idx_lessons_title_level_unique",
		).Scan(&name)
		if err != nil {
			t.Fatalf("query unique index error: %v", err)
		}
	})
}

func TestImportLessonsFromFile_RejectsExistingDuplicateLessons(t *testing.T) {
	db := openTestDB(t)
	insertRawLesson(t, db, "Importer Duplicate", "N5")
	insertRawLesson(t, db, "Importer Duplicate", "N5")

	filePath := writeTempJSON(t, []map[string]any{
		lessonImportJSON("Importer Duplicate", "N5", "https://example.com/new.mp3", true, 2),
	})
	_, err := cli.ImportLessonsFromFile(db, filePath)
	if err == nil {
		t.Fatal("ImportLessonsFromFile error = nil, want duplicate cleanup error")
	}
	if !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("ImportLessonsFromFile error = %v, want cleanup guidance", err)
	}
}

func TestRunLessonDuplicateCommands(t *testing.T) {
	reportDBPath := tempDBPath(t)
	if code := cli.Run([]string{"report-lesson-duplicates", "--db", reportDBPath}); code != 0 {
		t.Fatalf("report-lesson-duplicates exit code = %d, want 0", code)
	}

	cleanupDBPath := tempDBPath(t)
	seedDuplicateLessonsAtPath(t, cleanupDBPath)
	if code := cli.Run([]string{"cleanup-lesson-duplicates", "--db", cleanupDBPath}); code != 0 {
		t.Fatalf("cleanup-lesson-duplicates dry-run exit code = %d, want 0", code)
	}
	if count := lessonCountByTitleAtPath(t, cleanupDBPath, "Command Duplicate"); count != 2 {
		t.Fatalf("cleanup-lesson-duplicates without --apply left %d rows, want 2", count)
	}
	if code := cli.Run([]string{"cleanup-lesson-duplicates", "--db", cleanupDBPath, "--apply"}); code != 0 {
		t.Fatalf("cleanup-lesson-duplicates --apply exit code = %d, want 0", code)
	}
	if count := lessonCountByTitleAtPath(t, cleanupDBPath, "Command Duplicate"); count != 1 {
		t.Fatalf("cleanup-lesson-duplicates --apply left %d rows, want 1", count)
	}

	createDBPath := tempDBPath(t)
	if code := cli.Run([]string{"create-lesson-unique-index", "--db", createDBPath}); code != 0 {
		t.Fatalf("create-lesson-unique-index exit code = %d, want 0", code)
	}
}
