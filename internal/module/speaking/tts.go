package speaking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// TTSSynthesizer defines the interface for text-to-speech synthesis.
type TTSSynthesizer interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

// VLLMTTSClient calls a vLLM-hosted TTS model (e.g. Qwen3-TTS) for speech synthesis.
type VLLMTTSClient struct {
	ttsURL     string
	model      string
	httpClient *http.Client
}

// NewVLLMTTSClient creates a VLLMTTSClient with the given vLLM TTS endpoint URL and model name.
func NewVLLMTTSClient(ttsURL, model string, timeout time.Duration) *VLLMTTSClient {
	return &VLLMTTSClient{
		ttsURL: ttsURL,
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type ttsRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

// Synthesize sends text to the vLLM TTS endpoint and returns the synthesized audio bytes.
func (c *VLLMTTSClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	slog.Debug("VLLMTTSClient.Synthesize called", "text_len", len(text))

	body, err := json.Marshal(ttsRequest{
		Model: c.model,
		Input: text,
		Voice: "ono_anna",
	})
	if err != nil {
		return nil, fmt.Errorf("speaking.VLLMTTSClient.Synthesize marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ttsURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("vllm tts returned status %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("empty audio response from vllm tts")
	}

	slog.Debug("VLLMTTSClient.Synthesize done", "audio_bytes", len(audio))
	return audio, nil
}
