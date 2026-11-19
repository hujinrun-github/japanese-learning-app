package studyplan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/testdb"
)

func TestCompleteListeningTask(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	for i, tc := range []struct {
		name, now, path, date string
		noPlan                bool
		want                  int
	}{
		{"internal link", "2026-11-20T04:00:00Z", "/listening?level=N2", "2026-11-20", false, 1},
		{"legacy official link", "2026-11-19T16:01:00Z", "https://www.jlpt.jp/e/samples/sampleindex.html", "2026-11-20", false, 1},
		{"detail link", "2026-11-20T04:00:00Z", "/listening/12", "2026-11-20", false, 1},
		{"rest day", "2026-11-20T16:01:00Z", "/listening", "2026-11-21", false, 0},
		{"unrelated custom target", "2026-11-20T04:00:00Z", "/lesson?level=N2", "2026-11-20", false, 0},
		{"no plan", "2026-11-20T04:00:00Z", "/listening", "2026-11-20", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uid := int64(i + 1)
			if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES($1,$2,'test')`, uid, tc.name+"@example.test"); err != nil {
				t.Fatal(err)
			}
			p := slottedPlan()
			p.TimeSlots[1].Tasks[0].Path = tc.path
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.noPlan {
				if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token) VALUES($1,$2,$3)`, uid, string(raw), tc.name); err != nil {
					t.Fatal(err)
				}
			}
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			for n := 0; n < 2; n++ {
				if err := CompleteListeningTask(context.Background(), db, uid, now); err != nil {
					t.Fatal(err)
				}
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM study_plan_task_completions WHERE user_id=$1`, uid).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != tc.want {
				t.Fatalf("completions=%d, want %d", count, tc.want)
			}
			if tc.want > 0 {
				var date, slot, task string
				if err := db.QueryRow(`SELECT study_date::text,slot_id,task_id FROM study_plan_task_completions WHERE user_id=$1`, uid).Scan(&date, &slot, &task); err != nil {
					t.Fatal(err)
				}
				if date != tc.date || slot != "evening" || task != "listening" {
					t.Fatalf("wrong completion: %s %s %s", date, slot, task)
				}
			}
		})
	}

	// The listening store can pass its transaction: a rollback must roll back
	// the task completion together with the training completion.
	if _, err := db.Exec(`DELETE FROM study_plan_task_completions WHERE user_id=1`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteListeningTask(context.Background(), tx, 1, time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM study_plan_task_completions WHERE user_id=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("completion escaped training transaction")
	}
}
