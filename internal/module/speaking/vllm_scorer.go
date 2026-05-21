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
	"time"
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

	fw, err := w.CreateFormFile("file", "audio.webm")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("write audio: %w", err)
	}
	w.WriteField("model", s.model)
	w.WriteField("language", "ja")
	w.WriteField("response_format", "text")
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.vllmURL, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("vllm returned status %d: %s", resp.StatusCode, string(body))
	}

	var asrResp struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&asrResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if asrResp.Text == "" {
		return "", fmt.Errorf("empty response from vllm")
	}

	return asrResp.Text, nil
}
