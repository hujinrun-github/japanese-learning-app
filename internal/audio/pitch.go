package audio

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
)

const (
	pitchThreshold   = 0.15
	pitchUnvoicedMax = 0.20
	pitchEnergyRatio = 0.05
	pitchInterpMax   = 3

	// Human F0 stays below 500 Hz, so 16 kHz is already far above Nyquist for
	// pitch analysis. CMND costs O(tau_max * frame) ~ O(rate^2); analyzing a
	// 48 kHz recording directly would be ~9x slower with no extra information.
	maxPitchAnalysisRate = 16000
)

// RawF0 estimates fundamental frequency per frame with a simplified YIN/CMND
// method. It returns Hz per frame and whether the frame is voiced.
func RawF0(samples []float64, sampleRate, frameMS, hopMS int) ([]float64, []bool, error) {
	slog.Debug("audio.RawF0 called", "samples", len(samples), "rate", sampleRate)
	if len(samples) == 0 {
		return nil, nil, fmt.Errorf("audio.RawF0: no samples")
	}
	if sampleRate <= 0 || frameMS <= 0 || hopMS <= 0 || hopMS > frameMS {
		return nil, nil, fmt.Errorf("audio.RawF0: invalid config rate=%d frame_ms=%d hop_ms=%d", sampleRate, frameMS, hopMS)
	}

	analysisRate := sampleRate
	if sampleRate > maxPitchAnalysisRate {
		factor := (sampleRate + maxPitchAnalysisRate - 1) / maxPitchAnalysisRate
		samples = decimateForPitch(samples, factor)
		analysisRate = sampleRate / factor
	}

	frame := analysisRate * frameMS / 1000
	hop := analysisRate * hopMS / 1000
	tauMin := analysisRate / 500
	tauMax := analysisRate / 70
	if frame < 2*tauMax || tauMin < 1 || hop < 1 {
		return nil, nil, fmt.Errorf("audio.RawF0: frame/hop too small for F0 range (frame=%d tau_max=%d)", frame, tauMax)
	}
	if len(samples) < frame {
		return nil, nil, fmt.Errorf("audio.RawF0: %d samples shorter than frame %d", len(samples), frame)
	}

	count := (len(samples)-frame)/hop + 1
	hz := make([]float64, count)
	voiced := make([]bool, count)
	energies := make([]float64, count)
	globalPeak := 0.0

	for i := 0; i < count; i++ {
		start := i * hop
		buf := samples[start : start+frame]
		energy := 0.0
		for _, v := range buf {
			energy += v * v
		}
		energies[i] = math.Sqrt(energy / float64(frame))
		if energies[i] > globalPeak {
			globalPeak = energies[i]
		}
	}
	energyFloor := globalPeak * pitchEnergyRatio

	for i := 0; i < count; i++ {
		if energies[i] <= energyFloor {
			continue
		}
		buf := samples[i*hop : i*hop+frame]
		tau, confidence := findPitchPeriod(buf, tauMin, tauMax)
		if tau <= 0 || confidence > pitchUnvoicedMax {
			continue
		}
		hz[i] = float64(analysisRate) / tau
		voiced[i] = true
	}

	slog.Debug("audio.RawF0 done", "frames", count, "analysis_rate", analysisRate,
		"tau_range", [2]int{tauMin, tauMax})
	return hz, voiced, nil
}

// decimateForPitch applies a boxcar anti-alias filter of width factor and
// keeps every factor-th sample. Low-band periodicity (what YIN needs) is
// preserved while the O(rate^2) difference function becomes rate-independent.
func decimateForPitch(samples []float64, factor int) []float64 {
	if factor <= 1 {
		return samples
	}
	out := make([]float64, 0, (len(samples)+factor-1)/factor)
	for i := 0; i < len(samples); i += factor {
		end := min(i+factor, len(samples))
		sum := 0.0
		for j := i; j < end; j++ {
			sum += samples[j]
		}
		out = append(out, sum/float64(end-i))
	}
	return out
}

