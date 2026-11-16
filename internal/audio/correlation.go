package audio

import (
	"fmt"
	"math"
)

// LagResult describes the best alignment offset between two envelopes.
type LagResult struct {
	LagMS int64   `json:"lag_ms"`
	Peak  float64 `json:"peak"`
}

// lagTieEpsilon treats near-identical correlations as a tie. Nearly flat
// (e.g. TTS) envelopes correlate at ~1 for every lag, so without a tie-break
// the first searched lag wins even when lag 0 has much more evidence.
const lagTieEpsilon = 1e-6

// BestLag finds the lag (in milliseconds) that maximizes the Pearson
// correlation between the reference envelope and the user envelope.
// A positive lag means the user started later than the reference. When
// correlations are effectively tied, the alignment with the longest overlap
// wins, then the one with the smallest absolute offset.
func BestLag(ref, user []float64, hopMS, maxLagMS int) (LagResult, error) {
	if len(ref) == 0 || len(user) == 0 {
		return LagResult{}, fmt.Errorf("audio.BestLag: empty sequence ref=%d user=%d", len(ref), len(user))
	}
	if hopMS <= 0 || maxLagMS <= 0 {
		return LagResult{}, fmt.Errorf("audio.BestLag: invalid config hop_ms=%d max_lag_ms=%d", hopMS, maxLagMS)
	}
	maxLag := maxLagMS / hopMS
	if maxLag < 1 {
		maxLag = 1
	}

	best := -2.0
	bestLag := 0
	bestOverlap := 0
	for lag := -maxLag; lag <= maxLag; lag++ {
		start := max(0, -lag)
		end := min(len(ref), len(user)-lag)
		overlap := end - start
		if overlap < 4 {
			continue
		}
		corr := pearson(ref[start:end], user[start+lag:end+lag])
		if math.IsNaN(corr) {
			continue
		}
		higher := corr > best+lagTieEpsilon
		tied := math.Abs(corr-best) <= lagTieEpsilon &&
			(overlap > bestOverlap || (overlap == bestOverlap && absInt(lag) < absInt(bestLag)))
		if higher || tied {
			best = corr
			bestLag = lag
			bestOverlap = overlap
		}
	}
	if best < -1 {
		return LagResult{}, fmt.Errorf("audio.BestLag: no overlapping window of sufficient length")
	}
	return LagResult{LagMS: int64(bestLag * hopMS), Peak: best}, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func pearson(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	meanA, meanB := 0.0, 0.0
	for i := range a {
		meanA += a[i]
		meanB += b[i]
	}
	meanA /= float64(len(a))
	meanB /= float64(len(b))

	varA, varB, cov := 0.0, 0.0, 0.0
	for i := range a {
		da := a[i] - meanA
		db := b[i] - meanB
		varA += da * da
		varB += db * db
		cov += da * db
	}
	if varA <= 1e-12 || varB <= 1e-12 {
		return 0
	}
	return cov / math.Sqrt(varA*varB)
}
