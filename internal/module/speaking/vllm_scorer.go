package speaking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"japanese-learning-app/internal/audio"
)

// ASRScorer defines the interface for automatic speech recognition scoring.
type ASRScorer interface {
	ScoreAudio(ctx context.Context, audio []byte, referenceText string) (score int, recognizedText string, err error)
}

// VLLMScorer scores pronunciation by sending audio to a vLLM-hosted Whisper model
// and computing CER (Character Error Rate) against the reference text.
type VLLMScorer struct {
	vllmURL    string
	model      string
	httpClient *http.Client
	whisperCPP bool
}

// NewVLLMScorer creates a VLLMScorer with the given vLLM endpoint URL and model name.
func NewVLLMScorer(vllmURL, model string, timeout time.Duration) *VLLMScorer {
	return &VLLMScorer{
		vllmURL: vllmURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// NewWhisperCPPScorer explicitly enables whisper.cpp's decoder options and
// short-WAV context sizing. Other OpenAI-compatible servers receive no such
// extensions, even when their URL happens to end in /inference.
func NewWhisperCPPScorer(endpoint, model string, timeout time.Duration) *VLLMScorer {
	scorer := NewVLLMScorer(endpoint, model, timeout)
	scorer.whisperCPP = true
	return scorer
}

// ScoreAudio sends audio to vLLM for ASR transcription, then computes CER against
// the reference text. Returns a score in [0, 100], the recognized text, and any error.
func (s *VLLMScorer) ScoreAudio(ctx context.Context, audio []byte, referenceText string) (int, string, error) {
	slog.Debug("VLLMScorer.ScoreAudio called", "audio_bytes", len(audio))

	recognizedText, err := s.transcribe(ctx, audio)
	if err != nil {
		slog.Error("VLLMScorer.ScoreAudio: transcription failed", "err", err)
		return 0, "", fmt.Errorf("speaking.VLLMScorer.ScoreAudio transcribe: %w", err)
	}

	if recognizedText == "" {
		slog.Error("VLLMScorer.ScoreAudio: empty transcription")
		return 0, "", fmt.Errorf("speaking.VLLMScorer.ScoreAudio: empty transcription result")
	}

	cer := ComputeCER(referenceText, recognizedText)
	score := ScoreFromCER(cer)

	slog.Debug("VLLMScorer.ScoreAudio done", "score", score, "cer", cer, "recognized", recognizedText)
	return score, recognizedText, nil
}

// transcribe uploads audio to vLLM's /v1/audio/transcriptions endpoint and returns
// the recognized text.
func (s *VLLMScorer) transcribe(ctx context.Context, audio []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", "attempt.wav")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("write audio: %w", err)
	}
	type formField struct{ name, value string }
	fields := []formField{
		{"model", s.model}, {"language", "ja"}, {"response_format", "json"},
	}
	if s.whisperCPP {
		fields = append(fields, []formField{
			{"best_of", "1"}, {"beam_size", "1"}, {"temperature", "0"}, {"temperature_inc", "0"},
			// JSON feedback uses text only. Preserve segment timestamps for
			// long-audio seek while skipping per-token timestamp computation.
			{"token_timestamps", "false"},
		}...)
		if size := whisperCPPAudioContext(audio); size > 0 {
			fields = append(fields, formField{"audio_ctx", strconv.Itoa(size)})
		}
	}
	for _, field := range fields {
		if err := w.WriteField(field.name, field.value); err != nil {
			return "", fmt.Errorf("write multipart field %s: %w", field.name, err)
		}
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.vllmURL, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("VLLMScorer: close transcription response failed", "err", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			return "", fmt.Errorf("vllm status %d, read error body: %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("vllm returned status %d: %s", resp.StatusCode, string(body))
	}

	var asrResp struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&asrResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if strings.TrimSpace(asrResp.Text) == "" {
		return "", fmt.Errorf("empty response from vllm")
	}
	if utf8.RuneCountInString(asrResp.Text) > 8192 {
		return "", fmt.Errorf("transcription exceeds 8192 characters")
	}

	return asrResp.Text, nil
}

func whisperCPPAudioContext(wav []byte) int {
	duration, err := audio.WAVDuration(wav)
	if err != nil {
		slog.Debug("speaking: keeping whisper.cpp model context; WAV duration unavailable", "err", err)
		return 0
	}
	// whisper.cpp's encoder context uses 20 ms steps (10 ms mel frames,
	// downsampled by two). Cover the complete recording plus two seconds of
	// headroom, round up to 128, and retain the tested minimum of 384 steps.
	// Long recordings keep the model's default 1500-step / 30-second windows;
	// never apply a fixed short context or a duration limit to a whole lesson.
	if duration >= 25*time.Second {
		return 0
	}
	const step = 20 * time.Millisecond
	size := int((duration + 2*time.Second + step - 1) / step)
	size = max(384, ((size+127)/128)*128)
	if size >= 1500 {
		return 0
	}
	slog.Debug("speaking: sizing whisper.cpp short-recording context", "duration_ms", duration.Milliseconds(), "audio_ctx", size)
	return size
}
