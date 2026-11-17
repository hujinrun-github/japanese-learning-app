package postgres

import (
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

// TestWordStoreListReviewCandidates exercises the real SQL behind the review
// queue against PostgreSQL: LIMIT pushdown, level scoping, same-day exclusion,
// ordering and batched example loading.
func TestWordStoreListReviewCandidates(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresWordStoreForDB(db)

	// The store clock is fixed at 2026-06-17 09:00 UTC by the test helper.
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	overdue := now.Add(-48 * time.Hour)
	reviewedToday := todayStart.Add(8 * time.Hour)
	future := now.Add(72 * time.Hour)

	// Five N5 words and one N4 word, each with two examples.
	ids := make([]int64, 0, 6)
	for i := 1; i <= 6; i++ {
		level := word.LevelN5
		if i == 6 {
			level = word.LevelN4
		}
		var id int64
		if err := db.QueryRow(
			`INSERT INTO words (kanji_form, reading, meaning, jlpt_level)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			fmt.Sprintf("語%d", i), fmt.Sprintf("ご%d", i), "释义", level,
		).Scan(&id); err != nil {
			t.Fatalf("insert word %d: %v", i, err)
		}
		ids = append(ids, id)
		for pos := 0; pos < 2; pos++ {
			if _, err := db.Exec(
				`INSERT INTO word_examples (word_id, position, japanese, chinese, furigana_html)
				 VALUES ($1, $2, $3, $4, '')`,
				id, pos, fmt.Sprintf("例句%d-%d", i, pos), "译文",
			); err != nil {
				t.Fatalf("insert example for word %d: %v", id, err)
			}
		}
	}

	insertUser := func(email string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(
			`INSERT INTO users (email, name, password_hash) VALUES ($1, $2, 'hash') RETURNING id`,
			email, "review user",
		).Scan(&id); err != nil {
			t.Fatalf("insert user %s: %v", email, err)
		}
		return id
	}
	insertRecord := func(userID, wordID int64, nextReviewAt, updatedAt time.Time) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO word_records
			   (user_id, word_id, mastery_level, next_review_at, ease_factor, interval_days, review_history_json, updated_at)
			 VALUES ($1, $2, 1, $3, 2.5, 1, '[]'::jsonb, $4)`,
			userID, wordID, nextReviewAt, updatedAt,
		); err != nil {
			t.Fatalf("insert record user=%d word=%d: %v", userID, wordID, err)
		}
	}

	// Each case uses its own user so fixtures do not interfere.
	users := map[string]int64{
		"due_order":  insertUser("queue-due-order@example.com"),
		"today":      insertUser("queue-today@example.com"),
		"level":      insertUser("queue-level@example.com"),
		"limits":     insertUser("queue-limits@example.com"),
		"no_records": insertUser("queue-none@example.com"),
	}

	insertRecord(users["due_order"], ids[0], overdue.Add(-3*time.Hour), overdue)
	insertRecord(users["due_order"], ids[1], overdue.Add(-2*time.Hour), overdue)
	insertRecord(users["due_order"], ids[2], overdue.Add(-1*time.Hour), overdue)

	insertRecord(users["today"], ids[0], overdue, reviewedToday)
	insertRecord(users["today"], ids[1], overdue, overdue)
	insertRecord(users["today"], ids[2], future, overdue) // not due yet

	insertRecord(users["level"], ids[0], overdue, overdue)
	insertRecord(users["limits"], ids[0], overdue, overdue)
	insertRecord(users["limits"], ids[1], overdue, overdue)

	tests := []struct {
		name         string
		userKey      string
		level        word.JLPTLevel
		dueLimit     int
		newLimit     int
		wantIDs      []int64
		wantDueFlags []bool
	}{
		{
			name: "到期词按到期时间排序且最多 dueLimit 张", userKey: "due_order",
			level: word.LevelN5, dueLimit: 2, newLimit: 3,
			wantIDs:      []int64{ids[0], ids[1], ids[3], ids[4]},
			wantDueFlags: []bool{true, true, false, false},
		},
		{
			name: "今天已复习的到期词被排除", userKey: "today",
			level: word.LevelN5, dueLimit: 2, newLimit: 2,
			wantIDs:      []int64{ids[1], ids[3], ids[4]},
			wantDueFlags: []bool{true, false, false},
		},
		{
			name: "等级隔离", userKey: "level",
			level: word.LevelN4, dueLimit: 2, newLimit: 5,
			wantIDs:      []int64{ids[5]},
			wantDueFlags: []bool{false},
		},
		{
			name: "两个 LIMIT 都下推到 SQL", userKey: "limits",
			level: word.LevelN5, dueLimit: 1, newLimit: 1,
			wantIDs:      []int64{ids[0], ids[2]},
			wantDueFlags: []bool{true, false},
		},
		{
			name: "没有学习记录时全是新词", userKey: "no_records",
			level: word.LevelN5, dueLimit: 1, newLimit: 2,
			wantIDs:      []int64{ids[0], ids[1]},
			wantDueFlags: []bool{false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.ListReviewCandidates(users[tt.userKey], tt.level, tt.dueLimit, tt.newLimit)
			if err != nil {
				t.Fatalf("ListReviewCandidates error: %v", err)
			}
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("len = %d, want %d (got=%+v)", len(got), len(tt.wantIDs), got)
			}
			for i, c := range got {
				if c.Word.ID != tt.wantIDs[i] {
					t.Errorf("candidates[%d].Word.ID = %d, want %d", i, c.Word.ID, tt.wantIDs[i])
				}
				if (c.Record != nil) != tt.wantDueFlags[i] {
					t.Errorf("candidates[%d] due = %v, want %v", i, c.Record != nil, tt.wantDueFlags[i])
				}
				if c.Word.JLPTLevel != tt.level {
					t.Errorf("candidates[%d].JLPTLevel = %s, want %s", i, c.Word.JLPTLevel, tt.level)
				}
				if len(c.Word.Examples) != 2 {
					t.Errorf("candidates[%d] examples = %d, want 2 (batched load)", i, len(c.Word.Examples))
				}
				if c.Record != nil && c.Record.WordID != c.Word.ID {
					t.Errorf("candidates[%d] record.WordID = %d, want %d", i, c.Record.WordID, c.Word.ID)
				}
			}
		})
	}
}
