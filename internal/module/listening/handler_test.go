package listening_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/listening"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestListeningTrainingHTTP(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	for _, uid := range []int64{1, 2} {
		if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES($1,$2,'test')`, uid, string(rune('a'+uid))+"@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	var lessonID int64
	if err := db.QueryRow(`INSERT INTO lessons(title,jlpt_level,char_count,shadowing_config_json) VALUES('Listening fixture','N2',8,'{"audio_url":"/audio/listening.wav"}') RETURNING id`).Scan(&lessonID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lesson_sentences(lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES($1,0,'[{"surface":"明日は休みです。","reading":""}]','明天休息。',0,3000)`, lessonID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO listening_questions(id,lesson_id,prompt,options_json,correct_option_index,explanation,evidence_sentence_index) VALUES('fixture-q1',$1,'明天怎么样？','["休息","上班"]',0,'原文说休み。',0)`, lessonID); err != nil {
		t.Fatal(err)
	}
	store := postgres.NewListeningStore(db)
	mux := http.NewServeMux()
	listening.NewHandler(store).RegisterRoutes(mux)
	h := user.AuthMiddleware("test", mux)
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
		h.ServeHTTP(rec, req)
		return rec
	}
	base := "/api/v1/listening/materials/" + jsonNumber(lessonID)
	assertOK := func(rec *httptest.ResponseRecorder) []byte {
		t.Helper()
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	if rec := request("GET", base, 0, nil); rec.Code != 401 {
		t.Fatalf("unauthorized=%d", rec.Code)
	}
	raw := assertOK(request("GET", base, 1, nil))
	if strings.Contains(string(raw), "correct_option_index") || strings.Contains(string(raw), "原文说休み") {
		t.Fatalf("unsubmitted answers leaked: %s", raw)
	}
	progress := map[string]any{"stage": "intensive", "expected_cycle": 1, "sentence_index": 0, "position_ms": 500, "bookmarks": []int{0}}
	if rec := request("PUT", base+"/progress", 1, progress); rec.Code != 409 {
		t.Fatalf("skipped blind=%d %s", rec.Code, rec.Body.String())
	}
	if rec := request("POST", base+"/complete", 1, nil); rec.Code != 409 {
		t.Fatalf("premature complete=%d", rec.Code)
	}
	blind := map[string]any{"phase": "blind", "answers": []map[string]any{{"question_id": "fixture-q1", "option_index": 1}}, "assisted": false, "idempotency_key": "blind-one"}
	first := assertOK(request("POST", base+"/attempts", 1, blind))
	if !strings.Contains(string(first), `"correct_count":0`) || !strings.Contains(string(first), `"correct_option_index":0`) {
		t.Fatalf("grading=%s", first)
	}
	if second := assertOK(request("POST", base+"/attempts", 1, blind)); !bytes.Equal(first, second) {
		t.Fatalf("idempotency mismatch: %s / %s", first, second)
	}
	blind["answers"] = []map[string]any{{"question_id": "fixture-q1", "option_index": 0}}
	if rec := request("POST", base+"/attempts", 1, blind); rec.Code != 409 {
		t.Fatalf("reused key payload=%d", rec.Code)
	}
	blind["idempotency_key"] = "blind-overwrite"
	if rec := request("POST", base+"/attempts", 1, blind); rec.Code != 409 {
		t.Fatalf("first score overwritten=%d", rec.Code)
	}
	assertOK(request("PUT", base+"/progress", 1, progress))
	raw = assertOK(request("GET", base, 1, nil))
	if !strings.Contains(string(raw), `"bookmarks":[0]`) {
		t.Fatalf("bookmark lost: %s", raw)
	}
	other := assertOK(request("GET", base, 2, nil))
	if strings.Contains(string(other), "correct_option_index") || strings.Contains(string(other), `"bookmarks":[0]`) {
		t.Fatalf("user leak=%s", other)
	}
	progress["stage"] = "retest"
	assertOK(request("PUT", base+"/progress", 1, progress))
	retest := map[string]any{"phase": "retest", "answers": []map[string]any{{"question_id": "fixture-q1", "option_index": 0}}, "assisted": false, "idempotency_key": "retest-one"}
	assertOK(request("POST", base+"/attempts", 1, retest))
	completed := assertOK(request("POST", base+"/complete", 1, nil))
	if !strings.Contains(string(completed), `"completed_count":1`) || !strings.Contains(string(completed), `"stage":"completed"`) {
		t.Fatalf("completion=%s", completed)
	}
	assertOK(request("POST", base+"/complete", 1, nil))
	var count int
	if err := db.QueryRow(`SELECT completed_count FROM listening_progress WHERE user_id=1 AND lesson_id=$1`, lessonID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("completion repeated")
	}
	for _, tc := range []struct {
		name, path string
		body       any
		code       int
	}{
		{"negative position", base + "/progress", map[string]any{"stage": "completed", "sentence_index": 0, "position_ms": -1, "bookmarks": []int{}}, 422},
		{"unknown sentence", base + "/progress", map[string]any{"stage": "completed", "sentence_index": 8, "position_ms": 0, "bookmarks": []int{}}, 422},
		{"invalid filter", "/api/v1/listening/materials?status=unknown", nil, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := "PUT"
			if tc.body == nil {
				method = "GET"
			}
			if rec := request(method, tc.path, 1, tc.body); rec.Code != tc.code {
				t.Fatalf("status %d want %d: %s", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
}

func jsonNumber(n int64) string { b, _ := json.Marshal(n); return string(b) }
