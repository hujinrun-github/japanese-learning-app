package shadowing_test

import (
	"math"
	"testing"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/module/shadowing"
)

// sparseSpeechWAV produces a TTS-like utterance with short tonal bursts
// separated by silence. The 24 kHz production reference audio only yields
// ~28-33% voiced F0 frames even though it contains ~100 voiced frames, which
// used to be mistaken for "not enough evidence" by the old ratio-based
// confidence formula.
func sparseSpeechWAV(rate, seconds int, duty float64) []byte {
	n := rate * seconds
	s := make([]float64, n)
	const periodMS = 500
	onMS := int(float64(periodMS) * duty)
	freqs := []float64{120, 150, 130}
	for i := range s {
		tMS := int(int64(i) * 1000 / int64(rate))
		burst := tMS / periodMS
		if tMS%periodMS >= onMS {
			continue
		}
		t := float64(i) / float64(rate)
		f := freqs[burst%len(freqs)]
		s[i] = 0.5 * (math.Sin(2*math.Pi*f*t) + 0.3*math.Sin(4*math.Pi*f*t))
	}
	out, err := audio.EncodeWAV(s, rate)
	if err != nil {
		panic(err)
	}
	return out
}

func TestS7CalibrationConfig(t *testing.T) {
	tests := []struct {
		name        string
		mode        shadowing.EvaluationMode
		wantVersion string
		wantSync    float64
		wantProsody float64
	}{
		{
			name: "shadow rubric", mode: shadowing.EvaluationModeShadow,
			wantVersion: "w-shadow-1.1", wantSync: 0.30, wantProsody: 0.30,
		},
		{
			name: "repeat rubric", mode: shadowing.EvaluationModeRepeat,
			wantVersion: "w-repeat-1.1", wantSync: 0.10, wantProsody: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := shadowing.ScoringConfigForMode(tt.mode)
			if cfg.WeightsVersion != tt.wantVersion {
				t.Errorf("WeightsVersion = %q, want %q", cfg.WeightsVersion, tt.wantVersion)
			}
			if cfg.Weights["sync"] != tt.wantSync || cfg.Weights["prosody"] != tt.wantProsody {
				t.Errorf("nominal weights sync=%v prosody=%v, want %v/%v",
					cfg.Weights["sync"], cfg.Weights["prosody"], tt.wantSync, tt.wantProsody)
			}
			sum := 0.0
			for _, w := range cfg.Weights {
				sum += w
			}
			if math.Abs(sum-1.0) > 1e-9 {
				t.Errorf("nominal weights sum = %v, want 1.0", sum)
			}
			if cfg.MinDimensionConfidence <= 0 || cfg.MinDimensionConfidence > 1 {
				t.Errorf("MinDimensionConfidence = %v", cfg.MinDimensionConfidence)
			}
			if cfg.MinEffectiveWeight <= 0 || cfg.MinEffectiveWeight > 1 {
				t.Errorf("MinEffectiveWeight = %v", cfg.MinEffectiveWeight)
			}
			if cfg.MinActiveDimensions < 2 {
				t.Errorf("MinActiveDimensions = %d, want >= 2 (never score from one dimension)", cfg.MinActiveDimensions)
			}
			if cfg.MinAchievableWeight <= 0 {
				t.Errorf("MinAchievableWeight = %v", cfg.MinAchievableWeight)
			}
			if cfg.ProsodyFullConfidenceFrames <= 0 || cfg.FluencyFullConfidenceFrames <= 0 {
				t.Errorf("frame-count calibration is unset: prosody=%d fluency=%d",
					cfg.ProsodyFullConfidenceFrames, cfg.FluencyFullConfidenceFrames)
			}
			if cfg.TargetLagMS <= 0 || cfg.LagSigmaMS <= 0 || cfg.MaxLagSearchMS <= 0 {
				t.Errorf("signal thresholds unset: %+v", cfg)
			}

			// The rubric is versioned and must not be mutable through a caller.
			cfg.Weights["sync"] = 999
			if got := shadowing.ScoringConfigForMode(tt.mode).Weights["sync"]; got != tt.wantSync {
				t.Errorf("config leaked a mutable map: sync = %v, want %v", got, tt.wantSync)
			}
		})
	}
	if shadowing.ScorerVersionLocal != "local-prosody-1.1" {
		t.Errorf("ScorerVersionLocal = %q, want local-prosody-1.1 after confidence recalibration",
			shadowing.ScorerVersionLocal)
	}
}

