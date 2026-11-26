package favorite_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	pg "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/favorite"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestFavoriteWorkflow(t *testing.T) {
	db := testdb.Open(t, (pg.Adapter{}).RunMigrations)
	for _, statement := range []string{
		`INSERT INTO users (id,email,password_hash) VALUES (1,'favorite@example.test','unused'),(2,'other@example.test','unused')`,
		`INSERT INTO words (id,kanji_form,reading,meaning,jlpt_level) VALUES (101,'今日','きょう','今天','N5')`,
		`INSERT INTO word_bookmarks (user_id,word_id) VALUES (1,101)`,
		`INSERT INTO grammar_points (id,name,meaning,jlpt_level) VALUES (102,'〜のに','虽然','N3')`,
		`INSERT INTO lessons (id,title,jlpt_level,shadowing_enabled) VALUES (103,'聞いてみよう','N5',true)`,
		`INSERT INTO lesson_sentences (lesson_id,position,tokens,chinese) VALUES (103,0,'[{"surface":"聞いてみよう"}]','听听看')`,
		`INSERT INTO listening_progress (user_id,lesson_id,bookmarks_json) VALUES (1,103,'[0]')`,
		`INSERT INTO notes (id,user_id,type,title) VALUES (104,2,'word','其他用户的笔记'),(105,1,'word','我的笔记')`,
		`INSERT INTO speaking_materials (id,type,title,text,jlpt_level) VALUES (106,'free','自己紹介','こんにちは','N5')`,
		`INSERT INTO translation_sources (id,title,source_type,raw_content) VALUES (107,'翻译练习','manual','你好')`,
		`INSERT INTO translation_sentences (id,source_id,direction,source_text,reference_translation,position) VALUES (108,107,'cn2jp','你好','こんにちは',0)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	favorite.RegisterRoutes(mux, pg.NewFavoriteStore(db))
	const secret = "favorite-integration-test"
	handler := user.AuthMiddleware(secret, mux)
	request := func(uid int64, method, path string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		if uid != 0 {
			token, _, err := user.SignToken(uid, secret, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: status %d, want %d, body %s", method, path, res.Code, want, res.Body)
		}
		return res.Body.Bytes()
	}
	list := func(uid int64) []favorite.Item {
		t.Helper()
		var body struct {
			Data []favorite.Item `json:"data"`
		}
		if err := json.Unmarshal(request(uid, http.MethodGet, "/api/v1/favorites", http.StatusOK), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data
	}

	request(0, http.MethodGet, "/api/v1/favorites", http.StatusUnauthorized)
	before := list(1)
	for _, want := range []struct{ kind, key, path string }{
		{"word", "101", "/search/words/101"},
		{"listening_sentence", "103:0", "/listening/103?sentence=0"},
	} {
		if !slices.ContainsFunc(before, func(item favorite.Item) bool {
			return item.Kind == want.kind && item.Key == want.key && item.Path == want.path
		}) {
			t.Fatalf("previous favorite is missing: %+v", before)
		}
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/v1/favorites/grammar/102", http.StatusOK},
		{"/api/v1/favorites/grammar/102", http.StatusOK},
		{"/api/v1/favorites/note/104", http.StatusNotFound},
		{"/api/v1/favorites/lesson/999", http.StatusNotFound},
		{"/api/v1/favorites/unknown/1", http.StatusBadRequest},
	} {
		request(1, http.MethodPut, tc.path, tc.want)
	}
	if got := list(1); len(got) != 3 {
		t.Fatalf("expected an idempotent addition: %+v", got)
	}
	request(2, http.MethodDelete, "/api/v1/favorites/grammar/102", http.StatusNoContent)
	if got := list(2); len(got) != 0 {
		t.Fatalf("found another learner's favorites: %+v", got)
	}
	for _, tc := range []struct{ kind, key, path string }{
		{"lesson", "103", "/lesson/103"},
		{"listening", "103", "/listening/103"},
		{"listening_sentence", "103:0", "/listening/103?sentence=0"},
		{"speaking", "106", "/speaking?material_id=106"},
		{"shadowing", "103", "/lesson/103/shadowing"},
		{"writing", "weekend", "/writing/practice?topic_id=weekend"},
		{"translation", "108", "/translation/practice?sentence_id=108&from=favorites"},
		{"note", "105", "/notes/105"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			url := "/api/v1/favorites/" + tc.kind + "/" + tc.key
			var result struct {
				Data favorite.Item `json:"data"`
			}
			if err := json.Unmarshal(request(1, http.MethodPut, url, http.StatusOK), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.Path != tc.path || result.Data.Title == "" {
				t.Fatalf("unexpected favorite: %+v", result.Data)
			}
			if tc.kind != "listening_sentence" {
				request(1, http.MethodDelete, url, http.StatusNoContent)
			}
		})
	}
	request(2, http.MethodPut, "/api/v1/favorites/listening_sentence/103:0", http.StatusOK)
	var progressCount int
	if err := db.QueryRow(`SELECT count(*) FROM listening_progress WHERE user_id=2`).Scan(&progressCount); err != nil {
		t.Fatal(err)
	}
	if progressCount != 0 {
		t.Fatal("favoriting a sentence created listening progress")
	}

	request(1, http.MethodDelete, "/api/v1/favorites/listening_sentence/103:0", http.StatusNoContent)
	request(1, http.MethodDelete, "/api/v1/favorites/word/101", http.StatusNoContent)
	if got := list(1); len(got) != 1 || got[0].Kind != "grammar" {
		t.Fatalf("unexpected favorites after deletion: %+v", got)
	}
}
