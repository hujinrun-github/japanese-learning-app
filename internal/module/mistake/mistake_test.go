package mistake_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/mistake"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
	appstore "japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
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
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	store := mistake.NewStore(db)
	mux := http.NewServeMux()
	mistake.RegisterRoutes(mux, store)
	translations := pgdata.NewTranslationStore(db, appstore.StoreDeps{})
	translation.NewTranslationHandler(translation.NewTranslationService(translations, nil)).RegisterRoutes(mux)
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
	if err := (pgdata.Adapter{}).RunMigrations(context.Background(), env.db); err != nil {
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

func TestTranslationMistakesFollowLocalAssessment(t *testing.T) {
	env := newTestEnv(t)
	translations := pgdata.NewTranslationStore(env.db, appstore.StoreDeps{})
	sourceID, err := translations.SaveSource(translation.TranslationSource{Title: "错题验收", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	saveSentence := func(position int, prompt, reference string) int64 {
		t.Helper()
		id, err := translations.SaveSentence(translation.TranslationSentence{
			SourceID: sourceID, Direction: "cn2jp", SourceText: prompt, ReferenceTranslation: reference, Position: position,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	const prompt = "今天不去。"
	const reference = "今日は行きません。"
	polarity := saveSentence(0, prompt, reference)
	fare := saveSentence(1, "公交车票价多少？", "バス賃はいくらですか？")
	noReference := saveSentence(2, "明天见。", "")

	submit := func(id int64, answer string, want translation.AssessmentResult) {
		t.Helper()
		res := env.request(t, http.MethodPost, "/api/v1/translation/submit", env.token1,
			fmt.Sprintf(`{"sentence_id":%d,"user_translation":%q}`, id, answer), http.StatusOK)
		var body struct {
			Data translation.TranslationRecord `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data.Assessment == nil || body.Data.Assessment.Result != want {
			t.Fatalf("assessment for %q = %+v, want %s", answer, body.Data.Assessment, want)
		}
	}
	for _, tc := range []struct {
		id     int64
		answer string
		want   translation.AssessmentResult
	}{
		{polarity, "行きません。", translation.AssessmentUnrecognized},
		{polarity, reference, translation.AssessmentReferenceMatch},
		{fare, "バスの運賃はいくらでしょうか。", translation.AssessmentEquivalent},
		{noReference, "また明日。", translation.AssessmentNoReference},
	} {
		t.Run(string(tc.want), func(t *testing.T) {
			submit(tc.id, tc.answer, tc.want)
			if page := env.list(t, env.token1, "?module=translation&status=all"); page.Total != 0 {
				t.Fatalf("unexpected translation mistakes: %+v", page)
			}
		})
	}

	env.request(t, http.MethodPost, "/api/v1/mistakes", env.token1,
		`{"module":"translation","item_key":"translation:999","prompt":"试题","user_answer":"试答"}`, http.StatusBadRequest)

	const wrongAnswer = "今日は行きます。"
	submit(polarity, wrongAnswer, translation.AssessmentMeaningChanged)
	page := env.list(t, env.token1, "?module=translation")
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("wrong answer was not collected: %+v", page)
	}
	item := page.Items[0]
	if item.Prompt != prompt || item.UserAnswer != wrongAnswer || item.CorrectAnswer != reference ||
		item.Status != mistake.Unmastered || item.WrongCount != 1 || item.Module != "translation" ||
		item.ItemKey != fmt.Sprintf("translation:%d", polarity) || item.SourcePath != fmt.Sprintf("/translation/practice?sentence_id=%d", polarity) {
		t.Fatalf("incorrect mistake details: %+v", item)
	}

	question := env.request(t, http.MethodGet, fmt.Sprintf("/api/v1/translation/sentences?sentence_id=%d", polarity),
		env.token1, "", http.StatusOK)
	var queue struct {
		Data []translation.TranslationSentence `json:"data"`
	}
	if err := json.Unmarshal(question.Body.Bytes(), &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Data) != 1 || queue.Data[0].ID != polarity || queue.Data[0].ReferenceTranslation != "" {
		t.Fatalf("retry did not load just the original question: %+v", queue.Data)
	}

	env.request(t, http.MethodPatch, fmt.Sprintf("/api/v1/mistakes/%d", item.ID), env.token1, `{"status":"mastered"}`, http.StatusOK)
	submit(polarity, "行きません。", translation.AssessmentUnrecognized)
	page = env.list(t, env.token1, "?module=translation&status=mastered")
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].WrongCount != 1 {
		t.Fatalf("self-check changed a mastered mistake: %+v", page)
	}
	submit(polarity, "今日は行きます！", translation.AssessmentMeaningChanged)
	page = env.list(t, env.token1, "?module=translation")
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != item.ID ||
		page.Items[0].WrongCount != 2 || page.Items[0].UserAnswer != "今日は行きます！" {
		t.Fatalf("repeat did not reopen the same item: %+v", page)
	}
	if page := env.list(t, env.token2, "?module=translation&status=all"); page.Total != 0 {
		t.Fatalf("another account saw a translation mistake: %+v", page)
	}

	if _, err := env.db.Exec(`ALTER TABLE mistakes ADD CONSTRAINT test_mistake_write_failure CHECK (wrong_count <= 2)`); err != nil {
		t.Fatal(err)
	}
	env.request(t, http.MethodPost, "/api/v1/translation/submit", env.token1,
		fmt.Sprintf(`{"sentence_id":%d,"user_translation":%q}`, polarity, wrongAnswer), http.StatusInternalServerError)
	var attempts int
	if err := env.db.QueryRow(`SELECT count(*) FROM translation_records WHERE user_id = 1 AND sentence_id = $1`, polarity).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 5 {
		t.Fatalf("mistake write failed but number of saved attempts = %d, want 5", attempts)
	}
}
