package speaking_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"japanese-learning-app/internal/audio"
	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/shadowing"
	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

const (
	evaluationRefText     = "すみません、駅はどこですか。あの角を右に曲がってください。"
	evaluationExplicitURL = "/audio/speaking/ref.wav"
)

// evaluationSpeechWAV builds a real 16 kHz PCM WAV with voiced segments so the
// local prosody scorer has evidence to work with (same shape as the shadowing
// evaluation tests).
func evaluationSpeechWAV(rate, seconds int) []byte {
	n := rate * seconds
	s := make([]float64, n)
	freqs := []float64{120, 150, 130}
	amps := []float64{0.5, 0.35, 0.45}
	for i := range s {
		t := float64(i) / float64(rate)
		seg := int(t) % len(freqs)
		amp := amps[seg]
		if math.Mod(t, 1.0) < 0.1 {
			amp = 0
		}
		s[i] = amp * (math.Sin(2*math.Pi*freqs[seg]*t) + 0.3*math.Sin(4*math.Pi*freqs[seg]*t))
	}
	out, err := audio.EncodeWAV(s, rate)
	if err != nil {
		panic(err)
	}
	return out
}

func writeEvaluationFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestSpeakingEvaluationHandler(t *testing.T) {
	ref := evaluationSpeechWAV(16000, 3)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(evaluationRefText)))[:16]
	tooLarge := bytes.Repeat([]byte{1}, 10<<20+1)

	tests := []struct {
		name          string
		materialID    string
		mode          string
		audio         []byte
		audioPart     bool
		writeRef      bool
		writeHashRef  bool
		wantStatus    int
		wantCode      string
		wantMode      string
		wantScoreable bool
	}{
		{
			name:          "scores with explicit audio_url",
			materialID:    "1",
			audio:         ref,
			audioPart:     true,
			writeRef:      true,
			wantStatus:    http.StatusOK,
			wantMode:      "repeat",
			wantScoreable: true,
		},
		{
			name:         "falls back to content-addressed TTS audio",
			materialID:   "2",
			audio:        ref,
			audioPart:    true,
			writeHashRef: true,
			wantStatus:   http.StatusOK,
			wantMode:     "shadow",
		},
		{
			name:       "mode override is honored",
			materialID: "1",
			mode:       "shadow",
			audio:      ref,
			audioPart:  true,
			writeRef:   true,
			wantStatus: http.StatusOK,
			wantMode:   "shadow",
		},
		{
			name:       "rejects invalid mode",
			materialID: "1",
			mode:       "karaoke",
			audio:      ref,
			audioPart:  true,
			writeRef:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ERR_SPEAKING_EVAL_INVALID",
		},
		{
			name:       "missing audio part",
			materialID: "1",
			audioPart:  false,
			writeRef:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ERR_SPEAKING_EVAL_INVALID",
		},
		{
			name:       "audio larger than limit",
			materialID: "1",
			audio:      tooLarge,
			audioPart:  true,
			writeRef:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ERR_SPEAKING_EVAL_INVALID",
		},
		{
			name:       "invalid material id",
			materialID: "abc",
			audio:      ref,
			audioPart:  true,
			writeRef:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ERR_SPEAKING_EVAL_INVALID",
		},
		{
			name:       "unknown material",
			materialID: "999",
			audio:      ref,
			audioPart:  true,
			writeRef:   true,
			wantStatus: http.StatusNotFound,
			wantCode:   "ERR_NOT_FOUND",
		},
		{
			name:       "reference audio missing",
			materialID: "1",
			audio:      ref,
			audioPart:  true,
			writeRef:   false,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "ERR_SPEAKING_REFERENCE_MISSING",
		},
		{
			name:         "content-addressed reference missing",
			materialID:   "2",
			audio:        ref,
			audioPart:    true,
			writeHashRef: false,
			wantStatus:   http.StatusUnprocessableEntity,
			wantCode:     "ERR_SPEAKING_REFERENCE_MISSING",
		},
		{
			name:       "user audio is not a WAV",
			materialID: "1",
			audio:      []byte("not a wav"),
			audioPart:  true,
			writeRef:   true,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "ERR_SPEAKING_AUDIO_UNSUPPORTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.writeRef {
				writeEvaluationFile(t, filepath.Join(dir, "speaking", "ref.wav"), ref)
			}
			if tt.writeHashRef {
				writeEvaluationFile(t, filepath.Join(dir, "examples", hash+".wav"), ref)
			}

			db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
			if _, err := db.Exec(`INSERT INTO speaking_materials
				(id, type, title, text, audio_url_legacy, jlpt_level) VALUES
				(1, 'free', '自由朗读', $1, $2, 'N5'),
				(2, 'shadow', '道を聞く', $3, '', 'N5')`,
				evaluationRefText, evaluationExplicitURL, evaluationRefText); err != nil {
				t.Fatalf("insert speaking_materials: %v", err)
			}

			svc := speaking.NewSpeakingService(pgdata.NewSpeakingStore(db, store.StoreDeps{}), speaking.NewWaveformScorer())
			mux := http.NewServeMux()
			speaking.NewEvaluationHandler(svc, dir, nil, shadowing.NewEvaluator(1)).RegisterRoutes(mux)

			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			if tt.audioPart {
				part, err := form.CreateFormFile("audio", "attempt.wav")
				if err != nil {
					t.Fatalf("CreateFormFile: %v", err)
				}
				if _, err := part.Write(tt.audio); err != nil {
					t.Fatalf("write audio part: %v", err)
				}
			}
			if err := form.WriteField("material_id", tt.materialID); err != nil {
				t.Fatalf("WriteField material_id: %v", err)
			}
			if tt.mode != "" {
				if err := form.WriteField("mode", tt.mode); err != nil {
					t.Fatalf("WriteField mode: %v", err)
				}
			}
			if err := form.Close(); err != nil {
				t.Fatalf("close form: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/speaking/evaluate", &body)
			req.Header.Set("Content-Type", form.FormDataContentType())
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var payload struct {
				Data struct {
					Status       string `json:"status"`
					Mode         string `json:"mode"`
					OverallScore *int   `json:"overall_score"`
					ScoreSource  string `json:"score_source"`
				} `json:"data"`
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response %q: %v", rec.Body.String(), err)
			}
			if tt.wantCode != "" && payload.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", payload.Code, tt.wantCode)
			}
			if tt.wantStatus == http.StatusOK {
				if payload.Data.Status == "" {
					t.Error("empty evaluation status")
				}
				if payload.Data.Mode != tt.wantMode {
					t.Errorf("mode = %q, want %q", payload.Data.Mode, tt.wantMode)
				}
				if payload.Data.ScoreSource != "local_prosody_v1" {
					t.Errorf("score_source = %q, want local_prosody_v1", payload.Data.ScoreSource)
				}
				if tt.wantScoreable && (payload.Data.Status != "scoreable" || payload.Data.OverallScore == nil) {
					t.Errorf("status = %q, overall_score = %v, want a scoreable result",
						payload.Data.Status, payload.Data.OverallScore)
				}
			}
		})
	}
}

