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

func TestPlanHTTP(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	_, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'plan@example.test','test')`)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db)
	public := http.NewServeMux()
	protected := http.NewServeMux()
	h.RegisterRoutes(protected, public)
	public.Handle("/api/v1/study-plan", user.AuthMiddleware("test", protected))
	public.Handle("/api/v1/study-plan/", user.AuthMiddleware("test", protected))
	request := func(method, path string, uid int64, body any) *httptest.ResponseRecorder {
		t.Helper()
		var b bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&b).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &b)
		if uid > 0 {
			token, _, err := user.SignToken(uid, "test", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		public.ServeHTTP(w, r)
		return w
	}
	if w := request("PUT", "/api/v1/study-plan", 0, validPlan()); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", "/api/v1/study-plan", 1, validPlan()); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var payload struct {
		Data struct {
			Plan *Plan  `json:"plan"`
			Path string `json:"calendar_path"`
		}
	}
	w := request("GET", "/api/v1/study-plan", 1, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Plan == nil || payload.Data.Path == "" {
		t.Fatal(w.Body.String())
	}
	path := payload.Data.Path
	updated := validPlan()
	updated.Words, updated.Grammar = 0, 0
	updated.Reading, updated.Speaking, updated.Writing, updated.Translation = 1, 2, 3, 4
	w = request("PUT", "/api/v1/study-plan", 1, updated)
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Path != path || payload.Data.Plan.Reading != 1 || payload.Data.Plan.Speaking != 2 || payload.Data.Plan.Writing != 3 || payload.Data.Plan.Translation != 4 || payload.Data.Plan.Revision != 2 {
		t.Fatalf("update must preserve link and increment revision: %s", w.Body.String())
	}

	if w := request("GET", "/api/v1/study-plan", 2, nil); !strings.Contains(w.Body.String(), `"plan":null`) {
		t.Fatal(w.Body.String())
	}
	w = request("GET", path, 0, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "BEGIN:VALARM\r\n") || !strings.Contains(w.Body.String(), "TRIGGER:-PT10M") {
		t.Fatal(w.Body.String())
	}
	unfolded := strings.ReplaceAll(w.Body.String(), "\r\n ", "")
	for _, goal := range []string{"阅读理解 1 篇", "口语 2 次", "写作 3 篇", "翻译 4 题"} {
		if !strings.Contains(unfolded, goal) {
			t.Errorf("missing calendar goal %q", goal)
		}
	}
	if w := request("GET", path+"invalid", 0, nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/api/v1/study-plan/calendar-token", 1, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request("GET", path, 0, nil); w.Code != 404 {
		t.Fatal("old link still valid")
	}
	if w := request("PUT", "/api/v1/study-plan", 1, Plan{}); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("DELETE", "/api/v1/study-plan", 1, nil); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/api/v1/study-plan", 1, nil); !strings.Contains(w.Body.String(), `"plan":null`) {
		t.Fatal(w.Body.String())
	}
}

func TestCalendarFormat(t *testing.T) {
	p := validPlan()
	p.Title = "学习,语法;复习\\继续"
	p.StartDate = "2026-11-13"
	p.EndDate = p.StartDate
	p.UpdatedAt = "2026-11-13T00:00:00Z"
	p.Revision = 2
	content, err := Calendar(p, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DTSTART:20261113T120000Z", "DTEND:20261113T123000Z", `SUMMARY:学习\,语法\;复习\\继续`, "SEQUENCE:2", "UID:7-2026-11-13@japanese-learning-app"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in %s", want, content)
		}
	}
	for _, line := range strings.Split(content, "\r\n") {
		if len(line) > 75 {
			t.Fatalf("unfolded line: %d", len(line))
		}
	}
}