// findPitchPeriod returns the refined period in samples and the CMND value at
// that period. A high confidence value means no clear periodicity.
func findPitchPeriod(buf []float64, tauMin, tauMax int) (float64, float64) {
	diff := make([]float64, tauMax+1)
	for tau := 1; tau <= tauMax; tau++ {
		sum := 0.0
		for j := 0; j+tau < len(buf); j++ {
			d := buf[j] - buf[j+tau]
			sum += d * d
		}
		diff[tau] = sum
	}

	cmnd := make([]float64, tauMax+1)
	running := 0.0
	for tau := 1; tau <= tauMax; tau++ {
		running += diff[tau]
		if running > 0 {
			cmnd[tau] = diff[tau] * float64(tau) / running
		} else {
			cmnd[tau] = 1
		}
	}

	tau := -1
	for t := tauMin; t < tauMax; t++ {
		if cmnd[t] < pitchThreshold && cmnd[t] <= cmnd[t-1] && cmnd[t] < cmnd[t+1] {
			tau = t
			break
		}
	}
	if tau < 0 {
		best := math.Inf(1)
		for t := tauMin; t <= tauMax; t++ {
			if cmnd[t] < best {
				best = cmnd[t]
				tau = t
			}
		}
	}
	if tau < tauMin || tau >= tauMax {
		return 0, 1
	}

	delta := 0.0
	denom := cmnd[tau-1] - 2*cmnd[tau] + cmnd[tau+1]
	if denom != 0 {
		delta = 0.5 * (cmnd[tau-1] - cmnd[tau+1]) / denom
	}
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		delta = 0
	}
	delta = math.Max(-1, math.Min(1, delta))
	return float64(tau) + delta, cmnd[tau]
}

// PitchContour converts Hz into a speaker-independent z-scored semitone
// contour. Unvoiced frames are linearly interpolated. Comparing contours
// instead of raw Hz removes absolute pitch differences (e.g. male learner vs
// female TTS reference).
func PitchContour(hz []float64, voiced []bool) ([]float64, error) {
	if len(hz) != len(voiced) {
		return nil, fmt.Errorf("audio.PitchContour: length mismatch hz=%d voiced=%d", len(hz), len(voiced))
	}
	if len(hz) == 0 {
		return nil, fmt.Errorf("audio.PitchContour: no frames")
	}

	var values []float64
	for i, ok := range voiced {
		if ok && hz[i] > 0 {
			values = append(values, hz[i])
		}
	}
	if len(values) < 3 {
		return nil, fmt.Errorf("audio.PitchContour: only %d voiced frames (need >= 3)", len(values))
	}
	median := medianOf(values)

	semitones := make([]float64, len(hz))
	for i := range hz {
		if voiced[i] && hz[i] > 0 {
			semitones[i] = 12 * math.Log2(hz[i]/median)
		} else {
			semitones[i] = math.NaN()
		}
	}
	interpolateGaps(semitones)

	mean := 0.0
	for _, v := range semitones {
		mean += v
	}
	mean /= float64(len(semitones))
	variance := 0.0
	for _, v := range semitones {
		variance += (v - mean) * (v - mean)
	}
	std := math.Sqrt(variance / float64(len(semitones)))
	if std < 1e-9 {
		return make([]float64, len(semitones)), nil
	}
	out := make([]float64, len(semitones))
	for i, v := range semitones {
		out[i] = (v - mean) / std
	}
	return out, nil
}

func interpolateGaps(values []float64) {
	n := len(values)
	for i := 0; i < n; {
		if !math.IsNaN(values[i]) {
			i++
			continue
		}
		start := i
		for i < n && math.IsNaN(values[i]) {
			i++
		}
		left := math.NaN()
		right := math.NaN()
		if start > 0 {
			left = values[start-1]
		}
		if i < n {
			right = values[i]
		}
		for j := start; j < i; j++ {
			switch {
			case math.IsNaN(left) && math.IsNaN(right):
				values[j] = 0
			case math.IsNaN(left):
				values[j] = right
			case math.IsNaN(right):
				values[j] = left
			default:
				ratio := float64(j-start+1) / float64(i-start+1)
				values[j] = left + (right-left)*ratio
			}
		}
	}
}

func medianOf(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted)%2 == 1 {
		return sorted[len(sorted)/2]
	}
	return (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
}
