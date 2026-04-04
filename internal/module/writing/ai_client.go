package writing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// AIReviewer defines the interface for AI-powered writing review.
type AIReviewer interface {
	Review(question, userAnswer string) (AIFeedback, error)
}

// StubReviewer is a deterministic fake AIReviewer for tests.
// It always returns a perfect score.
type StubReviewer struct {
	Feedback AIFeedback
	Err      error
}

// Review returns the preset feedback or error.
func (s *StubReviewer) Review(_, _ string) (AIFeedback, error) {
	return s.Feedback, s.Err
}

// ClaudeClient calls the Claude (Anthropic) API to review a writing submission.
type ClaudeClient struct {
	apiKey  string
	model   string
	baseURL string
}

// NewClaudeClient creates a ClaudeClient with the given API key.
func NewClaudeClient(apiKey string) *ClaudeClient {
	return &ClaudeClient{
		apiKey:  apiKey,
		model:   "claude-3-haiku-20240307",
		baseURL: "https://api.anthropic.com/v1/messages",
	}
}

// claudeMessage is a single message in the Anthropic messages API format.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeRequest is the request body for the Anthropic messages API.
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

// claudeResponse is the response body from the Anthropic messages API.
type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Review calls Claude to evaluate the user's Japanese writing and returns structured feedback.
func (c *ClaudeClient) Review(question, userAnswer string) (AIFeedback, error) {
	slog.Debug("ClaudeClient.Review called", "question", question)

	prompt := fmt.Sprintf(`You are a Japanese writing tutor. Evaluate the following student answer.

Question: %s
Student Answer: %s

Reply ONLY with a valid JSON object in this exact format (no markdown, no extra text):
{
  "score": <integer 0-100>,
  "grammar_correct": <true|false>,
  "vocab_correct": <true|false>,
  "issue_description": "<brief description, empty if correct>",
  "corrected_sentence": "<corrected version or same if correct>",
  "alternative_phrases": ["<phrase1>", "<phrase2>"],
  "reference_answer": "<ideal answer>"
}`, question, userAnswer)

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: 512,
		Messages: []claudeMessage{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("ClaudeClient.Review: HTTP request failed", "err", err)
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("ClaudeClient.Review: unexpected status", "status", resp.StatusCode, "body", string(body))
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review: status %d", resp.StatusCode)
	}

	var claudeResp claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review decode response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review: empty response content")
	}

	var feedback AIFeedback
	if err := json.Unmarshal([]byte(claudeResp.Content[0].Text), &feedback); err != nil {
		return AIFeedback{}, fmt.Errorf("writing.ClaudeClient.Review parse feedback: %w", err)
	}

	slog.Debug("ClaudeClient.Review done", "score", feedback.Score)
	return feedback, nil
}
