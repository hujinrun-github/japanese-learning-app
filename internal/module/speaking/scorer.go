package speaking

import (
	"encoding/binary"
	"math"
	"time"
)

// AudioScorer defines the interface for scoring user speech against reference audio.
type AudioScorer interface {
	Score(referenceAudio, userAudio []byte) (ScoreResult, error)
}

// WaveformScorer implements AudioScorer using RMS energy and zero-crossing rate
// cosine similarity to produce a basic pronunciation quality score.
type WaveformScorer struct{}

// NewWaveformScorer creates a WaveformScorer.
func NewWaveformScorer() *WaveformScorer {
	return &WaveformScorer{}
}

// Score compares two raw PCM audio byte slices (16-bit little-endian samples)
// and returns a ScoreResult with OverallScore in [0, 100].
//
// Algorithm:
//  1. Parse both byte slices into []float64 samples.
//  2. Compute feature vectors: [RMS energy, zero-crossing rate].
//  3. Compute per-feature normalized similarity and combine.
func (w *WaveformScorer) Score(referenceAudio, userAudio []byte) (ScoreResult, error) {
	start := time.Now()

	refSamples := parsePCM(referenceAudio)
	userSamples := parsePCM(userAudio)

	if len(refSamples) == 0 || len(userSamples) == 0 {
		return ScoreResult{OverallScore: 0, FeedbackMS: time.Since(start).Milliseconds()}, nil
	}

	refFeatures := extractFeatures(refSamples)
	userFeatures := extractFeatures(userSamples)

	// Compute similarity per feature using normalized difference:
	// sim_i = 1 - |ref_i - user_i| / max(ref_i, user_i, epsilon)
	const epsilon = 1e-9
	var totalSim float64
	for i := range refFeatures {
		r, u := refFeatures[i], userFeatures[i]
		denom := math.Max(math.Max(r, u), epsilon)
		sim := 1.0 - math.Abs(r-u)/denom
		if sim < 0 {
			sim = 0
		}
		totalSim += sim
	}
	avgSim := totalSim / float64(len(refFeatures))

	score := int(math.Round(avgSim * 100))
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return ScoreResult{
		OverallScore: score,
		FeedbackMS:   time.Since(start).Milliseconds(),
	}, nil
}

// parsePCM converts raw 16-bit little-endian PCM bytes to float64 samples normalized to [-1, 1].
func parsePCM(data []byte) []float64 {
	n := len(data) / 2
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		raw := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
		samples[i] = float64(raw) / 32768.0
	}
	return samples
}

// extractFeatures computes a 2-element feature vector: [RMS energy, zero-crossing rate].
func extractFeatures(samples []float64) [2]float64 {
	if len(samples) == 0 {
		return [2]float64{0, 0}
	}

	// RMS energy
	var sumSq float64
	for _, s := range samples {
		sumSq += s * s
	}
	rms := math.Sqrt(sumSq / float64(len(samples)))

	// Zero-crossing rate
	crossings := 0
	for i := 1; i < len(samples); i++ {
		if (samples[i] >= 0) != (samples[i-1] >= 0) {
			crossings++
		}
	}
	zcr := float64(crossings) / float64(len(samples))

	return [2]float64{rms, zcr}
}

// cosineSimilarity returns the cosine similarity of two 2-element vectors.
// Retained for potential future use.
func cosineSimilarity(a, b [2]float64) float64 { //nolint:unused
	dot := a[0]*b[0] + a[1]*b[1]
	magA := math.Sqrt(a[0]*a[0] + a[1]*a[1])
	magB := math.Sqrt(b[0]*b[0] + b[1]*b[1])

	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (magA * magB)
}
