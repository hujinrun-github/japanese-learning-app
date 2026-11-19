package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/module/errata"
)

func TestErrataAudioRegeneratesCanonicalTarget(t *testing.T) {
	for _, tc := range []struct {
		name, module, itemKey, target, source, checkSQL, directory string
	}{
		{"word", "word", "101:dictation", "word:101", "こうじょう", `SELECT reading, local_audio_url, audio_object_id IS NULL FROM words WHERE id=101`, "words"},
		{"word example", "word", "101:dictation", "word_example:201", "この町に工場があります。", `SELECT japanese, '', audio_object_id IS NULL FROM word_examples WHERE id=201`, "examples"},
		{"grammar example", "grammar", "301", "grammar_example:401", "町へ行きます。", `SELECT japanese, '', audio_object_id IS NULL FROM grammar_examples WHERE id=401`, "examples"},
		{"speaking", "speaking", "501", "speaking:501", "町を歩きます。", `SELECT text, audio_url_legacy, audio_object_id IS NULL FROM speaking_materials WHERE id=501`, "examples"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			db := openAudioTestDB(t)
			seedErrataAudio(t, db)
			mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description) VALUES(1,1,$1,$2,'工場','audio','町的读音错误')`, tc.module, tc.itemKey)
			wav := errataAudioWAV(t)

			var synthText string
			tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input string `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				synthText = body.Input
				_, _ = w.Write(wav)
			}))
			defer tts.Close()
			h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db)})
			mux := h.RegisterRoutes()
			revision := audioContentRevision(t, mux)
			body := map[string]string{"target_key": tc.target, "revision": revision, "text": "まちにこうじょうがあります。", "provider": "vllm", "tts_url": tts.URL, "tts_model": "test", "voice": "test"}
			resp := audioErrataRequest(t, mux, http.MethodPost, "/api/admin/errata/1/audio", body)
			if resp.Code != http.StatusOK {
				t.Fatalf("regeneration = %d %s", resp.Code, resp.Body)
			}
			if synthText != body["text"] {
				t.Fatalf("synthesized %q, want override %q", synthText, body["text"])
			}
			wantURL := "/audio/" + tc.directory + "/" + audioTestFilename(tc.source)
			var result struct {
				AudioURL string `json:"audio_url"`
				Content  struct {
					Revision string `json:"revision"`
				} `json:"content"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.AudioURL != wantURL || result.Content.Revision == "" {
				t.Fatalf("response = %s, want audio URL %s and refreshed content", resp.Body, wantURL)
			}
			data, err := os.ReadFile(filepath.Join("data", filepath.FromSlash(strings.TrimPrefix(wantURL, "/"))))
			if err != nil || !bytes.Equal(data, wav) {
				t.Fatalf("generated audio = %q, err %v", data, err)
			}
			var source, storedURL, status string
			var cleared bool
			if err := db.QueryRow(tc.checkSQL).Scan(&source, &storedURL, &cleared); err != nil {
				t.Fatal(err)
			}
			if source != tc.source || !cleared || ((tc.module == "speaking" || tc.target == "word:101") && storedURL != wantURL) {
				t.Fatalf("stored source %q, URL %q, stale object cleared %v", source, storedURL, cleared)
			}
			if err := db.QueryRow(`SELECT status FROM errata_reports WHERE id=1`).Scan(&status); err != nil || status != "pending" {
				t.Fatalf("status %q, err %v; generation must not resolve report", status, err)
			}
			// A second replacement must invalidate the first editor version even
			// when the target's audio_object_id was already NULL.
			firstRevision := result.Content.Revision
			body["revision"] = firstRevision
			resp = audioErrataRequest(t, mux, http.MethodPost, "/api/admin/errata/1/audio", body)
			if resp.Code != http.StatusOK {
				t.Fatalf("repeat generation = %d %s", resp.Code, resp.Body)
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Content.Revision == firstRevision {
				t.Fatal("repeat regeneration did not invalidate the previous content version")
			}
		})
	}
}

func TestErrataAudioRestoresExistingFileOnCommitFailure(t *testing.T) {
	db := openAudioTestDB(t)
	seedErrataAudio(t, db)
	mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description) VALUES(1,1,'word','101:dictation','工場','audio','读音错误')`)
	storePath := t.TempDir()
	path := filepath.Join(storePath, "words", audioTestFilename("こうじょう"))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("original-audio"), 0644); err != nil {
		t.Fatal(err)
	}
	// A deferred trigger makes the update succeed and the final COMMIT fail.
	mustExec(t, db, `CREATE FUNCTION fail_audio_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test deferred audio failure'; END; $$`)
	mustExec(t, db, `CREATE CONSTRAINT TRIGGER fail_audio_commit AFTER UPDATE ON words DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_audio_commit()`)
	wav := errataAudioWAV(t)
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(wav) }))
	defer tts.Close()
	h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db), AudioStorePath: storePath})
	mux := h.RegisterRoutes()
	revision := audioContentRevision(t, mux)
	resp := audioErrataRequest(t, mux, http.MethodPost, "/api/admin/errata/1/audio", map[string]string{"target_key": "word:101", "revision": revision, "text": "こうじょう", "provider": "vllm", "tts_url": tts.URL})
	if resp.Code == http.StatusOK {
		t.Fatalf("commit failure reported success: %s", resp.Body)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original-audio" {
		t.Fatalf("existing file was not restored: %q, %v", data, err)
	}
	var objectID sql.NullInt64
	if err := db.QueryRow(`SELECT audio_object_id FROM words WHERE id=101`).Scan(&objectID); err != nil || !objectID.Valid || objectID.Int64 != 1 {
		t.Fatalf("binding changed after commit failure: %v, %v", objectID, err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM errata_reports WHERE id=1`).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("status changed after commit failure: %q, %v", status, err)
	}
}

func TestErrataAudioRejectsInvalidOrFailedRequests(t *testing.T) {
	for _, tc := range []struct {
		name, target, revision, reportStatus, provider string
		ttsStatus, want, calls                         int
	}{
		{"unrelated example", "word_example:202", "current", "pending", "vllm", 200, 400, 0},
		{"invalid target", "../../outside", "current", "pending", "vllm", 200, 400, 0},
		{"stale content", "word:101", "stale", "pending", "vllm", 200, 409, 0},
		{"resolved report", "word:101", "current", "resolved", "vllm", 200, 409, 0},
		{"unsupported provider", "word:101", "current", "pending", "unknown", 200, 400, 0},
		{"tts failure", "word:101", "current", "pending", "vllm", 502, 502, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			db := openAudioTestDB(t)
			seedErrataAudio(t, db)
			mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description,status) VALUES(1,1,'word','101:dictation','工場','audio','读音错误',$1)`, tc.reportStatus)
			calls := 0
			tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.ttsStatus)
				_, _ = w.Write([]byte("tts response"))
			}))
			defer tts.Close()
			h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db)})
			mux := h.RegisterRoutes()
			revision := tc.revision
			if revision == "current" {
				revision = audioContentRevision(t, mux)
			}
			resp := audioErrataRequest(t, mux, http.MethodPost, "/api/admin/errata/1/audio", map[string]string{"target_key": tc.target, "revision": revision, "text": "こうじょう", "provider": tc.provider, "tts_url": tts.URL})
			if resp.Code != tc.want || calls != tc.calls {
				t.Fatalf("status %d (want %d), calls %d (want %d), body %s", resp.Code, tc.want, calls, tc.calls, resp.Body)
			}
			var status string
			if err := db.QueryRow(`SELECT status FROM errata_reports WHERE id=1`).Scan(&status); err != nil || status != tc.reportStatus {
				t.Fatalf("status changed to %q, err %v", status, err)
			}
			if _, err := os.Stat(filepath.Join("data", "audio", "words", audioTestFilename("こうじょう"))); !os.IsNotExist(err) {
				t.Fatalf("failed request unexpectedly wrote canonical audio: %v", err)
			}
		})
	}
}

