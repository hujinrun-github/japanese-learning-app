package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/module/listening"
	"japanese-learning-app/internal/module/studyplan"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestPostgresListeningRoutes(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresServerDB(t)
	cfg := &config.Config{DatabaseURL: testdb.URL(t, db), DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: time.Minute, AppTimezone: time.UTC, JWTSecret: "listening-route-test"}
	mux, cleanup, err := buildPostgresServerMux(context.Background(), cfg, t.TempDir(), t.TempDir(), &user.StubMailer{}, "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, authorized := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/listening/materials?level=N2", nil)
		if authorized {
			token, _, err := user.SignToken(1, cfg.JWTSecret, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !authorized {
			if rec.Code != 401 {
				t.Fatalf("unauthorized=%d", rec.Code)
			}
			continue
		}
		if rec.Code != 200 {
			t.Fatalf("listening route=%d %s", rec.Code, rec.Body.String())
		}
		var payload struct{ Data listening.Materials }
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.Materials == nil {
			t.Fatal("empty materials must be an array")
		}
	}
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'listening-route@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow(`INSERT INTO lessons(title,jlpt_level,shadowing_config_json) VALUES('Listening route training','N2','{"audio_url":"/audio/route.wav"}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lesson_sentences(lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES($1,0,'[{"surface":"明日は休みです。"}]','明天休息。',0,2000)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO listening_questions(id,lesson_id,prompt,options_json,correct_option_index,explanation) VALUES('route-q1',$1,'明天？','["休息","工作"]',0,'原文说休み。')`, id); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any) {
		t.Helper()
		var data bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&data).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &data)
		token, _, err := user.SignToken(1, cfg.JWTSecret, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s %s returned %d: %s", method, path, rec.Code, rec.Body.String())
		}
	}
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	request("PUT", "/api/v1/study-plan", studyplan.Plan{Title: "Listening integration", StartDate: date, EndDate: date, Timezone: "UTC", Weekdays: []int{int(now.Weekday())}, TimeSlots: []studyplan.TimeSlot{{ID: "evening", Title: "听力", Time: "18:40", Duration: 20, Tasks: []studyplan.Task{{ID: "listening", Title: "听力训练", Path: "/listening", Minutes: 20}}}}})
	base := fmt.Sprintf("/api/v1/listening/materials/%d", id)
	request("POST", base+"/attempts", map[string]any{"phase": "blind", "answers": []map[string]any{{"question_id": "route-q1", "option_index": 0}}, "assisted": false, "idempotency_key": "route-blind"})
	for _, stage := range []string{"intensive", "retest"} {
		request("PUT", base+"/progress", map[string]any{"stage": stage, "expected_cycle": 1, "sentence_index": 0, "position_ms": 0, "bookmarks": []int{}})
	}
	request("POST", base+"/attempts", map[string]any{"phase": "retest", "answers": []map[string]any{{"question_id": "route-q1", "option_index": 0}}, "assisted": false, "idempotency_key": "route-retest"})
	request("POST", base+"/complete", nil)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM study_plan_task_completions WHERE user_id=1 AND study_date=$1 AND slot_id='evening' AND task_id='listening'`, date).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("training did not complete today's listening task: count=%d", count)
	}
}
