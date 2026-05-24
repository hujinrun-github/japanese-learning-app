package speaking_test

import (
	"testing"

	"japanese-learning-app/internal/module/speaking"
)

func TestComputeCER_Identical(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		rec      string
		expected float64
	}{
		{name: "empty both", ref: "", rec: "", expected: 0},
		{name: "identical kana", ref: "おはよう", rec: "おはよう", expected: 0},
		{name: "identical mixed", ref: "今日は良い天気です", rec: "今日は良い天気です", expected: 0},
		{name: "identical english", ref: "hello", rec: "hello", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := speaking.ComputeCER(tt.ref, tt.rec)
			if got != tt.expected {
				t.Errorf("ComputeCER(%q, %q) = %f, want %f", tt.ref, tt.rec, got, tt.expected)
			}
		})
	}
}

func TestComputeCER_Different(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		rec      string
		minCER   float64
		maxCER   float64
	}{
		{name: "one substitution", ref: "おはよう", rec: "おはよあ", minCER: 0.1, maxCER: 0.3},
		{name: "completely different", ref: "あああ", rec: "いいい", minCER: 0.9, maxCER: 1.0},
		{name: "insertion", ref: "こんにちは", rec: "こんにちはね", minCER: 0.1, maxCER: 0.3},
		{name: "deletion", ref: "おはようございます", rec: "おはよう", minCER: 0.4, maxCER: 0.6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := speaking.ComputeCER(tt.ref, tt.rec)
			if got < tt.minCER || got > tt.maxCER {
				t.Errorf("ComputeCER(%q, %q) = %f, want in [%f, %f]", tt.ref, tt.rec, got, tt.minCER, tt.maxCER)
			}
		})
	}
}

func TestComputeCER_EmptyReference(t *testing.T) {
	// Empty reference vs non-empty recognized returns max CER (1.0)
	got := speaking.ComputeCER("", "hello")
	if got != 1.0 {
		t.Errorf("ComputeCER(\"\", \"hello\") = %f, want 1.0", got)
	}
}

func TestComputeCER_EmptyRecognized(t *testing.T) {
	// Non-empty reference vs empty recognized: CER = 1.0 (all chars deleted)
	got := speaking.ComputeCER("hello", "")
	if got != 1.0 {
		t.Errorf("ComputeCER(\"hello\", \"\") = %f, want 1.0", got)
	}
}

func TestScoreFromCER(t *testing.T) {
	tests := []struct {
		name      string
		cer       float64
		wantScore int
	}{
		{name: "perfect", cer: 0.0, wantScore: 100},
		{name: "half wrong", cer: 0.5, wantScore: 50},
		{name: "all wrong", cer: 1.0, wantScore: 0},
		{name: "mostly correct", cer: 0.1, wantScore: 90},
		{name: "beyond max", cer: 1.5, wantScore: 0}, // clamped at 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := speaking.ScoreFromCER(tt.cer)
			if got != tt.wantScore {
				t.Errorf("ScoreFromCER(%f) = %d, want %d", tt.cer, got, tt.wantScore)
			}
		})
	}
}
