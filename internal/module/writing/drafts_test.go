package writing_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/writing"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

func TestWritingDrafts(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES (1, 'draft1@example.com', 'unused'), (2, 'draft2@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	const secret = "writing-draft-test"
	request := func(userID int64, method, path string, body any, reviewer writing.AIReviewer) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		// Recreate the handler/store for each request: drafts must survive sessions.
		mux := http.NewServeMux()
		writing.NewWritingHandler(writing.NewWritingService(pgdata.NewWritingStore(db, store.StoreDeps{}), reviewer)).RegisterRoutes(mux)
		req := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		if userID != 0 {
			token, _, err := user.SignToken(userID, secret, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp := httptest.NewRecorder()
		user.AuthMiddleware(secret, mux).ServeHTTP(resp, req)
		return resp
	}
	const path = "/api/v1/writing/topics/weekend/draft"
	for _, tc := range []struct {
		name               string
		userID             int64
		path, mode, answer string
		status             int
	}{
		{"authentication required", 0, path, "essay", "下書き", http.StatusUnauthorized},
		{"save essay", 1, path, "essay", "  土曜日は図書館に行きました。\n本を読みました。  ", http.StatusOK},
		{"save sentence separately", 1, path, "sentence", "天気がいいですから、散歩します。", http.StatusOK},
		{"update essay", 1, path, "essay", "日曜日は友だちと映画を見ました。", http.StatusOK},
		{"different topic", 1, "/api/v1/writing/topics/favorite-place/draft", "essay", "図書館が好きです。", http.StatusOK},
		{"another user", 2, path, "essay", "私の下書きです。", http.StatusOK},
		{"draft may exceed submission limit", 2, path, "essay", strings.Repeat("あ", 1001), http.StatusOK},
		{"invalid mode", 1, path, "input", "下書き", http.StatusBadRequest},
		{"unknown topic", 1, "/api/v1/writing/topics/missing/draft", "essay", "下書き", http.StatusNotFound},
		{"oversized draft", 1, path, "essay", strings.Repeat("あ", 10001), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := request(tc.userID, http.MethodPut, tc.path, map[string]any{"type": tc.mode, "user_answer": tc.answer, "user_id": 99}, nil)
			if resp.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", resp.Code, tc.status, resp.Body)
			}
			if tc.status == http.StatusOK {
				var result struct {
					Data struct {
						UserAnswer string `json:"user_answer"`
					} `json:"data"`
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Data.UserAnswer != tc.answer {
					t.Fatalf("draft whitespace/content changed: %q", result.Data.UserAnswer)
				}
			}
		})
	}
	list := func(userID int64) map[string]string {
		t.Helper()
		resp := request(userID, http.MethodGet, "/api/v1/writing/drafts", nil, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", resp.Code, resp.Body)
		}
		var result struct {
			Data []struct {
				TopicID    string `json:"topic_id"`
				Type       string `json:"type"`
				UserAnswer string `json:"user_answer"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		for _, draft := range result.Data {
			values[draft.TopicID+":"+draft.Type] = draft.UserAnswer
		}
		return values
	}
	if drafts := list(1); len(drafts) != 3 || drafts["weekend:essay"] != "日曜日は友だちと映画を見ました。" {
		t.Fatalf("user 1 drafts=%v", drafts)
	}
	if drafts := list(2); len(drafts) != 1 || len([]rune(drafts["weekend:essay"])) != 1001 {
		t.Fatalf("user 2 draft isolation failed")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM writing_records").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("saving a draft counted as practice")
	}
	body := map[string]string{"type": "essay", "user_answer": "日曜日は映画を見ました。"}
	resp := request(1, http.MethodPost, "/api/v1/writing/topics/weekend", body, &writing.StubReviewer{Err: errors.New("review unavailable")})
	if resp.Code != http.StatusInternalServerError || len(list(1)) != 3 {
		t.Fatal("failed submission lost its draft")
	}
	resp = request(1, http.MethodPost, "/api/v1/writing/topics/weekend", body, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("submit status=%d body=%s", resp.Code, resp.Body)
	}
	if drafts := list(1); len(drafts) != 2 || drafts["weekend:sentence"] == "" || drafts["favorite-place:essay"] == "" {
		t.Fatalf("submission cleared unrelated drafts: %v", drafts)
	}
	resp = request(1, http.MethodPut, path, map[string]string{"type": "sentence", "user_answer": ""}, nil)
	if resp.Code != http.StatusOK || len(list(1)) != 1 {
		t.Fatal("clearing a draft failed")
	}
	if len(list(2)) != 1 {
		t.Fatal("clearing a draft affected another user")
	}
}