func TestSpeakingLongRecording(t *testing.T) {
	// The longest textbook reference is 99.3 seconds. Also exercise the full
	// five-minute allowance so slower learners have room for pauses and repeats.
	for _, seconds := range []int{100, 300} {
		t.Run(fmt.Sprintf("%d seconds", seconds), func(t *testing.T) {
			ref := evaluationSpeechWAV(8000, seconds)
			recording := evaluationSpeechWAV(16000, seconds)
			dir := t.TempDir()
			writeEvaluationFile(t, filepath.Join(dir, "speaking", "ref.wav"), ref)
			db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
			if _, err := db.Exec(`INSERT INTO speaking_materials
				(id,type,title,text,audio_url_legacy,jlpt_level)
				VALUES (1,'shadow','長い課文',$1,$2,'N4')`, evaluationRefText, evaluationExplicitURL); err != nil {
				t.Fatal(err)
			}
			svc := speaking.NewSpeakingService(pgdata.NewSpeakingStore(db, store.StoreDeps{}), speaking.NewWaveformScorer())
			mux := http.NewServeMux()
			speaking.NewEvaluationHandler(svc, dir, nil, shadowing.NewEvaluator(1)).RegisterRoutes(mux)
			var body bytes.Buffer
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
			// Use repeat mode to exercise all scoring dimensions, rather than the
			// shadow-mode reference-bleed guard for these matching test signals.
			if err := form.WriteField("mode", "repeat"); err != nil {
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
				t.Fatalf("status=%d: %s", response.Code, response.Body.String())
			}
			var payload struct {
				Data struct {
					Status       string `json:"status"`
					OverallScore *int   `json:"overall_score"`
					ScoreSource  string `json:"score_source"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Data.Status != "scoreable" || payload.Data.OverallScore == nil || payload.Data.ScoreSource != "local_prosody_v1" {
				t.Fatalf("long recording was not scored: %s", response.Body.String())
			}
		})
	}
}
