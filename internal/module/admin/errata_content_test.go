package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/errata"
	"japanese-learning-app/internal/testdb"
)

type errataContentTestEnv struct {
	db  *sql.DB
	mux http.Handler
}

func newErrataContentTestEnv(t *testing.T) *errataContentTestEnv {
	t.Helper()
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users (id,email,password_hash) VALUES (1,'correction@example.com','unused')`); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(HandlerConfig{AdminToken: "correction-token", DB: db, ErrataStore: errata.NewStore(db)})
	return &errataContentTestEnv{db: db, mux: h.RegisterRoutes()}
}

func (e *errataContentTestEnv) report(t *testing.T, module, key string) int64 {
	t.Helper()
	var id int64
	if err := e.db.QueryRow(`INSERT INTO errata_reports (user_id,module,item_key,item_label,category,description) VALUES (1,$1,$2,'摘要','answer','需要修正') RETURNING id`, module, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *errataContentTestEnv) request(t *testing.T, method, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer correction-token")
	res := httptest.NewRecorder()
	e.mux.ServeHTTP(res, req)
	if res.Code != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, res.Code, want, res.Body.String())
	}
	return res
}

type errataContentResponse struct {
	Revision string `json:"revision"`
	Fields   []struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Readonly bool   `json:"readonly"`
	} `json:"fields"`
	AudioTargets []struct {
		Key string `json:"key"`
	} `json:"audio_targets"`
}

func (e *errataContentTestEnv) content(t *testing.T, reportID int64) errataContentResponse {
	t.Helper()
	res := e.request(t, "GET", fmt.Sprintf("/api/admin/errata/%d/content", reportID), nil, http.StatusOK)
	var got errataContentResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Revision == "" {
		t.Fatal("missing revision")
	}
	return got
}

func TestErrataContentWordPreservesExamplesAndAudio(t *testing.T) {
	e := newErrataContentTestEnv(t)
	_, err := e.db.Exec(`INSERT INTO audio_objects (id,bucket,object_key,kind,visibility,content_sha256,size_bytes,mime_type) VALUES (1,'test','example.wav','word_example','public','test',1,'audio/wav');
	INSERT INTO words (id,kanji_form,reading,meaning,jlpt_level) VALUES (10,'工場','こうじょう','工厂','N5');
	INSERT INTO word_examples (id,word_id,position,japanese,chinese,furigana_html,audio_object_id) VALUES (20,10,0,'町に工場があります。','镇上有工厂。','旧注音',1)`)
	if err != nil {
		t.Fatal(err)
	}
	id := e.report(t, "word", "10:dictation")
	before := e.content(t, id)
	if len(before.AudioTargets) != 2 || before.AudioTargets[1].Key != "word_example:20" {
		t.Fatalf("targets: %+v", before.AudioTargets)
	}
	path := fmt.Sprintf("/api/admin/errata/%d/content", id)
	e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{"word:10.meaning": "工厂，工场", "word_example:20.furigana_html": "<ruby>町<rt>まち</rt></ruby>に工場があります。"}}, http.StatusOK)
	var meaning, furigana, status string
	var exampleID, audioID int64
	if err := e.db.QueryRow(`SELECT w.meaning,e.id,e.audio_object_id,e.furigana_html,r.status FROM words w JOIN word_examples e ON e.word_id=w.id CROSS JOIN errata_reports r WHERE w.id=10 AND r.id=$1`, id).Scan(&meaning, &exampleID, &audioID, &furigana, &status); err != nil {
		t.Fatal(err)
	}
	if meaning != "工厂，工场" || exampleID != 20 || audioID != 1 || !strings.Contains(furigana, "まち") || status != "pending" {
		t.Fatalf("saved values: %q %d %d %q %q", meaning, exampleID, audioID, furigana, status)
	}
	e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{"word:10.meaning": "覆盖"}}, http.StatusConflict)
	current := e.content(t, id)
	e.request(t, "PUT", path, map[string]any{"revision": current.Revision, "values": map[string]string{"word:10.meaning": "不得部分写入", "word_example:999.chinese": "越界"}}, http.StatusBadRequest)
	if err := e.db.QueryRow(`SELECT meaning FROM words WHERE id=10`).Scan(&meaning); err != nil {
		t.Fatal(err)
	}
	if meaning != "工厂，工场" {
		t.Fatalf("invalid save partially committed: %q", meaning)
	}
}

func TestErrataContentTargetsOnlyReportedExample(t *testing.T) {
	e := newErrataContentTestEnv(t)
	_, err := e.db.Exec(`INSERT INTO words (id,kanji_form,reading,meaning,jlpt_level) VALUES (10,'工場','こうじょう','工厂','N5'), (11,'町','まち','小镇','N5');
	INSERT INTO word_examples (id,word_id,position,japanese,chinese) VALUES (20,10,0,'工場です。','是工厂。'), (21,10,1,'工場へ行きます。','去工厂。'), (22,11,0,'町です。','是小镇。');
	INSERT INTO grammar_points (id,name,meaning,jlpt_level) VALUES (30,'〜から','因为','N5'), (33,'〜まで','直到','N5');
	INSERT INTO grammar_examples (id,grammar_point_id,position,japanese,chinese) VALUES (31,30,0,'雨だから。','因为下雨。'), (32,30,1,'雪だから。','因为下雪。'), (34,33,0,'夜まで。','直到晚上。');
	INSERT INTO grammar_quiz_questions (id,grammar_point_id,position,type,prompt,options,answer) VALUES (35,30,0,'multi_choice','雨です。','["から","まで"]','から')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		module, key, foreignKey, targetField, siblingField, query string
	}{
		{"word", "10:example:20", "10:example:22", "word_example:20.chinese", "word_example:21.chinese", `SELECT chinese FROM word_examples WHERE id=20`},
		{"grammar", "30:example:31", "30:example:34", "grammar_example:31.chinese", "grammar_example:32.chinese", `SELECT chinese FROM grammar_examples WHERE id=31`},
	} {
		t.Run(tc.module, func(t *testing.T) {
			id := e.report(t, tc.module, tc.key)
			path := fmt.Sprintf("/api/admin/errata/%d/content", id)
			before := e.content(t, id)
			for _, field := range before.Fields {
				if field.Key == tc.siblingField || strings.HasPrefix(field.Key, "grammar_quiz:") {
					t.Fatalf("unrelated field in example editor: %s", field.Key)
				}
			}
			e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{tc.siblingField: "不能覆盖"}}, http.StatusBadRequest)
			e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{tc.targetField: "修正译文"}}, http.StatusOK)
			var got string
			if err := e.db.QueryRow(tc.query).Scan(&got); err != nil || got != "修正译文" {
				t.Fatalf("corrected translation = %q, err = %v", got, err)
			}
			foreignReport := e.report(t, tc.module, tc.foreignKey)
			e.request(t, "GET", fmt.Sprintf("/api/admin/errata/%d/content", foreignReport), nil, http.StatusNotFound)
		})
	}
}

