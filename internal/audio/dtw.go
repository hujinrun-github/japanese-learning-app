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
	if bandRatio < 0 || math.IsNaN(bandRatio) {
		return DTWResult{}, fmt.Errorf("audio.DTW: invalid band ratio %v", bandRatio)
	}

	n, m := len(a), len(b)
	band := int(bandRatio * float64(max(n, m)))
	if band < 1 {
		band = 1
	}

	inf := math.Inf(1)
	cum := make([][]float64, n+1)
	steps := make([][]int, n+1)
	for i := range cum {
		cum[i] = make([]float64, m+1)
		steps[i] = make([]int, m+1)
		for j := range cum[i] {
			cum[i][j] = inf
		}
	}
	cum[0][0] = 0

	for i := 1; i <= n; i++ {
		jStart := max(1, i-band)
		jEnd := min(m, i+band)
		for j := jStart; j <= jEnd; j++ {
			best := cum[i-1][j-1]
			bestStep := steps[i-1][j-1]
			if cum[i-1][j] < best {
				best = cum[i-1][j]
				bestStep = steps[i-1][j]
			}
			if cum[i][j-1] < best {
				best = cum[i][j-1]
				bestStep = steps[i][j-1]
			}
			if math.IsInf(best, 1) {
				continue
			}
			cum[i][j] = best + math.Abs(a[i-1]-b[j-1])
			steps[i][j] = bestStep + 1
		}
	}

	dist := cum[n][m]
	if math.IsInf(dist, 1) {
		return DTWResult{}, fmt.Errorf("audio.DTW: no alignment path within band %d (n=%d m=%d)", band, n, m)
	}
	pathLen := steps[n][m]
	if pathLen < 1 {
		pathLen = 1
	}
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