func TestErrataAudioStoreAndSynthesisRaces(t *testing.T) {
	for _, tc := range []struct {
		name, duringSynthesis string
		blockedStore          bool
		want                  int
	}{
		{"configured shared directory", "", false, http.StatusOK},
		{"storage failure retains database binding", "", true, http.StatusInternalServerError},
		{"source edited while synthesizing", `UPDATE words SET reading='こうば' WHERE id=101`, false, http.StatusConflict},
		{"report resolved while synthesizing", `UPDATE errata_reports SET status='resolved' WHERE id=1`, false, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			db := openAudioTestDB(t)
			seedErrataAudio(t, db)
			mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description) VALUES(1,1,'word','101:dictation','工場','audio','读音错误')`)
			storePath := filepath.Join(t.TempDir(), "shared-audio")
			if tc.blockedStore {
				if err := os.WriteFile(storePath, []byte("not a directory"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			wav := errataAudioWAV(t)
			tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.duringSynthesis != "" {
					mustExec(t, db, tc.duringSynthesis)
				}
				_, _ = w.Write(wav)
			}))
			defer tts.Close()
			h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db), AudioStorePath: storePath})
			mux := h.RegisterRoutes()
			revision := audioContentRevision(t, mux)
			resp := audioErrataRequest(t, mux, http.MethodPost, "/api/admin/errata/1/audio", map[string]string{"target_key": "word:101", "revision": revision, "text": "こうじょう", "provider": "vllm", "tts_url": tts.URL})
			if resp.Code != tc.want {
				t.Fatalf("status %d, want %d; %s", resp.Code, tc.want, resp.Body)
			}
			var source, localURL string
			var objectID sql.NullInt64
			if err := db.QueryRow(`SELECT reading,local_audio_url,audio_object_id FROM words WHERE id=101`).Scan(&source, &localURL, &objectID); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(storePath, "words", audioTestFilename("こうじょう"))
			if tc.want == http.StatusOK {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, wav) {
					t.Fatalf("shared audio %q: %v", data, err)
				}
				if objectID.Valid || localURL == "" {
					t.Fatalf("audio binding not replaced: object %v local %q", objectID, localURL)
				}
				if _, err := os.Stat(filepath.Join("data", "audio", "words", audioTestFilename("こうじょう"))); !os.IsNotExist(err) {
					t.Fatalf("audio also written outside configured storage: %v", err)
				}
			} else {
				if !objectID.Valid || objectID.Int64 != 1 || localURL != "" {
					t.Fatalf("failed generation changed binding: object %v local %q", objectID, localURL)
				}
				if !tc.blockedStore {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("stale generation wrote file: %v", err)
					}
				}
			}
		})
	}
}

func TestErrataAudioRejectsBrokenWAV(t *testing.T) {
	db := openAudioTestDB(t)
	seedErrataAudio(t, db)
	mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description) VALUES(1,1,'word','101:dictation','工場','audio','读音错误')`)
	storePath := t.TempDir()
	path := filepath.Join(storePath, "words", audioTestFilename("こうじょう"))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing-audio"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{"<html>TTS failed</html>", "RIFF\x00\x00\x00\x00WAVE", ""} {
		t.Run(payload, func(t *testing.T) {
			tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(payload)) }))
			defer tts.Close()
			h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db), AudioStorePath: storePath})
			mux := h.RegisterRoutes()
			revision := audioContentRevision(t, mux)
			resp := audioErrataRequest(t, mux, http.MethodPost, "/api/admin/errata/1/audio", map[string]string{"target_key": "word:101", "revision": revision, "text": "こうじょう", "provider": "vllm", "tts_url": tts.URL})
			if resp.Code != http.StatusBadGateway {
				t.Fatalf("invalid TTS response was accepted: %d %s", resp.Code, resp.Body)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "existing-audio" {
				t.Fatalf("invalid TTS response overwrote audio: %q, %v", data, err)
			}
			var objectID sql.NullInt64
			if err := db.QueryRow(`SELECT audio_object_id FROM words WHERE id=101`).Scan(&objectID); err != nil || !objectID.Valid {
				t.Fatalf("invalid audio changed DB binding: %v, %v", objectID, err)
			}
		})
	}
}

