package translation_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/admin"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

func TestDailyTranslationKnowledgeLinks(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	for _, statement := range []string{
		`INSERT INTO users (id, email, password_hash, daily_goals_json) VALUES (1, 'links@example.com', 'unused', '{"translation":2}')`,
		`INSERT INTO words (id, kanji_form, reading, meaning, jlpt_level) VALUES
			(41, '勉強', 'べんきょう', '学习', 'N5'), (42, '毎日', 'まいにち', '每天', 'N5')`,
		`INSERT INTO grammar_points (id, name, meaning, jlpt_level) VALUES (41, '〜ています', '正在', 'N5'), (42, '〜ようにする', '努力做到', 'N4')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	translations := pgdata.NewTranslationStore(db, store.StoreDeps{})
	sourceID, err := translations.SaveSource(translation.TranslationSource{Title: "Knowledge links", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	sentenceID, err := translations.SaveSentence(translation.TranslationSentence{
		SourceID: sourceID, Direction: "cn2jp", SourceText: "我每天学习。", ReferenceTranslation: "毎日勉強しています。", JLPTLevel: "N5",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := translations.SaveSentence(translation.TranslationSentence{
		SourceID: sourceID, Direction: "cn2jp", SourceText: "这是下一题。", Position: 1, JLPTLevel: "N5",
	})
	if err != nil {
		t.Fatal(err)
	}

	const adminToken = "translation-links-admin-test"
	const jwtSecret = "translation-links-user-test"
	adminRoutes := admin.NewHandler(admin.HandlerConfig{AdminToken: adminToken, TranslationStore: translations}).RegisterRoutes()
	learnerMux := http.NewServeMux()
	translation.NewTranslationHandler(translation.NewTranslationService(translations, nil)).RegisterRoutes(learnerMux)
	learnerRoutes := user.AuthMiddleware(jwtSecret, learnerMux)
	learnerToken, _, err := user.SignToken(1, jwtSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := func(handler http.Handler, token, method, path, body string, status int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: status = %d, want %d; body = %s", method, path, res.Code, status, res.Body)
		}
		return res.Body.String()
	}
	linksPath := fmt.Sprintf("/api/admin/translation/%d/links", sentenceID)
	setLinks := func(path, body string, status int) string {
		return request(adminRoutes, adminToken, http.MethodPut, path, body, status)
	}
	request(adminRoutes, learnerToken, http.MethodGet, linksPath, "", http.StatusUnauthorized)
	request(adminRoutes, adminToken, http.MethodGet, linksPath, "", http.StatusOK)
	setLinks(linksPath, `{"links":[{"kind":"grammar","id":42},{"kind":"word","id":41},{"kind":"grammar","id":41},{"kind":"word","id":42}]}`, http.StatusNoContent)
	for _, item := range []struct{ name, payload string }{
		{"duplicate", `{"links":[{"kind":"word","id":41},{"kind":"word","id":41}]}`},
		{"unknown id", `{"links":[{"kind":"word","id":42},{"kind":"grammar","id":999}]}`},
		{"unknown kind", `{"links":[{"kind":"invalid","id":41}]}`},
		{"missing links", `{}`},
	} {
		t.Run(item.name, func(t *testing.T) {
			setLinks(linksPath, item.payload, http.StatusBadRequest)
		})
	}
	setLinks("/api/admin/translation/999/links", `{"links":[]}`, http.StatusNotFound)
	checkTitles := func(raw, field string, submitted bool) {
		t.Helper()
		if submitted {
			var response struct{ Data json.RawMessage }
			if err := json.Unmarshal([]byte(raw), &response); err != nil {
				t.Fatal(err)
			}
			raw = string(response.Data)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			t.Fatal(err)
		}
		var links []struct{ Title string }
		if err := json.Unmarshal(fields[field], &links); err != nil {
			t.Fatal(err)
		}
		var titles []string
		for _, link := range links {
			titles = append(titles, link.Title)
		}
		if got := strings.Join(titles, " / "); got != "〜ようにする / 勉強 / 〜ています / 毎日" {
			t.Fatalf("ordered titles = %q in %s", got, raw)
		}
	}
	checkTitles(request(adminRoutes, adminToken, http.MethodGet, linksPath, "", http.StatusOK), "links", false)

	for _, path := range []string{
		"/api/v1/translation/queue",
		fmt.Sprintf("/api/v1/translation/sentences?source_id=%d", sourceID),
		fmt.Sprintf("/api/v1/translation/sentences?sentence_id=%d", sentenceID),
	} {
		body := request(learnerRoutes, learnerToken, http.MethodGet, path, "", http.StatusOK)
		if strings.Contains(body, "knowledge_links") || strings.Contains(body, "〜ようにする") || strings.Contains(body, "勉強しています") {
			t.Fatalf("%s leaked links or reference before submission: %s", path, body)
		}
	}
	request(learnerRoutes, learnerToken, http.MethodPost, "/api/v1/translation/submit",
		fmt.Sprintf(`{"sentence_id":%d,"user_translation":"  "}`, sentenceID), http.StatusBadRequest)
	submit := func(id int64) string {
		return request(learnerRoutes, learnerToken, http.MethodPost, "/api/v1/translation/submit",
			fmt.Sprintf(`{"sentence_id":%d,"user_translation":"毎日勉強しています。"}`, id), http.StatusOK)
	}
	checkTitles(submit(sentenceID), "knowledge_links", true)
	progress := request(learnerRoutes, learnerToken, http.MethodGet, "/api/v1/translation/progress", "", http.StatusOK)
	if !strings.Contains(progress, `"today_completed":1`) {
		t.Fatalf("unexpected progress after one submission: %s", progress)
	}
	if next := submit(otherID); strings.Contains(next, "knowledge_links") {
		t.Fatalf("links leaked to the next result: %s", next)
	}
	setLinks(linksPath, `{"links":[]}`, http.StatusNoContent)
	if cleared := request(adminRoutes, adminToken, http.MethodGet, linksPath, "", http.StatusOK); !strings.Contains(cleared, `"links":[]`) {
		t.Fatalf("links were not cleared: %s", cleared)
	}
}
