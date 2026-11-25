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

func TestWordReviewPlanTracksRatingsAndChecksInAtTarget(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 17, 0, 0, 0, time.UTC) // Friday in the saved plan's timezone.
	p := slottedPlan()
	p.TimeSlots[0].Tasks[0].Title = "复习２０个单词"
	p.TimeSlots[0].Tasks[0].Path = "/words/review?level=N2"
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'word-plan@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token,plan_id) VALUES(1,$1,'calendar-word','plan-word')`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,encrypted_secret) VALUES(1,'connection-word','test-key','dictionary','test-workspace','test-secret')`); err != nil {
		t.Fatal(err)
	}
	insertWord := func(n int, level string, at time.Time) {
		t.Helper()
		var id int64
		if err := db.QueryRow(`INSERT INTO words(kanji_form,reading,meaning,jlpt_level) VALUES($1,'テスト','词',$2) RETURNING id`, fmt.Sprintf("テスト%d", n), level).Scan(&id); err != nil {
			t.Fatal(err)
		}
		// Two ratings of the same word still contribute one item.
		if _, err := db.Exec(`INSERT INTO word_records(user_id,word_id,next_review_at,review_history_json) VALUES(1,$1,$2,jsonb_build_array(jsonb_build_object('reviewed_at',$2::timestamptz),jsonb_build_object('reviewed_at',$2::timestamptz)))`, id, at); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 19; i++ {
		insertWord(i, "N2", now.Add(-30*time.Minute))
	}
	insertWord(21, "N5", now)
	insertWord(22, "N2", now.Add(-24*time.Hour))
	h := NewHandler(db)
	h.now = func() time.Time { return now }
	assertTask := func(wantCount int, wantDone bool) {
		t.Helper()
		today, err := h.readToday(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		task := today.Slots[0].Tasks[0]
		if task.WordReview == nil || task.WordReview.Target != 20 || task.WordReview.Completed != wantCount || task.Completed != wantDone {
			t.Fatalf("task = %+v, progress = %+v, want %d/20, completed=%t", task, task.WordReview, wantCount, wantDone)
		}
	}
	assertTask(19, false)
	insertWord(20, "N2", now)
	assertTask(20, true)
	assertTask(20, true)
	var total int
	if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox WHERE user_id=1 AND payload_json::jsonb->>'reason'='word_review_goal'`).Scan(&total); err != nil || total != 1 {
		t.Fatalf("completion events = %d, err = %v", total, err)
	}
	now = now.AddDate(0, 0, 3)
	assertTask(0, false)
}

func TestWordReviewTargetUsesExplicitConfigBeforeLegacyTitle(t *testing.T) {
	manual, target := 0, 12
	for _, tc := range []struct {
		name string
		task Task
		want int
	}{
		{"legacy quantity", Task{Title: "复习 ２０ 个单词"}, 20},
		{"manual override", Task{Title: "复习20个单词", ReviewTarget: &manual}, 0},
		{"explicit quantity", Task{Title: "词汇巩固", ReviewTarget: &target}, 12},
		{"ambiguous title", Task{Title: "花20分钟复习词汇"}, 0},
		{"out of range", Task{Title: "复习201个单词"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wordReviewTarget(tc.task); got != tc.want {
				t.Fatalf("target = %d, want %d", got, tc.want)
			}
		})
	}
}
