package audio_test

import (
	"math"
	"testing"

	"japanese-learning-app/internal/audio"
)

func TestRMSFrames(t *testing.T) {
	tests := []struct {
		name     string
		samples  []float64
		rate     int
		frameMS  int
		hopMS    int
		wantLen  int
		wantZero bool
		wantErr  bool
	}{
		{
			name:    "silence",
			samples: make([]float64, 1600),
			rate:    16000, frameMS: 20, hopMS: 10,
			wantLen: 9, wantZero: true,
		},
		{
			name:    "constant tone",
			samples: constantSignal(16000, 0.5),
			rate:    16000, frameMS: 20, hopMS: 10,
			wantLen: 99,
		},
		{name: "empty", samples: nil, rate: 16000, frameMS: 20, hopMS: 10, wantErr: true},
		{name: "zero rate", samples: []float64{1}, rate: 0, frameMS: 20, hopMS: 10, wantErr: true},
		{name: "zero frame", samples: []float64{1}, rate: 16000, frameMS: 0, hopMS: 10, wantErr: true},
		{name: "hop larger than frame", samples: []float64{1}, rate: 16000, frameMS: 10, hopMS: 20, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.RMSFrames(tt.samples, tt.rate, tt.frameMS, tt.hopMS)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RMSFrames() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len(frames) = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantZero {
				for i, v := range got {
					if v != 0 {
						t.Fatalf("frames[%d] = %v, want 0", i, v)
					}
				}
				return
			}
			if math.Abs(got[0]-0.5) > 1e-6 {
				t.Errorf("frames[0] = %v, want 0.5", got[0])
			}
		})
	}
}

func TestNormalizeFrames(t *testing.T) {
	tests := []struct {
		name      string
		frames    []float64
		wantAll0  bool
		wantUpper float64
	}{
		{name: "empty", frames: nil, wantAll0: true},
		{name: "all zero", frames: []float64{0, 0, 0}, wantAll0: true},
		{name: "mixed", frames: []float64{0, 0.1, 0.2, 0.4, 1.0}, wantUpper: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := audio.NormalizeFrames(tt.frames)
			if len(got) != len(tt.frames) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.frames))
			}
			for i, v := range got {
				if tt.wantAll0 && v != 0 {
					t.Errorf("got[%d] = %v, want 0", i, v)
				}
				if v > tt.wantUpper+1e-9 && tt.wantUpper > 0 {
					t.Errorf("got[%d] = %v exceeds %v", i, v, tt.wantUpper)
				}
			}
		})
	}
}

func TestDetectVAD(t *testing.T) {
	tests := []struct {
		name          string
		frames        []float64
		wantVoicedMin float64
		wantVoicedMax float64
		wantSpeechMS  int64
		wantPauses    int
		wantErr       bool
	}{
		{
			name:          "silence",
			frames:        make([]float64, 100),
			wantVoicedMin: 0, wantVoicedMax: 0, wantSpeechMS: 0,
		},
		{
			name:          "all voiced",
			frames:        filledFrames(100, 1.0),
			wantVoicedMin: 0.9, wantVoicedMax: 1.0, wantSpeechMS: 1000,
		},
		{
			name:          "two blocks with long gap",
			frames:        framesWithGap(),
			wantVoicedMin: 0.3, wantVoicedMax: 0.7, wantPauses: 1,
		},
		{name: "empty", frames: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.DetectVAD(tt.frames, audio.DefaultHopMS)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DetectVAD() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.VoicedRatio < tt.wantVoicedMin || got.VoicedRatio > tt.wantVoicedMax {
				t.Errorf("VoicedRatio = %v, want [%v, %v]", got.VoicedRatio, tt.wantVoicedMin, tt.wantVoicedMax)
			}
			if tt.wantSpeechMS > 0 && math.Abs(float64(got.SpeechMS-tt.wantSpeechMS)) > 100 {
				t.Errorf("SpeechMS = %d, want ~%d", got.SpeechMS, tt.wantSpeechMS)
			}
			if len(got.Pauses) != tt.wantPauses {
				t.Errorf("Pauses = %d, want %d (%+v)", len(got.Pauses), tt.wantPauses, got.Pauses)
			}
		})
	}
}

func constantSignal(n int, v float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = v
	}
	return s
}

func filledFrames(n int, v float64) []float64 {
	f := make([]float64, n)
	for i := range f {
		f[i] = v
	}
	return f
}

func framesWithGap() []float64 {
	f := make([]float64, 100)
	for i := 0; i < 30; i++ {
		f[i] = 1
	}
	for i := 70; i < 100; i++ {
		f[i] = 1
	}
	return f
}
