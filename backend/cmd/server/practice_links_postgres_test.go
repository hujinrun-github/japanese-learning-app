package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/config"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/admin"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestPostgresPracticeKnowledgeLinkVisibility(t *testing.T) {
	db := newMigratedPostgresServerDB(t)
	for _, statement := range []string{
		`INSERT INTO users(id,email,password_hash) VALUES(1,'practice-links@example.test','test')`,
		`INSERT INTO lessons(id,title,jlpt_level,shadowing_config_json) VALUES(890,'Knowledge audio','N5','{"audio_url":"/audio/practice.wav"}')`,
		`INSERT INTO lesson_sentences(id,lesson_id,position,tokens,start_ms,end_ms) VALUES(891,890,0,'[{"surface":"休みです。"}]',0,2000)`,
		`INSERT INTO listening_questions(id,lesson_id,prompt,options_json,correct_option_index,explanation) VALUES('knowledge-q',890,'何ですか。','["休み","仕事"]',0,'休み。')`,
		`INSERT INTO speaking_materials(id,type,title,text,jlpt_level) VALUES(892,'free','Knowledge oral','休みです。','N5')`,
		`INSERT INTO writing_questions(id,type,prompt,expected_answer,jlpt_level) VALUES(893,'input','やすみ','休み','N5'),(894,'sentence','指定の文型を使う','','N5')`,
		`INSERT INTO words(id,kanji_form,reading,meaning,jlpt_level) VALUES(895,'知識語','ちしきご','训练关联词','N5')`,
		`INSERT INTO grammar_points(id,name,meaning,jlpt_level) VALUES(896,'知識文法','训练关联语法','N5')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{DatabaseURL: testdb.URL(t, db), DBMaxOpenConns: 4, DBMaxIdleConns: 2, DBConnMaxLifetime: time.Minute, AppTimezone: time.UTC, JWTSecret: "practice-link-test"}
	mux, cleanup, err := buildPostgresServerMux(context.Background(), cfg, t.TempDir(), t.TempDir(), &user.StubMailer{}, "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	token, _, err := user.SignToken(1, cfg.JWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != 200 {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		return res.Body.String()
	}
	const listeningPath = "/api/v1/listening/materials/890"
	listeningLinks := func(stage string, cycle int) string {
		return request("GET", fmt.Sprintf("%s/knowledge-links?stage=%s&cycle=%d", listeningPath, stage, cycle), "")
	}
	assertEntries := func(body string, word, grammar bool) {
		t.Helper()
		for _, check := range []struct {
			entry string
			want  bool
		}{{"知識語", word}, {"知識文法", grammar}} {
			if strings.Contains(body, check.entry) != check.want {
				t.Fatalf("expected %s present=%t: %s", check.entry, check.want, body)
			}
		}
	}
	assertEntries(listeningLinks("blind", 1), false, false)

	const adminToken = "local-practice-links"
	adminMux := admin.NewHandler(admin.HandlerConfig{AdminToken: adminToken, PracticeLinks: pgdata.NewPracticeLinkStore(db)}).RegisterRoutes()
	adminRequest := func(method, path, body, bearer string, status int) string {
		t.Helper()
		req := httptest.NewRequest(method, "/api/admin/practice-links/"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bearer)
		res := httptest.NewRecorder()
		adminMux.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, res.Code, status, res.Body.String())
		}
		return res.Body.String()
	}
	adminRequest("GET", "listening-sentence", "", token, 401)
	for _, seed := range []struct {
		target, id string
		hint       bool
	}{
		{"listening-material", "890", false},
		{"listening-sentence", "891", false},
		{"listening-question", "knowledge-q", false},
		{"speaking-material", "892", false},
		{"writing-topic", "weekend", true},
		{"writing-question", "893", false},
		{"writing-question", "894", true},
	} {
		catalog := adminRequest("GET", seed.target+"?search="+seed.id, "", adminToken, 200)
		if !strings.Contains(catalog, fmt.Sprintf(`"id":%q`, seed.id)) {
			t.Fatalf("source missing from catalog: %s", catalog)
		}
		path := seed.target + "/" + seed.id
		payload := fmt.Sprintf(`{"links":[{"kind":"word","id":895},{"kind":"grammar","id":896,"hint":%t}]}`, seed.hint)
		adminRequest("PUT", path, payload, adminToken, 204)
		assertEntries(adminRequest("GET", path, "", adminToken, 200), true, true)
	}
	adminRequest("PUT", "writing-question/893", `{"links":[{"kind":"grammar","id":896,"hint":true}]}`, adminToken, 400)
	adminRequest("PUT", "writing-topic/weekend", `{"links":[{"kind":"word","id":895,"hint":true}]}`, adminToken, 400)
	adminRequest("PUT", "speaking-material/892", `{"links":[{"kind":"word","id":895},{"kind":"grammar","id":999}]}`, adminToken, 400)
	adminRequest("GET", "speaking-material/999", "", adminToken, 404)
	assertEntries(listeningLinks("intensive", 1), false, false)
	attempt := func(phase string) {
		request("POST", listeningPath+"/attempts", fmt.Sprintf(`{"phase":%q,"answers":[{"question_id":"knowledge-q","option_index":0}],"idempotency_key":%q}`, phase, "links-"+phase))
	}
	move := func(stage string) {
		request("PUT", listeningPath+"/progress", fmt.Sprintf(`{"stage":%q,"expected_cycle":1,"sentence_index":0,"position_ms":0}`, stage))
	}
	attempt("blind")
	firstReview := listeningLinks("blind", 1)
	assertEntries(firstReview, true, true)
	if strings.Contains(firstReview, `"material":`) || strings.Contains(firstReview, `"sentences":`) {
		t.Fatalf("blind review included transcript associations: %s", firstReview)
	}
	move("intensive")
	intensive := listeningLinks("intensive", 1)
	assertEntries(intensive, true, true)
	for _, key := range []string{`"material"`, `"sentences"`, `"questions"`, `"0"`, `"knowledge-q"`} {
		if !strings.Contains(intensive, key) {
			t.Fatalf("missing %s in %s", key, intensive)
		}
	}
	move("retest")
	for _, stage := range []string{"blind", "intensive", "retest", "completed"} {
		assertEntries(listeningLinks(stage, 1), false, false)
	}
	attempt("retest")
	assertEntries(listeningLinks("retest", 1), true, true)
	request("POST", listeningPath+"/complete", "")
	assertEntries(listeningLinks("completed", 1), true, true)
	request("PUT", listeningPath+"/progress", `{"stage":"blind","expected_cycle":1,"restart":true,"sentence_index":0,"position_ms":0}`)
	assertEntries(listeningLinks("intensive", 1), false, false)
	assertEntries(listeningLinks("blind", 2), false, false)

	assertEntries(request("GET", "/api/v1/speaking/materials/892/knowledge-links", ""), true, true)
	assertEntries(request("GET", "/api/v1/writing/topics/weekend/knowledge-links?mode=essay", ""), false, false)
	assertEntries(request("GET", "/api/v1/writing/topics/weekend/knowledge-links?mode=sentence", ""), false, true)
	assertEntries(request("GET", "/api/v1/writing/questions/893/knowledge-links", ""), false, false)
	assertEntries(request("GET", "/api/v1/writing/questions/894/knowledge-links", ""), false, true)
	assertEntries(request("POST", "/api/v1/writing/input", `{"question_id":893,"question":"やすみ","user_answer":"休み"}`), true, true)
	assertEntries(request("POST", "/api/v1/writing/sentence", `{"question_id":894,"user_answer":"休みです。"}`), true, true)
	assertEntries(request("POST", "/api/v1/writing/topics/weekend", `{"type":"sentence","user_answer":"休みだったから出かけました。"}`), true, true)
}
