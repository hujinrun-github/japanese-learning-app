package translation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// TranslationReviewer evaluates user translations.
type TranslationReviewer interface {
	Review(direction, sourceText, userTranslation, referenceTranslation string) (TranslationFeedback, error)
}

// StubReviewer returns preset feedback for tests, or local scoring and grammar
// feedback when no preset is configured.
type StubReviewer struct {
	Feedback TranslationFeedback
	Err      error
}

func (s *StubReviewer) Review(direction, sourceText, userTranslation, referenceTranslation string) (TranslationFeedback, error) {
	if s.Err != nil {
		return TranslationFeedback{}, s.Err
	}
	if s.Feedback.AIScore != 0 || len(s.Feedback.GrammarExplanations) > 0 ||
		s.Feedback.IssueDescription != "" || s.Feedback.CorrectedTranslation != "" || s.Feedback.ReferenceTranslation != "" {
		return s.Feedback, nil
	}

	return TranslationFeedback{
		AIScore:             computeRuleScoreForReference(direction, sourceText, userTranslation, referenceTranslation),
		GrammarExplanations: detectGrammarExplanations(direction, sourceText, userTranslation, referenceTranslation),
	}, nil
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

	prompt := buildTranslationReviewPrompt(direction, sourceText, userTranslation, referenceTranslation)

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: 1024,
		Messages:  []claudeMessage{{Role: "user", Content: prompt}},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review marshal request: %w", err)
	}

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
	if err := json.Unmarshal([]byte(normalizeJSONResponse(claudeResp.Content[0].Text)), &feedback); err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review parse feedback: %w", err)
	}

	slog.Debug("ClaudeTranslationReviewer.Review done", "ai_score", feedback.AIScore)
	return feedback, nil
}

func normalizeJSONResponse(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 {
			lines = lines[1:]
			if strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			trimmed = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	return trimmed
}

func buildTranslationReviewPrompt(direction, sourceText, userTranslation, referenceTranslation string) string {
	return fmt.Sprintf(`You are a Japanese translation tutor. Evaluate the student's translation.

Direction: %s (source → target)
Source text: %s
Student translation: %s
Reference translation (optional): %s

Analyze Japanese grammar separately from translation accuracy:
- For jp2cn, identify grammar points in the Japanese source text.
- For cn2jp, identify grammar points in the Japanese student translation; if it is empty or invalid Japanese, identify useful grammar from the reference translation when available.
- Return every relevant Japanese grammar pattern you can identify, including particles and verb constructions. Do not leave this array empty when the Japanese text contains an identifiable pattern.

Reply ONLY with a valid JSON object in this exact format (no markdown, no extra text):
{
  "ai_score": <integer 0-100>,
  "grammar_explanations": [
    {
      "grammar_point": "<grammar name in Japanese>",
      "explanation": "<brief explanation in the user's language>",
      "matched_db_id": <integer, use 0 if no match>
    }
  ],
  "issue_description": "<brief description of issues, empty string if perfect>",
  "corrected_translation": "<corrected version, same as student if correct>",
  "reference_translation": "<ideal translation>"
}`, direction, sourceText, userTranslation, referenceTranslation)
}
