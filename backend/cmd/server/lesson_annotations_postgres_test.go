package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/config"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestPostgresLessonAnnotations(t *testing.T) {
	db := newMigratedPostgresServerDB(t)
	var lessonID, ownerID, otherID, wordID, grammarID int64
	for _, seed := range []struct {
		query string
		id    *int64
	}{
		{`INSERT INTO lessons (title, jlpt_level) VALUES ('課文標注', 'N5') RETURNING id`, &lessonID},
		{`INSERT INTO users (email, password_hash) VALUES ('annotation-owner@example.test', 'test') RETURNING id`, &ownerID},
		{`INSERT INTO users (email, password_hash) VALUES ('annotation-other@example.test', 'test') RETURNING id`, &otherID},
		{`INSERT INTO words (kanji_form, reading, meaning, jlpt_level) VALUES ('散歩', 'さんぽ', '散步', 'N5') RETURNING id`, &wordID},
		{`INSERT INTO grammar_points (name, meaning, jlpt_level) VALUES ('〜ます', '礼貌体', 'N5') RETURNING id`, &grammarID},
	} {
		if err := db.QueryRow(seed.query).Scan(seed.id); err != nil {
			t.Fatalf("seed annotation test: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO lesson_sentences (lesson_id, position, tokens) VALUES (
		$1, 0, '[{"surface":"𠮷野","reading":"よしの"},{"surface":"で散歩します。","reading":""}]')`, lessonID); err != nil {
		t.Fatalf("seed annotation lesson text: %v", err)
	}

	cfg := &config.Config{
		DatabaseURL:       testdb.URL(t, db),
		DBMaxOpenConns:    4,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: time.Minute,
		AppTimezone:       time.UTC,
		JWTSecret:         "annotation-test-secret",
	}
	mux, cleanup, err := buildPostgresServerMux(context.Background(), cfg, t.TempDir(), t.TempDir(), &user.StubMailer{}, "http://localhost")
	if err != nil {
		t.Fatalf("build annotation test mux: %v", err)
	}
	defer cleanup()

	request := func(userID int64, method, path string, body any, status int) json.RawMessage {
		t.Helper()
		var encoded []byte
		if body != nil {
			var err error
			encoded, err = json.Marshal(body)
			if err != nil {
				t.Fatalf("encode request: %v", err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		if userID > 0 {
			token, _, err := user.SignToken(userID, cfg.JWTSecret, time.Minute)
			if err != nil {
				t.Fatalf("sign test token: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s %s status = %d, want %d; body = %s", method, path, rec.Code, status, rec.Body.String())
		}
		if status != http.StatusOK {
			return nil
		}
		var reply struct{ Data json.RawMessage }
		if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
			t.Fatalf("decode annotation reply: %v", err)
		}
		return reply.Data
	}

	references := "/api/v1/lessons/annotation-references"
	if data := request(ownerID, http.MethodGet, references+"?kind=word&q=散歩", nil, http.StatusOK); !bytes.Contains(data, []byte("さんぽ")) {
		t.Fatalf("word search reply = %s", data)
	}
	if data := request(ownerID, http.MethodGet, references+"?kind=grammar&q=ます", nil, http.StatusOK); !bytes.Contains(data, []byte("礼貌体")) {
		t.Fatalf("grammar search reply = %s", data)
	}

	path := fmt.Sprintf("/api/v1/lessons/%d/annotations", lessonID)
	request(0, http.MethodGet, path, nil, http.StatusUnauthorized)
	annotation := map[string]any{"start": 0, "end": 5, "quote": "𠮷野で散歩", "comment": "読み方", "links": []map[string]any{
		{"kind": "word", "id": wordID},
		{"kind": "grammar", "id": grammarID},
	}}
	request(ownerID, http.MethodPut, path, annotation, http.StatusOK)
	annotation["comment"] = "更新した注釈"
	request(ownerID, http.MethodPut, path, annotation, http.StatusOK)
	data := request(ownerID, http.MethodGet, path, nil, http.StatusOK)
	var saved []struct {
		Comment string
		Links   []struct{ Title string }
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode saved annotation: %v", err)
	}
	if len(saved) != 1 || saved[0].Comment != "更新した注釈" || len(saved[0].Links) != 2 ||
		saved[0].Links[0].Title != "散歩" || saved[0].Links[1].Title != "〜ます" {
		t.Fatalf("saved annotations = %s", data)
	}
	request(otherID, http.MethodDelete, path+"/0/5", nil, http.StatusNoContent)
	if data := request(ownerID, http.MethodGet, path, nil, http.StatusOK); string(data) == "[]" {
		t.Fatal("another user removed the owner's annotation")
	}
	if data := request(otherID, http.MethodGet, path, nil, http.StatusOK); string(data) != "[]" {
		t.Fatalf("another user read an annotation: %s", data)
	}
	request(ownerID, http.MethodDelete, path+"/0/5", nil, http.StatusNoContent)
	if data := request(ownerID, http.MethodGet, path, nil, http.StatusOK); string(data) != "[]" {
		t.Fatalf("deleted annotation is still present: %s", data)
	}
}
