package studyplan

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestTodayAndTaskCompletionHTTP(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	for _, uid := range []int{1, 2} {
		if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES($1,$2,'test')`, uid, string(rune('a'+uid))+"@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(db)
	now := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return now }
	public, protected := http.NewServeMux(), http.NewServeMux()
	h.RegisterRoutes(protected, public)
	public.Handle("/api/v1/study-plan", user.AuthMiddleware("test", protected))
	public.Handle("/api/v1/study-plan/", user.AuthMiddleware("test", protected))
	request := func(method, path string, uid int64, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &buf)
		if uid > 0 {
			token, _, err := user.SignToken(uid, "test", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		public.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) TodayPlan {
		t.Helper()
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Data TodayPlan `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Data
	}
	if got := request("GET", "/api/v1/study-plan/today", 0, nil); got.Code != 401 {
		t.Fatalf("unauthorized = %d", got.Code)
	}
	if got := decode(request("GET", "/api/v1/study-plan/today", 1, nil)); got.HasPlan || got.Reason != "no_plan" || len(got.Slots) != 0 {
		t.Fatalf("no plan: %+v", got)
	}
	for _, uid := range []int64{1, 2} {
		if rec := request("PUT", "/api/v1/study-plan", uid, slottedPlan()); rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
	}
	got := decode(request("GET", "/api/v1/study-plan/today", 1, nil))
	if got.Date != "2026-11-20" || len(got.Slots) != 3 || got.Slots[2].Tasks[0].ID != "quiz" {
		t.Fatalf("today = %+v", got)
	}
	path := "/api/v1/study-plan/tasks/lunch/words"
	body := map[string]any{"date": "2026-11-20", "completed": true}
	got = decode(request("PUT", path, 1, body))
	if !got.Slots[0].Tasks[0].Completed {
		t.Fatal("task not completed")
	}
	if got := decode(request("GET", "/api/v1/study-plan/today", 1, nil)); !got.Slots[0].Tasks[0].Completed {
		t.Fatal("completion did not persist")
	}
	if got := decode(request("GET", "/api/v1/study-plan/today", 2, nil)); got.Slots[0].Tasks[0].Completed {
		t.Fatal("completion leaked across users")
	}
	if got := decode(request("PUT", path, 1, body)); !got.Slots[0].Tasks[0].Completed {
		t.Fatal("idempotent completion failed")
	}
	body["completed"] = false
	if got := decode(request("PUT", path, 1, body)); got.Slots[0].Tasks[0].Completed {
		t.Fatal("undo failed")
	}
	for _, tc := range []struct {
		name, path string
		body       any
		code       int
	}{
		{"old date", path, map[string]any{"date": "2026-11-19", "completed": true}, 409},
		{"inactive weekday task", "/api/v1/study-plan/tasks/night/grammar", body, 404},
		{"unknown task", "/api/v1/study-plan/tasks/lunch/unknown", body, 404},
		{"missing completed", path, map[string]any{"date": "2026-11-20"}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := request("PUT", tc.path, 1, tc.body); rec.Code != tc.code {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
	body["completed"] = true
	decode(request("PUT", path, 1, body))
	now = time.Date(2026, 11, 20, 16, 1, 0, 0, time.UTC)
	if got := decode(request("GET", "/api/v1/study-plan/today", 1, nil)); !got.RestDay || got.Date != "2026-11-21" || len(got.Slots) != 0 {
		t.Fatalf("weekend: %+v", got)
	}
	if rec := request("PUT", path, 1, body); rec.Code != 409 {
		t.Fatalf("midnight stale completion = %d", rec.Code)
	}
	now = time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	if got := decode(request("GET", "/api/v1/study-plan/today", 1, nil)); got.Slots[0].Tasks[0].Completed {
		t.Fatal("completion leaked across dates")
	}
	if rec := request("DELETE", "/api/v1/study-plan", 1, nil); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM study_plan_task_completions WHERE user_id=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("deleted plan retained completions")
	}
	if rec := request("GET", "/api/v1/study-plan/today", 1, nil); !strings.Contains(rec.Body.String(), `"has_plan":false`) {
		t.Fatal(rec.Body.String())
	}
}