func TestS7AggregateModeAwareWeights(t *testing.T) {
	local := map[string]shadowing.DimensionScore{
		"sync":      dimension(80, 0.95, true),
		"prosody":   dimension(85, 0.95, true),
		"fluency":   dimension(75, 0.95, true),
		"coverage":  dimension(0, 0, false),
		"segmental": dimension(0, 0, false),
	}

	tests := []struct {
		name string
		mode shadowing.EvaluationMode
		want map[string]float64
	}{
		{
			name: "shadow local rubric", mode: shadowing.EvaluationModeShadow,
			want: map[string]float64{"sync": 0.40, "prosody": 0.40, "fluency": 0.20, "coverage": 0, "segmental": 0},
		},
		{
			name: "repeat local rubric keeps prosody dominant", mode: shadowing.EvaluationModeRepeat,
			want: map[string]float64{"sync": 0.20, "prosody": 0.50, "fluency": 0.30, "coverage": 0, "segmental": 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shadowing.Aggregate(local, tt.mode)
			if !got.Valid {
				t.Fatalf("Aggregate() invalid (code=%q)", got.Code)
			}
			if !got.Partial {
				t.Error("Partial = false, want true (ASR dimensions unavailable)")
			}
			sum := 0.0
			for dim, want := range tt.want {
				weight := got.Weights[dim]
				if math.Abs(weight-want) > 1e-9 {
					t.Errorf("effective weight[%s] = %v, want %v", dim, weight, want)
				}
				sum += weight
			}
			if math.Abs(sum-1.0) > 1e-9 {
				t.Errorf("effective weights sum = %v, want 1.0", sum)
			}
		})
	}
}

func TestS7AggregateRejectsInsufficientEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mode   shadowing.EvaluationMode
		scores map[string]shadowing.DimensionScore
	}{
		{
			name: "repeat with only one trusted dimension", mode: shadowing.EvaluationModeRepeat,
			scores: map[string]shadowing.DimensionScore{
				"sync": dimension(90, 0.95, true), "prosody": dimension(0, 0, false),
				"fluency": dimension(0, 0, false), "coverage": dimension(0, 0, false),
				"segmental": dimension(0, 0, false),
			},
		},
		{
			name: "shadow with only one trusted dimension", mode: shadowing.EvaluationModeShadow,
			scores: map[string]shadowing.DimensionScore{
				"sync": dimension(90, 0.95, true), "prosody": dimension(0, 0, false),
				"fluency": dimension(0, 0, false), "coverage": dimension(0, 0, false),
				"segmental": dimension(0, 0, false),
			},
		},
		{
			name: "repeat with three local dims but only sync is trustworthy", mode: shadowing.EvaluationModeRepeat,
			scores: map[string]shadowing.DimensionScore{
				"sync": dimension(90, 0.95, true), "prosody": dimension(85, 0.20, true),
				"fluency": dimension(75, 0.20, true), "coverage": dimension(0, 0, false),
				"segmental": dimension(0, 0, false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shadowing.Aggregate(tt.scores, tt.mode)
			if got.Valid {
				t.Errorf("Aggregate() valid with overall=%d, want low_confidence", got.Overall)
			}
			if got.Code != "low_confidence" {
				t.Errorf("Code = %q, want low_confidence", got.Code)
			}
		})
	}
}

