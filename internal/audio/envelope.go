package audio

import (
	"fmt"
	"math"
	"sort"
)

const (
	// DefaultFrameMS and DefaultHopMS define the energy envelope resolution (100 fps).
	DefaultFrameMS = 20
	DefaultHopMS   = 10
	// PitchFrameMS and PitchHopMS define the pitch analysis resolution.
	PitchFrameMS = 40
	PitchHopMS   = 10

	vadHangoverFrames = 3
	vadOnRatio        = 0.12
	vadOffRatio       = 0.08
	pauseThresholdMS  = 200
)

// PauseSpan is a silent gap inside the detected speech region.
type PauseSpan struct {
	StartMS int64 `json:"start_ms"`
	EndMS   int64 `json:"end_ms"`
}

// VADResult summarizes voice activity for an utterance.
type VADResult struct {
	VoicedRatio   float64     `json:"voiced_ratio"`
	SpeechStartMS int64       `json:"speech_start_ms"`
	SpeechEndMS   int64       `json:"speech_end_ms"`
	SpeechMS      int64       `json:"speech_ms"`
	Pauses        []PauseSpan `json:"pauses"`
}

// RMSFrames computes windowed RMS energy frames.
func RMSFrames(samples []float64, sampleRate, frameMS, hopMS int) ([]float64, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("audio.RMSFrames: no samples")
	}
	if sampleRate <= 0 || frameMS <= 0 || hopMS <= 0 || hopMS > frameMS {
		return nil, fmt.Errorf("audio.RMSFrames: invalid config rate=%d frame_ms=%d hop_ms=%d", sampleRate, frameMS, hopMS)
	}
	frame := sampleRate * frameMS / 1000
	hop := sampleRate * hopMS / 1000
	if frame < 1 || hop < 1 {
		return nil, fmt.Errorf("audio.RMSFrames: frame or hop smaller than one sample")
	}
	if len(samples) < frame {
		return nil, fmt.Errorf("audio.RMSFrames: %d samples shorter than frame %d", len(samples), frame)
	}

	count := (len(samples)-frame)/hop + 1
	frames := make([]float64, count)
	for i := range frames {
		start := i * hop
		sumSq := 0.0
		for j := 0; j < frame; j++ {
			v := samples[start+j]
			sumSq += v * v
		}
		frames[i] = math.Sqrt(sumSq / float64(frame))
	}
	return frames, nil
}

// NormalizeFrames compresses energy frames into [0, 1] using log scaling and
// the frame maximum. All-zero input stays all-zero.
func NormalizeFrames(frames []float64) []float64 {
	out := make([]float64, len(frames))
	peak := 0.0
	for _, v := range frames {
		if v > peak {
			peak = v
		}
	}
	if peak <= 0 {
		return out
	}
	const k = 9.0
	denom := math.Log1p(k)
	for i, v := range frames {
		if v <= 0 {
			continue
		}
		out[i] = math.Min(1, math.Log1p(k*v/peak)/denom)
	}
	return out
}

// DetectVAD applies adaptive thresholds with hysteresis and returns the speech
// region, voiced ratio and long internal pauses.
func DetectVAD(frames []float64, hopMS int) (VADResult, error) {
	if len(frames) == 0 {
		return VADResult{}, fmt.Errorf("audio.DetectVAD: no frames")
	}
	if hopMS <= 0 {
		return VADResult{}, fmt.Errorf("audio.DetectVAD: invalid hop_ms %d", hopMS)
	}

	peak := 0.0
	for _, v := range frames {
		if v > peak {
			peak = v
		}
	}
	on := peak * vadOnRatio
	off := peak * vadOffRatio

	voiced := make([]bool, len(frames))
	active := false
	above, below := 0, 0
	for i, v := range frames {
		switch {
		case v > on:
			above++
			below = 0
		case v < off:
			below++
			above = 0
		default:
			above, below = 0, 0
		}
		if above >= vadHangoverFrames {
			active = true
		}
		if below >= vadHangoverFrames {
			active = false
		}
		voiced[i] = active
	}

	first, last := -1, -1
	count := 0
	for i, ok := range voiced {
		if ok {
			count++
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	result := VADResult{
		VoicedRatio: float64(count) / float64(len(frames)),
		Pauses:      []PauseSpan{},
	}
	if first < 0 {
		return result, nil
	}

	result.SpeechStartMS = int64(first * hopMS)
	result.SpeechEndMS = int64((last + 1) * hopMS)
	result.SpeechMS = result.SpeechEndMS - result.SpeechStartMS

	gapStart := -1
	for i := first; i <= last; i++ {
		if !voiced[i] && gapStart < 0 {
			gapStart = i
		}
		if voiced[i] && gapStart >= 0 {
			result.Pauses = appendPause(result.Pauses, gapStart, i, hopMS)
			gapStart = -1
		}
	}
	if gapStart >= 0 {
		result.Pauses = appendPause(result.Pauses, gapStart, last+1, hopMS)
	}
	return result, nil
}

func appendPause(pauses []PauseSpan, startFrame, endFrame, hopMS int) []PauseSpan {
	if (endFrame-startFrame)*hopMS < pauseThresholdMS {
		return pauses
	}
	return append(pauses, PauseSpan{
		StartMS: int64(startFrame * hopMS),
		EndMS:   int64(endFrame * hopMS),
	})
}

// percentile returns the p-th percentile (0..1) of a copy of frames.
// Kept as a small helper for calibration work; sorting is confined to the copy.
func percentile(frames []float64, p float64) float64 {
	if len(frames) == 0 {
		return 0
	}
	sorted := append([]float64(nil), frames...)
	sort.Float64s(sorted)
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}
