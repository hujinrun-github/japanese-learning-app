package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
	storepkg "japanese-learning-app/internal/store"
)

func TestUserStoreCreateUser(t *testing.T) {
	cases := []struct {
		name       string
		input      user.User
		wantLevels []string
	}{
		{
			name: "explicit levels",
			input: user.User{
				Name:       "N4 User",
				Email:      "n4@example.com",
				JLPTLevels: []string{"N4"},
			},
			wantLevels: []string{"N4"},
		},
		{
			name: "default level",
			input: user.User{
				Name:  "Default User",
				Email: "default@example.com",
			},
			wantLevels: []string{"N5"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newPostgresUserStoreForTest(t)

			created, err := store.CreateUser(tc.input, "hashedpwd")
			if err != nil {
				t.Fatalf("CreateUser() error = %v", err)
			}
			if created == nil {
				t.Fatal("CreateUser() returned nil user")
			}
			if created.ID == 0 {
				t.Fatal("CreateUser() returned ID = 0")
			}
			if created.Email != tc.input.Email {
				t.Fatalf("CreateUser() email = %q, want %q", created.Email, tc.input.Email)
			}
			if len(created.JLPTLevels) != len(tc.wantLevels) || created.JLPTLevels[0] != tc.wantLevels[0] {
				t.Fatalf("CreateUser() levels = %v, want %v", created.JLPTLevels, tc.wantLevels)
			}
		})
	}
}

func TestUserStoreCreateUserDuplicateEmail(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	_, err := store.CreateUser(user.User{
		Name:       "First User",
		Email:      "duplicate@example.com",
		JLPTLevels: []string{"N5"},
	}, "hash1")
	if err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	_, err = store.CreateUser(user.User{
		Name:       "Second User",
		Email:      "duplicate@example.com",
		JLPTLevels: []string{"N4"},
	}, "hash2")
	if !errors.Is(err, user.ErrEmailTaken) {
		t.Fatalf("second CreateUser() error = %v, want ErrEmailTaken", err)
	}
}

func TestUserStoreGetUserByEmail(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	created, err := store.CreateUser(user.User{
		Name:       "Lookup User",
		Email:      "lookup@example.com",
		JLPTLevels: []string{"N3"},
	}, "securehash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, hash, err := store.GetUserByEmail("lookup@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetUserByEmail() returned nil user")
	}
	if got.ID != created.ID {
		t.Fatalf("GetUserByEmail() ID = %d, want %d", got.ID, created.ID)
	}
	if hash != "securehash" {
		t.Fatalf("GetUserByEmail() hash = %q, want %q", hash, "securehash")
	}
}

func TestUserStoreGetUserByEmailNotFound(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	got, _, err := store.GetUserByEmail("missing@example.com")
	if got != nil {
		t.Fatalf("GetUserByEmail() user = %+v, want nil", got)
	}
	if !errors.Is(err, storepkg.ErrNotFound) {
		t.Fatalf("GetUserByEmail() error = %v, want ErrNotFound", err)
	}
}

