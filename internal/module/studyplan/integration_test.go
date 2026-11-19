package studyplan

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestStudyTaskIdentityCannotBeReusedWithinPlan(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'identity@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db)
	public, protected := http.NewServeMux(), http.NewServeMux()
	h.RegisterRoutes(protected, public)
	authenticated := user.AuthMiddleware("identity-jwt", protected)
	token, _, err := user.SignToken(1, "identity-jwt", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	put := func(plan Plan, want int) {
		t.Helper()
		raw, _ := json.Marshal(plan)
		req := httptest.NewRequest("PUT", "/api/v1/study-plan", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		authenticated.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status=%d want=%d: %s", w.Code, want, w.Body.String())
		}
	}
	p := slottedPlan()
	put(p, 200)
	originalPath := p.TimeSlots[0].Tasks[0].Path
	p.TimeSlots[0].Tasks[0].Title = "Renamed words"
	put(p, 200)
	p.TimeSlots[0].Tasks[0].Path = "/grammar"
	put(p, 409)
	p.TimeSlots[0].Tasks[0].Path = originalPath
	original := p.TimeSlots[0].Tasks[0]
	p.TimeSlots[0].Tasks[0].ID = "words-new"
	put(p, 200)
	p.TimeSlots[0].Tasks[0] = original
	put(p, 409)
}

func TestIntegrationCompletionTransactionalAndVersioned(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'integration@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	p := slottedPlan()
	raw, _ := json.Marshal(p)
	if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token) VALUES(1,$1,'integration-test')`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,workspace_name,encrypted_secret) VALUES(1,'conn-test','v1','japanese-test','workspace-test','Test','ciphertext')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	for i, completed := range []bool{true, true, false, false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := SetTaskCompletion(ctx, tx, 1, "2026-11-20", "evening", "listening", completed, "manual", now); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		want := []int{1, 1, 2, 2, 3}[i]
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox WHERE kind='event'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("step %d: events=%d want %d", i, n, want)
		}
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM flowspace_outbox WHERE kind='event' ORDER BY sequence DESC LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event CompletionEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatal(err)
	}
	if event.StateRevision != 3 || !event.Completed || event.StudyDate != "2026-11-20" || event.Timezone != "Asia/Shanghai" || event.PlanID == "" || event.SourceInstanceID != "japanese-test" {
		t.Fatalf("event=%+v", event)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetTaskCompletion(ctx, tx, 1, "2026-11-20", "evening", "listening", false, "manual", now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox WHERE kind='event'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("rollback leaked outbox: %d", count)
	}
	if err := db.QueryRow(`SELECT count(*) FROM study_plan_task_completions WHERE user_id=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("rollback changed completion")
	}
}

func TestIntegrationCatalogSurvivesPlanRecreation(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'catalog@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,workspace_name,encrypted_secret) VALUES(1,'conn-test','v1','japanese-test','workspace-test','Test','ciphertext')`); err != nil {
		t.Fatal(err)
	}
	p := slottedPlan()
	raw, _ := json.Marshal(p)
	var ids []string
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token) VALUES(1,$1,'catalog-test')`, string(raw)); err != nil {
			t.Fatal(err)
		}
		var id string
		if err := db.QueryRow(`SELECT plan_id FROM study_plans WHERE user_id=1`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := QueueCatalog(ctx, tx, 1, true, time.Now()); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM study_plans WHERE user_id=1`); err != nil {
			t.Fatal(err)
		}
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := QueueCatalog(ctx, tx, 1, true, time.Now()); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("plan identity reused")
	}
	var revision int64
	if err := db.QueryRow(`SELECT revision FROM study_plan_catalog_versions WHERE user_id=1`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 4 {
		t.Fatalf("revision=%d", revision)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM flowspace_outbox ORDER BY sequence DESC LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var catalog Catalog
	if err := json.Unmarshal([]byte(payload), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.CatalogRevision != 4 || catalog.Plan != nil {
		t.Fatalf("catalog=%+v", catalog)
	}
}

func TestListeningOutboxSharesTrainingTransaction(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'listening-outbox@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	p := slottedPlan()
	raw, _ := json.Marshal(p)
	if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token) VALUES(1,$1,'listening-outbox')`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,workspace_name,encrypted_secret) VALUES(1,'conn-listening','v1','japanese-test','workspace-test','Test','encrypted')`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteListeningTask(ctx, tx, 1, now); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("training rollback leaked completion event")
	}
	for i := 0; i < 2; i++ {
		if err := CompleteListeningTask(ctx, db, 1, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("listening emitted %d events", count)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM flowspace_outbox`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event CompletionEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatal(err)
	}
	if event.Reason != "listening_training" || event.SlotID != "evening" || event.TaskID != "listening" {
		t.Fatalf("event=%+v", event)
	}
}
