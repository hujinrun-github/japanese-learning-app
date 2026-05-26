package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/word"
)

// ExampleGenerator generates example sentences for a Japanese word using an LLM.
type ExampleGenerator struct {
	apiKey   string
	endpoint string
	model    string
}

// NewExampleGenerator creates an ExampleGenerator.
// If endpoint is empty, defaults to the Anthropic Messages API.
func NewExampleGenerator(apiKey, endpoint string) *ExampleGenerator {
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}
	return &ExampleGenerator{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    "claude-3-haiku-20240307",
	}
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

type llmResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

type generatedExamples struct {
	Examples []word.WordExample `json:"examples"`
}

// GenerateExamples calls the LLM to generate 2-3 example sentences for the word.
func (g *ExampleGenerator) GenerateExamples(w word.Word) ([]word.WordExample, error) {
	slog.Debug("ExampleGenerator.GenerateExamples called", "kanji_form", w.KanjiForm)

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
	req.Header.Set("x-api-key", g.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

	var llmResp llmResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return nil, fmt.Errorf("admin.ExampleGenerator decode response: %w", err)
	}
	if len(llmResp.Content) == 0 {
		return nil, fmt.Errorf("admin.ExampleGenerator: empty response")
	}

	var gen generatedExamples
	if err := json.Unmarshal([]byte(llmResp.Content[0].Text), &gen); err != nil {
		return nil, fmt.Errorf("admin.ExampleGenerator parse examples: %w", err)
	}

	slog.Debug("ExampleGenerator.GenerateExamples done", "count", len(gen.Examples))
	return gen.Examples, nil
}
