package translation_test

import (
	"encoding/json"
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

func TestTranslationPracticeLocalAssessment(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	store := pgdata.NewTranslationStore(db, store.StoreDeps{})
	const secret = "translation-answer-test"
	for _, tc := range []struct {
		name, source, answer, reference string
		japanese                        bool
		readings                        map[string]string
		want                            translation.AssessmentResult
		wantGrammar                     string
	}{
		{name: "equivalent Chinese answer", source: "バスの運賃はいくらですか？", reference: "公车票价多少？", answer: "公交车的票价是多少钱？", want: translation.AssessmentEquivalent},
		{name: "equivalent Japanese answer", source: "公车票价多少？", reference: "バス賃はいくらですか？", answer: "バスの運賃はいくらでしょうか。", japanese: true, readings: map[string]string{"賃": "ちん"}, want: translation.AssessmentEquivalent},
		{name: "Japanese source grammar still available", source: "激しい雨が降っています。", reference: "雨下得很大。", answer: "雨下得真大。", want: translation.AssessmentUnrecognized, wantGrammar: "local"},
		{name: "negative ending changes meaning", source: "今天不去。", reference: "今日は行きません。", answer: "今日は行きます。", japanese: true, readings: map[string]string{"今日": "きょう", "行き": "いき"}, want: translation.AssessmentMeaningChanged},
		{name: "no reference available", source: "今日は晴れです。", answer: "今天是晴天。", want: translation.AssessmentNoReference},
		{name: "unresolved date keeps its original text", source: "是四月一号。", reference: "四月一日です。", answer: "四月一日です。", japanese: true, readings: map[string]string{"四月": "しがつ"}, want: translation.AssessmentReferenceMatch},
		{name: "kana-only answer", source: "谢谢。", reference: "ありがとうございます。", answer: "ありがとうございます。", japanese: true, want: translation.AssessmentReferenceMatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceID, err := store.SaveSource(translation.TranslationSource{Title: tc.name, SourceType: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sentence := translation.TranslationSentence{
				SourceID: sourceID, Direction: "jp2cn", SourceText: tc.source, ReferenceTranslation: tc.reference, JLPTLevel: "N5",
			}
			if tc.japanese {
				sentence.Direction = "cn2jp"
			}
			sentenceID, err := store.SaveSentence(sentence)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			translation.NewTranslationHandler(translation.NewTranslationService(store, nil)).RegisterRoutes(mux)
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
					ID         int64                              `json:"id"`
					Reference  string                             `json:"reference_translation"`
					Assessment *translation.TranslationAssessment `json:"assessment"`
					AIFeedback json.RawMessage                    `json:"ai_feedback"`
					Tokens     []struct {
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
				if result.Data.Reference != tc.reference {
					t.Errorf("reference = %q, want %q", result.Data.Reference, tc.reference)
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
					if surfaces.String() != tc.reference {
						t.Errorf("annotated text = %q, want %q", surfaces.String(), tc.reference)
					}
					if !maps.Equal(readings, tc.readings) {
						t.Errorf("readings = %v, want %v", readings, tc.readings)
					}
				}
				if result.Data.Assessment == nil || result.Data.Assessment.Result != tc.want || result.Data.Assessment.GrammarSource != tc.wantGrammar {
					t.Errorf("assessment = %+v, want %q with grammar source %q", result.Data.Assessment, tc.want, tc.wantGrammar)
				}
				if len(result.Data.AIFeedback) != 0 {
					t.Errorf("new result unexpectedly includes AI feedback: %s", result.Data.AIFeedback)
				}
				return result.Data.ID
			}
			body := fmt.Sprintf(`{"sentence_id":%d,"user_translation":%q}`, sentenceID, tc.answer)
			recordID := readReference(request(1, http.MethodPost, "/api/v1/translation/submit", body, http.StatusOK))
			if _, err := db.Exec(`UPDATE translation_sentences SET reference_translation = $1 WHERE id = $2`, "修订后的译文。", sentenceID); err != nil {
				t.Fatal(err)
			}
			recordPath := fmt.Sprintf("/api/v1/translation/records/%d", recordID)
			readReference(request(1, http.MethodGet, recordPath, "", http.StatusOK))
			request(2, http.MethodGet, recordPath, "", http.StatusNotFound)
		})
	}
}
