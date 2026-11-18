package shadowing_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/shadowing"
)

// The seeded shadowing lesson ships as 8 kHz mono PCM, while browser WAV
// recorders commonly capture at 16 kHz. These tests keep that real-world
// mismatch from silently breaking the scoring pipeline again.
const pilotAudioRelPath = "../../../data/audio/lessons/shadowing-pilot-n5-day.wav"

func pilotAudioPath(t *testing.T) string {
	t.Helper()
	path := filepath.Clean(pilotAudioRelPath)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("pilot lesson audio not available: %v", err)
	}
	return path
}

// resampleWAV re-encodes a mono WAV at another sample rate with linear
// interpolation, imitating a recorder whose rate differs from the reference.
func resampleWAV(t *testing.T, src []byte, targetRate int) []byte {
	t.Helper()
	parsed, err := audio.ParseWAV(src)
	if err != nil {
		t.Fatalf("ParseWAV: %v", err)
	}
	ratio := float64(targetRate) / float64(parsed.SampleRate)
	n := int(float64(len(parsed.Samples)) * ratio)
	out := make([]float64, n)
	for i := range out {
		pos := float64(i) / ratio
		i0 := int(pos)
		frac := pos - float64(i0)
		switch {
		case i0+1 < len(parsed.Samples):
			out[i] = parsed.Samples[i0]*(1-frac) + parsed.Samples[i0+1]*frac
		case i0 < len(parsed.Samples):
			out[i] = parsed.Samples[i0]
		}
	}
	encoded, err := audio.EncodeWAV(out, targetRate)
	if err != nil {
		t.Fatalf("EncodeWAV: %v", err)
	}
	return encoded
}

func TestEvaluateWAVMixedSampleRates(t *testing.T) {
	tests := []struct {
		name    string
		refRate int
		usrRate int
	}{
		{name: "8kHz reference with 16kHz recording", refRate: 8000, usrRate: 16000},
		{name: "16kHz reference with 8kHz recording", refRate: 16000, usrRate: 8000},
		{name: "44.1kHz reference with 16kHz recording", refRate: 44100, usrRate: 16000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := speechWAV(tt.refRate, 3)
			user := speechWAV(tt.usrRate, 3)
			got, err := shadowing.EvaluateWAV(ref, user, shadowing.EvaluationModeRepeat)
			if err != nil {
				t.Fatalf("EvaluateWAV() err = %v", err)
			}
			if got.Status != "scoreable" {
				t.Fatalf("Status = %q (code=%q), want scoreable", got.Status, got.UnscorableCode)
			}
			if got.OverallScore == nil {
				t.Fatal("scoreable result missing OverallScore")
			}
			t.Logf("ref=%dHz user=%dHz overall=%d partial=%v scores=%+v",
				tt.refRate, tt.usrRate, *got.OverallScore, got.Partial, got.Scores)
		})
	}
}

