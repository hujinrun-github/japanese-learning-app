package audio_test

import (
	"fmt"
	"math"
	"testing"

	"japanese-learning-app/internal/audio"
)

func sineSamples(rate, seconds int, freq float64) []float64 {
	n := rate * seconds
	s := make([]float64, n)
	for i := range s {
		s[i] = 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(rate))
	}
	return s
}

func TestRawF0(t *testing.T) {
	tests := []struct {
		name      string
		samples   []float64
		wantHz    float64
		wantVoice bool
		wantErr   bool
	}{
		{name: "120 Hz", samples: sineSamples(16000, 1, 120), wantHz: 120, wantVoice: true},
		{name: "200 Hz", samples: sineSamples(16000, 1, 200), wantHz: 200, wantVoice: true},
		{name: "440 Hz", samples: sineSamples(16000, 1, 440), wantHz: 440, wantVoice: true},
		{name: "silence", samples: make([]float64, 16000), wantVoice: false},
		{name: "empty", samples: nil, wantErr: true},
		{name: "zero rate", samples: []float64{1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hz, voiced, err := audio.RawF0(tt.samples, 16000, audio.PitchFrameMS, audio.PitchHopMS)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RawF0() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(hz) != len(voiced) || len(hz) == 0 {
				t.Fatalf("len(hz)=%d len(voiced)=%d", len(hz), len(voiced))
			}
			count := 0
			sum := 0.0
			for i, ok := range voiced {
				if ok {
					count++
					sum += hz[i]
				}
			}
			if !tt.wantVoice {
				if count != 0 {
					t.Errorf("voiced frames = %d, want 0", count)
				}
				return
			}
			if count < len(hz)/2 {
				t.Fatalf("voiced frames = %d/%d, want majority", count, len(hz))
			}
			mean := sum / float64(count)
			if math.Abs(mean-tt.wantHz) > 5 {
				t.Errorf("mean F0 = %.2f Hz, want %.2f Hz (±5)", mean, tt.wantHz)
			}
		})
	}
}

// Browser recorders commonly capture at 44.1/48 kHz. F0 estimation must stay
// accurate and keep the 10 ms frame grid after the internal rate decimation.
func TestRawF0HighSampleRate(t *testing.T) {
	for _, rate := range []int{32000, 44100, 48000} {
		t.Run(fmt.Sprintf("%dHz", rate), func(t *testing.T) {
			samples := sineSamples(rate, 1, 200)
			hz, voiced, err := audio.RawF0(samples, rate, audio.PitchFrameMS, audio.PitchHopMS)
			if err != nil {
				t.Fatalf("RawF0(): %v", err)
			}
			if len(hz) != len(voiced) || len(hz) < 90 || len(hz) > 101 {
				t.Fatalf("frames = %d, want ~100 (10ms hop over 1s)", len(hz))
			}
			count, sum := 0, 0.0
			for i, ok := range voiced {
				if ok {
					count++
					sum += hz[i]
				}
			}
			if count < len(hz)/2 {
				t.Fatalf("voiced frames = %d/%d, want majority", count, len(hz))
			}
			mean := sum / float64(count)
			if math.Abs(mean-200) > 5 {
				t.Errorf("mean F0 = %.2f Hz, want 200 Hz (±5)", mean)
			}
		})
	}
}

func TestPitchContour(t *testing.T) {
	base := []float64{100, 110, 120, 130, 140, 150, 160, 150, 140, 130}
	voiced := []bool{true, true, true, true, true, true, true, true, true, true}
	shifted := make([]float64, len(base))
	for i, v := range base {
		shifted[i] = v * 1.5
	}

	tests := []struct {
		name    string
		hz      []float64
		voiced  []bool
		wantErr bool
	}{
		{name: "shape invariant", hz: base, voiced: voiced},
		{name: "pitch shifted", hz: shifted, voiced: voiced},
		{name: "with unvoiced gap", hz: base, voiced: []bool{true, true, true, false, true, true, true, true, true, true}},
		{name: "all unvoiced", hz: base, voiced: make([]bool, len(base)), wantErr: true},
		{name: "length mismatch", hz: base, voiced: []bool{true}, wantErr: true},
	}

	contours := make(map[string][]float64)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.PitchContour(tt.hz, tt.voiced)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PitchContour() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.hz) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.hz))
			}
			mean := 0.0
			for _, v := range got {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("non-finite contour value: %v", v)
				}
				mean += v
			}
			mean /= float64(len(got))
			if math.Abs(mean) > 1e-9 {
				t.Errorf("contour mean = %v, want 0 (z-score)", mean)
			}
			contours[tt.name] = got
		})
	}

	baseContour := contours["shape invariant"]
	shiftedContour := contours["pitch shifted"]
	if baseContour == nil || shiftedContour == nil {
		t.Fatal("missing contours for invariance check")
	}
	for i := range baseContour {
		if math.Abs(baseContour[i]-shiftedContour[i]) > 1e-6 {
			t.Errorf("contour[%d] = %v vs shifted %v, shape should be invariant", i, baseContour[i], shiftedContour[i])
		}
	}
}
