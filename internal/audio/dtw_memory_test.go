package audio_test

import (
	"runtime"
	"testing"

	"japanese-learning-app/internal/audio"
)

func TestDTWUsesLinearMemory(t *testing.T) {
	a, b := make([]float64, 1000), make([]float64, 1500)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	result, err := audio.DTW(a, b, 0.5)
	runtime.ReadMemStats(&after)
	if err != nil || result.Distance != 0 {
		t.Fatalf("alignment=%+v err=%v", result, err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Fatalf("DTW allocated %d bytes; want <1 MiB for 1000 x 1500 frames", allocated)
	}
}
