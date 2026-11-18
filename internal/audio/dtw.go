package audio

import (
	"fmt"
	"math"
)

// DTWResult holds the accumulated cost and the alignment path length.
type DTWResult struct {
	Distance           float64 `json:"distance"`
	PathLen            int     `json:"path_len"`
	NormalizedDistance float64 `json:"normalized_distance"`
}

// DTW aligns two feature sequences with a Sakoe-Chiba band and returns the
// accumulated absolute-difference cost normalized by the path length.
func DTW(a, b []float64, bandRatio float64) (DTWResult, error) {
	if len(a) == 0 || len(b) == 0 {
		return DTWResult{}, fmt.Errorf("audio.DTW: empty sequence a=%d b=%d", len(a), len(b))
	}
	if bandRatio < 0 || math.IsNaN(bandRatio) || math.IsInf(bandRatio, 0) {
		return DTWResult{}, fmt.Errorf("audio.DTW: invalid band ratio %v", bandRatio)
	}

	n, m := len(a), len(b)
	band := int(bandRatio * float64(max(n, m)))
	if band < 1 {
		band = 1
	}

	// Only adjacent rows are needed for distance and path length. Clear cells
	// that leave the band so values from two rows ago cannot become a path.
	inf := math.Inf(1)
	previous, current := make([]float64, m+1), make([]float64, m+1)
	previousSteps, currentSteps := make([]int, m+1), make([]int, m+1)
	for j := range previous {
		previous[j] = inf
		current[j] = inf
	}
	previous[0] = 0
	previousStart, previousEnd := 0, 0
	currentStart, currentEnd := 1, 0
	for i := 1; i <= n; i++ {
		for j := currentStart; j <= currentEnd; j++ {
			current[j] = inf
			currentSteps[j] = 0
		}
		jStart, jEnd := max(1, i-band), min(m, i+band)
		for j := jStart; j <= jEnd; j++ {
			best, bestStep := previous[j-1], previousSteps[j-1]
			if previous[j] < best {
				best, bestStep = previous[j], previousSteps[j]
			}
			if current[j-1] < best {
				best, bestStep = current[j-1], currentSteps[j-1]
			}
			if !math.IsInf(best, 1) {
				current[j] = best + math.Abs(a[i-1]-b[j-1])
				currentSteps[j] = bestStep + 1
			}
		}
		previous, current = current, previous
		previousSteps, currentSteps = currentSteps, previousSteps
		currentStart, currentEnd = previousStart, previousEnd
		previousStart, previousEnd = jStart, jEnd
	}
	dist := previous[m]
	if math.IsInf(dist, 1) {
		return DTWResult{}, fmt.Errorf("audio.DTW: no alignment path within band %d (n=%d m=%d)", band, n, m)
	}
	pathLen := max(1, previousSteps[m])
	return DTWResult{
		Distance:           dist,
		PathLen:            pathLen,
		NormalizedDistance: dist / float64(pathLen),
	}, nil
}

// Similarity maps a normalized DTW distance to a 0..100 score.
func Similarity(normalizedDistance, k float64) float64 {
	if normalizedDistance <= 0 {
		return 100
	}
	if k <= 0 || math.IsNaN(k) {
		k = 1
	}
	return 100 * math.Exp(-normalizedDistance/k)
}
