package postgres

import (
	"database/sql"
	"testing"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/shadowing"
)

func TestShadowingStoreUpsertProgressKeepsSingleRow(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := NewShadowingStore(db)
	userID := insertShadowingUserFixture(t, db, "shadowing_progress_pg@example.com")
	lessonID := insertLessonFixture(t, db, lessonFixtureInput{Title: "PG Shadowing Progress", Level: lesson.LevelN5})

	missing, err := store.GetProgress(userID, lessonID, 2)
	if err != nil {
		t.Fatalf("GetProgress missing error: %v", err)
	}
	if missing != nil {
		t.Fatalf("GetProgress missing = %+v, want nil", missing)
	}

	first := shadowing.Progress{
		UserID:            userID,
		LessonID:          lessonID,
		ShadowingVersion:  2,
		LastSentenceIndex: 0,
		LastPositionMS:    500,
		LastPracticeMode:  shadowing.PracticeModeNormal,
	}
	if err := store.UpsertProgress(first); err != nil {
		t.Fatalf("UpsertProgress first error: %v", err)
	}

	second := first
	second.LastSentenceIndex = 1
	second.LastPositionMS = 1500
	second.LastPracticeMode = shadowing.PracticeModeSlow
	if err := store.UpsertProgress(second); err != nil {
		t.Fatalf("UpsertProgress second error: %v", err)
	}

	got, err := store.GetProgress(userID, lessonID, 2)
	if err != nil {
		t.Fatalf("GetProgress error: %v", err)
	}
	if got == nil {
		t.Fatal("GetProgress returned nil")
	}
	if got.LastSentenceIndex != 1 || got.LastPositionMS != 1500 || got.LastPracticeMode != shadowing.PracticeModeSlow {
		t.Fatalf("progress = %+v, want updated second values", got)
	}

	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM lesson_shadowing_progress
		WHERE user_id = $1 AND lesson_id = $2 AND shadowing_version = $3`,
		userID, lessonID, 2,
	).Scan(&count); err != nil {
		t.Fatalf("query progress count error: %v", err)
	}
	if count != 1 {
		t.Fatalf("progress row count = %d, want 1", count)
	}
}

func TestShadowingStoreNullableScoreSummaryAndVersionIsolation(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := NewShadowingStore(db)
	userID := insertShadowingUserFixture(t, db, "shadowing_summary_pg@example.com")
	lessonID := insertLessonFixture(t, db, lessonFixtureInput{Title: "PG Shadowing Summary", Level: lesson.LevelN5})

	score60 := 60
	score90 := 90
	attempts := []shadowing.Attempt{
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 1, SentenceIndex: 0, PracticeMode: shadowing.PracticeModeRecord, PlaybackRate: 1, SelfScore: &score60},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 2, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeLoop, PlaybackRate: 0.75, LoopCount: 3, SelfScore: nil},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 2, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeRecord, PlaybackRate: 1, SelfScore: &score90},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 2, SentenceIndex: 2, PracticeMode: shadowing.PracticeModeLoop, PlaybackRate: 0.75, LoopCount: 3, SelfScore: nil},
	}
	for i, attempt := range attempts {
		if err := store.InsertAttempt(attempt); err != nil {
			t.Fatalf("InsertAttempt %d error: %v", i, err)
		}
	}

	var nullScore sql.NullInt64
	if err := db.QueryRow(`
		SELECT self_score
		FROM lesson_shadowing_attempts
		WHERE user_id = $1 AND lesson_id = $2 AND shadowing_version = 2 AND sentence_index = 2
		ORDER BY id DESC
		LIMIT 1`,
		userID, lessonID,
	).Scan(&nullScore); err != nil {
		t.Fatalf("query nil score error: %v", err)
	}
	if nullScore.Valid {
		t.Fatalf("self_score = %d, want SQL NULL", nullScore.Int64)
	}

	summary, err := store.ListAttemptSummary(userID, lessonID, 2)
	if err != nil {
		t.Fatalf("ListAttemptSummary error: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("summary len = %d, want 2: %+v", len(summary), summary)
	}
	bySentence := map[int]shadowing.SentenceAttemptSummary{}
	for _, row := range summary {
		bySentence[row.SentenceIndex] = row
		if row.SentenceIndex == 0 {
			t.Fatalf("summary included old version sentence: %+v", summary)
		}
	}
	if bySentence[1].AttemptCount != 2 {
		t.Fatalf("sentence 1 attempt_count = %d, want 2", bySentence[1].AttemptCount)
	}
	if bySentence[1].BestScore == nil || *bySentence[1].BestScore != 90 {
		t.Fatalf("sentence 1 best_score = %v, want 90", bySentence[1].BestScore)
	}
	if bySentence[1].LastScore == nil || *bySentence[1].LastScore != 90 {
		t.Fatalf("sentence 1 last_score = %v, want 90", bySentence[1].LastScore)
	}
	if bySentence[2].BestScore != nil || bySentence[2].LastScore != nil {
		t.Fatalf("sentence 2 scores = best %v last %v, want nil/nil", bySentence[2].BestScore, bySentence[2].LastScore)
	}

	completed, err := store.ListCompletedSentenceIndexes(userID, lessonID, 2)
	if err != nil {
		t.Fatalf("ListCompletedSentenceIndexes error: %v", err)
	}
	if len(completed) != 2 || completed[0] != 1 || completed[1] != 2 {
		t.Fatalf("completed = %+v, want [1 2]", completed)
	}
}

func insertShadowingUserFixture(t *testing.T, db queryer, email string) int64 {
	t.Helper()

	var userID int64
	if err := db.QueryRowContext(
		t.Context(),
		`INSERT INTO users (email, name, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		email,
		"Shadowing User",
		"hash",
	).Scan(&userID); err != nil {
		t.Fatalf("insert shadowing user fixture: %v", err)
	}
	return userID
}
