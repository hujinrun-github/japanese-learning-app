package speaking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// TTSSynthesizer defines the interface for text-to-speech synthesis.
type TTSSynthesizer interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

// ================================================================================
// VLLMTTSClient — vLLM-hosted TTS model (e.g. Qwen3-TTS)
// ================================================================================

// VLLMTTSClient calls a vLLM-hosted TTS model (e.g. Qwen3-TTS) for speech synthesis.
type VLLMTTSClient struct {
	ttsURL       string
	model        string
	voice        string
	language     string
	instructions string
	httpClient   *http.Client
}

// VLLMTTSOption configures optional TTS parameters.
type VLLMTTSOption func(*VLLMTTSClient)

// WithVoice sets the voice name.
func WithVoice(v string) VLLMTTSOption { return func(c *VLLMTTSClient) { c.voice = v } }

// WithLanguage sets the language hint (e.g. "Japanese").
func WithLanguage(l string) VLLMTTSOption { return func(c *VLLMTTSClient) { c.language = l } }

// WithInstructions sets style/pronunciation instructions for the TTS model.
func WithInstructions(i string) VLLMTTSOption { return func(c *VLLMTTSClient) { c.instructions = i } }

// NewVLLMTTSClient creates a VLLMTTSClient with the given vLLM TTS endpoint URL and model name.
func NewVLLMTTSClient(ttsURL, model string, timeout time.Duration, opts ...VLLMTTSOption) *VLLMTTSClient {
	c := &VLLMTTSClient{
		ttsURL: ttsURL,
		model:  model,
		voice:  "ono_anna",
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type ttsRequest struct {
	Model        string  `json:"model"`
	Input        string  `json:"input"`
	Voice        string  `json:"voice"`
	Language     string  `json:"language,omitempty"`
	Instructions string  `json:"instructions,omitempty"`
	Speed        float64 `json:"speed,omitempty"`
}

// Synthesize sends text to the vLLM TTS endpoint and returns the synthesized audio bytes.
func (c *VLLMTTSClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	slog.Debug("VLLMTTSClient.Synthesize called", "text_len", len(text))

	body, err := json.Marshal(ttsRequest{
		Model:        c.model,
		Input:        text,
		Voice:        c.voice,
		Language:     c.language,
		Instructions: c.instructions,
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

// ================================================================================
// StyleBertVITSClient — style-bert-vits2 FastAPI TTS service
// ================================================================================

// StyleBertVITSClient calls the style-bert-vits2 FastAPI /voice endpoint for speech synthesis.
// The server must be started via: python server_fastapi.py [--cpu]
type StyleBertVITSClient struct {
	baseURL      string
	modelName    string
	speakerName  string
	style        string
	styleWeight  float64
	sdpRatio     float64
	noise        float64
	noiseW       float64
	length       float64
	language     string
	httpClient   *http.Client
}

// StyleBertVITSOption configures optional style-bert-vits2 TTS parameters.
type StyleBertVITSOption func(*StyleBertVITSClient)

// WithSBVModel sets the model name (directory name under model_assets).
func WithSBVModel(m string) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.modelName = m }
}

// WithSBVSpeaker sets the speaker name.
func WithSBVSpeaker(s string) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.speakerName = s }
}

// WithSBVStyle sets the style name (e.g. "Neutral").
func WithSBVStyle(s string) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.style = s }
}

// WithSBVStyleWeight sets the style weight (0-50, default 1).
func WithSBVStyleWeight(w float64) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.styleWeight = w }
}

// WithSBVSDPRatio sets SDP/DP混合比 (0-1, default 0.2).
func WithSBVSDPRatio(r float64) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.sdpRatio = r }
}

// WithSBVNoise sets the noise scale (0.1-2, default 0.6).
func WithSBVNoise(n float64) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.noise = n }
}

// WithSBVNoiseW sets the SDP noise (0.1-2, default 0.8).
func WithSBVNoiseW(n float64) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.noiseW = n }
}

// WithSBVLength sets the speech speed (0.1-2, default 1).
func WithSBVLength(l float64) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.length = l }
}

// WithSBVLanguage sets the language hint (JP/EN/ZH, default "JP").
func WithSBVLanguage(l string) StyleBertVITSOption {
	return func(c *StyleBertVITSClient) { c.language = l }
}

// NewStyleBertVITSClient creates a StyleBertVITSClient for the given base URL and timeout.
// baseURL should be the root URL of the FastAPI server (e.g. "http://127.0.0.1:5000").
func NewStyleBertVITSClient(baseURL string, timeout time.Duration, opts ...StyleBertVITSOption) *StyleBertVITSClient {
	c := &StyleBertVITSClient{
		baseURL:     baseURL,
		modelName:   "amitaro",
		speakerName: "あみたろ",
		style:       "Neutral",
		styleWeight: 1.0,
		sdpRatio:    0.5,
		noise:       0.6,
		noiseW:      0.8,
		length:      1.0,
		language:    "JP",
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Synthesize sends text to the style-bert-vits2 /voice endpoint and returns WAV audio bytes.
func (c *StyleBertVITSClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	slog.Debug("StyleBertVITSClient.Synthesize called", "text_len", len(text), "model", c.modelName, "speaker", c.speakerName)

	u, err := url.Parse(c.baseURL + "/voice")
	if err != nil {
		return nil, fmt.Errorf("speaking.StyleBertVITSClient.Synthesize parse url: %w", err)
	}

	q := u.Query()
	q.Set("text", text)
	q.Set("model_name", c.modelName)
	q.Set("speaker_name", c.speakerName)
	q.Set("style", c.style)
	q.Set("style_weight", fmt.Sprintf("%.1f", c.styleWeight))
	q.Set("sdp_ratio", fmt.Sprintf("%.1f", c.sdpRatio))
	q.Set("noise", fmt.Sprintf("%.1f", c.noise))
	q.Set("noisew", fmt.Sprintf("%.1f", c.noiseW))
	q.Set("length", fmt.Sprintf("%.1f", c.length))
	q.Set("language", c.language)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("style-bert-vits http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		slog.Error("style-bert-vits returned error", "status", resp.StatusCode, "body", string(errBody))
		return nil, fmt.Errorf("style-bert-vits tts returned status %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("empty audio response from style-bert-vits tts")
	}

	slog.Debug("StyleBertVITSClient.Synthesize done", "audio_bytes", len(audio))
	return audio, nil
}
