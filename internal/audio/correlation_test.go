package audio_test

import (
	"math"
	"os"
	"testing"

	"japanese-learning-app/internal/audio"
)

func testSignal(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.Sin(2*math.Pi*3*float64(i)/float64(n)) +
			0.4*math.Sin(2*math.Pi*7*float64(i)/float64(n))
	}
	return s
}

func shiftRight(s []float64, lag int) []float64 {
	out := make([]float64, len(s))
	for i := 0; i+lag < len(s); i++ {
		out[i+lag] = s[i]
	}
	return out
}

func shiftLeft(s []float64, lag int) []float64 {
	out := make([]float64, len(s))
	for i := 0; i+lag < len(s); i++ {
		out[i] = s[i+lag]
	}
	return out
}

func TestBestLag(t *testing.T) {
	ref := testSignal(200)
	tests := []struct {
		name        string
		ref         []float64
		user        []float64
		hopMS       int
		maxLagMS    int
		wantLagMS   int64
		wantPeakMin float64
		wantErr     bool
	}{
		{name: "identical", ref: ref, user: ref, hopMS: 10, maxLagMS: 2000, wantLagMS: 0, wantPeakMin: 0.99},
		{name: "delayed 50ms", ref: ref, user: shiftRight(ref, 5), hopMS: 10, maxLagMS: 2000, wantLagMS: 50, wantPeakMin: 0.8},
		{name: "early 30ms", ref: ref, user: shiftLeft(ref, 3), hopMS: 10, maxLagMS: 2000, wantLagMS: -30, wantPeakMin: 0.8},
		{name: "empty", ref: nil, user: ref, hopMS: 10, maxLagMS: 2000, wantErr: true},
		{name: "zero hop", ref: ref, user: ref, hopMS: 0, maxLagMS: 2000, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.BestLag(tt.ref, tt.user, tt.hopMS, tt.maxLagMS)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BestLag() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.LagMS != tt.wantLagMS {
				t.Errorf("LagMS = %d, want %d", got.LagMS, tt.wantLagMS)
			}
			if got.Peak < tt.wantPeakMin {
				t.Errorf("Peak = %v, want >= %v", got.Peak, tt.wantPeakMin)
			}
		})
	}
}

func TestBestLagConstantSignal(t *testing.T) {
	flat := make([]float64, 100)
	got, err := audio.BestLag(flat, flat, 10, 2000)
	if err != nil {
		t.Fatalf("BestLag() on flat signal: %v", err)
	}
	if math.IsNaN(got.Peak) || math.IsInf(got.Peak, 0) {
		t.Errorf("Peak = %v, want finite", got.Peak)
	}
}

// A TTS reference can yield a nearly flat normalized envelope whose energy
// oscillates periodically. Every lag then correlates at ~1, so the first
// candidate used to win even though lag 0 has far more overlapping evidence.
func TestBestLagNearConstantPeriodicEnvelope(t *testing.T) {
	const n = 300
	flat := make([]float64, n)
	for i := range flat {
		flat[i] = 0.995 + 0.005*math.Sin(2*math.Pi*float64(i)/25)
	}
	got, err := audio.BestLag(flat, flat, 10, 2000)
	if err != nil {
		t.Fatalf("BestLag(): %v", err)
	}
	if got.LagMS != 0 {
		t.Errorf("LagMS = %d, want 0 (peak=%f)", got.LagMS, got.Peak)
	}
	if got.Peak < 0.99 {
		t.Errorf("Peak = %f, want >= 0.99", got.Peak)
	}
}

// Regression against the real seeded 8 kHz lesson audio: comparing a sentence
// with itself must produce zero lag, otherwise bleed-through detection and
// shadow-mode sync scoring are wrong.
func TestBestLagSelfOnPilotAudio(t *testing.T) {
	data, err := os.ReadFile("../../data/audio/lessons/shadowing-pilot-n5-day.wav")
	if err != nil {
		t.Skipf("pilot lesson audio not available: %v", err)
	}
	sliced, err := audio.SliceWAV(data, 0, 3000)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := audio.ParseWAV(sliced)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := audio.RMSFrames(parsed.Samples, parsed.SampleRate, audio.DefaultFrameMS, audio.DefaultHopMS)
	if err != nil {
		t.Fatal(err)
	}
	env := audio.NormalizeFrames(frames)
	got, err := audio.BestLag(env, env, audio.DefaultHopMS, 2000)
	if err != nil {
		t.Fatalf("BestLag(): %v", err)
	}
	if got.LagMS != 0 || got.Peak < 0.99 {
		t.Errorf("self-comparison = %+v, want lag 0 with peak >= 0.99", got)
	}
}