func TestUserStoreGetUserByID(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	created, err := store.CreateUser(user.User{
		Name:       "ByID User",
		Email:      "byid@example.com",
		JLPTLevels: []string{"N2"},
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, err := store.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("GetUserByID() user = %+v, want ID %d", got, created.ID)
	}
}

func TestUserStorePasswordResetLifecycle(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	created, err := store.CreateUser(user.User{
		Name:       "Reset User",
		Email:      "reset@example.com",
		JLPTLevels: []string{"N5"},
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	if err := store.CreateResetToken("token-123", created.ID, expiresAt); err != nil {
		t.Fatalf("CreateResetToken() error = %v", err)
	}

	token, err := store.GetResetToken("token-123")
	if err != nil {
		t.Fatalf("GetResetToken() error = %v", err)
	}
	if token.UserID != created.ID {
		t.Fatalf("GetResetToken() user ID = %d, want %d", token.UserID, created.ID)
	}
	if token.Used {
		t.Fatal("GetResetToken() used = true, want false")
	}

	if err := store.MarkTokenUsed("token-123"); err != nil {
		t.Fatalf("MarkTokenUsed() error = %v", err)
	}

	token, err = store.GetResetToken("token-123")
	if err != nil {
		t.Fatalf("GetResetToken() after MarkTokenUsed error = %v", err)
	}
	if !token.Used {
		t.Fatal("GetResetToken() used = false, want true")
	}
}

func TestUserStoreUpdatePasswordNotFound(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	err := store.UpdatePassword(999999, "new-hash")
	if !errors.Is(err, storepkg.ErrNotFound) {
		t.Fatalf("UpdatePassword() error = %v, want ErrNotFound", err)
	}
}

func TestUserStoreGetStatsIncludesTranslation(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	userStore := newPostgresUserStoreForDB(db)
	translationStore := NewTranslationStore(db, storepkg.StoreDeps{
		Clock:       fixedClock{now: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)},
		AppTimezone: time.UTC,
	})

	created, err := userStore.CreateUser(user.User{
		Name:  "Translation Stats",
		Email: "translation-stats@example.com",
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser error: %v", err)
	}
	sourceID, err := translationStore.SaveSource(translation.TranslationSource{
		Title: "Daily goal test", SourceType: "manual",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	records := []struct {
		name        string
		score       int
		practicedAt time.Time
	}{
		{name: "mastered today", score: 90, practicedAt: time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)},
		{name: "attempted today", score: 70, practicedAt: time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)},
		{name: "mastered before today", score: 85, practicedAt: time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)},
	}
	for position, record := range records {
		t.Run(record.name, func(t *testing.T) {
			sentenceID, saveErr := translationStore.SaveSentence(translation.TranslationSentence{
				SourceID: sourceID, Direction: "cn2jp", SourceText: record.name, Position: position,
			})
			if saveErr != nil {
				t.Fatalf("SaveSentence error: %v", saveErr)
			}
			if _, saveErr = translationStore.SaveRecord(translation.TranslationRecord{
				UserID: created.ID, SentenceID: sentenceID, UserTranslation: "訳文",
				Score: record.score, RuleScore: record.score, PracticedAt: record.practicedAt,
			}); saveErr != nil {
				t.Fatalf("SaveRecord error: %v", saveErr)
			}
		})
	}

	stats, err := userStore.GetStats(created.ID)
	if err != nil {
		t.Fatalf("GetStats error: %v", err)
	}
	translationStat, ok := stats.ModuleStats["translation"]
	if !ok {
		t.Fatal("GetStats modules does not contain translation")
	}
	if translationStat.TotalCount != len(records) {
		t.Errorf("translation TotalCount = %d, want %d", translationStat.TotalCount, len(records))
	}
	if translationStat.MasteredCount != 2 {
		t.Errorf("translation MasteredCount = %d, want 2", translationStat.MasteredCount)
	}
	if translationStat.TodayCompleted != 2 {
		t.Errorf("translation TodayCompleted = %d, want 2", translationStat.TodayCompleted)
	}
	if translationStat.DailyGoal != 5 {
		t.Errorf("translation DailyGoal = %d, want 5", translationStat.DailyGoal)
	}
}

// TestUserStoreRecordStudyDay pins the streak transition rules on PostgreSQL.
// The test store uses a fixed clock (2026-06-17 09:00 UTC) and UTC, so
// "today" is 2026-06-17 throughout.
func TestUserStoreRecordStudyDay(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresUserStoreForDB(db)

	cases := []struct {
		name       string
		seedDate   any // nil means no previous check-in
		seedStreak int
		wantStreak int
	}{
		{name: "first ever check-in starts at one", seedStreak: 5, wantStreak: 1},
		{name: "repeat check-in holds the streak", seedDate: "2026-06-17", seedStreak: 4, wantStreak: 4},
		{name: "consecutive day advances", seedDate: "2026-06-16", seedStreak: 4, wantStreak: 5},
		{name: "gap restarts at one", seedDate: "2026-06-12", seedStreak: 9, wantStreak: 1},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := store.CreateUser(user.User{
				Email: fmt.Sprintf("streak-%d@example.com", i),
				Name:  "streak",
			}, "hash")
			if err != nil {
				t.Fatalf("CreateUser error: %v", err)
			}

			if _, err := db.Exec(
				`UPDATE users SET streak_days = $1, last_study_date = $2 WHERE id = $3`,
				tc.seedStreak, tc.seedDate, created.ID,
			); err != nil {
				t.Fatalf("seed user: %v", err)
			}

			got, err := store.RecordStudyDay(created.ID)
			if err != nil {
				t.Fatalf("RecordStudyDay error: %v", err)
			}
			if got != tc.wantStreak {
				t.Errorf("RecordStudyDay = %d, want %d", got, tc.wantStreak)
			}

			var stored sql.NullString
			if err := db.QueryRow(
				`SELECT last_study_date::text FROM users WHERE id = $1`, created.ID,
			).Scan(&stored); err != nil {
				t.Fatalf("read back last_study_date: %v", err)
			}
			if !stored.Valid || stored.String != "2026-06-17" {
				t.Errorf("last_study_date = %v, want 2026-06-17", stored)
			}
		})
	}
}

// TestUserStoreRecordStudyDayUnknownUser keeps a missing user from silently
// reporting a streak.
func TestUserStoreRecordStudyDayUnknownUser(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	if _, err := store.RecordStudyDay(999999); err == nil {
		t.Fatal("RecordStudyDay error = nil, want an error for an unknown user")
	}
}