func TestErrataAudioExamplePreviewMatchesLearner(t *testing.T) {
	for _, tc := range []struct {
		name, module, itemKey, target, source string
	}{
		{"word example", "word", "101:dictation", "word_example:201", "この町に工場があります。"},
		{"grammar example", "grammar", "301", "grammar_example:401", "町へ行きます。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openAudioTestDB(t)
			seedErrataAudio(t, db)
			mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description) VALUES(1,1,$1,$2,'原文','audio','例句读音错误')`, tc.module, tc.itemKey)
			h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db)})
			before, err := h.loadErrataContent(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			assertURL := func(content ErrataContent, source string) {
				t.Helper()
				for _, target := range content.AudioTargets {
					if target.Key == tc.target {
						want := "/audio/examples/" + audioTestFilename(source)
						if target.AudioURL != want {
							t.Errorf("preview URL %q, want learner's canonical URL %q", target.AudioURL, want)
						}
						return
					}
				}
				t.Fatalf("missing target %s", tc.target)
			}
			// Both examples point to the noncanonical old.wav object. Learner
			// playback still requests the text hash, preferring any local override.
			assertURL(before, tc.source)
			newJapanese := "このまちには大きな工場があります。"
			resp := audioErrataRequest(t, h.RegisterRoutes(), http.MethodPut, "/api/admin/errata/1/content", map[string]any{"revision": before.Revision, "values": map[string]string{tc.target + ".japanese": newJapanese}})
			if resp.Code != http.StatusOK {
				t.Fatalf("save source: %d %s", resp.Code, resp.Body)
			}
			after, err := h.loadErrataContent(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			assertURL(after, newJapanese)
		})
	}
}

func TestErrataAudioWordAndSpeakingPreviewMatchesLearner(t *testing.T) {
	db := openAudioTestDB(t)
	seedErrataAudio(t, db)
	mustExec(t, db, `INSERT INTO errata_reports(id,user_id,module,item_key,item_label,category,description) VALUES(1,1,'word','101:dictation','工場','audio','读音错误'),(2,1,'speaking','501','散步','audio','读音错误')`)
	h := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db, ErrataStore: errata.NewStore(db)})
	for _, tc := range []struct {
		name, setup string
		reportID    int64
		want        string
	}{
		{"word object precedes local", `UPDATE words SET audio_object_id=1,local_audio_url='/audio/words/local.wav' WHERE id=101`, 1, "/api/v1/audio/1/stream"},
		{"word local filename normalized", `UPDATE words SET audio_object_id=NULL,local_audio_url='local.wav' WHERE id=101`, 1, "/audio/words/local.wav"},
		{"word without configured audio", `UPDATE words SET audio_object_id=NULL,local_audio_url='' WHERE id=101`, 1, ""},
		{"speaking legacy precedes object", `UPDATE speaking_materials SET audio_object_id=1,audio_url_legacy='/audio/examples/reference.wav' WHERE id=501`, 2, "/audio/examples/reference.wav"},
		{"speaking missing legacy remains missing", `UPDATE speaking_materials SET audio_object_id=1,audio_url_legacy='' WHERE id=501`, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustExec(t, db, tc.setup)
			content, err := h.loadErrataContent(t.Context(), tc.reportID)
			if err != nil {
				t.Fatal(err)
			}
			if len(content.AudioTargets) == 0 || content.AudioTargets[0].AudioURL != tc.want {
				t.Fatalf("targets %+v, want current learner URL %q", content.AudioTargets, tc.want)
			}
		})
	}
}

func errataAudioWAV(t *testing.T) []byte {
	t.Helper()
	data, err := audio.EncodeWAV([]float64{0, 0.4, -0.4, 0.2, -0.2, 0}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func seedErrataAudio(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO users(id,email,password_hash) VALUES(1,'errata-audio@example.com','hash')`)
	mustExec(t, db, `INSERT INTO audio_objects(id,bucket,object_key,kind,visibility,content_sha256,size_bytes,mime_type) VALUES(1,'audio','old.wav','tts','public','hash',0,'audio/wav')`)
	mustExec(t, db, `INSERT INTO words(id,kanji_form,reading,meaning,jlpt_level,audio_object_id) VALUES(101,'工場','こうじょう','工厂','N5',1),(102,'町','まち','城镇','N5',1)`)
	mustExec(t, db, `INSERT INTO word_examples(id,word_id,position,japanese,chinese,audio_object_id) VALUES(201,101,0,'この町に工場があります。','这个镇上有工厂。',1),(202,102,0,'別の例文です。','其他例句。',1)`)
	mustExec(t, db, `INSERT INTO grammar_points(id,name,meaning,jlpt_level) VALUES(301,'へ','方向','N5')`)
	mustExec(t, db, `INSERT INTO grammar_examples(id,grammar_point_id,position,japanese,chinese,audio_object_id) VALUES(401,301,0,'町へ行きます。','去镇上。',1)`)
	mustExec(t, db, `INSERT INTO speaking_materials(id,type,title,text,jlpt_level,audio_object_id,audio_url_legacy) VALUES(501,'read_aloud','散步','町を歩きます。','N5',1,'/api/v1/audio/1/stream')`)
}

func audioContentRevision(t *testing.T, mux http.Handler) string {
	t.Helper()
	resp := audioErrataRequest(t, mux, http.MethodGet, "/api/admin/errata/1/content", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("load content = %d %s", resp.Code, resp.Body)
	}
	var content struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &content); err != nil || content.Revision == "" {
		t.Fatalf("decode content %s: %v", resp.Body, err)
	}
	return content.Revision
}

func audioErrataRequest(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	return resp
}
