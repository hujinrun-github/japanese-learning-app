package speaking_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"japanese-learning-app/internal/audio"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/shadowing"
	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

func TestSpeakingEvaluationPronunciation(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO speaking_materials
		(id,type,title,text,audio_url_legacy,jlpt_level)
		VALUES (1,'free','道を聞く','駅はどこですか。',$1,'N5')`, evaluationExplicitURL); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	wav := evaluationSpeechWAV(16000, 3)
	writeEvaluationFile(t, filepath.Join(dir, "speaking", "ref.wav"), wav)
	for _, tt := range []struct {
		name, wantStatus string
		configured       bool
		asrStatus        int
		silent           bool
	}{
		{"configured ASR marks wrong word", "available", true, http.StatusOK, false},
		{"ASR failure retains prosody score", "error", true, http.StatusServiceUnavailable, false},
		{"unconfigured ASR is explicit", "unavailable", false, http.StatusOK, false},
		{"silent recording does not call ASR", "unavailable", true, http.StatusOK, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var asrCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				asrCalls.Add(1)
				w.WriteHeader(tt.asrStatus)
				if _, err := w.Write([]byte(`{"text":"海はどこですか。"}`)); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			svc := speaking.NewSpeakingService(pgdata.NewSpeakingStore(db, store.StoreDeps{}), speaking.NewWaveformScorer())
			handler := speaking.NewEvaluationHandler(svc, dir, nil, shadowing.NewEvaluator(1))
			if tt.configured {
				analyzer, err := speaking.NewPronunciationAnalyzer(speaking.NewVLLMScorer(srv.URL, "whisper-test", time.Second))
				if err != nil {
					t.Fatal(err)
				}
				handler.SetPronunciationAnalyzer(analyzer)
			}
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			var body bytes.Buffer
			recording := wav
			if tt.silent {
				var err error
				recording, err = audio.EncodeWAV(make([]float64, 16000*3), 16000)
				if err != nil {
					t.Fatal(err)
				}
			}
			form := multipart.NewWriter(&body)
			part, err := form.CreateFormFile("audio", "attempt.wav")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(recording); err != nil {
				t.Fatal(err)
			}
			if err := form.WriteField("material_id", "1"); err != nil {
				t.Fatal(err)
			}
			if err := form.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/speaking/evaluate", &body)
			req.Header.Set("Content-Type", form.FormDataContentType())
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var result struct {
				Data struct {
					shadowing.EvaluationResult
					Pronunciation speaking.PronunciationFeedback `json:"pronunciation"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !tt.silent && (result.Data.Status != "scoreable" || result.Data.OverallScore == nil || result.Data.ScoreSource != "local_prosody_v1") {
				t.Errorf("prosody score was lost: %s", response.Body.String())
			}
			if tt.silent && (result.Data.Status != "unscorable" || asrCalls.Load() != 0) {
				t.Errorf("silent recording must stay unscorable without calling ASR: %s calls=%d", response.Body.String(), asrCalls.Load())
			}
			feedback := result.Data.Pronunciation
			if tt.silent && feedback.Reason != "insufficient_audio" {
				t.Errorf("silent recording must explain missing audio evidence: %+v", feedback)
			}
			if feedback.Status != tt.wantStatus {
				t.Fatalf("pronunciation status=%s want %s: %s", feedback.Status, tt.wantStatus, response.Body.String())
			}
			if tt.wantStatus == "available" {
				if result.Data.RecognizedText != "海はどこですか。" || len(feedback.Words) == 0 || feedback.Words[0].Surface != "駅" || feedback.Words[0].Status != "mismatch" {
					t.Errorf("wrong-word feedback missing: %s", response.Body.String())
				}
			} else if len(feedback.Words) != 0 || result.Data.RecognizedText != "" {
				t.Errorf("unavailable ASR emitted judgements: %s", response.Body.String())
			}
		})
	}
}
