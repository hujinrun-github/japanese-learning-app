package shadowing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/shadowing"
)

func speechWAV(rate, seconds int) []byte {
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

func shiftedWAV(ref []byte, shiftMS int) []byte {
	parsed, err := audio.ParseWAV(ref)
	if err != nil {
		panic(err)
	}
	shift := parsed.SampleRate * shiftMS / 1000
	out := make([]float64, len(parsed.Samples))
	copy(out[shift:], parsed.Samples[:len(parsed.Samples)-shift])
	encoded, err := audio.EncodeWAV(out, parsed.SampleRate)
	if err != nil {
		panic(err)
	}
	return encoded
}

func silentWAV(rate, seconds int) []byte {
	out, err := audio.EncodeWAV(make([]float64, rate*seconds), rate)
	if err != nil {
		panic(err)
	}
	return out
}

func dimension(score, confidence float64, available bool) shadowing.DimensionScore {
	return shadowing.DimensionScore{Score: score, Confidence: confidence, Available: available}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		f    shadowing.AudioFeatures
		mode shadowing.EvaluationMode
		want string
	}{
		{
			name: "silence",
			f:    shadowing.AudioFeatures{VoicedRatio: 0.01, SpeechMS: 3000, ReferenceMS: 3000},
			mode: shadowing.EvaluationModeShadow,
			want: "silence",
		},
		{
			name: "too short",
			f:    shadowing.AudioFeatures{VoicedRatio: 0.8, SpeechMS: 300, ReferenceMS: 3000},
			mode: shadowing.EvaluationModeShadow,
			want: "too_short",
		},
		{
			name: "speaker bleed through",
			f:    shadowing.AudioFeatures{VoicedRatio: 0.9, SpeechMS: 3000, ReferenceMS: 3000, LagMS: 0, Peak: 0.95},
			mode: shadowing.EvaluationModeShadow,
			want: "bleed_through",
		},
		{
			name: "repeat mode does not check bleed through",
			f:    shadowing.AudioFeatures{VoicedRatio: 0.9, SpeechMS: 3000, ReferenceMS: 3000, LagMS: 0, Peak: 0.95},
			mode: shadowing.EvaluationModeRepeat,
			want: "",
		},
		{
			name: "normal shadowing lag",
			f:    shadowing.AudioFeatures{VoicedRatio: 0.8, SpeechMS: 2900, ReferenceMS: 3000, LagMS: 600, Peak: 0.92},
			mode: shadowing.EvaluationModeShadow,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shadowing.Classify(tt.f, tt.mode); got != tt.want {
				t.Errorf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAggregate(t *testing.T) {
	three := map[string]shadowing.DimensionScore{
		"sync":      dimension(70, 0.9, true),
		"prosody":   dimension(85, 0.9, true),
		"coverage":  dimension(0, 0, false),
		"fluency":   dimension(80, 0.9, true),
		"segmental": dimension(0, 0, false),
	}
	lowConfidence := map[string]shadowing.DimensionScore{
		"sync":      dimension(70, 0.3, true),
		"prosody":   dimension(85, 0.2, true),
		"coverage":  dimension(0, 0, false),
		"fluency":   dimension(80, 0.2, true),
		"segmental": dimension(0, 0, false),
	}

	tests := []struct {
		name        string
		scores      map[string]shadowing.DimensionScore
		mode        shadowing.EvaluationMode
		wantValid   bool
		wantPartial bool
		wantMin     int
		wantMax     int
		wantCode    string
	}{
		{
			name:   "three dimensions with unavailable ASR dimensions",
			scores: three, mode: shadowing.EvaluationModeShadow,
			wantValid: true, wantPartial: true, wantMin: 70, wantMax: 90,
		},
		{
			name: "all dimensions available",
			scores: map[string]shadowing.DimensionScore{
				"sync": dimension(80, 0.9, true), "prosody": dimension(80, 0.9, true),
				"coverage": dimension(80, 0.9, true), "fluency": dimension(80, 0.9, true),
				"segmental": dimension(80, 0.9, true),
			},
			mode:      shadowing.EvaluationModeRepeat,
			wantValid: true, wantPartial: false, wantMin: 80, wantMax: 80,
		},
		{
			name:   "low confidence everywhere",
			scores: lowConfidence, mode: shadowing.EvaluationModeShadow,
			wantValid: false, wantCode: "low_confidence",
		},
		{
			name: "empty scores", scores: map[string]shadowing.DimensionScore{},
			mode:      shadowing.EvaluationModeShadow,
			wantValid: false, wantCode: "low_confidence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shadowing.Aggregate(tt.scores, tt.mode)
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v, want %v (code=%q)", got.Valid, tt.wantValid, got.Code)
			}
			if !tt.wantValid {
				if got.Code != tt.wantCode {
					t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
				}
				return
			}
			if got.Partial != tt.wantPartial {
				t.Errorf("Partial = %v, want %v", got.Partial, tt.wantPartial)
			}
			if got.Overall < tt.wantMin || got.Overall > tt.wantMax {
				t.Errorf("Overall = %d, want [%d, %d]", got.Overall, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEvaluateWAV(t *testing.T) {
	ref := speechWAV(16000, 3)
	tests := []struct {
		name       string
		ref        []byte
		user       []byte
		mode       shadowing.EvaluationMode
		wantStatus string
		wantCode   string
		wantMin    int
		wantErr    bool
	}{
		{
			name: "shifted shadowing is scoreable", ref: ref,
			user: shiftedWAV(ref, 600), mode: shadowing.EvaluationModeShadow,
			wantStatus: "scoreable", wantMin: 55,
		},
		{
			name: "repeat identical is scoreable", ref: ref,
			user: ref, mode: shadowing.EvaluationModeRepeat,
			wantStatus: "scoreable", wantMin: 55,
		},
		{
			name: "reference audio fed back as user", ref: ref,
			user: ref, mode: shadowing.EvaluationModeShadow,
			wantStatus: "unscorable", wantCode: "bleed_through",
		},
		{
			name: "silent recording", ref: ref,
			user: silentWAV(16000, 3), mode: shadowing.EvaluationModeShadow,
			wantStatus: "unscorable", wantCode: "silence",
		},
		{
			name: "truncated recording", ref: ref,
			user: func() []byte {
				p, _ := audio.ParseWAV(ref)
				w, _ := audio.EncodeWAV(p.Samples[:4000], p.SampleRate)
				return w
			}(),
			mode:       shadowing.EvaluationModeShadow,
			wantStatus: "unscorable", wantCode: "too_short",
		},
		{
			name: "invalid wav", ref: []byte("not wav"),
			user: ref, mode: shadowing.EvaluationModeShadow,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shadowing.EvaluateWAV(tt.ref, tt.user, tt.mode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EvaluateWAV() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q (code=%q)", got.Status, tt.wantStatus, got.UnscorableCode)
			}
			if tt.wantCode != "" && got.UnscorableCode != tt.wantCode {
				t.Errorf("UnscorableCode = %q, want %q", got.UnscorableCode, tt.wantCode)
			}
			if tt.wantStatus == "unscorable" && got.OverallScore != nil {
				t.Errorf("unscorable result must not carry a score, got %d", *got.OverallScore)
			}
			if tt.wantStatus == "scoreable" {
				if got.OverallScore == nil {
					t.Fatal("scoreable result missing OverallScore")
				}
				if *got.OverallScore < tt.wantMin {
					t.Errorf("OverallScore = %d, want >= %d (scores=%+v)", *got.OverallScore, tt.wantMin, got.Scores)
				}
			}
		})
	}
}

type lessonReaderFunc func(id int64) (*lesson.Lesson, error)

func (f lessonReaderFunc) GetDetail(id int64) (*lesson.Lesson, error) { return f(id) }

func testLesson(audioURL string) *lesson.Lesson {
	return &lesson.Lesson{
		LessonSummary: lesson.LessonSummary{
			ID:               1,
			AudioURL:         audioURL,
			ShadowingEnabled: true,
			ShadowingVersion: 1,
		},
		Sentences: []lesson.Sentence{{Index: 0, StartMS: 0, EndMS: 3000}},
	}
}

func TestReferenceAudioReader(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	full := speechWAV(16000, 4)
	if err := os.WriteFile(filepath.Join(dir, "lessons", "unit.wav"), full, 0o644); err != nil {
		t.Fatal(err)
	}
	reader := shadowing.NewReferenceAudioReader(dir, nil)

	tests := []struct {
		name        string
		url         string
		sentence    lesson.Sentence
		wantSamples int
		wantErr     bool
	}{
		{
			name: "legacy local path", url: "/audio/lessons/unit.wav",
			sentence: lesson.Sentence{Index: 0, StartMS: 1000, EndMS: 2000}, wantSamples: 16000,
		},
		{
			name: "clamped end", url: "/audio/lessons/unit.wav",
			sentence: lesson.Sentence{Index: 0, StartMS: 3500, EndMS: 9000}, wantSamples: 8000,
		},
		{
			name: "object stream url without loader", url: "/api/v1/audio/7/stream",
			sentence: lesson.Sentence{Index: 0, StartMS: 0, EndMS: 1000}, wantErr: true,
		},
		{
			name: "unsupported url", url: "https://cdn.example.com/a.wav",
			sentence: lesson.Sentence{Index: 0, StartMS: 0, EndMS: 1000}, wantErr: true,
		},
		{
			name: "start beyond audio", url: "/audio/lessons/unit.wav",
			sentence: lesson.Sentence{Index: 0, StartMS: 99999, EndMS: 100000}, wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reader.ReadSentence(context.Background(), testLesson(tt.url), tt.sentence)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadSentence() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			parsed, err := audio.ParseWAV(got)
			if err != nil {
				t.Fatalf("ParseWAV(ReadSentence()): %v", err)
			}
			if len(parsed.Samples) != tt.wantSamples {
				t.Errorf("len(Samples) = %d, want %d", len(parsed.Samples), tt.wantSamples)
			}
		})
	}
}

func TestReferenceAudioReaderObjectLoader(t *testing.T) {
	full := speechWAV(16000, 4)
	loader := func(_ context.Context, objectID int64) ([]byte, error) {
		if objectID != 7 {
			t.Fatalf("unexpected object id %d", objectID)
		}
		return full, nil
	}
	reader := shadowing.NewReferenceAudioReader(t.TempDir(), loader)
	got, err := reader.ReadSentence(context.Background(), testLesson("/api/v1/audio/7/stream"),
		lesson.Sentence{Index: 0, StartMS: 0, EndMS: 2000})
	if err != nil {
		t.Fatalf("ReadSentence() with loader: %v", err)
	}
	parsed, err := audio.ParseWAV(got)
	if err != nil {
		t.Fatalf("ParseWAV(): %v", err)
	}
	if len(parsed.Samples) != 32000 {
		t.Errorf("len(Samples) = %d, want 32000", len(parsed.Samples))
	}
}

func TestEvaluateHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	full := speechWAV(16000, 4)
	if err := os.WriteFile(filepath.Join(dir, "lessons", "unit.wav"), full, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := shadowing.NewEvaluationService(
		lessonReaderFunc(func(id int64) (*lesson.Lesson, error) {
			if id != 1 {
				return nil, os.ErrNotExist
			}
			return testLesson("/audio/lessons/unit.wav"), nil
		}),
		shadowing.NewReferenceAudioReader(dir, nil),
	)
	mux := http.NewServeMux()
	shadowing.NewEvaluationHandler(svc).RegisterRoutes(mux)

	ref, err := svcReferenceSentence(dir, full)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		audio      []byte
		fields     map[string]string
		wantStatus int
		wantResult string
		wantCode   string
	}{
		{
			name:  "scoreable shadowing",
			audio: shiftedWAV(ref, 600),
			fields: map[string]string{
				"sentence_index": "0", "mode": "shadow", "shadowing_version": "1",
			},
			wantStatus: http.StatusOK, wantResult: "scoreable",
		},
		{
			name:  "silent recording",
			audio: silentWAV(16000, 3),
			fields: map[string]string{
				"sentence_index": "0", "mode": "shadow", "shadowing_version": "1",
			},
			wantStatus: http.StatusOK, wantResult: "unscorable", wantCode: "silence",
		},
		{
			name:  "stale version",
			audio: shiftedWAV(ref, 600),
			fields: map[string]string{
				"sentence_index": "0", "mode": "shadow", "shadowing_version": "99",
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:  "invalid mode",
			audio: shiftedWAV(ref, 600),
			fields: map[string]string{
				"sentence_index": "0", "mode": "karaoke", "shadowing_version": "1",
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:  "missing audio part",
			audio: nil,
			fields: map[string]string{
				"sentence_index": "0", "mode": "shadow", "shadowing_version": "1",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if tt.audio != nil {
				part, err := writer.CreateFormFile("audio", "attempt.wav")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := part.Write(tt.audio); err != nil {
					t.Fatal(err)
				}
			}
			for k, v := range tt.fields {
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

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantResult == "" {
				return
			}
			var resp struct {
				Data shadowing.EvaluationResult `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v, body = %s", err, rec.Body.String())
			}
			if resp.Data.Status != tt.wantResult {
				t.Errorf("Status = %q, want %q, body = %s", resp.Data.Status, tt.wantResult, rec.Body.String())
			}
			if tt.wantCode != "" && resp.Data.UnscorableCode != tt.wantCode {
				t.Errorf("UnscorableCode = %q, want %q", resp.Data.UnscorableCode, tt.wantCode)
			}
		})
	}
}

func svcReferenceSentence(dir string, full []byte) ([]byte, error) {
	reader := shadowing.NewReferenceAudioReader(dir, nil)
	return reader.ReadSentence(context.Background(), testLesson("/audio/lessons/unit.wav"),
		lesson.Sentence{Index: 0, StartMS: 0, EndMS: 3000})
}
