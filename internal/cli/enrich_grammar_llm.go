package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ── LLM client (OpenAI-compatible /chat/completions) ────────────────────────

// GrammarLLMConfig points the enricher at an OpenAI-compatible chat endpoint.
// DeepSeek, Moonshot, DashScope-compatible Qwen and OpenAI all speak this shape.
type GrammarLLMConfig struct {
	Endpoint string
	APIKey   string
	Model    string
	Timeout  time.Duration
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// llmGenerated is the JSON shape we ask the model to return.
type llmGenerated struct {
	ConjunctionRule string       `json:"conjunction_rule"`
	UsageNote       string       `json:"usage_note"`
	Examples        []llmExample `json:"examples"`
}

type llmExample struct {
	Japanese string `json:"japanese"`
	Chinese  string `json:"chinese"`
}

// completeChat calls the chat endpoint and returns the assistant message text.
func completeChat(ctx context.Context, cfg GrammarLLMConfig, client *http.Client, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: "你是一名资深日语教师，只输出 JSON，不要任何解释或 Markdown 代码块。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
		MaxTokens:   1200,
	})
	if err != nil {
		return "", fmt.Errorf("cli.completeChat marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("cli.completeChat new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cli.completeChat do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cli.completeChat read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("cli.completeChat status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("cli.completeChat unmarshal: %w", err)
	}
	if len(parsed.Choices) == 0 {
		if parsed.Error.Message != "" {
			return "", fmt.Errorf("cli.completeChat api error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("cli.completeChat: empty choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

// parseLLMGenerated extracts the JSON payload, tolerating ```json fences.
func parseLLMGenerated(text string) (llmGenerated, error) {
	cleaned := strings.TrimSpace(text)
	if idx := strings.Index(cleaned, "```"); idx >= 0 {
		cleaned = strings.TrimPrefix(cleaned[idx:], "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		if end := strings.Index(cleaned, "```"); end >= 0 {
			cleaned = cleaned[:end]
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	var out llmGenerated
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return llmGenerated{}, fmt.Errorf("cli.parseLLMGenerated unmarshal %q: %w", truncate(cleaned, 200), err)
	}
	return out, nil
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// ── Enricher ───────────────────────────────────────────────────────────────

// GrammarEnrichOptions tunes the LLM enrichment run.
type GrammarEnrichOptions struct {
	LLM              GrammarLLMConfig
	Level            string
	Limit            int
	Concurrency      int
	ExamplesPerPoint int
	Force            bool
	DryRun           bool
}

// GrammarEnrichResult reports what the run did.
type GrammarEnrichResult struct {
	Selected  int
	Updated   int
	Unchanged int
	Failed    int
	Examples  int
	Failures  []string
}

// grammarPointRow is a grammar point together with its current examples.
type grammarPointRow struct {
	ID              int64
	Name            string
	Meaning         string
	Level           string
	ConjunctionRule string
	UsageNote       string
	Examples        []grammarExampleRow
}

type grammarExampleRow struct {
	Position int
	Japanese string
	Chinese  string
}

// EnrichGrammarWithLLM fills in 接续 (conjunction_rule), 用法 (usage_note) and
// missing example sentences for grammar points using an LLM.
func EnrichGrammarWithLLM(ctx context.Context, db *sql.DB, opts GrammarEnrichOptions) (*GrammarEnrichResult, error) {
	if opts.LLM.Endpoint == "" {
		return nil, fmt.Errorf("cli.EnrichGrammarWithLLM: llm endpoint is required")
	}
	if opts.LLM.Model == "" {
		opts.LLM.Model = "deepseek-chat"
	}
	if opts.LLM.Timeout <= 0 {
		opts.LLM.Timeout = 60 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.ExamplesPerPoint <= 0 {
		opts.ExamplesPerPoint = 3
	}

	points, err := selectGrammarPoints(ctx, db, opts)
	if err != nil {
		return nil, err
	}

	result := &GrammarEnrichResult{Selected: len(points)}
	if len(points) == 0 {
		return result, nil
	}

	client := &http.Client{Timeout: opts.LLM.Timeout}

	type job struct {
		index int
		point grammarPointRow
	}

	jobs := make(chan job)
	generated := make([]llmGenerated, len(points))
	errs := make([]error, len(points))

	var wg sync.WaitGroup
	for w := 0; w < opts.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				text, err := completeChatWithRetry(ctx, opts.LLM, client, buildGrammarPrompt(j.point, opts.ExamplesPerPoint))
				if err != nil {
					errs[j.index] = err
					continue
				}
				parsed, err := parseLLMGenerated(text)
				if err != nil {
					errs[j.index] = err
					continue
				}
				generated[j.index] = parsed
			}
		}()
	}

	for i, p := range points {
		jobs <- job{index: i, point: p}
	}
	close(jobs)
	wg.Wait()

	for i, p := range points {
		if err := errs[i]; err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s(%s): %v", p.Name, p.Level, err))
			slog.Warn("cli.EnrichGrammarWithLLM: generation failed", "name", p.Name, "err", err)
			continue
		}
		gen := generated[i]
		if gen.ConjunctionRule == "" && gen.UsageNote == "" && len(gen.Examples) == 0 {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s(%s): empty LLM payload", p.Name, p.Level))
			continue
		}

		if opts.DryRun {
			result.Updated++
			result.Examples += len(gen.Examples)
			continue
		}

		changed, added, err := applyGrammarEnrichment(ctx, db, p, gen, opts)
		if err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s(%s): %v", p.Name, p.Level, err))
			continue
		}
		if changed || added > 0 {
			result.Updated++
		} else {
			result.Unchanged++
		}
		result.Examples += added
	}

	sort.Strings(result.Failures)
	return result, nil
}

func completeChatWithRetry(ctx context.Context, cfg GrammarLLMConfig, client *http.Client, prompt string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		text, err := completeChat(ctx, cfg, client, prompt)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return "", lastErr
}

func selectGrammarPoints(ctx context.Context, db *sql.DB, opts GrammarEnrichOptions) ([]grammarPointRow, error) {
	query := `SELECT id, name, meaning, jlpt_level, conjunction_rule, usage_note
		 FROM grammar_points`
	args := []any{}
	if opts.Level != "" {
		query += " WHERE jlpt_level = $1"
		args = append(args, opts.Level)
	}
	query += " ORDER BY jlpt_level, id"
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
		args = append(args, opts.Limit)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cli.selectGrammarPoints query: %w", err)
	}
	defer rows.Close()

	var out []grammarPointRow
	for rows.Next() {
		var p grammarPointRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Meaning, &p.Level, &p.ConjunctionRule, &p.UsageNote); err != nil {
			return nil, fmt.Errorf("cli.selectGrammarPoints scan: %w", err)
		}
		if !opts.Force && p.ConjunctionRule != "" && p.UsageNote != "" {
			continue
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cli.selectGrammarPoints rows: %w", err)
	}

	for i := range out {
		examples, err := loadGrammarExamples(ctx, db, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Examples = examples
		if !opts.Force && len(examples) >= opts.ExamplesPerPoint && out[i].ConjunctionRule != "" && out[i].UsageNote != "" {
			continue
		}
	}

	return out, nil
}

func loadGrammarExamples(ctx context.Context, db *sql.DB, pointID int64) ([]grammarExampleRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT position, japanese, chinese FROM grammar_examples WHERE grammar_point_id = $1 ORDER BY position`,
		pointID)
	if err != nil {
		return nil, fmt.Errorf("cli.loadGrammarExamples query: %w", err)
	}
	defer rows.Close()

	out := make([]grammarExampleRow, 0)
	for rows.Next() {
		var e grammarExampleRow
		if err := rows.Scan(&e.Position, &e.Japanese, &e.Chinese); err != nil {
			return nil, fmt.Errorf("cli.loadGrammarExamples scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cli.loadGrammarExamples rows: %w", err)
	}
	return out, nil
}

func applyGrammarEnrichment(ctx context.Context, db *sql.DB, p grammarPointRow, gen llmGenerated, opts GrammarEnrichOptions) (bool, int, error) {
	rule := p.ConjunctionRule
	note := p.UsageNote
	if opts.Force || rule == "" {
		rule = strings.TrimSpace(gen.ConjunctionRule)
	}
	if opts.Force || note == "" {
		note = strings.TrimSpace(gen.UsageNote)
	}

	changed := false
	if rule != p.ConjunctionRule || note != p.UsageNote {
		if _, err := db.ExecContext(ctx,
			`UPDATE grammar_points SET conjunction_rule = $2, usage_note = $3 WHERE id = $1`,
			p.ID, rule, note); err != nil {
			return false, 0, fmt.Errorf("cli.applyGrammarEnrichment update %q: %w", p.Name, err)
		}
		changed = true
	}

	added := 0
	room := opts.ExamplesPerPoint - len(p.Examples)
	if opts.Force {
		room = opts.ExamplesPerPoint
	}
	if room > 0 {
		nextPos := 0
		for _, e := range p.Examples {
			if e.Position+1 > nextPos {
				nextPos = e.Position + 1
			}
		}
		for i := 0; i < room && i < len(gen.Examples); i++ {
			e := gen.Examples[i]
			if strings.TrimSpace(e.Japanese) == "" || strings.TrimSpace(e.Chinese) == "" {
				continue
			}
			res, err := db.ExecContext(ctx,
				`INSERT INTO grammar_examples (grammar_point_id, position, japanese, chinese)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (grammar_point_id, position) DO NOTHING`,
				p.ID, nextPos+i, e.Japanese, e.Chinese)
			if err != nil {
				return changed, added, fmt.Errorf("cli.applyGrammarEnrichment example %q: %w", p.Name, err)
			}
			affected, err := res.RowsAffected()
			if err != nil {
				return changed, added, fmt.Errorf("cli.applyGrammarEnrichment rows affected: %w", err)
			}
			added += int(affected)
		}
	}

	return changed, added, nil
}

func buildGrammarPrompt(p grammarPointRow, examplesPerPoint int) string {
	var sb strings.Builder
	sb.WriteString("请为下面这个日语语法点补充学习资料，用简体中文回答。\n\n")
	fmt.Fprintf(&sb, "语法点：%s\n", p.Name)
	fmt.Fprintf(&sb, "等级：%s\n", p.Level)
	if p.Meaning != "" {
		fmt.Fprintf(&sb, "释义：%s\n", p.Meaning)
	}
	if len(p.Examples) > 0 {
		sb.WriteString("已有例句：\n")
		for _, e := range p.Examples {
			fmt.Fprintf(&sb, "- %s / %s\n", e.Japanese, e.Chinese)
		}
	}
	fmt.Fprintf(&sb, "\n请输出严格 JSON，字段如下：\n")
	sb.WriteString("{\n")
	sb.WriteString(`  "conjunction_rule": "接续方式，例如「动词た形 + あげく」，不超过 40 字",` + "\n")
	sb.WriteString(`  "usage_note": "用法说明与语感、注意事项，60~120 字",` + "\n")
	fmt.Fprintf(&sb, `  "examples": [{"japanese": "...", "chinese": "..."}]  // 补充 %d 条例句，要与已有例句不重复，难度贴合 %s`+"\n", examplesPerPoint, p.Level)
	sb.WriteString("}\n")
	return sb.String()
}

// ── CLI wiring helper ──────────────────────────────────────────────────────

func grammarLLMConfigFromEnv() GrammarLLMConfig {
	return GrammarLLMConfig{
		Endpoint: os.Getenv("AI_API_ENDPOINT"),
		APIKey:   os.Getenv("AI_API_KEY"),
		Model:    os.Getenv("AI_MODEL"),
	}
}
