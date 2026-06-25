package data

import (
	"database/sql"
	"testing"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/shadowing"
)

func TestShadowingStore_UpsertProgressKeepsSingleRow(t *testing.T) {
	store := NewShadowingStore(testDB)
	userID := int64(9600)
	insertTestUser(t, userID, "shadowing_progress@example.com")
	lessonID := insertTestLesson(t, "Shadowing Progress", lesson.LevelN5, `["shadowing"]`)

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
	if err := testDB.QueryRow(`
		SELECT COUNT(*)
		FROM lesson_shadowing_progress
		WHERE user_id = ? AND lesson_id = ? AND shadowing_version = ?`,
		userID, lessonID, 2,
	).Scan(&count); err != nil {
		t.Fatalf("query progress count error: %v", err)
	}
	if count != 1 {
		t.Fatalf("progress row count = %d, want 1", count)
	}
}

func TestShadowingStore_InsertAttemptStoresNilSelfScoreAsNull(t *testing.T) {
	store := NewShadowingStore(testDB)
	userID := int64(9601)
	insertTestUser(t, userID, "shadowing_nil_score@example.com")
	lessonID := insertTestLesson(t, "Shadowing Nil Score", lesson.LevelN5, `["shadowing"]`)

	err := store.InsertAttempt(shadowing.Attempt{
		UserID:           userID,
		LessonID:         lessonID,
		ShadowingVersion: 1,
		SentenceIndex:    0,
		PracticeMode:     shadowing.PracticeModeRecord,
		PlaybackRate:     1,
		LoopCount:        0,
		SelfScore:        nil,
	})
	if err != nil {
		t.Fatalf("InsertAttempt error: %v", err)
	}

	var score sql.NullInt64
	if err := testDB.QueryRow(`
		SELECT self_score
		FROM lesson_shadowing_attempts
		WHERE user_id = ? AND lesson_id = ?
		ORDER BY id DESC
		LIMIT 1`,
		userID, lessonID,
	).Scan(&score); err != nil {
		t.Fatalf("query self_score error: %v", err)
	}
	if score.Valid {
		t.Fatalf("self_score = %d, want SQL NULL", score.Int64)
	}
}

func TestShadowingStore_ListAttemptSummaryNullableScoreSemantics(t *testing.T) {
	store := NewShadowingStore(testDB)
	userID := int64(9602)
	insertTestUser(t, userID, "shadowing_summary@example.com")
	lessonID := insertTestLesson(t, "Shadowing Summary", lesson.LevelN5, `["shadowing"]`)

	score80 := 80
	score70 := 70
	attempts := []shadowing.Attempt{
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 3, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeLoop, PlaybackRate: 0.75, LoopCount: 2, SelfScore: nil},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 3, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeLoop, PlaybackRate: 0.75, LoopCount: 2, SelfScore: &score80},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 3, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeRecord, PlaybackRate: 1, LoopCount: 0, SelfScore: &score70},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 3, SentenceIndex: 2, PracticeMode: shadowing.PracticeModeRecord, PlaybackRate: 1, LoopCount: 0, SelfScore: nil},
	}
	for i, attempt := range attempts {
		if err := store.InsertAttempt(attempt); err != nil {
			t.Fatalf("InsertAttempt %d error: %v", i, err)
		}
	}

	summary, err := store.ListAttemptSummary(userID, lessonID, 3)
	if err != nil {
		t.Fatalf("ListAttemptSummary error: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("summary len = %d, want 2: %+v", len(summary), summary)
	}

	bySentence := map[int]shadowing.SentenceAttemptSummary{}
	for _, row := range summary {
		bySentence[row.SentenceIndex] = row
	}

	sentence1 := bySentence[1]
	if sentence1.AttemptCount != 3 {
		t.Fatalf("sentence 1 attempt_count = %d, want 3", sentence1.AttemptCount)
	}
	if sentence1.BestScore == nil || *sentence1.BestScore != 80 {
		t.Fatalf("sentence 1 best_score = %v, want 80", sentence1.BestScore)
	}
	if sentence1.LastScore == nil || *sentence1.LastScore != 70 {
		t.Fatalf("sentence 1 last_score = %v, want 70", sentence1.LastScore)
	}

	sentence2 := bySentence[2]
	if sentence2.AttemptCount != 1 {
		t.Fatalf("sentence 2 attempt_count = %d, want 1", sentence2.AttemptCount)
	}
	if sentence2.BestScore != nil {
		t.Fatalf("sentence 2 best_score = %v, want nil", sentence2.BestScore)
	}
	if sentence2.LastScore != nil {
		t.Fatalf("sentence 2 last_score = %v, want nil", sentence2.LastScore)
	}
}

func TestShadowingStore_VersionIsolationForSummaryAndCompletion(t *testing.T) {
	store := NewShadowingStore(testDB)
	userID := int64(9603)
	insertTestUser(t, userID, "shadowing_version@example.com")
	lessonID := insertTestLesson(t, "Shadowing Version Isolation", lesson.LevelN5, `["shadowing"]`)

	score60 := 60
	score90 := 90
	attempts := []shadowing.Attempt{
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 1, SentenceIndex: 0, PracticeMode: shadowing.PracticeModeRecord, PlaybackRate: 1, LoopCount: 0, SelfScore: &score60},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 2, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeRecord, PlaybackRate: 1, LoopCount: 0, SelfScore: &score90},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 2, SentenceIndex: 2, PracticeMode: shadowing.PracticeModeLoop, PlaybackRate: 0.8, LoopCount: 3, SelfScore: nil},
		{UserID: userID, LessonID: lessonID, ShadowingVersion: 2, SentenceIndex: 1, PracticeMode: shadowing.PracticeModeLoop, PlaybackRate: 0.8, LoopCount: 1, SelfScore: nil},
	}
	for i, attempt := range attempts {
		if err := store.InsertAttempt(attempt); err != nil {
			t.Fatalf("InsertAttempt %d error: %v", i, err)
		}
	}

	summary, err := store.ListAttemptSummary(userID, lessonID, 2)
	if err != nil {
		t.Fatalf("ListAttemptSummary error: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("summary len = %d, want 2: %+v", len(summary), summary)
	}
	for _, row := range summary {
		if row.SentenceIndex == 0 {
			t.Fatalf("summary included old version sentence: %+v", summary)
		}
	}

	completed, err := store.ListCompletedSentenceIndexes(userID, lessonID, 2)
	if err != nil {
		t.Fatalf("ListCompletedSentenceIndexes error: %v", err)
	}
	if len(completed) != 2 || completed[0] != 1 || completed[1] != 2 {
		t.Fatalf("completed = %+v, want [1 2]", completed)
	}
}
