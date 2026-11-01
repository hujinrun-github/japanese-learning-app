package data

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestUserStore_Create(t *testing.T) {
	store := &UserStore{db: testDB}

	u, err := store.Create("", "newuser@example.com", "hashedpwd", `["N5"]`)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if u == nil {
		t.Fatal("Create returned nil user")
	}
	if u.ID == 0 {
		t.Error("Create returned user with ID=0")
	}
	if u.Email != "newuser@example.com" {
		t.Errorf("Email = %q, want %q", u.Email, "newuser@example.com")
	}
	if len(u.JLPTLevels) == 0 || u.JLPTLevels[0] != "N5" {
		t.Errorf("JLPTLevels = %v, want [N5]", u.JLPTLevels)
	}
}

func TestUserStore_Create_DuplicateEmail(t *testing.T) {
	store := &UserStore{db: testDB}

	email := "duplicate@example.com"
	_, err := store.Create("", email, "hash1", `["N5"]`)
	if err != nil {
		t.Fatalf("first Create error: %v", err)
	}

	_, err = store.Create("", email, "hash2", `["N4"]`)
	if err == nil {
		t.Fatal("second Create with duplicate email expected error, got nil")
	}
}

func TestUserStore_GetByEmail(t *testing.T) {
	store := &UserStore{db: testDB}

	email := "getbyemail@example.com"
	created, err := store.Create("", email, "securehash", `["N4"]`)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := store.GetByEmail(email)
	if err != nil {
		t.Fatalf("GetByEmail error: %v", err)
	}
	if got == nil {
		t.Fatal("GetByEmail returned nil")
	}
	if got.ID != created.ID {
		t.Errorf("GetByEmail ID = %d, want %d", got.ID, created.ID)
	}
	if got.Email != email {
		t.Errorf("GetByEmail Email = %q, want %q", got.Email, email)
	}
}

func TestUserStore_GetByEmail_NotFound(t *testing.T) {
	store := &UserStore{db: testDB}

	got, err := store.GetByEmail("nonexistent@example.com")
	if err == nil {
		t.Fatal("GetByEmail(nonexistent) expected error, got nil")
	}
	if got != nil {
		t.Errorf("GetByEmail(nonexistent) expected nil, got %+v", got)
	}
}

func TestUserStore_GetByID(t *testing.T) {
	store := &UserStore{db: testDB}

	created, err := store.Create("", "getbyid@example.com", "hash", `["N3"]`)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := store.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetByID ID = %d, want %d", got.ID, created.ID)
	}
}

func TestUserStore_UpdateStreak(t *testing.T) {
	store := &UserStore{db: testDB}

	created, err := store.Create("", "streak@example.com", "hash", `["N5"]`)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := store.UpdateStreak(created.ID, 5); err != nil {
		t.Fatalf("UpdateStreak error: %v", err)
	}

	got, err := store.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after UpdateStreak error: %v", err)
	}
	if got.StreakDays != 5 {
		t.Errorf("StreakDays = %d, want 5", got.StreakDays)
	}
}

func TestUserStore_UpdatePassword_NotFound(t *testing.T) {
	store := &UserStore{db: testDB}

	err := store.UpdatePassword(999999, "new-hash")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("UpdatePassword missing user error = %v, want sql.ErrNoRows", err)
	}
}

// TestUserStore_CountTodayCompleted_UsesReviewEvents pins the today counter to
// real review/quiz events instead of updated_at. A record whose updated_at is
// fresh but which has no review event today must not be counted.
func TestUserStore_CountTodayCompleted_UsesReviewEvents(t *testing.T) {
	store := &UserStore{db: testDB}

	insertWord := func(kanji string) int64 {
		res, err := testDB.Exec(
			// N1 keeps these fixtures out of the seeded N5/N4 word counts that
			// other tests in this package assert on.
			`INSERT INTO words (kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level)
			 VALUES (?, ?, '', ?, '[]', 'N1')`, kanji, "よみ", "いみ",
		)
		if err != nil {
			t.Fatalf("insert word %q: %v", kanji, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id for %q: %v", kanji, err)
		}
		return id
	}

	todayEvent := `[{"rating":"easy","reviewed_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}]`
	staleEvent := `[{"rating":"normal","reviewed_at":"` + time.Now().UTC().AddDate(0, 0, -3).Format(time.RFC3339) + `"}]`

	cases := []struct {
		name     string
		history  string
		wantWord int
	}{
		{"reviewed today counts", todayEvent, 1},
		{"stale review with fresh updated_at does not count", staleEvent, 0},
		{"no review events does not count", `[]`, 0},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A dedicated user per case: the counter is cumulative per user,
			// so sharing one user would leak rows between cases.
			user, err := store.Create("", fmt.Sprintf("today-counter-%d@example.com", i), "hash", `["N5"]`)
			if err != nil {
				t.Fatalf("Create error: %v", err)
			}

			wordID := insertWord(fmt.Sprintf("count-%d", i))
			if _, err := testDB.Exec(
				`INSERT INTO word_records (user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at)
				 VALUES (?, ?, 1, datetime('now'), 2.5, 1, ?, datetime('now'))`,
				user.ID, wordID, tc.history,
			); err != nil {
				t.Fatalf("insert word_record: %v", err)
			}

			if got := store.countTodayCompleted(user.ID, "word"); got != tc.wantWord {
				t.Errorf("countTodayCompleted(word) = %d, want %d", got, tc.wantWord)
			}
		})
	}
}

