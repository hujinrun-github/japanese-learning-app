package audio_test

import (
	"math"
	"testing"

	"japanese-learning-app/internal/audio"
)

func TestDTW(t *testing.T) {
	rising := []float64{0, 0.2, 0.4, 0.6, 0.8, 1.0}
	falling := []float64{1.0, 0.8, 0.6, 0.4, 0.2, 0}
	stretched := []float64{0, 0, 0.2, 0.2, 0.4, 0.4, 0.6, 0.6, 0.8, 0.8, 1.0, 1.0}

	tests := []struct {
		name        string
		a           []float64
		b           []float64
		bandRatio   float64
		wantDistMax float64
		wantErr     bool
	}{
		{name: "identical", a: rising, b: rising, bandRatio: 0.5, wantDistMax: 1e-9},
		{name: "time stretched", a: rising, b: stretched, bandRatio: 0.5, wantDistMax: 0.2},
		{name: "reversed", a: rising, b: falling, bandRatio: 0.5, wantDistMax: 10},
		{name: "empty a", a: nil, b: rising, bandRatio: 0.5, wantErr: true},
		{name: "band too tight", a: rising, b: stretched, bandRatio: 0.01, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.DTW(tt.a, tt.b, tt.bandRatio)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DTW() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.PathLen < len(tt.a) || got.PathLen < len(tt.b) {
				t.Errorf("PathLen = %d, want >= max(%d, %d)", got.PathLen, len(tt.a), len(tt.b))
			}
			if got.NormalizedDistance > tt.wantDistMax {
				t.Errorf("NormalizedDistance = %v, want <= %v", got.NormalizedDistance, tt.wantDistMax)
			}
			if math.IsNaN(got.Distance) || math.IsInf(got.Distance, 0) {
				t.Errorf("non-finite Distance = %v", got.Distance)
			}
		})
	}
}

func TestDTWMonotonic(t *testing.T) {
	a := []float64{0, 0.2, 0.4, 0.6, 0.8, 1.0}
	near, err := audio.DTW(a, []float64{0, 0.2, 0.4, 0.6, 0.8, 0.9}, 0.5)
	if err != nil {
		t.Fatalf("near DTW: %v", err)
	}
	far, err := audio.DTW(a, []float64{1, 0.8, 0.6, 0.4, 0.2, 0}, 0.5)
	if err != nil {
		t.Fatalf("far DTW: %v", err)
	}
	if near.NormalizedDistance >= far.NormalizedDistance {
		t.Errorf("near distance %v should be < far distance %v", near.NormalizedDistance, far.NormalizedDistance)
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		name string
		dist float64
		k    float64
		want float64
	}{
		{name: "zero distance", dist: 0, k: 1, want: 100},
		{name: "one k", dist: 1, k: 1, want: 100 * math.Exp(-1)},
		{name: "large distance", dist: 10, k: 1, want: 100 * math.Exp(-10)},
		{name: "zero k falls back", dist: 1, k: 0, want: 100 * math.Exp(-1)},
		{name: "negative distance clamped", dist: -1, k: 1, want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := audio.Similarity(tt.dist, tt.k)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("Similarity(%v, %v) = %v, want %v", tt.dist, tt.k, got, tt.want)
			}
		})
	}
}
