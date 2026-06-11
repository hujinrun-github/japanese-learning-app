package speaking

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GradioTTSClient calls a Gradio TTS endpoint exposed at /run_instruct.
type GradioTTSClient struct {
	baseURL      string
	language     string
	speaker      string
	instructions string
	httpClient   *http.Client
}

// GradioTTSOption configures optional Gradio TTS parameters.
type GradioTTSOption func(*GradioTTSClient)

// WithGradioLanguage sets the language dropdown value.
func WithGradioLanguage(language string) GradioTTSOption {
	return func(c *GradioTTSClient) { c.language = language }
}

// WithGradioSpeaker sets the speaker dropdown value.
func WithGradioSpeaker(speaker string) GradioTTSOption {
	return func(c *GradioTTSClient) { c.speaker = speaker }
}

// WithGradioInstructions sets the optional instruction textbox value.
func WithGradioInstructions(instructions string) GradioTTSOption {
	return func(c *GradioTTSClient) { c.instructions = instructions }
}

// NewGradioTTSClient creates a GradioTTSClient for a Gradio app root URL.
func NewGradioTTSClient(baseURL string, timeout time.Duration, opts ...GradioTTSOption) *GradioTTSClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8000"
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	c := &GradioTTSClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		language:     "Japanese",
		speaker:      "Vivian",
		instructions: "",
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type gradioRunRequest struct {
	Text         string `json:"text"`
	Language     string `json:"lang_disp"`
	Speaker      string `json:"spk_disp"`
	Instructions string `json:"instruct"`
}

type gradioEventResponse struct {
	EventID string `json:"event_id"`
}

type gradioFileData struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

var errNoGradioAudioYet = errors.New("no gradio audio in event payload")

// Synthesize sends text to Gradio and returns the downloaded audio bytes.
func (c *GradioTTSClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	slog.Debug("GradioTTSClient.Synthesize called", "text_len", len(text), "base_url", c.baseURL, "speaker", c.speaker)

	eventID, err := c.start(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("speaking.GradioTTSClient.Synthesize start: %w", err)
	}

	audioURL, err := c.waitForAudioURL(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("speaking.GradioTTSClient.Synthesize wait: %w", err)
	}

	audio, err := c.downloadAudio(ctx, audioURL)
	if err != nil {
		return nil, fmt.Errorf("speaking.GradioTTSClient.Synthesize download: %w", err)
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("empty audio response from gradio tts")
	}

	slog.Debug("GradioTTSClient.Synthesize done", "audio_bytes", len(audio))
	return audio, nil
}

func (c *GradioTTSClient) start(ctx context.Context, text string) (string, error) {
	body, err := json.Marshal(gradioRunRequest{
		Text:         text,
		Language:     c.language,
		Speaker:      c.speaker,
		Instructions: c.instructions,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/gradio_api/call/v2/run_instruct", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("gradio tts start returned status %d: %s", resp.StatusCode, string(errBody))
	}

	var eventResp gradioEventResponse
	if err := json.NewDecoder(resp.Body).Decode(&eventResp); err != nil {
		return "", fmt.Errorf("decode event response: %w", err)
	}
	if eventResp.EventID == "" {
		return "", fmt.Errorf("empty event_id from gradio tts")
	}
	return eventResp.EventID, nil
}

func (c *GradioTTSClient) waitForAudioURL(ctx context.Context, eventID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/gradio_api/call/run_instruct/"+url.PathEscape(eventID), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("gradio tts event returned status %d: %s", resp.StatusCode, string(errBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	currentEvent := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if currentEvent == "heartbeat" || payload == "" || payload == "null" || payload == "[]" {
			continue
		}
		if currentEvent == "error" {
			return "", fmt.Errorf("gradio tts event error: %s", payload)
		}
		audioURL, err := parseGradioAudioURL(payload)
		if err != nil {
			if errors.Is(err, errNoGradioAudioYet) {
				continue
			}
			return "", err
		}
		return c.resolveURL(audioURL)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read event stream: %w", err)
	}
	return "", fmt.Errorf("missing audio url in gradio event stream")
}

func parseGradioAudioURL(payload string) (string, error) {
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &items); err != nil {
		return "", fmt.Errorf("parse gradio data payload: %w", err)
	}
	if len(items) == 0 {
		return "", errNoGradioAudioYet
	}
	if string(items[0]) == "null" {
		return "", errNoGradioAudioYet
	}

	var file gradioFileData
	if err := json.Unmarshal(items[0], &file); err != nil {
		return "", fmt.Errorf("parse gradio file data: %w", err)
	}
	if file.URL == "" {
		return "", fmt.Errorf("missing audio url in gradio file data")
	}
	return file.URL, nil
}

func (c *GradioTTSClient) resolveURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse audio url: %w", err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}

	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	return base.ResolveReference(parsed).String(), nil
}

func (c *GradioTTSClient) downloadAudio(ctx context.Context, audioURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("gradio tts audio returned status %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio response: %w", err)
	}
	return audio, nil
}
