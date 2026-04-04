package speaking_test

import (
	"math"
	"testing"

	"japanese-learning-app/internal/module/speaking"
)

// generateSine creates a sine wave as 16-bit PCM bytes, producing zero crossings.
func generateSine(samples int, freq float64, sampleRate float64) []byte {
	data := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		val := int16(math.Sin(2*math.Pi*freq*float64(i)/sampleRate) * 16000)
		data[i*2] = byte(val)
		data[i*2+1] = byte(val >> 8)
	}
	return data
}

func generateSilence(samples int) []byte {
	return make([]byte, samples*2)
}

func TestWaveformScorer_SameAudio(t *testing.T) {
	scorer := speaking.NewWaveformScorer()
	audio := generateSine(4000, 440, 16000) // 440 Hz sine

	result, err := scorer.Score(audio, audio)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallScore < 90 {
		t.Errorf("identical audio should score >= 90, got %d", result.OverallScore)
	}
}

func TestWaveformScorer_SilenceVsAudio(t *testing.T) {
	scorer := speaking.NewWaveformScorer()
	ref := generateSine(4000, 440, 16000) // active audio
	userAudio := generateSilence(4000)    // silence

	result, err := scorer.Score(ref, userAudio)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallScore > 30 {
		t.Errorf("silence vs audio should score <= 30, got %d", result.OverallScore)
	}
}

func TestWaveformScorer_SimilarFrequency(t *testing.T) {
	scorer := speaking.NewWaveformScorer()
	ref := generateSine(4000, 440, 16000)  // 440 Hz
	user := generateSine(4000, 450, 16000) // 450 Hz (close)

	result, err := scorer.Score(ref, user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Similar audio should score reasonably high
	if result.OverallScore < 50 {
		t.Errorf("similar frequency audio should score >= 50, got %d", result.OverallScore)
	}
}

func TestWaveformScorer_DifferentFrequency(t *testing.T) {
	scorer := speaking.NewWaveformScorer()
	ref := generateSine(4000, 440, 16000)   // 440 Hz
	user := generateSine(4000, 2000, 16000) // 2000 Hz (very different ZCR)

	result, err := scorer.Score(ref, user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Noticeably different audio should score lower
	if result.OverallScore > 80 {
		t.Errorf("very different audio should score <= 80, got %d", result.OverallScore)
	}
}

func TestWaveformScorer_ScoreRange(t *testing.T) {
	scorer := speaking.NewWaveformScorer()
	ref := generateSine(4000, 440, 16000)
	user := generateSine(4000, 1000, 16000)

	result, err := scorer.Score(ref, user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallScore < 0 || result.OverallScore > 100 {
		t.Errorf("score must be in [0, 100], got %d", result.OverallScore)
	}
}

func TestWaveformScorer_EmptyAudio(t *testing.T) {
	scorer := speaking.NewWaveformScorer()

	result, err := scorer.Score([]byte{}, []byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallScore != 0 {
		t.Errorf("empty audio should score 0, got %d", result.OverallScore)
	}
}

func TestWaveformScorer_FeedbackMSNonNegative(t *testing.T) {
	scorer := speaking.NewWaveformScorer()
	ref := generateSine(8000, 440, 16000)
	user := generateSine(8000, 440, 16000)

	result, err := scorer.Score(ref, user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FeedbackMS < 0 {
		t.Errorf("FeedbackMS must be non-negative, got %d", result.FeedbackMS)
	}
}

func TestAudioScorer_Interface(t *testing.T) {
	// Verify WaveformScorer satisfies AudioScorer interface
	var _ speaking.AudioScorer = speaking.NewWaveformScorer()
}
