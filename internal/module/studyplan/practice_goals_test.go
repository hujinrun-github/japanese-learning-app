package studyplan

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/testdb"
)

func TestGrammarPracticeGoalUsesPlanTimezoneAndDistinctPoints(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 17, 0, 0, 0, time.UTC)
	p := slottedPlan()
	p.TimeSlots = p.TimeSlots[:1]
	p.TimeSlots[0].Tasks = []Task{{ID: "grammar", Title: "语法２项", Path: "/grammar?level=N2", Minutes: 20}}
	save := func() {
		t.Helper()
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token,plan_id)
			VALUES(1,$1,'calendar-grammar','plan-grammar') ON CONFLICT(user_id) DO UPDATE SET plan_json=excluded.plan_json`, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'grammar-plan@example.test','test');
		INSERT INTO grammar_points(id,name,meaning,jlpt_level) VALUES
		(1,'〜ほど','程度','N2'), (2,'〜ながら','一边','N2'), (3,'〜です','是','N5'), (4,'〜にもかかわらず','尽管','N2');
		INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,encrypted_secret)
		VALUES(1,'connection-grammar','test-key','dictionary','test-workspace','test-secret')`); err != nil {
		t.Fatal(err)
	}
	save()
	attempt := func(id int, at time.Time) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO grammar_records(user_id,grammar_point_id,quiz_history_json)
			VALUES(1,$1,jsonb_build_array(jsonb_build_object('attempted_at',$2::timestamptz),jsonb_build_object('attempted_at',$2::timestamptz)))`, id, at); err != nil {
			t.Fatal(err)
		}
	}
	attempt(1, now.Add(-30*time.Minute))
	attempt(3, now)
	attempt(4, now.Add(-2*time.Hour))
	h := NewHandler(db)
	h.now = func() time.Time { return now }
	assertTask := func(count, target int, checked bool) {
		t.Helper()
		today, err := h.readToday(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		task := today.Slots[0].Tasks[0]
		if task.Progress == nil || task.Progress.Kind != "grammar" || task.Progress.Level != "N2" ||
			task.Progress.Completed != count || task.Progress.Target != target || task.Completed != checked {
			t.Fatalf("task = %+v, progress = %+v, want %d/%d, completed=%t", task, task.Progress, count, target, checked)
		}
		if want := fmt.Sprintf("语法%d项", target); task.Title != want {
			t.Fatalf("task title = %q, want %q", task.Title, want)
		}
	}
	assertTask(1, 2, false)
	attempt(2, now)
	assertTask(2, 2, true)
	assertTask(2, 2, true)
	var events int
	if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox WHERE user_id=1 AND payload_json::jsonb->>'reason'='grammar_goal'`).Scan(&events); err != nil || events != 1 {
		t.Fatalf("completion events = %d, err = %v", events, err)
	}
	raised := 3
	p.TimeSlots[0].Tasks[0].PracticeTarget = &raised
	save()
	assertTask(2, 3, false)
	now = now.AddDate(0, 0, 3)
	assertTask(0, 3, false)
}