func TestS7SparseVoicingIsScoreable(t *testing.T) {
	ref := sparseSpeechWAV(16000, 3, 0.20)
	tests := []struct {
		name string
		ref  []byte
		user []byte
		mode shadowing.EvaluationMode
	}{
		{name: "repeat identical sparse TTS", ref: ref, user: ref, mode: shadowing.EvaluationModeRepeat},
		{name: "repeat 8kHz recording of sparse TTS", ref: ref, user: resampleWAV(t, ref, 8000), mode: shadowing.EvaluationModeRepeat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shadowing.EvaluateWAV(tt.ref, tt.user, tt.mode)
			if err != nil {
				t.Fatalf("EvaluateWAV() err = %v", err)
			}
			if got.Status != "scoreable" {
				t.Fatalf("Status = %q (code=%q), want scoreable; scores=%+v",
					got.Status, got.UnscorableCode, got.Scores)
			}
			if got.OverallScore == nil {
				t.Fatal("scoreable result missing OverallScore")
			}
			if !got.Partial {
				t.Error("Partial = false, want true before ASR is wired")
			}
			for _, dim := range []string{"sync", "prosody", "fluency"} {
				d := got.Scores[dim]
				if !d.Available || d.Confidence < shadowing.ScoringConfigForMode(tt.mode).MinDimensionConfidence {
					t.Errorf("%s not trusted: %+v", dim, d)
				}
			}
			if got.Scores["coverage"].Available || got.Scores["segmental"].Available {
				t.Error("coverage/segmental must stay unavailable without ASR")
			}
			sum := 0.0
			for _, d := range got.Scores {
				sum += d.Weight
			}
			if math.Abs(sum-1.0) > 1e-9 {
				t.Errorf("returned dimension weights sum = %v, want 1.0", sum)
			}
			if got.WeightsVersion != shadowing.WeightsVersionRepeat {
				t.Errorf("WeightsVersion = %q, want %q", got.WeightsVersion, shadowing.WeightsVersionRepeat)
			}
			t.Logf("sparse voicing: overall=%d scores=%+v", *got.OverallScore, got.Scores)
		})
	}
}

func TestS7PilotAudioRegression(t *testing.T) {
	mux, sentenceWAV, recording16k := pilotEvaluationMux(t)
	tests := []struct {
		name       string
		audio      []byte
		mode       string
		wantStatus string
		wantCode   string
	}{
		{
			name: "repeat identical 8kHz", audio: sentenceWAV, mode: "repeat",
			wantStatus: "scoreable",
		},
		{
			name: "repeat 16kHz browser recording", audio: recording16k, mode: "repeat",
			wantStatus: "scoreable",
		},
		{
			name: "shadow with reference fed back", audio: sentenceWAV, mode: "shadow",
			wantStatus: "unscorable", wantCode: "bleed_through",
		},
		{
			name: "silence", audio: silentWAV(16000, 3), mode: "shadow",
			wantStatus: "unscorable", wantCode: "silence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, got := postEvaluation(t, mux, tt.audio, tt.mode)
			if status != 200 {
				t.Fatalf("status = %d, want 200", status)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q (code=%q), want %q", got.Status, got.UnscorableCode, tt.wantStatus)
			}
			if tt.wantCode != "" && got.UnscorableCode != tt.wantCode {
				t.Errorf("UnscorableCode = %q, want %q", got.UnscorableCode, tt.wantCode)
			}
			if len(got.Issues) == 0 {
				t.Error("evaluation returned no improvement advice")
			}
			if tt.wantStatus == "scoreable" {
				if got.OverallScore == nil {
					t.Fatal("scoreable result missing OverallScore")
				}
				sum := 0.0
				for _, d := range got.Scores {
					sum += d.Weight
				}
				if math.Abs(sum-1.0) > 1e-9 {
					t.Errorf("returned dimension weights sum = %v, want 1.0", sum)
				}
				t.Logf("pilot %s: overall=%d scores=%+v", tt.mode, *got.OverallScore, got.Scores)
			}
		})
	}
}
