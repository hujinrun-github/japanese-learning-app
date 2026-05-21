package translation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// TranslationReviewer evaluates user translations.
type TranslationReviewer interface {
	Review(direction, sourceText, userTranslation, referenceTranslation string) (TranslationFeedback, error)
}

// StubReviewer returns preset feedback for tests.
type StubReviewer struct {
	Feedback TranslationFeedback
	Err      error
}

func (s *StubReviewer) Review(_, _, _, _ string) (TranslationFeedback, error) {
	return s.Feedback, s.Err
}

// ClaudeTranslationReviewer calls the Claude API for translation review.
type ClaudeTranslationReviewer struct {
	apiKey  string
	model   string
	baseURL string
}

func NewClaudeTranslationReviewer(apiKey string) *ClaudeTranslationReviewer {
	return &ClaudeTranslationReviewer{
		apiKey:  apiKey,
		model:   "claude-3-haiku-20240307",
		baseURL: "https://api.anthropic.com/v1/messages",
	}
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (c *ClaudeTranslationReviewer) Review(direction, sourceText, userTranslation, referenceTranslation string) (TranslationFeedback, error) {
	slog.Debug("ClaudeTranslationReviewer.Review called", "direction", direction, "source_len", len(sourceText))

	prompt := fmt.Sprintf(`You are a Japanese translation tutor. Evaluate the student's translation.

Direction: %s (source → target)
Source text: %s
Student translation: %s
Reference translation (optional): %s

Reply ONLY with a valid JSON object in this exact format (no markdown, no extra text):
{
  "ai_score": <integer 0-100>,
  "grammar_explanations": [
    {
      "grammar_point": "<grammar name in Japanese>",
      "explanation": "<brief explanation>",
      "matched_db_id": <integer, use 0 if no match>
    }
  ],
  "issue_description": "<brief description of issues, empty string if perfect>",
  "corrected_translation": "<corrected version, same as student if correct>",
  "reference_translation": "<ideal translation>"
}`, direction, sourceText, userTranslation, referenceTranslation)

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: 1024,
		Messages:  []claudeMessage{{Role: "user", Content: prompt}},
	}

	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequest(http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("ClaudeTranslationReviewer.Review: HTTP request failed", "err", err)
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("ClaudeTranslationReviewer.Review: unexpected status", "status", resp.StatusCode, "body", string(body))
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review: status %d", resp.StatusCode)
	}

	var claudeResp claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review decode response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review: empty response content")
	}

	var feedback TranslationFeedback
	if err := json.Unmarshal([]byte(claudeResp.Content[0].Text), &feedback); err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review parse feedback: %w", err)
	}

	slog.Debug("ClaudeTranslationReviewer.Review done", "ai_score", feedback.AIScore)
	return feedback, nil
}