func TestErrataContentModuleCorrections(t *testing.T) {
	e := newErrataContentTestEnv(t)
	_, err := e.db.Exec(`INSERT INTO grammar_points (id,name,meaning,jlpt_level) VALUES (30,'〜から','因为','N5');
	INSERT INTO grammar_examples (id,grammar_point_id,position,japanese,chinese) VALUES (31,30,0,'雨だから。','因为下雨。');
	INSERT INTO grammar_quiz_questions (id,grammar_point_id,position,type,prompt,options,answer) VALUES (32,30,0,'multi_choice','雨です。','["から","まで"]','から');
	INSERT INTO speaking_materials (id,type,title,text,jlpt_level) VALUES (40,'shadow','问候','こんにちは','N5');
	INSERT INTO writing_questions (id,type,prompt,expected_answer) VALUES (50,'sentence','请造句','こんにちは');
	INSERT INTO translation_sources (id,title,source_type,raw_content) VALUES (60,'翻译','manual','原文');
	INSERT INTO translation_sentences (id,source_id,direction,source_text,reference_translation) VALUES (61,60,'jp2cn','こんにちは','你好');
	INSERT INTO lessons (id,title,jlpt_level,shadowing_version) VALUES (70,'课文','N5',2);
	INSERT INTO lesson_sentences (id,lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES (71,70,0,'[{"surface":"町","reading":"ちょう"}]','小镇',100,1000)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, module, item, key, value, query string }{
		{"grammar quiz", "grammar", "30:quiz:32", "grammar_quiz:32.answer", "まで", `SELECT answer FROM grammar_quiz_questions WHERE id=32`},
		{"grammar example", "grammar", "30", "grammar_example:31.furigana_html", "<ruby>雨<rt>あめ</rt></ruby>だから。", `SELECT furigana_html FROM grammar_examples WHERE id=31`},
		{"speaking", "speaking", "40", "speaking:40.text", "こんばんは", `SELECT text FROM speaking_materials WHERE id=40`},
		{"legacy writing", "writing", "50:sentence", "writing_question:50.expected_answer", "こんばんは", `SELECT expected_answer FROM writing_questions WHERE id=50`},
		{"writing topic", "writing", "weekend:essay", "writing_topic:weekend.essay_prompt", "请写一段周末日记。", `SELECT essay_prompt FROM writing_topics WHERE id='weekend'`},
		{"translation", "translation", "61", "translation:61.reference_translation", "您好", `SELECT reference_translation FROM translation_sentences WHERE id=61`},
		{"lesson", "lesson", "70:sentence:0", "lesson_sentence:71.chinese", "城镇", `SELECT chinese FROM lesson_sentences WHERE id=71`},
		{"shadowing", "shadowing", "70:v2:sentence:0", "lesson_sentence:71.tokens", "[{\"surface\":\"町\",\"reading\":\"まち\"}]", `SELECT tokens->0->>'reading' FROM lesson_sentences WHERE id=71`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := e.report(t, tc.module, tc.item)
			before := e.content(t, id)
			e.request(t, "PUT", fmt.Sprintf("/api/admin/errata/%d/content", id), map[string]any{"revision": before.Revision, "values": map[string]string{tc.key: tc.value}}, http.StatusOK)
			var got string
			if err := e.db.QueryRow(tc.query).Scan(&got); err != nil {
				t.Fatal(err)
			}
			want := tc.value
			if tc.name == "shadowing" {
				want = "まち"
			}
			if got != want {
				t.Fatalf("saved %q, want %q", got, want)
			}
		})
	}
}

func TestErrataContentRejectsInvalidTargetsAndPayloads(t *testing.T) {
	e := newErrataContentTestEnv(t)
	if _, err := e.db.Exec(`INSERT INTO lessons (id,title,jlpt_level,shadowing_version) VALUES (70,'课文','N5',2)`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, module, key string
		status            int
	}{
		{"malformed id", "word", "x:dictation", 400},
		{"malformed mode", "word", "1:unknown", 400},
		{"missing content", "word", "999", 404},
		{"wrong grammar parent", "grammar", "70:quiz:32", 404},
		{"missing sentence", "lesson", "70:sentence:99", 404},
		{"stale shadowing", "shadowing", "70:v1:sentence:0", 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := e.report(t, tc.module, tc.key)
			e.request(t, "GET", fmt.Sprintf("/api/admin/errata/%d/content", id), nil, tc.status)
		})
	}
	e.request(t, "GET", "/api/admin/errata/99999/content", nil, 404)
}

func TestErrataResolutionRequiresConfirmation(t *testing.T) {
	e := newErrataContentTestEnv(t)
	id := e.report(t, "word", "10:dictation")
	path := fmt.Sprintf("/api/admin/errata/%d", id)
	e.request(t, "PATCH", path, map[string]any{"status": "resolved"}, http.StatusBadRequest)
	e.request(t, "PATCH", path, map[string]any{"status": "resolved", "confirmed": true}, http.StatusBadRequest)
	var status string
	if err := e.db.QueryRow(`SELECT status FROM errata_reports WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("missing revision changed status to %q", status)
	}
	if _, err := e.db.Exec(`INSERT INTO words (id,kanji_form,reading,meaning,jlpt_level) VALUES (10,'工場','こうじょう','工厂','N5')`); err != nil {
		t.Fatal(err)
	}
	before := e.content(t, id)
	e.request(t, "PATCH", path, map[string]any{"status": "resolved", "confirmed": true, "revision": before.Revision}, http.StatusNoContent)
	e.request(t, "PATCH", path, map[string]any{"status": "pending"}, http.StatusNoContent)
	if _, err := e.db.Exec(`UPDATE words SET meaning='工场' WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	e.request(t, "PATCH", path, map[string]any{"status": "resolved", "confirmed": true, "revision": before.Revision}, http.StatusConflict)
	current := e.content(t, id)
	e.request(t, "PATCH", path, map[string]any{"status": "resolved", "confirmed": true, "revision": current.Revision}, http.StatusNoContent)
	e.request(t, "PUT", path+"/content", map[string]any{"revision": current.Revision, "values": map[string]string{"word:10.meaning": "修改已处理内容"}}, http.StatusConflict)
}

func TestErrataContentMediaAndValidation(t *testing.T) {
	e := newErrataContentTestEnv(t)
	_, err := e.db.Exec(`INSERT INTO lessons (id,title,jlpt_level,shadowing_version,shadowing_config_json) VALUES (70,'课文','N5',2,'{"audio_url":"/audio/original.wav","custom":"retained"}');
	INSERT INTO lesson_sentences (id,lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES (71,70,0,'[{"surface":"町","reading":"ちょう"}]','小镇',100,1000);
	INSERT INTO words (id,kanji_form,reading,meaning,jlpt_level) VALUES (10,'工場','こうじょう','工厂','N5');
	INSERT INTO word_examples (id,word_id,position,japanese,chinese) VALUES (20,10,0,'町に工場があります。','镇上有工厂。')`)
	if err != nil {
		t.Fatal(err)
	}
	id := e.report(t, "lesson", "70:sentence:0")
	before := e.content(t, id)
	path := fmt.Sprintf("/api/admin/errata/%d/content", id)
	e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{"lesson:70.audio_url": "https://example.com/repaired.wav"}}, http.StatusOK)
	var mediaURL, retained string
	var start, end, sentenceID int64
	if err := e.db.QueryRow(`SELECT shadowing_config_json->>'audio_url',shadowing_config_json->>'custom',s.id,s.start_ms,s.end_ms FROM lessons l JOIN lesson_sentences s ON s.lesson_id=l.id WHERE l.id=70`).Scan(&mediaURL, &retained, &sentenceID, &start, &end); err != nil {
		t.Fatal(err)
	}
	if mediaURL != "https://example.com/repaired.wav" || retained != "retained" || sentenceID != 71 || start != 100 || end != 1000 {
		t.Fatalf("media save lost metadata: %q %q %d %d %d", mediaURL, retained, sentenceID, start, end)
	}
	for _, tc := range []struct{ name, key, value string }{
		{"script URL", "lesson:70.audio_url", "javascript:alert(1)"},
		{"invalid tokens", "lesson_sentence:71.tokens", "{}"},
		{"invalid timing", "lesson_sentence:71.end_ms", "99"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := e.content(t, id)
			e.request(t, "PUT", path, map[string]any{"revision": current.Revision, "values": map[string]string{tc.key: tc.value}}, 400)
		})
	}
	wordReport := e.report(t, "word", "10")
	wordContent := e.content(t, wordReport)
	e.request(t, "PUT", fmt.Sprintf("/api/admin/errata/%d/content", wordReport), map[string]any{"revision": wordContent.Revision, "values": map[string]string{"word:10.meaning": "不会保存", "word_example:20.furigana_html": "<img src=x onerror=alert(1)>"}}, 400)
	var meaning string
	if err := e.db.QueryRow(`SELECT meaning FROM words WHERE id=10`).Scan(&meaning); err != nil {
		t.Fatal(err)
	}
	if meaning != "工厂" {
		t.Fatalf("unsafe markup partially saved: %q", meaning)
	}
	for _, method := range []string{"GET", "PUT"} {
		req := httptest.NewRequest(method, path, nil)
		res := httptest.NewRecorder()
		e.mux.ServeHTTP(res, req)
		if res.Code != 401 {
			t.Fatalf("unauthenticated %s = %d", method, res.Code)
		}
	}
}

func TestErrataContentPracticedTranslationLocksSource(t *testing.T) {
	e := newErrataContentTestEnv(t)
	_, err := e.db.Exec(`INSERT INTO translation_sources (id,title,source_type,raw_content) VALUES (60,'翻译','manual','原文');
	INSERT INTO translation_sentences (id,source_id,direction,source_text,reference_translation) VALUES (61,60,'jp2cn','こんにちは','你好');
	INSERT INTO translation_records (user_id,sentence_id,user_translation,source_text_snapshot,direction_snapshot,position_snapshot) VALUES (1,61,'你好','こんにちは','jp2cn',0)`)
	if err != nil {
		t.Fatal(err)
	}
	id := e.report(t, "translation", "61")
	before := e.content(t, id)
	readonly := false
	for _, f := range before.Fields {
		if f.Key == "translation:61.source_text" {
			readonly = f.Readonly
		}
	}
	if !readonly {
		t.Fatal("practiced source must be readonly")
	}
	path := fmt.Sprintf("/api/admin/errata/%d/content", id)
	e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{"translation:61.source_text": "別の文"}}, 400)
	e.request(t, "PUT", path, map[string]any{"revision": before.Revision, "values": map[string]string{"translation:61.reference_translation": "您好"}}, 200)
}