func TestPilotAudioIsDecodable(t *testing.T) {
	data, err := os.ReadFile(pilotAudioPath(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := audio.ParseWAV(data)
	if err != nil {
		t.Fatalf("ParseWAV(pilot): %v", err)
	}
	if parsed.SampleRate != 8000 {
		t.Errorf("SampleRate = %d, want 8000", parsed.SampleRate)
	}
	durationMS := int64(len(parsed.Samples)) * 1000 / int64(parsed.SampleRate)
	if durationMS != 19000 {
		t.Errorf("duration = %dms, want 19000ms", durationMS)
	}

	sliced, err := audio.SliceWAV(data, 0, 3000)
	if err != nil {
		t.Fatalf("SliceWAV(pilot, 0, 3000): %v", err)
	}
	sentence, err := audio.ParseWAV(sliced)
	if err != nil {
		t.Fatalf("ParseWAV(sliced): %v", err)
	}
	if len(sentence.Samples) != 24000 {
		t.Errorf("sentence samples = %d, want 24000", len(sentence.Samples))
	}
	t.Logf("pilot: rate=%dHz frames=%d duration=%dms sentence_frames=%d",
		parsed.SampleRate, len(parsed.Samples), durationMS, len(sentence.Samples))
}

func pilotEvaluationMux(t *testing.T) (*http.ServeMux, []byte, []byte) {
	t.Helper()
	path := pilotAudioPath(t)
	root, err := filepath.Abs(filepath.Dir(filepath.Dir(path)))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sentenceWAV, err := audio.SliceWAV(data, 0, 3000)
	if err != nil {
		t.Fatal(err)
	}
	recording16k := resampleWAV(t, sentenceWAV, 16000)

	l := testLesson("/audio/lessons/shadowing-pilot-n5-day.wav")
	l.Sentences = []lesson.Sentence{{Index: 0, StartMS: 0, EndMS: 3000}}
	svc := shadowing.NewEvaluationService(
		lessonReaderFunc(func(id int64) (*lesson.Lesson, error) {
			if id != 1 {
				return nil, sql.ErrNoRows
			}
			return l, nil
		}),
		shadowing.NewReferenceAudioReader(root, nil),
		shadowing.NewEvaluator(1),
	)
	mux := http.NewServeMux()
	shadowing.NewEvaluationHandler(svc).RegisterRoutes(mux)
	return mux, sentenceWAV, recording16k
}

func postEvaluation(t *testing.T, mux *http.ServeMux, wav []byte, mode string) (int, shadowing.EvaluationResult) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "attempt.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(wav); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"sentence_index": "0", "mode": mode, "shadowing_version": "1",
	} {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lessons/1/shadowing/evaluations", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp struct {
		Data shadowing.EvaluationResult `json:"data"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v, body=%s", err, rec.Body.String())
		}
	}
	return rec.Code, resp.Data
}

// Exercise a real TCP round trip (not just a ResponseRecorder) so multipart
// streaming, MaxBytesReader and the JSON envelope are covered end to end.
func TestPilotLiveHTTPEvaluation(t *testing.T) {
	mux, _, recording16k := pilotEvaluationMux(t)
	server := httptest.NewServer(mux)
	defer server.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "attempt.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(recording16k); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"sentence_index": "0", "mode": "repeat", "shadowing_version": "1",
	} {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(server.URL+"/api/v1/lessons/1/shadowing/evaluations",
		writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var envelope struct {
		Data      shadowing.EvaluationResult `json:"data"`
		RequestID string                     `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := envelope.Data
	if got.Status != "scoreable" || got.OverallScore == nil {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.ScoreSource != shadowing.ScoreSourceLocal || got.ScorerVersion != shadowing.ScorerVersionLocal {
		t.Errorf("unexpected version tuple: source=%q scorer=%q", got.ScoreSource, got.ScorerVersion)
	}
	if got.WeightsVersion != shadowing.WeightsVersionRepeat || got.ModelVersion != shadowing.ModelVersionLocalOnly {
		t.Errorf("unexpected weights/model version: %q / %q", got.WeightsVersion, got.ModelVersion)
	}
	if got.ShadowingVersion != 1 {
		t.Errorf("ShadowingVersion = %d, want 1", got.ShadowingVersion)
	}
	t.Logf("live HTTP: overall=%d latency_ms=%d request_id=%q", *got.OverallScore, got.LatencyMS, envelope.RequestID)
}

// The real lesson audio used to produce an empty issues array. Guard against
// that regression and make sure every suggestion quotes a measured number.
func TestPilotAdviceIsConcrete(t *testing.T) {
	mux, _, recording16k := pilotEvaluationMux(t)
	status, got := postEvaluation(t, mux, recording16k, "repeat")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got.Status != "scoreable" {
		t.Fatalf("Status = %q (code=%q), want scoreable", got.Status, got.UnscorableCode)
	}
	if len(got.Issues) == 0 {
		t.Fatal("real lesson audio produced no advice")
	}
	for _, issue := range got.Issues {
		evidence, _ := json.Marshal(issue.EvidenceZH)
		t.Logf("advice: type=%s dimension=%s evidence=%s action=%s",
			issue.Type, issue.Dimension, evidence, issue.ActionZH)
		hasDigit := false
		for _, r := range issue.EvidenceZH {
			if unicode.IsDigit(r) {
				hasDigit = true
				break
			}
		}
		if !hasDigit {
			t.Errorf("advice %q evidence %q has no measured number", issue.Type, issue.EvidenceZH)
		}
		if issue.ActionZH == "" {
			t.Errorf("advice %q has no concrete action", issue.Type)
		}
	}
}

func TestPilotEndToEndEvaluation(t *testing.T) {
	mux, sentenceWAV, recording16k := pilotEvaluationMux(t)

	tests := []struct {
		name       string
		audio      []byte
		mode       string
		wantStatus int
		wantResult string
		wantCode   string
	}{
		{
			name: "repeat with identical 8kHz recording", audio: sentenceWAV, mode: "repeat",
			wantStatus: http.StatusOK, wantResult: "scoreable",
		},
		{
			name: "repeat with 16kHz browser-style recording", audio: recording16k, mode: "repeat",
			wantStatus: http.StatusOK, wantResult: "scoreable",
		},
		{
			name: "shadow with reference fed back", audio: sentenceWAV, mode: "shadow",
			wantStatus: http.StatusOK, wantResult: "unscorable", wantCode: "bleed_through",
		},
		{
			name: "silence", audio: silentWAV(16000, 3), mode: "shadow",
			wantStatus: http.StatusOK, wantResult: "unscorable", wantCode: "silence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, got := postEvaluation(t, mux, tt.audio, tt.mode)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if got.Status != tt.wantResult {
				t.Fatalf("Status = %q (code=%q), want %q", got.Status, got.UnscorableCode, tt.wantResult)
			}
			if tt.wantCode != "" && got.UnscorableCode != tt.wantCode {
				t.Errorf("UnscorableCode = %q, want %q", got.UnscorableCode, tt.wantCode)
			}
			if len(got.Issues) == 0 {
				t.Error("evaluation returned no improvement advice")
			}
			for i, issue := range got.Issues {
				if issue.EvidenceZH == "" || issue.ActionZH == "" {
					t.Errorf("advice[%d] (%s) missing evidence or action: %+v", i, issue.Type, issue)
				}
			}
			if tt.wantResult == "scoreable" {
				if got.OverallScore == nil {
					t.Fatal("scoreable result missing OverallScore")
				}
				if !got.Partial {
					t.Error("S2 result should be partial (no ASR dimensions)")
				}
				if got.Scores["coverage"].Available || got.Scores["segmental"].Available {
					t.Error("coverage/segmental must stay unavailable before ASR is wired")
				}
			}
			payload, _ := json.Marshal(got)
			t.Logf("result: %s", payload)
		})
	}
}
