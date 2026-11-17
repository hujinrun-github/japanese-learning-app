package mistake_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/mistake"
	"japanese-learning-app/internal/module/user"
)

const testSecret = "mistake-test-secret"

type testEnv struct {
	handler http.Handler
	store   *mistake.Store
	db      *sql.DB
	token1  string
	token2  string
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	db, err := data.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := data.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	store := mistake.NewStore(db)
	mux := http.NewServeMux()
	mistake.RegisterRoutes(mux, store)
	token1, _, err := user.SignToken(1, testSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token2, _, err := user.SignToken(2, testSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{
		handler: user.AuthMiddleware(testSecret, mux),
		store:   store,
		db:      db,
		token1:  token1,
		token2:  token2,
	}
}

func (env testEnv) request(t *testing.T, method, path, token, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	env.handler.ServeHTTP(res, req)
	if res.Code != want {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, res.Code, want, res.Body.String())
	}
	return res
}

func (env testEnv) list(t *testing.T, token, query string) mistake.Page {
	t.Helper()
	res := env.request(t, http.MethodGet, "/api/v1/mistakes"+query, token, "", http.StatusOK)
	var payload struct {
		Data mistake.Page `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}

const wordMistake = `{"module":"word","item_key":"word:7:reading","prompt":"猫","user_answer":"ねこ","correct_answer":"ねこ","explanation":"猫（ねこ）","source_path":"/words/review"}`
const grammarMistake = `{"module":"grammar","item_key":"grammar:3:1","prompt":"＿＿＿に行きます。","user_answer":"を","correct_answer":"に","explanation":"目的地用に","source_path":"/grammar/3"}`

func TestMistakeWorkflow(t *testing.T) {
	env := newTestEnv(t)

	// Sign-in is mandatory for every mistake endpoint.
	env.request(t, http.MethodPost, "/api/v1/mistakes", "", wordMistake, http.StatusUnauthorized)
	env.request(t, http.MethodGet, "/api/v1/mistakes", "", "", http.StatusUnauthorized)
	env.request(t, http.MethodPatch, "/api/v1/mistakes/1", "", `{"status":"mastered"}`, http.StatusUnauthorized)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"unknown module", strings.Replace(wordMistake, `"word"`, `"unknown"`, 1)},
		{"empty item key", strings.Replace(wordMistake, `word:7:reading`, ``, 1)},
		{"invalid item key", strings.Replace(wordMistake, `word:7:reading`, `word:7/../reading`, 1)},
		{"empty prompt", strings.Replace(wordMistake, `"prompt":"猫"`, `"prompt":""`, 1)},
		{"oversized prompt", fmt.Sprintf(`{"module":"word","item_key":"word:7:reading","prompt":%q,"user_answer":"","correct_answer":"","explanation":"","source_path":"/words/review"}`, strings.Repeat("あ", 501))},
		{"external source path", strings.Replace(wordMistake, `/words/review`, `https://evil.example`, 1)},
		{"invalid status", `{"status":"deleted"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "invalid status" {
				env.request(t, http.MethodPatch, "/api/v1/mistakes/1", env.token1, tc.body, http.StatusBadRequest)
				return
			}
			env.request(t, http.MethodPost, "/api/v1/mistakes", env.token1, tc.body, http.StatusBadRequest)
		})
	}

	created := env.request(t, http.MethodPost, "/api/v1/mistakes", env.token1, wordMistake, http.StatusCreated)
	var createdBody struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	id := createdBody.Data.ID
	if id <= 0 {
		t.Fatalf("created id = %d, want positive", id)
	}

	// Migrations run on every startup. Re-running them must not destroy collected mistakes.
	if err := data.RunMigrations(env.db); err != nil {
		t.Fatal(err)
	}

	page := env.list(t, env.token1, "")
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("first list: total %d, items %d, want 1/1", page.Total, len(page.Items))
	}
	got := page.Items[0]
	if got.ID != id || got.Module != "word" || got.ItemKey != "word:7:reading" || got.Prompt != "猫" ||
		got.UserAnswer != "ねこ" || got.CorrectAnswer != "ねこ" || got.Explanation != "猫（ねこ）" ||
		got.SourcePath != "/words/review" || got.Status != mistake.Unmastered || got.WrongCount != 1 {
		t.Fatalf("unexpected mistake: %+v", got)
	}

	// Re-answering the same item updates the row instead of creating a duplicate.
	repeat := strings.Replace(wordMistake, `"user_answer":"ねこ"`, `"user_answer":"ネコ"`, 1)
	env.request(t, http.MethodPost, "/api/v1/mistakes", env.token1, repeat, http.StatusCreated)
	page = env.list(t, env.token1, "")
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].WrongCount != 2 || page.Items[0].UserAnswer != "ネコ" {
		t.Fatalf("repeat did not upsert: %+v", page)
	}

	env.request(t, http.MethodPost, "/api/v1/mistakes", env.token1, grammarMistake, http.StatusCreated)
	for _, tc := range []struct {
		name, query string
		wantTotal   int
	}{
		{"all unmastered", "", 2},
		{"grammar only", "?module=grammar", 1},
		{"word only", "?module=word", 1},
		{"mastered filter", "?status=mastered", 0},
		{"status all", "?status=all", 2},
		{"module all", "?module=all&status=all", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := env.list(t, env.token1, tc.query)
			if got.Total != tc.wantTotal || len(got.Items) != tc.wantTotal {
				t.Fatalf("query %q: total %d items %d, want %d", tc.query, got.Total, len(got.Items), tc.wantTotal)
			}
		})
	}

	env.request(t, http.MethodGet, "/api/v1/mistakes?module=unknown", env.token1, "", http.StatusBadRequest)

	env.request(t, http.MethodPatch, fmt.Sprintf("/api/v1/mistakes/%d", id), env.token1, `{"status":"mastered"}`, http.StatusOK)
	if page := env.list(t, env.token1, ""); page.Total != 1 {
		t.Fatalf("after mastering: total %d, want 1", page.Total)
	}
	if page := env.list(t, env.token1, "?status=mastered"); page.Total != 1 || page.Items[0].ID != id {
		t.Fatalf("mastered list: %+v", page)
	}

	// Getting it wrong again reopens a mastered mistake.
	env.request(t, http.MethodPost, "/api/v1/mistakes", env.token1, wordMistake, http.StatusCreated)
	page = env.list(t, env.token1, "")
	if page.Total != 2 {
		t.Fatalf("after reopening: total %d, want 2", page.Total)
	}
	for _, item := range page.Items {
		if item.ID == id && (item.Status != mistake.Unmastered || item.WrongCount != 3) {
			t.Fatalf("reopened row: %+v", item)
		}
	}

	// User isolation: another learner cannot see, patch or count these rows.
	if page := env.list(t, env.token2, ""); page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("other learner sees %+v", page)
	}
	env.request(t, http.MethodPatch, fmt.Sprintf("/api/v1/mistakes/%d", id), env.token2, `{"status":"mastered"}`, http.StatusNotFound)
	env.request(t, http.MethodPatch, "/api/v1/mistakes/9999", env.token1, `{"status":"mastered"}`, http.StatusNotFound)
}

func TestMistakeListPagination(t *testing.T) {
	env := newTestEnv(t)
	for i := 0; i < 25; i++ {
		if _, err := env.store.Save(t.Context(), 1, mistake.Input{
			Module: "grammar", ItemKey: fmt.Sprintf("grammar:3:%d", i),
			Prompt: fmt.Sprintf("题目 %d", i), UserAnswer: "を", CorrectAnswer: "に",
			Explanation: "", SourcePath: "/grammar/3",
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name, query string
		wantStatus  int
		wantCount   int
		wantTotal   int
	}{
		{"first page default size", "", http.StatusOK, 20, 25},
		{"second page", "?page=2", http.StatusOK, 5, 25},
		{"custom size", "?page=1&size=5", http.StatusOK, 5, 25},
		{"third page past the end", "?page=3&size=5", http.StatusOK, 5, 25},
		{"last empty page", "?page=6&size=5", http.StatusOK, 0, 25},
		{"invalid page zero", "?page=0", http.StatusBadRequest, 0, 0},
		{"invalid size", "?size=0", http.StatusBadRequest, 0, 0},
		{"size over limit", "?size=101", http.StatusBadRequest, 0, 0},
		{"invalid status", "?status=nope", http.StatusBadRequest, 0, 0},
		{"invalid module", "?module=nope", http.StatusBadRequest, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := env.request(t, http.MethodGet, "/api/v1/mistakes"+tc.query, env.token1, "", tc.wantStatus)
			if tc.wantStatus != http.StatusOK {
				return
			}
			var payload struct {
				Data mistake.Page `json:"data"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Data.Items) != tc.wantCount || payload.Data.Total != tc.wantTotal {
				t.Fatalf("query %q: items %d total %d, want %d/%d", tc.query, len(payload.Data.Items), payload.Data.Total, tc.wantCount, tc.wantTotal)
			}
		})
	}
}
