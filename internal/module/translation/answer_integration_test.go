package translation_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

func TestTranslationPracticeReferenceAnswer(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	store := pgdata.NewTranslationStore(db, store.StoreDeps{})
	const secret = "translation-answer-test"
	const reference = "雨下得很大。"
	const generated = "雨下得很猛。"
	for _, tc := range []struct {
		name     string
		stored   string
		reviewer translation.TranslationReviewer
		want     string
		japanese bool
		readings map[string]string
	}{
		{name: "bank answer without AI", stored: reference, want: reference},
		{name: "bank answer wins over AI alternative", stored: reference, reviewer: &translation.StubReviewer{
			Feedback: translation.TranslationFeedback{ReferenceTranslation: generated},
		}, want: reference},
		{name: "AI creates missing reference", reviewer: &translation.StubReviewer{
			Feedback: translation.TranslationFeedback{ReferenceTranslation: generated},
		}, want: generated},
		{name: "bank answer survives AI error", stored: reference, reviewer: &translation.StubReviewer{Err: errors.New("review unavailable")}, want: reference},
		{name: "no reference available", reviewer: &translation.StubReviewer{}},
		{name: "Japanese bank answer", stored: "バス賃はいくらですか？", want: "バス賃はいくらですか？", japanese: true, readings: map[string]string{"賃": "ちん"}},
		{name: "Japanese generated answer", reviewer: &translation.StubReviewer{
			Feedback: translation.TranslationFeedback{ReferenceTranslation: "今日、一人で行きます。"},
		}, want: "今日、一人で行きます。", japanese: true, readings: map[string]string{"今日": "きょう", "一人": "ひとり", "行き": "いき"}},
		{name: "unresolved date keeps its original text", stored: "四月一日です。", want: "四月一日です。", japanese: true, readings: map[string]string{"四月": "しがつ"}},
		{name: "kana-only answer", stored: "ありがとうございます。", want: "ありがとうございます。", japanese: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceID, err := store.SaveSource(translation.TranslationSource{Title: tc.name, SourceType: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sentence := translation.TranslationSentence{
				SourceID: sourceID, Direction: "jp2cn", SourceText: "激しい雨が降っています。", ReferenceTranslation: tc.stored, JLPTLevel: "N5",
			}
			if tc.japanese {
				sentence.Direction, sentence.SourceText = "cn2jp", "公车票价多少？"
			}
			sentenceID, err := store.SaveSentence(sentence)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			translation.NewTranslationHandler(translation.NewTranslationService(store, tc.reviewer, nil)).RegisterRoutes(mux)
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
				if len(questions.Data) == 0 {
					t.Fatalf("GET %s returned no practice questions", path)
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
					Tokens    []struct {
						Surface string `json:"surface"`
						Reading string `json:"reading"`
					} `json:"reference_tokens"`
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
				if len(tc.readings) == 0 && len(result.Data.Tokens) > 0 {
					t.Errorf("unexpected reference tokens: %+v", result.Data.Tokens)
				}
				if len(tc.readings) > 0 {
					var surfaces strings.Builder
					readings := make(map[string]string)
					for _, part := range result.Data.Tokens {
						surfaces.WriteString(part.Surface)
						if part.Reading != "" {
							readings[part.Surface] = part.Reading
						}
					}
					if surfaces.String() != tc.want {
						t.Errorf("annotated text = %q, want %q", surfaces.String(), tc.want)
					}
					if !maps.Equal(readings, tc.readings) {
						t.Errorf("readings = %v, want %v", readings, tc.readings)
					}
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