// TestUserStore_CountTodayCompleted_GrammarUsesQuizAttempts mirrors the word
// counter for grammar: only attempts made today are counted.
func TestUserStore_CountTodayCompleted_GrammarUsesQuizAttempts(t *testing.T) {
	store := &UserStore{db: testDB}

	user, err := store.Create("", "today-grammar@example.com", "hash", `["N5"]`)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	pointID := int64(0)
	if err := testDB.QueryRow(
		`INSERT INTO grammar_points (name, meaning, jlpt_level) VALUES ('〜テスト', '测试', 'N5') RETURNING id`,
	).Scan(&pointID); err != nil {
		t.Fatalf("insert grammar_point: %v", err)
	}

	if _, err := testDB.Exec(
		`INSERT INTO grammar_records (user_id, grammar_point_id, status, next_review_at, quiz_history_json)
		 VALUES (?, ?, 'learning', datetime('now'), ?)`,
		user.ID, pointID,
		`[{"score":80,"attempted_at":"`+time.Now().UTC().Format(time.RFC3339)+`"}]`,
	); err != nil {
		t.Fatalf("insert grammar_record: %v", err)
	}

	if got := store.countTodayCompleted(user.ID, "grammar"); got != 1 {
		t.Errorf("countTodayCompleted(grammar) = %d, want 1", got)
	}
}

// TestUserStore_RecordStudyDay pins the streak transition rules:
// hold on a repeat check-in, advance on a consecutive day, restart after a gap.
func TestUserStore_RecordStudyDay(t *testing.T) {
	store := &UserStore{db: testDB}

	cases := []struct {
		name string
		// seed is written before the check-in; empty means no check-in yet.
		seedLastStudyDate string
		seedStreak        int
		// daysBack is how long ago "yesterday" is relative to today.
		wantStreak int
	}{
		{name: "first ever check-in starts at one", seedStreak: 5, wantStreak: 1},
		{name: "repeat check-in holds the streak", seedLastStudyDate: todayDateUTC(), seedStreak: 4, wantStreak: 4},
		{name: "consecutive day advances", seedLastStudyDate: daysAgoDateUTC(1), seedStreak: 4, wantStreak: 5},
		{name: "gap restarts at one", seedLastStudyDate: daysAgoDateUTC(3), seedStreak: 9, wantStreak: 1},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, err := store.Create("", fmt.Sprintf("streak-%d@example.com", i), "hash", `["N5"]`)
			if err != nil {
				t.Fatalf("Create error: %v", err)
			}

			var seedDate any
			if tc.seedLastStudyDate != "" {
				seedDate = tc.seedLastStudyDate
			}
			if _, err := testDB.Exec(
				`UPDATE users SET streak_days = ?, last_study_date = ? WHERE id = ?`,
				tc.seedStreak, seedDate, user.ID,
			); err != nil {
				t.Fatalf("seed user: %v", err)
			}

			got, err := store.RecordStudyDay(user.ID)
			if err != nil {
				t.Fatalf("RecordStudyDay error: %v", err)
			}
			if got != tc.wantStreak {
				t.Errorf("RecordStudyDay = %d, want %d", got, tc.wantStreak)
			}

			var storedDate sql.NullString
			if err := testDB.QueryRow(
				`SELECT last_study_date FROM users WHERE id = ?`, user.ID,
			).Scan(&storedDate); err != nil {
				t.Fatalf("read back last_study_date: %v", err)
			}
			if !storedDate.Valid || storedDate.String != todayDateUTC() {
				t.Errorf("last_study_date = %v, want %q", storedDate, todayDateUTC())
			}
		})
	}
}

func todayDateUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}

func daysAgoDateUTC(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
}
