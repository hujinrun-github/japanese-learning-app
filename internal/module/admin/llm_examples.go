package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"japanese-learning-app/internal/module/word"
)

// ExampleGenerator generates example sentences for a Japanese word using an LLM.
// Supports both Anthropic Messages API and OpenAI-compatible APIs (DeepSeek, etc.).
type ExampleGenerator struct {
	apiKey   string
	endpoint string
	model    string
}

// NewExampleGenerator creates an ExampleGenerator.
// Defaults to DeepSeek API if endpoint/model not specified.
func NewExampleGenerator(apiKey, endpoint, model string) *ExampleGenerator {
	if endpoint == "" {
		endpoint = "https://api.deepseek.com/v1/chat/completions"
	}
	if model == "" {
		model = defaultModel(endpoint)
	}
	return &ExampleGenerator{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
	}
}

func defaultModel(endpoint string) string {
	if strings.Contains(endpoint, "deepseek") {
		return "deepseek-chat"
	}
	if strings.Contains(endpoint, "openai") {
		return "gpt-4o-mini"
	}
	return "claude-3-haiku-20240307"
}

// isAnthropic returns true if the endpoint is Anthropic's Messages API.
func (g *ExampleGenerator) isAnthropic() bool {
	return strings.Contains(g.endpoint, "anthropic")
}

type llmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []llmMessage `json:"messages"`
}

// Anthropic response
type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// OpenAI-compatible response (DeepSeek, OpenAI, etc.)
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type generatedExamples struct {
	Examples []word.WordExample `json:"examples"`
}

// GenerateExamples calls the LLM to generate 2-3 example sentences for the word.
func (g *ExampleGenerator) GenerateExamples(w word.Word) ([]word.WordExample, error) {
	slog.Debug("ExampleGenerator.GenerateExamples called", "kanji_form", w.KanjiForm, "endpoint", g.endpoint)

	prompt := fmt.Sprintf(`You are a Japanese language teacher. Generate 2-3 natural example sentences for this word:

Word: %s (reading: %s)
Meaning: %s
JLPT: %s

Requirements:
- Use natural, level-appropriate Japanese
- The word MUST appear in each sentence
- Provide Chinese translation
- Provide furigana_html with <ruby> tags for ALL kanji characters

Reply ONLY with a valid JSON object (no markdown, no extra text):
{
  "examples": [
    {
      "japanese": "朝ごはんを食べる",
      "chinese": "吃早饭",
      "furigana_html": "<ruby>朝<rt>あさ</rt></ruby>ごはんを<ruby>食<rt>た</rt></ruby>べる"
    }
  ]
}`, w.KanjiForm, w.Reading, w.Meaning, string(w.JLPTLevel))

	reqBody := llmRequest{
		Model:     g.model,
		MaxTokens: 1024,
		Messages:  []llmMessage{{Role: "user", Content: prompt}},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("admin.ExampleGenerator marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, g.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("admin.ExampleGenerator new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if g.isAnthropic() {
		req.Header.Set("x-api-key", g.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("ExampleGenerator: HTTP request failed", "err", err)
		return nil, fmt.Errorf("admin.ExampleGenerator request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("ExampleGenerator: unexpected status", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("admin.ExampleGenerator: status %d", resp.StatusCode)
	}

	content, err := g.parseResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	var gen generatedExamples
	if err := json.Unmarshal([]byte(content), &gen); err != nil {
		return nil, fmt.Errorf("admin.ExampleGenerator parse examples: %w, raw: %s", err, content)
	}

	slog.Debug("ExampleGenerator.GenerateExamples done", "count", len(gen.Examples))
	return gen.Examples, nil
}

func (g *ExampleGenerator) parseResponse(body io.Reader) (string, error) {
	if g.isAnthropic() {
		var resp anthropicResponse
		if err := json.NewDecoder(body).Decode(&resp); err != nil {
			return "", fmt.Errorf("admin.ExampleGenerator decode response: %w", err)
		}
		if len(resp.Content) == 0 {
			return "", fmt.Errorf("admin.ExampleGenerator: empty response")
		}
		return resp.Content[0].Text, nil
	}

	// OpenAI-compatible
	var resp openAIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return "", fmt.Errorf("admin.ExampleGenerator decode response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("admin.ExampleGenerator: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
