package data

import (
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

// TestWordStore_ListReviewCandidates exercises the SQLite implementation of
// the same batched contract the PostgreSQL store uses. A dedicated user id
// keeps these fixtures isolated from the shared seed database.
func TestWordStore_ListReviewCandidates(t *testing.T) {
	store := &WordStore{db: testDB}
	const userID int64 = 987654

	var firstN5 int64
	if err := testDB.QueryRow(`SELECT id FROM words WHERE jlpt_level = 'N5' ORDER BY id LIMIT 1`).Scan(&firstN5); err != nil {
		t.Fatalf("find a seeded N5 word: %v", err)
	}

	tests := []struct {
		name        string
		level       word.JLPTLevel
		dueLimit    int
		newLimit    int
		seedRecord  bool
		updatedAt   time.Time
		wantDue     int
		wantNew     int
		wantDueWord int64
	}{
		{
			name: "无记录时只取 newLimit 个新词", level: word.LevelN5,
			dueLimit: 1, newLimit: 3, wantDue: 0, wantNew: 3,
		},
		{
			name: "到期词作为候选返回", level: word.LevelN5,
			dueLimit: 1, newLimit: 2, seedRecord: true,
			updatedAt: time.Now().AddDate(0, 0, -3),
			wantDue:   1, wantNew: 2, wantDueWord: firstN5,
		},
		{
			name: "今天复习过的到期词被排除", level: word.LevelN5,
			dueLimit: 1, newLimit: 2, seedRecord: true,
			updatedAt: time.Now(),
			wantDue:   0, wantNew: 2,
		},
		{
			name: "等级隔离", level: word.LevelN4,
			dueLimit: 1, newLimit: 2, wantDue: 0, wantNew: 2,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each case gets its own user so the shared fixture stays isolated.
			caseUser := userID + int64(i)
			if tt.seedRecord {
				if _, err := testDB.Exec(
					`INSERT INTO users (id, email, password_hash) VALUES (?, ?, 'hash')`,
					caseUser, fmt.Sprintf("queue-probe-%d@example.com", caseUser),
				); err != nil {
					t.Fatalf("insert user: %v", err)
				}
				if _, err := testDB.Exec(
					`INSERT INTO word_records
					   (user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at)
					 VALUES (?, ?, 1, ?, 2.5, 1, '[]', ?)`,
					caseUser, firstN5, formatSQLiteTime(time.Now().Add(-48*time.Hour)), formatSQLiteTime(tt.updatedAt),
				); err != nil {
					t.Fatalf("insert record: %v", err)
				}
			}

			got, err := store.ListReviewCandidates(caseUser, tt.level, tt.dueLimit, tt.newLimit)
			if err != nil {
				t.Fatalf("ListReviewCandidates error: %v", err)
			}

			due, fresh := 0, 0
			for _, c := range got {
				if c.Record != nil {
					due++
					if c.Record.WordID != tt.wantDueWord {
						t.Errorf("due record word = %d, want %d", c.Record.WordID, tt.wantDueWord)
					}
				} else {
					fresh++
				}
				if c.Word.JLPTLevel != tt.level {
					t.Errorf("word %d level = %s, want %s", c.Word.ID, c.Word.JLPTLevel, tt.level)
				}
			}
			if due != tt.wantDue {
				t.Errorf("due count = %d, want %d", due, tt.wantDue)
			}
			if fresh != tt.wantNew {
				t.Errorf("new count = %d, want %d", fresh, tt.wantNew)
			}
			if len(got) > tt.dueLimit+tt.newLimit {
				t.Errorf("len = %d, exceeds dueLimit+newLimit = %d", len(got), tt.dueLimit+tt.newLimit)
			}
		})
	}
}
