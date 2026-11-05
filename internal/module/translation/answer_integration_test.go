package translation_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
)

func TestTranslationPracticeReferenceAnswer(t *testing.T) {
	db, err := data.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := data.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	store := data.NewTranslationStore(db)
	const secret = "translation-answer-test"
	const reference = "雨下得很大。"
	const generated = "雨下得很猛。"
	for _, tc := range []struct {
		name     string
		stored   string
		reviewer translation.TranslationReviewer
		want     string
	}{
		{"bank answer without AI", reference, nil, reference},
		{"bank answer wins over AI alternative", reference, &translation.StubReviewer{
			Feedback: translation.TranslationFeedback{ReferenceTranslation: generated},
		}, reference},
		{"AI creates missing reference", "", &translation.StubReviewer{
			Feedback: translation.TranslationFeedback{ReferenceTranslation: generated},
		}, generated},
		{"bank answer survives AI error", reference, &translation.StubReviewer{Err: errors.New("review unavailable")}, reference},
		{"no reference available", "", &translation.StubReviewer{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceID, err := store.SaveSource(translation.TranslationSource{Title: tc.name, SourceType: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sentenceID, err := store.SaveSentence(translation.TranslationSentence{
				SourceID: sourceID, Direction: "jp2cn", SourceText: "激しい雨が降っています。", ReferenceTranslation: tc.stored,
			})
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			translation.NewTranslationHandler(translation.NewTranslationService(store, tc.reviewer)).RegisterRoutes(mux)
			handler := user.AuthMiddleware(secret, mux)
			request := func(userID int64, method, path, body string, status int) *httptest.ResponseRecorder {
				t.Helper()
				token, _, err := user.SignToken(userID, secret, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+token)
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)
				if resp.Code != status {
					t.Fatalf("%s %s: status = %d, want %d; body = %s", method, path, resp.Code, status, resp.Body)
				}
				return resp
			}

			for _, path := range []string{
				"/api/v1/translation/queue",
				fmt.Sprintf("/api/v1/translation/sentences?source_id=%d", sourceID),
			} {
				resp := request(2, http.MethodGet, path, "", http.StatusOK)
				var questions struct {
					Data []translation.TranslationSentence `json:"data"`
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &questions); err != nil {
					t.Fatal(err)
				}
				for _, q := range questions.Data {
					if q.ReferenceTranslation != "" {
						t.Errorf("GET %s revealed a reference translation before submission", path)
					}
				}
			}

			type recordResponse struct {
				Data struct {
					ID        int64  `json:"id"`
					Reference string `json:"reference_translation"`
				} `json:"data"`
			}
			readReference := func(resp *httptest.ResponseRecorder) int64 {
				t.Helper()
				var result recordResponse
				if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Data.Reference != tc.want {
					t.Errorf("reference = %q, want %q", result.Data.Reference, tc.want)
				}
				return result.Data.ID
			}
			body := fmt.Sprintf(`{"sentence_id":%d,"user_translation":"我的回答。"}`, sentenceID)
			recordID := readReference(request(1, http.MethodPost, "/api/v1/translation/submit", body, http.StatusOK))
			recordPath := fmt.Sprintf("/api/v1/translation/records/%d", recordID)
			readReference(request(1, http.MethodGet, recordPath, "", http.StatusOK))
			request(2, http.MethodGet, recordPath, "", http.StatusNotFound)
		})
	}
}
