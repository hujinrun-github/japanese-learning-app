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

func TestWritingTopicPractice(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec("INSERT INTO users (id, email, password_hash) VALUES (1, 'writer@example.com', 'unused')"); err != nil {
		t.Fatal(err)
	}
	const secret = "writing-test-secret"
	token, _, err := user.SignToken(1, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store := pgdata.NewWritingStore(db, store.StoreDeps{})
	request := func(reviewer writing.AIReviewer, method, path, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		writing.NewWritingHandler(writing.NewWritingService(store, reviewer)).RegisterRoutes(mux)
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()
		user.AuthMiddleware(secret, mux).ServeHTTP(resp, req)
		return resp
	}

	const topicsPath = "/api/v1/writing/topics"
	resp := request(nil, http.MethodGet, topicsPath, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("GET topics status = %d, body = %s", resp.Code, resp.Body)
	}
	var topics struct {
		Data []writing.WritingTopic `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &topics); err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	seenTitles := make(map[string]bool)
	var topic writing.WritingTopic
	for _, item := range topics.Data {
		if item.ID == "" || item.Title == "" || item.EssayPrompt == "" || item.SentencePrompt == "" || item.EssayPrompt == item.SentencePrompt {
			t.Fatalf("incomplete topic: %+v", item)
		}
		if seenTitles[item.Title] {
			t.Fatalf("duplicate topic title: %q", item.Title)
		}
		seenTitles[item.Title] = true
		counts[item.JLPTLevel]++
		if item.JLPTLevel == "N2" && topic.ID == "" {
			topic = item
		}
	}
	for _, tc := range []struct {
		level string
		min   int
	}{{"N5", 10}, {"N4", 12}, {"N3", 11}, {"N2", 24}, {"N1", 8}} {
		if counts[tc.level] < tc.min {
			t.Errorf("%s: got %d topics, want at least %d", tc.level, counts[tc.level], tc.min)
		}
	}
	if topic.ID == "" {
		t.Fatal("no N2 topic available to submit")
	}

	reviewer := &writing.StubReviewer{Feedback: writing.AIFeedback{Score: 86, IssueDescription: "注意时态"}}
	cases := []struct {
		name, topicID, mode, answer, prompt string
		reviewer                            writing.AIReviewer
		status                              int
	}{
		{"essay", topic.ID, "essay", "図書館に行きました。\n本を読みました。", topic.EssayPrompt, reviewer, http.StatusOK},
		{"sentence", topic.ID, "sentence", "友だちと映画を見ました。", topic.SentencePrompt, reviewer, http.StatusOK},
		{"without reviewer", topic.ID, "essay", "友だちと出かけました。", topic.EssayPrompt, nil, http.StatusOK},
		{"legacy mode", topic.ID, "input", "映画", "", reviewer, http.StatusBadRequest},
		{"blank answer", topic.ID, "essay", " \n ", "", reviewer, http.StatusBadRequest},
		{"answer too long", topic.ID, "essay", strings.Repeat("あ", 1001), "", reviewer, http.StatusBadRequest},
		{"unknown topic", "missing-topic", "essay", "図書館に行きました。", "", reviewer, http.StatusNotFound},
		{"reviewer error", topic.ID, "essay", "図書館に行きました。", "", &writing.StubReviewer{Err: errors.New("review failed")}, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"type": tc.mode, "user_answer": tc.answer, "question": "client-supplied question"})
			if err != nil {
				t.Fatal(err)
			}
			resp := request(tc.reviewer, http.MethodPost, topicsPath+"/"+tc.topicID, string(body))
			if resp.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %s", resp.Code, tc.status, resp.Body)
			}
			if tc.status != http.StatusOK {
				return
			}
			var result struct {
				Data writing.WritingRecord `json:"data"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			rec := result.Data
			if string(rec.Type) != tc.mode || rec.UserAnswer != tc.answer ||
				!strings.Contains(rec.Question, topic.Title) || !strings.Contains(rec.Question, tc.prompt) {
				t.Fatalf("unexpected record: %+v", rec)
			}
			if tc.reviewer == nil && rec.AIFeedback != nil {
				t.Fatalf("expected an ungraded record, got %+v", rec.AIFeedback)
			}
			if tc.reviewer != nil && (rec.AIFeedback == nil || rec.Score != 86) {
				t.Fatalf("expected reviewer feedback, got %+v", rec)
			}
		})
	}
	records, err := store.ListRecords(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d saved records, want 3", len(records))
	}
}

func TestWritingTopicCorrectionsReachLearner(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users (id,email,password_hash) VALUES (1,'topic-editor@example.com','unused')`); err != nil {
		t.Fatal(err)
	}
	const corrected = "用日语介绍你这周末的学习安排。"
	if _, err := db.Exec(`UPDATE writing_topics SET title='修正后的周末',essay_prompt=$1 WHERE id='weekend'`, corrected); err != nil {
		t.Fatal(err)
	}
	const secret = "topic-correction-test"
	token, _, err := user.SignToken(1, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	st := pgdata.NewWritingStore(db, store.StoreDeps{})
	mux := http.NewServeMux()
	writing.NewWritingHandler(writing.NewWritingService(st, nil)).RegisterRoutes(mux)
	handler := user.AuthMiddleware(secret, mux)
	for _, tc := range []struct{ name, method, path, body string }{
		{"list", http.MethodGet, "/api/v1/writing/topics", ""},
		{"submit", http.MethodPost, "/api/v1/writing/topics/weekend", `{"type":"essay","user_answer":"日本語を勉強します。"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), corrected) || !strings.Contains(res.Body.String(), "修正后的周末") {
				t.Fatalf("corrected topic unavailable: status=%d body=%s", res.Code, res.Body)
			}
		})
	}
}
