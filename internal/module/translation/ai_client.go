package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
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

	grammar := detectGrammarExplanations(grammarText(direction, sourceText, referenceTranslation))
	feedback := TranslationFeedback{
		AIScore:             computeRuleScoreForReference(direction, sourceText, userTranslation, referenceTranslation),
		GrammarExplanations: grammar,
	}
	if len(grammar) > 0 {
		feedback.GrammarSource = grammarSourceLocal
	}
	return feedback, nil
}

// ClaudeTranslationReviewer calls the Claude API for translation review.
type ClaudeTranslationReviewer struct {
	apiKey  string
	model   string
	baseURL string
	timeout time.Duration
}

func NewClaudeTranslationReviewer(apiKey, endpoint string, timeout time.Duration) *ClaudeTranslationReviewer {
	return &ClaudeTranslationReviewer{
		apiKey:  apiKey,
		model:   "claude-haiku-5-5",
		baseURL: endpoint,
		timeout: timeout,
	}
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	Messages     []claudeMessage    `json:"messages"`
	OutputConfig claudeOutputConfig `json:"output_config"`
}

type claudeOutputConfig struct {
	Effort string `json:"effort"`
}

type claudeResponse struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (c *ClaudeTranslationReviewer) Review(direction, sourceText, userTranslation, referenceTranslation string) (TranslationFeedback, error) {
	slog.Debug("ClaudeTranslationReviewer.Review called", "direction", direction, "source_len", len(sourceText))

	prompt := buildTranslationReviewPrompt(direction, sourceText, userTranslation, referenceTranslation)

	reqBody := claudeRequest{
		Model:        c.model,
		MaxTokens:    2048,
		Messages:     []claudeMessage{{Role: "user", Content: prompt}},
		OutputConfig: claudeOutputConfig{Effort: "low"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
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

	switch claudeResp.StopReason {
	case "refusal", "max_tokens", "model_context_window_exceeded":
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review: response stopped with %s", claudeResp.StopReason)
	}
	var answer strings.Builder
	for _, block := range claudeResp.Content {
		if block.Type == "text" || block.Type == "" {
			answer.WriteString(block.Text)
		}
	}

	var feedback TranslationFeedback
	if err := json.Unmarshal([]byte(normalizeJSONResponse(answer.String())), &feedback); err != nil {
		return TranslationFeedback{}, fmt.Errorf("translation.ClaudeTranslationReviewer.Review parse feedback: %w", err)
	}
	feedback.GrammarSource = ""

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

Provide a natural reference translation of the source in the target language, independently of the student's answer.

Analyze the exercise's Japanese grammar separately from grading the student:
- For jp2cn, analyze only the Japanese source text.
- For cn2jp, analyze only the Japanese reference translation, using the supplied reference if present or your generated reference if absent. Discuss the student's grammar mistakes in issue_description.
- Give a few high-confidence points. Each point must quote an exact, meaningful span of the Japanese text as evidence and explain its function in context. Kana inside a word do not count as particles or endings. Omit uncertain points and return an empty array if none can be verified.

Reply ONLY with a valid JSON object in this exact format (no markdown, no extra text):
{
  "ai_score": <integer 0-100>,
  "grammar_explanations": [
    {
      "grammar_point": "<grammar name in Japanese>",
      "explanation": "<brief explanation of the usage in this sentence, in Chinese>",
      "evidence": "<exact Japanese text showing the grammar in context>"
    }
  ],
  "issue_description": "<brief description of issues, empty string if perfect>",
  "corrected_translation": "<corrected version, same as student if correct>",
  "reference_translation": "<ideal translation>"
}`, direction, sourceText, userTranslation, referenceTranslation)
}
