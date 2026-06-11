package admin

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"japanese-learning-app/internal/data"
)

func TestBatchGenerateAudioSpeaking(t *testing.T) {
	t.Chdir(t.TempDir())

	db := openAudioTestDB(t)
	defer db.Close()
	mustExec(t, db, `CREATE TABLE speaking_materials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		text TEXT NOT NULL,
		audio_url TEXT NOT NULL DEFAULT '',
		jlpt_level TEXT NOT NULL
	)`)
	mustExec(t, db, `INSERT INTO speaking_materials (type, title, text, audio_url, jlpt_level) VALUES
		('read_aloud', 'n5', 'alpha', '', 'N5'),
		('read_aloud', 'n4', 'beta', '', 'N4')`)

	var synthRequests []string
	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("tts method = %s, want POST", r.Method)
		}
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode tts request: %v", err)
		}
		synthRequests = append(synthRequests, body.Input)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("wav:" + body.Input))
	}))
	defer ttsServer.Close()

	handler := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	reqBody := fmt.Sprintf(`{"module":"speaking","level":"N5","provider":"vllm","tts_url":%q,"tts_model":"test","voice":"test"}`, ttsServer.URL)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/audio/batch", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /audio/batch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Module    string `json:"module"`
		Total     int    `json:"total"`
		Generated int    `json:"generated"`
		Existing  int    `json:"existing"`
		Failed    int    `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Module != "speaking" || got.Total != 1 || got.Generated != 1 || got.Existing != 0 || got.Failed != 0 {
		t.Fatalf("response = %+v", got)
	}
	if fmt.Sprint(synthRequests) != fmt.Sprint([]string{"alpha"}) {
		t.Fatalf("synth requests = %v, want [alpha]", synthRequests)
	}
	if _, err := os.Stat(filepath.Join("data", "audio", "examples", audioTestFilename("alpha"))); err != nil {
		t.Fatalf("expected audio file for alpha: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data", "audio", "examples", audioTestFilename("beta"))); !os.IsNotExist(err) {
		t.Fatalf("expected no audio file for beta, stat err = %v", err)
	}
}

func TestBatchGenerateAudioUsesExplicitTexts(t *testing.T) {
	t.Chdir(t.TempDir())

	db := openAudioTestDB(t)
	defer db.Close()
	mustExec(t, db, `CREATE TABLE speaking_materials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		text TEXT NOT NULL,
		audio_url TEXT NOT NULL DEFAULT '',
		jlpt_level TEXT NOT NULL
	)`)
	mustExec(t, db, `INSERT INTO speaking_materials (type, title, text, audio_url, jlpt_level) VALUES
		('read_aloud', 'n5', 'alpha', '', 'N5'),
		('read_aloud', 'n4', 'beta', '', 'N4')`)

	var synthRequests []string
	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode tts request: %v", err)
		}
		synthRequests = append(synthRequests, body.Input)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("wav:" + body.Input))
	}))
	defer ttsServer.Close()

	handler := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	reqBody := fmt.Sprintf(`{"module":"speaking","level":"N5","texts":["beta"],"provider":"vllm","tts_url":%q,"tts_model":"test","voice":"test"}`, ttsServer.URL)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/audio/batch", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /audio/batch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if fmt.Sprint(synthRequests) != fmt.Sprint([]string{"beta"}) {
		t.Fatalf("synth requests = %v, want [beta]", synthRequests)
	}
	if _, err := os.Stat(filepath.Join("data", "audio", "examples", audioTestFilename("beta"))); err != nil {
		t.Fatalf("expected audio file for beta: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data", "audio", "examples", audioTestFilename("alpha"))); !os.IsNotExist(err) {
		t.Fatalf("expected no audio file for alpha, stat err = %v", err)
	}
}

func TestBatchGenerateAudioUsesExplicitWordIDs(t *testing.T) {
	t.Chdir(t.TempDir())

	db := openAudioTestDB(t)
	defer db.Close()
	mustExec(t, db, `CREATE TABLE words (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kanji_form TEXT NOT NULL,
		reading TEXT NOT NULL,
		audio_url TEXT NOT NULL DEFAULT '',
		jlpt_level TEXT NOT NULL
	)`)
	mustExec(t, db, `INSERT INTO words (kanji_form, reading, audio_url, jlpt_level) VALUES
		('一', 'いち', '', 'N5'),
		('二', 'に', '', 'N5')`)

	var synthRequests []string
	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode tts request: %v", err)
		}
		synthRequests = append(synthRequests, body.Input)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("wav:" + body.Input))
	}))
	defer ttsServer.Close()

	handler := NewHandler(HandlerConfig{AdminToken: "test-token", DB: db})
	srv := httptest.NewServer(handler.RegisterRoutes())
	defer srv.Close()

	reqBody := fmt.Sprintf(`{"module":"words","word_ids":[2],"provider":"vllm","tts_url":%q,"tts_model":"test","voice":"test"}`, ttsServer.URL)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/audio/batch", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /audio/batch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if fmt.Sprint(synthRequests) != fmt.Sprint([]string{"に"}) {
		t.Fatalf("synth requests = %v, want [に]", synthRequests)
	}

	var audioURL string
	if err := db.QueryRow("SELECT audio_url FROM words WHERE id = 2").Scan(&audioURL); err != nil {
		t.Fatalf("query audio_url: %v", err)
	}
	if audioURL != audioTestFilename("に") {
		t.Fatalf("audio_url = %q, want %q", audioURL, audioTestFilename("に"))
	}
	if _, err := os.Stat(filepath.Join("data", "audio", "words", audioTestFilename("いち"))); !os.IsNotExist(err) {
		t.Fatalf("expected no audio file for いち, stat err = %v", err)
	}
}

func openAudioTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := data.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func audioTestFilename(text string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
	return hash + ".wav"
}
