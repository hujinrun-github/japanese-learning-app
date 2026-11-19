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

func TestWritingTopicProgressSurvivesSessions(t *testing.T) {
	for _, mode := range []string{"essay", "sentence"} {
		t.Run(mode, func(t *testing.T) {
			db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
			if _, err := db.Exec(`INSERT INTO users (id,email,password_hash) VALUES (1,'progress1@example.com','unused'),(2,'progress2@example.com','unused')`); err != nil {
				t.Fatal(err)
			}
			const secret = "writing-progress-test"
			request := func(userID int64, method, path, body string, reviewer writing.AIReviewer) *httptest.ResponseRecorder {
				t.Helper()
				mux := http.NewServeMux()
				writing.NewWritingHandler(writing.NewWritingService(pgdata.NewWritingStore(db, store.StoreDeps{}), reviewer)).RegisterRoutes(mux)
				token, _, err := user.SignToken(userID, secret, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+token)
				resp := httptest.NewRecorder()
				user.AuthMiddleware(secret, mux).ServeHTTP(resp, req)
				return resp
			}
			assertProgress := func(userID int64, want bool) {
				t.Helper()
				resp := request(userID, http.MethodGet, "/api/v1/writing/topics", "", nil)
				if resp.Code != http.StatusOK {
					t.Fatalf("topics status=%d body=%s", resp.Code, resp.Body)
				}
				var result struct {
					Data []struct {
						ID        string `json:"id"`
						Practiced *bool  `json:"practiced"`
					} `json:"data"`
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				for _, topic := range result.Data {
					if topic.ID == "weekend" {
						if topic.Practiced == nil || *topic.Practiced != want {
							t.Fatalf("user %d practiced=%v want=%v", userID, topic.Practiced, want)
						}
						return
					}
				}
				t.Fatal("weekend topic missing from full topic list")
			}
			assertProgress(1, false)
			body := `{"type":"` + mode + `","user_answer":"週末は図書館に行きました。"}`
			resp := request(1, http.MethodPut, "/api/v1/writing/topics/weekend/draft", body, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("save draft status=%d body=%s", resp.Code, resp.Body)
			}
			assertProgress(1, false)
			resp = request(1, http.MethodPost, "/api/v1/writing/topics/weekend", body, &writing.StubReviewer{Err: errors.New("review unavailable")})
			if resp.Code != http.StatusInternalServerError {
				t.Fatalf("failed review status=%d body=%s", resp.Code, resp.Body)
			}
			assertProgress(1, false)
			resp = request(1, http.MethodPost, "/api/v1/writing/topics/weekend", body, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("submit status=%d body=%s", resp.Code, resp.Body)
			}
			assertProgress(1, true)
			assertProgress(2, false)
			if _, err := db.Exec(`UPDATE writing_records SET practiced_at=now()-interval '3 days'; UPDATE writing_topics SET title='更新后的主题',essay_prompt='更新后的短文题目',sentence_prompt='更新后的造句题目' WHERE id='weekend'`); err != nil {
				t.Fatal(err)
			}
			assertProgress(1, true)
			// Practiced topics remain valid for explicit revision and draft saving.
			for _, method := range []string{http.MethodPut, http.MethodPost} {
				path := "/api/v1/writing/topics/weekend"
				if method == http.MethodPut {
					path += "/draft"
				}
				if resp := request(1, method, path, body, nil); resp.Code != http.StatusOK {
					t.Fatalf("repeat %s status=%d body=%s", method, resp.Code, resp.Body)
				}
			}
		})
	}
}
