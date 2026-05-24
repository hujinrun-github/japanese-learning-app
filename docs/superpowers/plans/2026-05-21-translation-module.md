# Translation Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a translation practice module supporting CN↔JP bidirectional translation, three import pipelines (manual paste, URL scrape, API CLI), and dual-layer scoring (local instant + AI async with grammar explanations).

**Architecture:** New `translation` module following the project's Store→Service→Handler pattern. Three new DB tables (`translation_sources`, `translation_sentences`, `translation_records`). AI review via a new `TranslationReviewer` interface with `ClaudeTranslationReviewer` implementation. Import pipeline: split sentences by punctuation → detect direction by char ratio → insert. Frontend: two new pages (`TranslationListPage`, `TranslationPracticePage`) under `/translation` route.

**Tech Stack:** Go 1.24+ with `net/http`, SQLite via `modernc.org/sqlite`, React 18+ with TypeScript, CSS Modules.

---

### Task 1: Database Migration

**Files:**
- Create: `internal/data/migrations/008_translation.sql`

- [ ] **Step 1: Write migration SQL**

```sql
CREATE TABLE translation_sources (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    title          TEXT    NOT NULL,
    source_type    TEXT    NOT NULL CHECK (source_type IN ('manual', 'url', 'api')),
    source_url     TEXT    NOT NULL DEFAULT '',
    api_endpoint   TEXT    NOT NULL DEFAULT '',
    raw_content    TEXT    NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE translation_sentences (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id             INTEGER NOT NULL REFERENCES translation_sources(id) ON DELETE CASCADE,
    direction             TEXT    NOT NULL CHECK (direction IN ('cn2jp', 'jp2cn')),
    source_text           TEXT    NOT NULL,
    reference_translation TEXT    NOT NULL DEFAULT '',
    position              INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE translation_records (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER  NOT NULL REFERENCES users(id),
    sentence_id       INTEGER  NOT NULL REFERENCES translation_sentences(id),
    user_translation  TEXT     NOT NULL,
    score             INTEGER  NOT NULL DEFAULT 0,
    rule_score        INTEGER  NOT NULL DEFAULT 0,
    ai_feedback_json  TEXT     NOT NULL DEFAULT '',
    practiced_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_translation_sentences_source ON translation_sentences(source_id);
CREATE INDEX idx_translation_records_user ON translation_records(user_id);
CREATE INDEX idx_translation_records_sentence ON translation_records(sentence_id);
```

- [ ] **Step 2: Verify migration runs**

```bash
go run ./backend/cmd/server/ import-words --help > /dev/null 2>&1
# Checks that migration runs embedded — confirm with:
sqlite3 data/app.db ".tables" | grep translation
```

- [ ] **Step 3: Commit**

```bash
git add internal/data/migrations/008_translation.sql
git commit -m "feat(translation): add translation_sources, translation_sentences, translation_records tables"
```

---

### Task 2: Translation Domain Model

**Files:**
- Create: `internal/module/translation/model.go`

- [ ] **Step 1: Write model.go**

```go
package translation

import "time"

// Direction represents the translation direction.
type Direction string

const (
	DirectionCN2JP Direction = "cn2jp"
	DirectionJP2CN Direction = "jp2cn"
)

// TranslationSource represents an imported source material.
type TranslationSource struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	SourceType  string    `json:"source_type"`  // "manual" | "url" | "api"
	SourceURL   string    `json:"source_url"`
	APIEndpoint string    `json:"api_endpoint"`
	RawContent  string    `json:"raw_content"`
	CreatedAt   time.Time `json:"created_at"`
}

// TranslationSentence is a single sentence unit for translation practice.
type TranslationSentence struct {
	ID                   int64  `json:"id"`
	SourceID             int64  `json:"source_id"`
	Direction            string `json:"direction"` // "cn2jp" | "jp2cn"
	SourceText           string `json:"source_text"`
	ReferenceTranslation string `json:"reference_translation"`
	Position             int    `json:"position"`
}

// GrammarExplanation links a grammar point found in the user's translation.
type GrammarExplanation struct {
	GrammarPoint string `json:"grammar_point"`
	Explanation  string `json:"explanation"`
	MatchedDBID  int64  `json:"matched_db_id,omitempty"`
}

// TranslationFeedback is the AI review result.
type TranslationFeedback struct {
	AIScore             int                  `json:"ai_score"`
	GrammarExplanations []GrammarExplanation  `json:"grammar_explanations"`
	IssueDescription    string               `json:"issue_description"`
	CorrectedTranslation string              `json:"corrected_translation"`
	ReferenceTranslation string              `json:"reference_translation"`
}

// TranslationRecord represents a user's translation practice.
type TranslationRecord struct {
	ID               int64                 `json:"id"`
	UserID           int64                 `json:"user_id"`
	SentenceID       int64                 `json:"sentence_id"`
	UserTranslation  string                `json:"user_translation"`
	Score            int                   `json:"score"`
	RuleScore        int                   `json:"rule_score"`
	AIFeedback       *TranslationFeedback  `json:"ai_feedback,omitempty"`
	PracticedAt      time.Time             `json:"practiced_at"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/module/translation/model.go
git commit -m "feat(translation): add translation domain model types"
```

---

### Task 3: Translation Store Interface + Data Store

**Files:**
- Create: `internal/module/translation/service.go`
- Create: `internal/data/translation_store.go`
- Create: `internal/data/translation_store_test.go`

- [ ] **Step 1: Write service.go with interface and stub service**

```go
package translation

import (
	"fmt"
	"log/slog"
	"time"
)

// StoreInterface defines data access methods required by TranslationService.
type StoreInterface interface {
	SaveSource(s TranslationSource) (int64, error)
	SaveSentence(s TranslationSentence) (int64, error)
	ListSources() ([]TranslationSource, error)
	ListSentencesBySource(sourceID int64) ([]TranslationSentence, error)
	GetSentenceByID(id int64) (*TranslationSentence, error)
	GetDailyQueue(userID int64, limit int) ([]TranslationSentence, error)
	SaveRecord(r TranslationRecord) (int64, error)
	GetRecord(id int64) (*TranslationRecord, error)
	ListRecords(userID int64) ([]TranslationRecord, error)
}

// TranslationService handles business logic for translation practice.
type TranslationService struct {
	store    StoreInterface
	reviewer TranslationReviewer
}

// NewTranslationService creates a TranslationService.
func NewTranslationService(store StoreInterface, reviewer TranslationReviewer) *TranslationService {
	return &TranslationService{store: store, reviewer: reviewer}
}
```

- [ ] **Step 2: Write failing data store test**

`internal/data/translation_store_test.go`:

```go
package data

import (
	"testing"

	"japanese-learning-app/internal/module/translation"
)

func TestTranslationStore_SaveAndGetSource(t *testing.T) {
	store := &TranslationStore{db: testDB}

	src := translation.TranslationSource{
		Title:      "Test Source",
		SourceType: "manual",
		RawContent: "今日はいい天気です。明日も晴れるでしょう。",
	}

	id, err := store.SaveSource(src)
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}
	if id == 0 {
		t.Error("SaveSource returned 0 id")
	}

	sources, err := store.ListSources()
	if err != nil {
		t.Fatalf("ListSources error: %v", err)
	}
	if len(sources) < 1 {
		t.Fatal("ListSources: no sources found")
	}

	found := false
	for _, s := range sources {
		if s.ID == id {
			found = true
			if s.Title != "Test Source" {
				t.Errorf("Title = %q, want %q", s.Title, "Test Source")
			}
			break
		}
	}
	if !found {
		t.Errorf("inserted source id=%d not found in ListSources", id)
	}
}

func TestTranslationStore_SaveSentence(t *testing.T) {
	store := &TranslationStore{db: testDB}

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "Sentence Test", SourceType: "manual", RawContent: "こんにちは。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	sent := translation.TranslationSentence{
		SourceID:             srcID,
		Direction:            "jp2cn",
		SourceText:           "こんにちは。",
		ReferenceTranslation: "你好。",
		Position:             0,
	}

	sentID, err := store.SaveSentence(sent)
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}
	if sentID == 0 {
		t.Error("SaveSentence returned 0 id")
	}

	sentences, err := store.ListSentencesBySource(srcID)
	if err != nil {
		t.Fatalf("ListSentencesBySource error: %v", err)
	}
	if len(sentences) != 1 {
		t.Fatalf("ListSentencesBySource count = %d, want 1", len(sentences))
	}
	if sentences[0].SourceText != "こんにちは。" {
		t.Errorf("SourceText = %q, want %q", sentences[0].SourceText, "こんにちは。")
	}
}

func TestTranslationStore_SaveAndListRecords(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9300, "tl_record@example.com")

	srcID, _ := store.SaveSource(translation.TranslationSource{
		Title: "Record Test", SourceType: "manual", RawContent: "おはよう。",
	})
	sentID, _ := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "おはよう。",
	})

	rec := translation.TranslationRecord{
		UserID:          9300,
		SentenceID:      sentID,
		UserTranslation: "早上好。",
		Score:           90,
		RuleScore:       85,
	}

	recID, err := store.SaveRecord(rec)
	if err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}
	if recID == 0 {
		t.Error("SaveRecord returned 0 id")
	}

	records, err := store.ListRecords(9300)
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRecords count = %d, want 1", len(records))
	}
}

func TestTranslationStore_GetDailyQueue(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9301, "tl_daily@example.com")

	srcID, _ := store.SaveSource(translation.TranslationSource{
		Title: "Daily Queue Test", SourceType: "manual", RawContent: "テスト。試験。課題。確認。",
	})

	texts := []string{"テスト。", "試験。", "課題。", "確認。"}
	for i, text := range texts {
		store.SaveSentence(translation.TranslationSentence{
			SourceID: srcID, Direction: "jp2cn", SourceText: text, Position: i,
		})
	}

	queue, err := store.GetDailyQueue(9301, 2)
	if err != nil {
		t.Fatalf("GetDailyQueue error: %v", err)
	}
	if len(queue) != 2 {
		t.Fatalf("GetDailyQueue count = %d, want 2", len(queue))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/data/ -run TestTranslationStore -v -count=1
```
Expected: build failure — `TranslationStore` not defined.

- [ ] **Step 4: Write TranslationStore implementation**

`internal/data/translation_store.go`:

```go
package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/translation"
)

// TranslationStore implements translation.StoreInterface.
type TranslationStore struct {
	db *sql.DB
}

// NewTranslationStore creates a TranslationStore.
func NewTranslationStore(db *sql.DB) *TranslationStore {
	return &TranslationStore{db: db}
}

func (s *TranslationStore) SaveSource(src translation.TranslationSource) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content)
		 VALUES (?, ?, ?, ?, ?)`,
		src.Title, src.SourceType, src.SourceURL, src.APIEndpoint, src.RawContent,
	)
	if err != nil {
		slog.Error("TranslationStore.SaveSource failed", "err", err)
		return 0, fmt.Errorf("data.TranslationStore.SaveSource: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.TranslationStore.SaveSource LastInsertId: %w", err)
	}
	return id, nil
}

func (s *TranslationStore) SaveSentence(sent translation.TranslationSentence) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO translation_sentences (source_id, direction, source_text, reference_translation, position)
		 VALUES (?, ?, ?, ?, ?)`,
		sent.SourceID, sent.Direction, sent.SourceText, sent.ReferenceTranslation, sent.Position,
	)
	if err != nil {
		slog.Error("TranslationStore.SaveSentence failed", "err", err)
		return 0, fmt.Errorf("data.TranslationStore.SaveSentence: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.TranslationStore.SaveSentence LastInsertId: %w", err)
	}
	return id, nil
}

func (s *TranslationStore) ListSources() ([]translation.TranslationSource, error) {
	rows, err := s.db.Query(
		`SELECT id, title, source_type, source_url, api_endpoint, raw_content, created_at
		 FROM translation_sources ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.ListSources: %w", err)
	}
	defer rows.Close()

	var sources []translation.TranslationSource
	for rows.Next() {
		var src translation.TranslationSource
		var createdAt string
		if err := rows.Scan(&src.ID, &src.Title, &src.SourceType, &src.SourceURL, &src.APIEndpoint, &src.RawContent, &createdAt); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListSources scan: %w", err)
		}
		src.CreatedAt, err = parseSQLiteTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListSources parse created_at: %w", err)
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

func (s *TranslationStore) ListSentencesBySource(sourceID int64) ([]translation.TranslationSentence, error) {
	rows, err := s.db.Query(
		`SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM translation_sentences WHERE source_id = ? ORDER BY position`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.ListSentencesBySource: %w", err)
	}
	defer rows.Close()

	var sentences []translation.TranslationSentence
	for rows.Next() {
		var sent translation.TranslationSentence
		if err := rows.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListSentencesBySource scan: %w", err)
		}
		sentences = append(sentences, sent)
	}
	return sentences, rows.Err()
}

func (s *TranslationStore) GetSentenceByID(id int64) (*translation.TranslationSentence, error) {
	row := s.db.QueryRow(
		`SELECT id, source_id, direction, source_text, reference_translation, position
		 FROM translation_sentences WHERE id = ?`, id,
	)
	var sent translation.TranslationSentence
	err := row.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetSentenceByID: %w", err)
	}
	return &sent, nil
}

func (s *TranslationStore) GetDailyQueue(userID int64, limit int) ([]translation.TranslationSentence, error) {
	rows, err := s.db.Query(
		`SELECT ts.id, ts.source_id, ts.direction, ts.source_text, ts.reference_translation, ts.position
		 FROM translation_sentences ts
		 WHERE ts.id NOT IN (
		 	SELECT tr.sentence_id FROM translation_records tr
		 	WHERE tr.user_id = ? AND tr.score >= 80
		 )
		 ORDER BY RANDOM()
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetDailyQueue: %w", err)
	}
	defer rows.Close()

	var sentences []translation.TranslationSentence
	for rows.Next() {
		var sent translation.TranslationSentence
		if err := rows.Scan(&sent.ID, &sent.SourceID, &sent.Direction, &sent.SourceText, &sent.ReferenceTranslation, &sent.Position); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.GetDailyQueue scan: %w", err)
		}
		sentences = append(sentences, sent)
	}
	return sentences, rows.Err()
}

func (s *TranslationStore) SaveRecord(r translation.TranslationRecord) (int64, error) {
	aiJSON := ""
	if r.AIFeedback != nil {
		raw, err := json.Marshal(r.AIFeedback)
		if err != nil {
			return 0, fmt.Errorf("data.TranslationStore.SaveRecord marshal ai_feedback: %w", err)
		}
		aiJSON = string(raw)
	}

	result, err := s.db.Exec(
		`INSERT OR REPLACE INTO translation_records
		 (user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json, practiced_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		r.UserID, r.SentenceID, r.UserTranslation, r.Score, r.RuleScore, aiJSON,
	)
	if err != nil {
		slog.Error("TranslationStore.SaveRecord failed", "err", err)
		return 0, fmt.Errorf("data.TranslationStore.SaveRecord: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.TranslationStore.SaveRecord LastInsertId: %w", err)
	}
	return id, nil
}

func (s *TranslationStore) GetRecord(id int64) (*translation.TranslationRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json, practiced_at
		 FROM translation_records WHERE id = ?`, id,
	)
	var r translation.TranslationRecord
	var aiJSON string
	var practicedAt string
	err := row.Scan(&r.ID, &r.UserID, &r.SentenceID, &r.UserTranslation, &r.Score, &r.RuleScore, &aiJSON, &practicedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("data.TranslationStore.GetRecord %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetRecord: %w", err)
	}
	r.PracticedAt, err = parseSQLiteTime(practicedAt)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.GetRecord parse practiced_at: %w", err)
	}
	if aiJSON != "" {
		var fb translation.TranslationFeedback
		if err := json.Unmarshal([]byte(aiJSON), &fb); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.GetRecord unmarshal ai_feedback: %w", err)
		}
		r.AIFeedback = &fb
	}
	return &r, nil
}

func (s *TranslationStore) ListRecords(userID int64) ([]translation.TranslationRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, sentence_id, user_translation, score, rule_score, ai_feedback_json, practiced_at
		 FROM translation_records WHERE user_id = ? ORDER BY practiced_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("data.TranslationStore.ListRecords: %w", err)
	}
	defer rows.Close()

	var records []translation.TranslationRecord
	for rows.Next() {
		var r translation.TranslationRecord
		var aiJSON string
		var practicedAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.SentenceID, &r.UserTranslation, &r.Score, &r.RuleScore, &aiJSON, &practicedAt); err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListRecords scan: %w", err)
		}
		r.PracticedAt, err = parseSQLiteTime(practicedAt)
		if err != nil {
			return nil, fmt.Errorf("data.TranslationStore.ListRecords parse practiced_at: %w", err)
		}
		if aiJSON != "" {
			var fb translation.TranslationFeedback
			if err := json.Unmarshal([]byte(aiJSON), &fb); err != nil {
				return nil, fmt.Errorf("data.TranslationStore.ListRecords unmarshal ai_feedback: %w", err)
			}
			r.AIFeedback = &fb
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/data/ -run TestTranslationStore -v -count=1
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/module/translation/service.go internal/data/translation_store.go internal/data/translation_store_test.go
git commit -m "feat(translation): add TranslationStore and TranslationService with StoreInterface"
```

---

### Task 4: AI Reviewer (Stub + Claude)

**Files:**
- Create: `internal/module/translation/ai_client.go`
- Create: `internal/module/translation/ai_client_test.go`

- [ ] **Step 1: Write ai_client.go**

```go
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
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
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
```

- [ ] **Step 2: Write ai_client_test.go**

```go
package translation_test

import (
	"testing"

	"japanese-learning-app/internal/module/translation"
)

func TestStubReviewer_Review(t *testing.T) {
	stub := &translation.StubReviewer{
		Feedback: translation.TranslationFeedback{
			AIScore: 90,
			GrammarExplanations: []translation.GrammarExplanation{
				{GrammarPoint: "〜ている", Explanation: "ongoing state"},
			},
			CorrectedTranslation: "我在吃饭。",
			ReferenceTranslation: "我在吃饭。",
		},
	}

	fb, err := stub.Review("jp2cn", "ご飯を食べています。", "我在吃饭。", "正在吃饭。")
	if err != nil {
		t.Fatalf("Review error: %v", err)
	}
	if fb.AIScore != 90 {
		t.Errorf("AIScore = %d, want 90", fb.AIScore)
	}
	if len(fb.GrammarExplanations) != 1 {
		t.Errorf("GrammarExplanations len = %d, want 1", len(fb.GrammarExplanations))
	}
}

func TestStubReviewer_Error(t *testing.T) {
	stub := &translation.StubReviewer{
		Err: fmt.Errorf("simulated error"),
	}

	_, err := stub.Review("cn2jp", "你好", "こんにちは", "")
	if err == nil {
		t.Error("expected error, got nil")
	}
}
```

Add import `"fmt"` to the test file.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/module/translation/ -run TestStub -v -count=1
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/module/translation/ai_client.go internal/module/translation/ai_client_test.go
git commit -m "feat(translation): add TranslationReviewer with Stub and Claude implementations"
```

---

### Task 5: Translation Importer (Sentence Split + Direction Detection)

**Files:**
- Create: `internal/module/translation/importer.go`
- Create: `internal/module/translation/importer_test.go`

- [ ] **Step 1: Write failing importer tests**

`internal/module/translation/importer_test.go`:

```go
package translation_test

import (
	"reflect"
	"testing"

	"japanese-learning-app/internal/module/translation"
)

func TestSplitSentences_Japanese(t *testing.T) {
	text := "今日はいい天気です。明日も晴れるでしょう。"
	result := translation.SplitSentences(text)

	want := []string{"今日はいい天気です。", "明日も晴れるでしょう。"}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("SplitSentences = %q, want %q", result, want)
	}
}

func TestSplitSentences_Chinese(t *testing.T) {
	text := "今天天气很好。明天也会放晴吧。"
	result := translation.SplitSentences(text)

	want := []string{"今天天气很好。", "明天也会放晴吧。"}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("SplitSentences = %q, want %q", result, want)
	}
}

func TestSplitSentences_MinLength(t *testing.T) {
	text := "はい。今日はいい天気です。ええ。"
	result := translation.SplitSentences(text)

	// "はい。" is 3 chars (with punctuation), "ええ。" is 3 chars — both pass min 3
	if len(result) != 3 {
		t.Errorf("SplitSentences count = %d, want 3", len(result))
	}
}

func TestSplitSentences_MaxLength(t *testing.T) {
	// Build a single long string with no punctuation — should be broken at max 200
	long := ""
	for i := 0; i < 250; i++ {
		long += "あ"
	}
	result := translation.SplitSentences(long)
	if len(result) < 2 {
		t.Errorf("long text should be split into at least 2 chunks, got %d", len(result))
	}
	for _, r := range result {
		if len([]rune(r)) > 200 {
			t.Errorf("chunk length %d exceeds max 200", len([]rune(r)))
		}
	}
}

func TestDetectDirection_JP(t *testing.T) {
	direction := translation.DetectDirection("これは日本語のテキストです。")
	if direction != "jp2cn" {
		t.Errorf("DetectDirection = %q, want jp2cn", direction)
	}
}

func TestDetectDirection_CN(t *testing.T) {
	direction := translation.DetectDirection("这是中文文本。")
	if direction != "cn2jp" {
		t.Errorf("DetectDirection = %q, want cn2jp", direction)
	}
}

func TestDetectDirection_Mixed(t *testing.T) {
	// More Japanese characters — should be jp2cn
	direction := translation.DetectDirection("日本語のtextです。")
	if direction != "jp2cn" {
		t.Errorf("DetectDirection = %q, want jp2cn", direction)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/module/translation/ -run TestSplit -v -count=1
```
Expected: FAIL — `SplitSentences` not defined.

- [ ] **Step 3: Write importer.go**

```go
package translation

import (
	"strings"
	"unicode"
)

// SplitSentences splits text into sentences by Japanese/Chinese punctuation marks.
// Min sentence length: 3 runes. Max: 200 runes (longer segments are further split).
func SplitSentences(text string) []string {
	// Split on common JP/CN punctuation, keeping the punctuation attached.
	splitter := func(r rune) bool {
		return false // handled manually
	}
	_ = splitter

	type boundary struct {
		idx int // byte position after the punctuation rune
		ch  rune
	}
	var boundaries []boundary
	runes := []rune(text)
	for i, r := range runes {
		switch r {
		case '。', '！', '？', '…', '.', '!', '?':
			// byte position after this rune
			bytePos := 0
			for j := 0; j <= i; j++ {
				bytePos += len(string(runes[j]))
			}
			boundaries = append(boundaries, boundary{idx: bytePos, ch: r})
		}
	}

	var parts []string
	prev := 0
	for _, b := range boundaries {
		part := strings.TrimSpace(text[prev:b.idx])
		if len([]rune(part)) >= 3 {
			parts = append(parts, part)
		}
		prev = b.idx
	}

	// Remaining text after last punctuation
	remaining := strings.TrimSpace(text[prev:])
	if len([]rune(remaining)) >= 3 {
		parts = append(parts, remaining)
	}

	// If no punctuation boundaries found, treat whole text as one chunk
	if len(boundaries) == 0 && len(runes) >= 3 {
		parts = append(parts, strings.TrimSpace(text))
	}

	// Split long chunks (>200 runes) into smaller pieces
	var result []string
	for _, p := range parts {
		if len([]rune(p)) > 200 {
			result = append(result, splitLongChunk(p)...)
		} else {
			result = append(result, p)
		}
	}
	return result
}

func splitLongChunk(s string) []string {
	var chunks []string
	runes := []rune(s)
	for i := 0; i < len(runes); i += 200 {
		end := i + 200
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// DetectDirection detects translation direction by counting CJK characters
// vs hiragana/katakana in the text. More kana = jp2cn, more CJK-only = cn2jp.
func DetectDirection(text string) string {
	var kana int
	var cjk int
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			kana++
		} else if unicode.Is(unicode.Han, r) {
			cjk++
		}
	}
	// If significant kana presence, it's Japanese → Chinese
	if kana > 0 && kana >= cjk/3 {
		return "jp2cn"
	}
	// If CJK but little kana, likely Chinese → Japanese
	if cjk > 0 && kana < cjk/3 {
		return "cn2jp"
	}
	// Default: more kana → jp2cn
	if kana >= cjk {
		return "jp2cn"
	}
	return "cn2jp"
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/module/translation/ -run "TestSplit|TestDetect" -v -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/module/translation/importer.go internal/module/translation/importer_test.go
git commit -m "feat(translation): add sentence splitting and direction detection"
```

---

### Task 6: TranslationService Business Logic

**Files:**
- Modify: `internal/module/translation/service.go` — add full method implementations
- Create: `internal/module/translation/service_test.go`

- [ ] **Step 1: Write failing service tests**

`internal/module/translation/service_test.go`:

```go
package translation_test

import (
	"testing"

	"japanese-learning-app/internal/module/translation"
)

type fakeStore struct {
	sources   map[int64]translation.TranslationSource
	sentences map[int64]translation.TranslationSentence
	records   map[int64]translation.TranslationRecord
	nextID    int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sources:   make(map[int64]translation.TranslationSource),
		sentences: make(map[int64]translation.TranslationSentence),
		records:   make(map[int64]translation.TranslationRecord),
		nextID:    1,
	}
}

func (f *fakeStore) SaveSource(s translation.TranslationSource) (int64, error) {
	s.ID = f.nextID
	f.sources[f.nextID] = s
	f.nextID++
	return s.ID, nil
}

func (f *fakeStore) SaveSentence(s translation.TranslationSentence) (int64, error) {
	s.ID = f.nextID
	f.sentences[f.nextID] = s
	f.nextID++
	return s.ID, nil
}

func (f *fakeStore) ListSources() ([]translation.TranslationSource, error) {
	var out []translation.TranslationSource
	for _, s := range f.sources {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeStore) ListSentencesBySource(sourceID int64) ([]translation.TranslationSentence, error) {
	var out []translation.TranslationSentence
	for _, s := range f.sentences {
		if s.SourceID == sourceID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) GetSentenceByID(id int64) (*translation.TranslationSentence, error) {
	s, ok := f.sentences[id]
	if !ok {
		return nil, nil
	}
	return &s, nil
}

func (f *fakeStore) GetDailyQueue(userID int64, limit int) ([]translation.TranslationSentence, error) {
	var done map[int64]bool
	// Find sentences this user has already scored >= 80
	for _, r := range f.records {
		if r.UserID == userID && r.Score >= 80 {
			if done == nil {
				done = make(map[int64]bool)
			}
			done[r.SentenceID] = true
		}
	}
	var out []translation.TranslationSentence
	for _, s := range f.sentences {
		if !done[s.ID] && len(out) < limit {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) SaveRecord(r translation.TranslationRecord) (int64, error) {
	r.ID = f.nextID
	f.records[f.nextID] = r
	f.nextID++
	return r.ID, nil
}

func (f *fakeStore) GetRecord(id int64) (*translation.TranslationRecord, error) {
	r, ok := f.records[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (f *fakeStore) ListRecords(userID int64) ([]translation.TranslationRecord, error) {
	var out []translation.TranslationRecord
	for _, r := range f.records {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestService_ImportSource(t *testing.T) {
	store := newFakeStore()
	svc := translation.NewTranslationService(store, &translation.StubReviewer{})

	src := translation.TranslationSource{
		Title:      "Test Import",
		SourceType: "manual",
		RawContent: "こんにちは。お元気ですか。",
	}

	result, err := svc.ImportSource(src)
	if err != nil {
		t.Fatalf("ImportSource error: %v", err)
	}
	if result == nil {
		t.Fatal("ImportSource returned nil source")
	}

	sentences, err := svc.GetFreeSentences(0, "", result.ID)
	if err != nil {
		t.Fatalf("GetFreeSentences error: %v", err)
	}
	if len(sentences) != 2 {
		t.Errorf("sentences count = %d, want 2", len(sentences))
	}
}

func TestService_SubmitTranslation(t *testing.T) {
	store := newFakeStore()
	svc := translation.NewTranslationService(store, &translation.StubReviewer{
		Feedback: translation.TranslationFeedback{
			AIScore: 88,
			GrammarExplanations: []translation.GrammarExplanation{
				{GrammarPoint: "です", Explanation: "polite copula"},
			},
			IssueDescription:    "",
			CorrectedTranslation: "你好。",
			ReferenceTranslation: "你好。",
		},
	})

	srcID, _ := store.SaveSource(translation.TranslationSource{
		Title: "Submit Test", SourceType: "manual", RawContent: "こんにちは。",
	})
	sentID, _ := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "こんにちは。",
	})

	rec, err := svc.SubmitTranslation(1, sentID, "你好。")
	if err != nil {
		t.Fatalf("SubmitTranslation error: %v", err)
	}
	if rec.RuleScore == 0 {
		t.Error("RuleScore should not be 0")
	}
	if rec.AIFeedback == nil {
		t.Error("AIFeedback should not be nil (stub reviewer)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/module/translation/ -run TestService -v -count=1
```
Expected: FAIL — methods not defined.

- [ ] **Step 3: Complete service.go with full method implementations**

Append to `internal/module/translation/service.go`:

```go
// ImportSource saves a source material, splits it into sentences, detects direction,
// and saves all sentences. Returns the saved source with ID populated.
func (s *TranslationService) ImportSource(src TranslationSource) (*TranslationSource, error) {
	slog.Debug("TranslationService.ImportSource called", "title", src.Title)

	id, err := s.store.SaveSource(src)
	if err != nil {
		return nil, fmt.Errorf("translation.TranslationService.ImportSource SaveSource: %w", err)
	}

	sentences := SplitSentences(src.RawContent)
	direction := DetectDirection(src.RawContent)

	for i, text := range sentences {
		sent := TranslationSentence{
			SourceID:  id,
			Direction: string(direction),
			SourceText: text,
			Position:  i,
		}
		if _, err := s.store.SaveSentence(sent); err != nil {
			slog.Error("TranslationService.ImportSource SaveSentence failed", "err", err, "text", text)
			return nil, fmt.Errorf("translation.TranslationService.ImportSource SaveSentence: %w", err)
		}
	}

	src.ID = id
	src.Direction = direction
	slog.Debug("TranslationService.ImportSource done", "source_id", id, "sentences", len(sentences))
	return &src, nil
}

// GetDailyQueue returns today's practice sentences for the user.
func (s *TranslationService) GetDailyQueue(userID int64, count int) ([]TranslationSentence, error) {
	slog.Debug("TranslationService.GetDailyQueue called", "user_id", userID)
	return s.store.GetDailyQueue(userID, count)
}

// GetFreeSentences returns sentences filtered by direction and/or source.
func (s *TranslationService) GetFreeSentences(userID int64, direction string, sourceID int64) ([]TranslationSentence, error) {
	if sourceID > 0 {
		return s.store.ListSentencesBySource(sourceID)
	}
	// Return all sentences — direction filtering done in-memory for simplicity
	// For MVP, list from specific source is the primary use case
	return nil, nil
}

// SubmitTranslation scores a user translation and triggers AI review.
func (s *TranslationService) SubmitTranslation(userID int64, sentenceID int64, userTranslation string) (TranslationRecord, error) {
	slog.Debug("TranslationService.SubmitTranslation called", "user_id", userID, "sentence_id", sentenceID)

	sent, err := s.store.GetSentenceByID(sentenceID)
	if err != nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation GetSentenceByID: %w", err)
	}
	if sent == nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation: sentence %d not found", sentenceID)
	}

	ruleScore := computeRuleScore(string(Direction(sent.Direction)), sent.SourceText, userTranslation)

	rec := TranslationRecord{
		UserID:          userID,
		SentenceID:      sentenceID,
		UserTranslation: userTranslation,
		RuleScore:       ruleScore,
		Score:           ruleScore, // initial score = rule score, updated after AI review
	}

	recID, err := s.store.SaveRecord(rec)
	if err != nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation SaveRecord: %w", err)
	}

	// Trigger AI review if reviewer is available
	if s.reviewer != nil {
		fb, aiErr := s.reviewer.Review(sent.Direction, sent.SourceText, userTranslation, sent.ReferenceTranslation)
		if aiErr != nil {
			slog.Error("TranslationService.SubmitTranslation: AI review failed", "err", aiErr)
		} else {
			rec.AIFeedback = &fb
			rec.Score = fb.AIScore
			// Re-save with AI feedback
			s.store.SaveRecord(rec)
		}
	}

	rec.ID = recID
	slog.Debug("TranslationService.SubmitTranslation done", "record_id", recID, "rule_score", ruleScore)
	return rec, nil
}

// GetResult returns a single record by ID.
func (s *TranslationService) GetResult(recordID int64) (*TranslationRecord, error) {
	return s.store.GetRecord(recordID)
}

// ListRecords returns all translation records for a user.
func (s *TranslationService) ListRecords(userID int64) ([]TranslationRecord, error) {
	return s.store.ListRecords(userID)
}

// computeRuleScore is the local rule-based scoring algorithm.
func computeRuleScore(direction, source, translation string) int {
	score := 50

	runes := []rune(translation)
	srcRunes := []rune(source)

	// Length ratio check
	if len(srcRunes) > 0 {
		ratio := float64(len(runes)) / float64(len(srcRunes))
		if ratio >= 0.5 && ratio <= 2.0 {
			score += 25
		}
	}

	// Grammar marker check for JP→CN
	if direction == "jp2cn" {
		markers := []string{"は", "が", "を", "に", "で", "へ", "と", "も", "か", "から", "まで", "より", "ので", "のに"}
		present := 0
		for _, m := range markers {
			if contains(source, m) {
				present++
			}
		}
		if present > 0 {
			score += min(present*2, 25)
		}
	}

	// Keyword check for CN→JP
	if direction == "cn2jp" {
		// Reward reasonable translation length
		if len(runes) >= 2 {
			score += 25
		}
	}

	if score > 100 {
		score = 100
	}
	return score
}

func contains(s, substr string) bool {
	runes := []rune(s)
	for i := 0; i <= len(runes)-len([]rune(substr)); i++ {
		match := true
		for j, r := range substr {
			if runes[i+j] != r {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
```

Also add `Direction` field to `TranslationSource` in model.go:

```go
Direction string `json:"direction,omitempty"`
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/module/translation/ -run TestService -v -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/module/translation/service.go internal/module/translation/model.go internal/module/translation/service_test.go
git commit -m "feat(translation): add TranslationService with import, scoring, and AI review"
```

---

### Task 7: Translation HTTP Handler

**Files:**
- Create: `internal/module/translation/handler.go`

- [ ] **Step 1: Write handler.go**

```go
package translation

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// TranslationHandler handles HTTP requests for the translation module.
type TranslationHandler struct {
	svc *TranslationService
}

// NewTranslationHandler creates a TranslationHandler.
func NewTranslationHandler(svc *TranslationService) *TranslationHandler {
	return &TranslationHandler{svc: svc}
}

// RegisterRoutes registers translation routes.
func (h *TranslationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/translation/queue", h.handleGetDailyQueue)
	mux.HandleFunc("GET /api/v1/translation/sentences", h.handleGetSentences)
	mux.HandleFunc("POST /api/v1/translation/submit", h.handleSubmit)
	mux.HandleFunc("GET /api/v1/translation/records/{id}", h.handleGetRecord)
	mux.HandleFunc("GET /api/v1/translation/records", h.handleListRecords)
	mux.HandleFunc("POST /api/v1/translation/sources/import", h.handleImportURL)
	mux.HandleFunc("POST /api/v1/translation/sources", h.handleImportSource)
	mux.HandleFunc("GET /api/v1/translation/sources", h.handleListSources)
}

// handleGetDailyQueue responds with daily practice sentences.
func (h *TranslationHandler) handleGetDailyQueue(w http.ResponseWriter, r *http.Request) {
	userID := user.UserIDFromContext(r.Context())
	count := 5

	countStr := r.URL.Query().Get("count")
	if countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 && n <= 20 {
			count = n
		}
	}

	sentences, err := h.svc.GetDailyQueue(userID, count)
	if err != nil {
		slog.Error("TranslationHandler.handleGetDailyQueue failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sentences, RequestID: r.Header.Get("X-Request-ID")})
}

// handleGetSentences responds with sentences filtered by source or direction.
func (h *TranslationHandler) handleGetSentences(w http.ResponseWriter, r *http.Request) {
	userID := user.UserIDFromContext(r.Context())
	_ = userID

	direction := r.URL.Query().Get("direction")
	sourceIDStr := r.URL.Query().Get("source_id")
	var sourceID int64
	if sourceIDStr != "" {
		sourceID, _ = strconv.ParseInt(sourceIDStr, 10, 64)
	}

	sentences, err := h.svc.GetFreeSentences(userID, direction, sourceID)
	if err != nil {
		slog.Error("TranslationHandler.handleGetSentences failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sentences, RequestID: r.Header.Get("X-Request-ID")})
}

type submitRequest struct {
	SentenceID       int64  `json:"sentence_id"`
	UserTranslation  string `json:"user_translation"`
}

// handleSubmit processes a translation submission.
func (h *TranslationHandler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	userID := user.UserIDFromContext(r.Context())

	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", r.Header.Get("X-Request-ID"))
		return
	}

	if req.SentenceID == 0 || req.UserTranslation == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "sentence_id and user_translation are required", r.Header.Get("X-Request-ID"))
		return
	}

	rec, err := h.svc.SubmitTranslation(userID, req.SentenceID, req.UserTranslation)
	if err != nil {
		slog.Error("TranslationHandler.handleSubmit failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec, RequestID: r.Header.Get("X-Request-ID")})
}

// handleGetRecord returns a single translation record.
func (h *TranslationHandler) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid record id", r.Header.Get("X-Request-ID"))
		return
	}

	rec, err := h.svc.GetResult(id)
	if err != nil {
		slog.Error("TranslationHandler.handleGetRecord failed", "err", err, "id", id)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "record not found", r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec, RequestID: r.Header.Get("X-Request-ID")})
}

// handleListRecords returns all translation records for the user.
func (h *TranslationHandler) handleListRecords(w http.ResponseWriter, r *http.Request) {
	userID := user.UserIDFromContext(r.Context())

	records, err := h.svc.ListRecords(userID)
	if err != nil {
		slog.Error("TranslationHandler.handleListRecords failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: records, RequestID: r.Header.Get("X-Request-ID")})
}

type importSourceRequest struct {
	Title      string `json:"title"`
	SourceType string `json:"source_type"`
	Content    string `json:"content"`
	SourceURL  string `json:"source_url"`
}

// handleImportSource imports from manual paste input.
func (h *TranslationHandler) handleImportSource(w http.ResponseWriter, r *http.Request) {
	var req importSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", r.Header.Get("X-Request-ID"))
		return
	}

	if req.Content == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "content is required", r.Header.Get("X-Request-ID"))
		return
	}
	if req.SourceType == "" {
		req.SourceType = "manual"
	}

	src := TranslationSource{
		Title:      req.Title,
		SourceType: req.SourceType,
		SourceURL:  req.SourceURL,
		RawContent: req.Content,
	}

	result, err := h.svc.ImportSource(src)
	if err != nil {
		slog.Error("TranslationHandler.handleImportSource failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: result, RequestID: r.Header.Get("X-Request-ID")})
}

type importURLRequest struct {
	URL string `json:"url"`
}

// handleImportURL imports from a URL by scraping its text content.
func (h *TranslationHandler) handleImportURL(w http.ResponseWriter, r *http.Request) {
	var req importURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", r.Header.Get("X-Request-ID"))
		return
	}

	if req.URL == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "url is required", r.Header.Get("X-Request-ID"))
		return
	}

	// Fetch URL content
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(req.URL)
	if err != nil {
		slog.Error("TranslationHandler.handleImportURL fetch failed", "url", req.URL, "err", err)
		httputil.WriteError(w, http.StatusBadRequest, "ERR_IMPORT_FAILED", "failed to fetch URL: "+err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_IMPORT_FAILED", "URL returned status "+resp.Status, r.Header.Get("X-Request-ID"))
		return
	}

	html, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB max
	text := extractTextFromHTML(string(html))

	if text == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_IMPORT_FAILED", "no text content found at URL", r.Header.Get("X-Request-ID"))
		return
	}

	src := TranslationSource{
		Title:      "Imported from URL",
		SourceType: "url",
		SourceURL:  req.URL,
		RawContent: text,
	}

	result, err := h.svc.ImportSource(src)
	if err != nil {
		slog.Error("TranslationHandler.handleImportURL import failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: result, RequestID: r.Header.Get("X-Request-ID")})
}

// handleListSources returns all translation sources.
func (h *TranslationHandler) handleListSources(w http.ResponseWriter, r *http.Request) {
	// Delegate to store via a simple approach
	// Since the handler does not have direct access to the store,
	// we add a ListSources method to the service if needed.
	// For MVP, use the same pattern as sentences.
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: []TranslationSource{}, RequestID: r.Header.Get("X-Request-ID")})
}
```

Add the missing imports: `"io"`, `"time"`.

And add `extractTextFromHTML` helper:

```go
import "strings"

// extractTextFromHTML strips HTML tags and extracts text from simple HTML.
func extractTextFromHTML(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}
```

Also add `ListSources()` method to `TranslationService` and `StoreInterface`:

In `service.go`, add to `StoreInterface`:
```go
ListSources() ([]TranslationSource, error)
```

In `TranslationService`:
```go
// ListSources returns all translation sources.
func (s *TranslationService) ListSources() ([]TranslationSource, error) {
	return s.store.ListSources()
}
```

Update `handleListSources` to use `h.svc.ListSources()`.

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/module/translation/handler.go internal/module/translation/service.go internal/module/translation/model.go
git commit -m "feat(translation): add TranslationHandler with all API endpoints"
```

---

### Task 8: Wire Translation Module into Server

**Files:**
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Add translation wiring to main.go**

In `backend/cmd/server/main.go`, add import:
```go
"japanese-learning-app/internal/module/translation"
```

After the existing store initialization block (around line 77), add:
```go
translationStore := data.NewTranslationStore(db)
```

After `aiReviewer` block (around line 86), add:
```go
var translationReviewer translation.TranslationReviewer
if aiAPIKey != "" {
    translationReviewer = translation.NewClaudeTranslationReviewer(aiAPIKey)
} else {
    slog.Warn("AI_API_KEY not set — using StubReviewer for translation (no real AI feedback)")
    translationReviewer = &translation.StubReviewer{}
}
```

After services block (around line 113), add:
```go
translationSvc := translation.NewTranslationService(translationStore, translationReviewer)
```

After handlers block (around line 148), add:
```go
translationH := translation.NewTranslationHandler(translationSvc)
```

After other `RegisterRoutes` calls (around line 166), add:
```go
translationH.RegisterRoutes(protectedMux)
```

After other `mux.Handle` auth middleware lines (around line 178), add:
```go
mux.Handle("/api/v1/translation/", user.AuthMiddleware(jwtSecret, protectedMux))
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./backend/cmd/server/
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/cmd/server/main.go
git commit -m "feat(translation): wire translation module into server"
```

---

### Task 9: CLI Import Command for Translation API Sources

**Files:**
- Create: `internal/cli/import_translation_api.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Write import_translation_api.go**

```go
package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"japanese-learning-app/internal/module/translation"
)

type translationAPIConfig struct {
	Title      string `json:"title"`
	Endpoint   string `json:"endpoint"`
	Method     string `json:"method"`
	JSONPath   string `json:"json_path"`
	Direction  string `json:"direction"`
}

func ImportTranslationFromAPIConfig(db *sql.DB, configPath string) (int, error) {
	slog.Debug("ImportTranslationFromAPIConfig called", "config", configPath)

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig ReadFile: %w", err)
	}

	var config translationAPIConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig Unmarshal: %w", err)
	}

	if config.Method == "" {
		config.Method = "GET"
	}

	// Fetch from API
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(config.Method, config.Endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig new request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig read body: %w", err)
	}

	text := extractTextFromJSONPath(string(body), config.JSONPath)
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig: no text extracted from API response")
	}

	// Use the importer to split and insert
	sentences := translation.SplitSentences(text)
	direction := translation.DetectDirection(text)
	if config.Direction != "" {
		direction = translation.Direction(config.Direction)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig Begin: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content)
		 VALUES (?, 'api', ?, ?, ?)`,
		config.Title, config.Endpoint, config.Endpoint, text,
	)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig insert source: %w", err)
	}
	sourceID, _ := result.LastInsertId()

	inserted := 0
	for i, s := range sentences {
		_, err := tx.Exec(
			`INSERT INTO translation_sentences (source_id, direction, source_text, position)
			 VALUES (?, ?, ?, ?)`,
			sourceID, string(direction), s, i,
		)
		if err != nil {
			return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig insert sentence: %w", err)
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cli.ImportTranslationFromAPIConfig Commit: %w", err)
	}

	slog.Debug("ImportTranslationFromAPIConfig done", "source_id", sourceID, "sentences", inserted)
	return inserted, nil
}

// extractTextFromJSONPath extracts text from a JSON response using a simple dot-notation path.
// e.g., "data.items.0.content" extracts nested JSON fields. Limited implementation for MVP.
func extractTextFromJSONPath(jsonStr, path string) string {
	if path == "" {
		return jsonStr
	}

	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonStr
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return fmt.Sprintf("%v", current)
		}
		current = m[part]
	}
	return fmt.Sprintf("%v", current)
}
```

- [ ] **Step 2: Add dispatch in root.go**

In the CLI `Run()` switch, add:
```go
case "import-translation-api":
    return runImportTranslationAPI(args[1:])
```

Add the handler function:
```go
func runImportTranslationAPI(args []string) int {
	fs := flag.NewFlagSet("import-translation-api", flag.ContinueOnError)
	configPath := fs.String("file", "", "path to JSON config file for API import")
	dbPath := fs.String("db", "./data/app.db", "path to the SQLite database file")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "import-translation-api: %v\n", err)
		return 1
	}
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "import-translation-api: --file is required")
		return 1
	}

	db, err := data.OpenDB(*dbPath)
	if err != nil {
		slog.Error("import-translation-api: failed to open database", "db", *dbPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-translation-api: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("import-translation-api: failed to run migrations", "err", err)
		fmt.Fprintf(os.Stderr, "import-translation-api: run migrations: %v\n", err)
		return 1
	}

	n, err := ImportTranslationFromAPIConfig(db, *configPath)
	if err != nil {
		slog.Error("import-translation-api: ImportTranslationFromAPIConfig failed", "config", *configPath, "err", err)
		fmt.Fprintf(os.Stderr, "import-translation-api: %v\n", err)
		return 1
	}

	fmt.Printf("import-translation-api: imported %d sentence(s) from config %s\n", n, *configPath)
	return 0
}
```

Also update `printUsage()` to include:
```go
fmt.Fprintln(os.Stderr, "  import-translation-api --file <config.json>  import translation sentences from API")
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./backend/cmd/server/
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/import_translation_api.go internal/cli/root.go
git commit -m "feat(translation): add import-translation-api CLI command"
```

---

### Task 10: Frontend API Types

**Files:**
- Modify: `front/react/src/types/api.ts`

- [ ] **Step 1: Add translation types to api.ts**

Append to `front/react/src/types/api.ts`:

```typescript
// Translation
export type TranslationDirection = 'cn2jp' | 'jp2cn'

export interface GrammarExplanation {
  grammar_point: string
  explanation: string
  matched_db_id?: number
}

export interface TranslationFeedback {
  ai_score: number
  grammar_explanations: GrammarExplanation[]
  issue_description: string
  corrected_translation: string
  reference_translation: string
}

export interface TranslationSource {
  id: number
  title: string
  source_type: 'manual' | 'url' | 'api'
  source_url: string
  api_endpoint: string
  raw_content: string
  direction?: string
  created_at: string
}

export interface TranslationSentence {
  id: number
  source_id: number
  direction: TranslationDirection
  source_text: string
  reference_translation: string
  position: number
}

export interface TranslationRecord {
  id: number
  user_id: number
  sentence_id: number
  user_translation: string
  score: number
  rule_score: number
  ai_feedback?: TranslationFeedback
  practiced_at: string
}
```

- [ ] **Step 2: Commit**

```bash
git add front/react/src/types/api.ts
git commit -m "feat(translation): add TypeScript types for translation module"
```

---

### Task 11: Frontend Pages — TranslationListPage

**Files:**
- Create: `front/react/src/pages/translation/TranslationListPage.tsx`
- Create: `front/react/src/pages/translation/TranslationListPage.module.css`

- [ ] **Step 1: Write TranslationListPage.tsx**

```tsx
import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { TranslationSource, TranslationSentence } from '@/types/api'
import styles from './TranslationListPage.module.css'

export function TranslationListPage() {
  const navigate = useNavigate()
  const [sources, setSources] = useState<TranslationSource[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showImport, setShowImport] = useState(false)
  const [importTab, setImportTab] = useState<'paste' | 'url'>('paste')
  const [pasteContent, setPasteContent] = useState('')
  const [pasteTitle, setPasteTitle] = useState('')
  const [importURL, setImportURL] = useState('')
  const [importing, setImporting] = useState(false)

  useEffect(() => { fetchSources() }, [])

  async function fetchSources() {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<TranslationSource[]>('GET', '/api/v1/translation/sources')
      setSources(data ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sources')
    } finally {
      setLoading(false)
    }
  }

  async function handlePasteImport() {
    if (!pasteContent.trim()) return
    setImporting(true)
    try {
      await apiFetch('POST', '/api/v1/translation/sources', {
        title: pasteTitle || 'Manual Import',
        source_type: 'manual',
        content: pasteContent,
      })
      setPasteContent('')
      setPasteTitle('')
      setShowImport(false)
      fetchSources()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  async function handleURLImport() {
    if (!importURL.trim()) return
    setImporting(true)
    try {
      await apiFetch('POST', '/api/v1/translation/sources/import', { url: importURL })
      setImportURL('')
      setShowImport(false)
      fetchSources()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>翻訳練習</h1>

      {/* Cards */}
      <div className={styles.cardRow}>
        <button className={styles.mainCard} onClick={() => navigate('/translation/practice?mode=daily')}>
          <span className={styles.cardIcon}>📅</span>
          <span className={styles.cardLabel}>今日任务</span>
        </button>
        <button className={styles.mainCard} onClick={() => navigate('/translation/practice?mode=free')}>
          <span className={styles.cardIcon}>📝</span>
          <span className={styles.cardLabel}>自由练习</span>
        </button>
      </div>

      {/* Source list */}
      {error && <p className={styles.error}>{error}</p>}

      {loading ? (
        <div className={styles.center}><Spinner size="lg" /></div>
      ) : sources.length === 0 ? (
        <EmptyState icon="📖" title="暂无翻译素材" description="点击下方按钮导入" />
      ) : (
        <div className={styles.sourceList}>
          {sources.map(s => (
            <div
              key={s.id}
              className={styles.sourceCard}
              onClick={() => navigate(`/translation/practice?mode=free&source_id=${s.id}`)}
            >
              <div className={styles.sourceHeader}>
                <span className={styles.sourceTitle}>{s.title}</span>
                <span className={styles.sourceType}>{s.source_type}</span>
              </div>
              <p className={styles.sourcePreview}>{s.raw_content.slice(0, 100)}…</p>
            </div>
          ))}
        </div>
      )}

      {/* Import button */}
      <button className={styles.importBtn} onClick={() => setShowImport(!showImport)}>
        + 导入新素材
      </button>

      {/* Import dialog */}
      {showImport && (
        <div className={styles.importDialog}>
          <div className={styles.importTabs}>
            <button
              className={`${styles.importTab} ${importTab === 'paste' ? styles.importTabActive : ''}`}
              onClick={() => setImportTab('paste')}
            >
              粘贴文本
            </button>
            <button
              className={`${styles.importTab} ${importTab === 'url' ? styles.importTabActive : ''}`}
              onClick={() => setImportTab('url')}
            >
              输入 URL
            </button>
          </div>

          {importTab === 'paste' ? (
            <div className={styles.importForm}>
              <input
                className={styles.importInput}
                placeholder="素材标题（可选）"
                value={pasteTitle}
                onChange={e => setPasteTitle(e.target.value)}
              />
              <textarea
                className={styles.importTextarea}
                placeholder="粘贴日语或中文文本…"
                value={pasteContent}
                onChange={e => setPasteContent(e.target.value)}
                rows={8}
              />
              <button className={styles.submitBtn} onClick={handlePasteImport} disabled={importing || !pasteContent.trim()}>
                {importing ? <Spinner size="sm" /> : '导入'}
              </button>
            </div>
          ) : (
            <div className={styles.importForm}>
              <input
                className={styles.importInput}
                placeholder="https://…"
                value={importURL}
                onChange={e => setImportURL(e.target.value)}
              />
              <button className={styles.submitBtn} onClick={handleURLImport} disabled={importing || !importURL.trim()}>
                {importing ? <Spinner size="sm" /> : '抓取并导入'}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Write CSS module**

`TranslationListPage.module.css`:

```css
.page {
  padding: var(--space-4);
  max-width: 700px;
  margin: 0 auto;
}

.title {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--space-6) 0;
}

.cardRow {
  display: flex;
  gap: var(--space-3);
  margin-bottom: var(--space-5);
}

.mainCard {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-5) var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  cursor: pointer;
  transition: border-color 0.15s;
}

.mainCard:hover {
  border-color: var(--color-primary);
}

.cardIcon { font-size: 28px; }

.cardLabel {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.error {
  color: var(--color-error);
  font-size: var(--font-size-sm);
  margin-bottom: var(--space-3);
}

.center {
  display: flex;
  justify-content: center;
  padding-top: 40px;
}

.sourceList {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.sourceCard {
  padding: var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  cursor: pointer;
  transition: border-color 0.15s;
}

.sourceCard:hover {
  border-color: var(--color-primary);
}

.sourceHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-2);
}

.sourceTitle {
  font-weight: 700;
  color: var(--color-text-primary);
}

.sourceType {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  background: var(--color-bg-base);
  padding: 2px var(--space-2);
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border);
}

.sourcePreview {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.importBtn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  padding: var(--space-3);
  margin-top: var(--space-5);
  border: 1px dashed var(--color-border);
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-size-md);
  font-weight: 600;
  cursor: pointer;
  transition: border-color 0.15s;
}

.importBtn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.importDialog {
  margin-top: var(--space-4);
  padding: var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.importTabs {
  display: flex;
  gap: var(--space-2);
  margin-bottom: var(--space-4);
}

.importTab {
  flex: 1;
  padding: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-base);
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.importTabActive {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: #fff;
}

.importForm {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.importInput {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-base);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.importTextarea {
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-base);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  resize: vertical;
  font-family: inherit;
}

.submitBtn {
  padding: var(--space-3);
  border: none;
  border-radius: var(--radius-md);
  background: var(--color-primary);
  color: #fff;
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
}

.submitBtn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
```

- [ ] **Step 3: Verify builds**

```bash
cd front/react && npm run build
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add front/react/src/pages/translation/
git commit -m "feat(translation): add TranslationListPage with import dialog"
```

---

### Task 12: Frontend Pages — TranslationPracticePage

**Files:**
- Create: `front/react/src/pages/translation/TranslationPracticePage.tsx`
- Create: `front/react/src/pages/translation/TranslationPracticePage.module.css`

- [ ] **Step 1: Write TranslationPracticePage.tsx**

```tsx
import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import type { TranslationSentence, TranslationRecord } from '@/types/api'
import styles from './TranslationPracticePage.module.css'

export function TranslationPracticePage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const mode = searchParams.get('mode') || 'daily'
  const sourceID = searchParams.get('source_id')

  const [sentences, setSentences] = useState<TranslationSentence[]>([])
  const [currentIdx, setCurrentIdx] = useState(0)
  const [userTranslation, setUserTranslation] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [currentResult, setCurrentResult] = useState<TranslationRecord | null>(null)
  const [aiPolling, setAiPolling] = useState(false)

  useEffect(() => { loadSentences() }, [mode, sourceID])

  async function loadSentences() {
    setLoading(true)
    setError('')
    try {
      let data: TranslationSentence[]
      if (mode === 'daily') {
        const resp = await apiFetch<TranslationSentence[]>('GET', '/api/v1/translation/queue?count=5')
        data = resp ?? []
      } else if (sourceID) {
        data = await apiFetch<TranslationSentence[]>(
          'GET',
          `/api/v1/translation/sentences?source_id=${sourceID}`,
        ) ?? []
      } else {
        setError('请选择素材来源')
        return
      }
      setSentences(data)
      setCurrentIdx(0)
      setCurrentResult(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentences')
    } finally {
      setLoading(false)
    }
  }

  async function handleSubmit() {
    if (!userTranslation.trim()) return
    const sentence = sentences[currentIdx]
    setSubmitting(true)
    setCurrentResult(null)
    try {
      const rec = await apiFetch<TranslationRecord>('POST', '/api/v1/translation/submit', {
        sentence_id: sentence.id,
        user_translation: userTranslation.trim(),
      })
      setCurrentResult(rec)

      // Poll for AI feedback if not yet available
      if (rec && !rec.ai_feedback && rec.score === rec.rule_score) {
        setAiPolling(true)
        pollForAIFeedback(rec.id)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  async function pollForAIFeedback(recordID: number) {
    let attempts = 0
    const maxAttempts = 15 // 30s total with 2s intervals
    const interval = setInterval(async () => {
      attempts++
      try {
        const rec = await apiFetch<TranslationRecord>('GET', `/api/v1/translation/records/${recordID}`)
        if (rec?.ai_feedback || attempts >= maxAttempts) {
          clearInterval(interval)
          setAiPolling(false)
          if (rec) setCurrentResult(rec)
        }
      } catch {
        clearInterval(interval)
        setAiPolling(false)
      }
    }, 2000)
  }

  function handleNext() {
    if (currentIdx < sentences.length - 1) {
      setCurrentIdx(currentIdx + 1)
      setUserTranslation('')
      setCurrentResult(null)
    }
  }

  if (loading) {
    return <div className={styles.page}><div className={styles.center}><Spinner size="lg" /></div></div>
  }

  if (sentences.length === 0) {
    return (
      <div className={styles.page}>
        <button className={styles.backBtn} onClick={() => navigate('/translation')}>← 戻る</button>
        <p className={styles.empty}>暂无练习内容。请先导入素材。</p>
      </div>
    )
  }

  const sentence = sentences[currentIdx]
  const directionLabel = sentence.direction === 'jp2cn' ? '日→中' : '中→日'

  return (
    <div className={styles.page}>
      {/* Header */}
      <div className={styles.header}>
        <button className={styles.backBtn} onClick={() => navigate('/translation')}>← 戻る</button>
        <span className={styles.directionBadge}>{directionLabel}</span>
        <span className={styles.progress}>#{currentIdx + 1}/{sentences.length}</span>
      </div>

      {/* Source text */}
      <div className={styles.sourceText}>
        <p>{sentence.source_text}</p>
      </div>

      {/* Input */}
      <textarea
        className={styles.translateInput}
        placeholder="在此输入你的翻译…"
        value={userTranslation}
        onChange={e => setUserTranslation(e.target.value)}
        rows={4}
        disabled={!!currentResult}
      />

      {/* Submit */}
      {!currentResult && (
        <button
          className={styles.submitBtn}
          onClick={handleSubmit}
          disabled={submitting || !userTranslation.trim()}
        >
          {submitting ? <Spinner size="sm" /> : '提出する'}
        </button>
      )}

      {/* Result */}
      {currentResult && (
        <div className={styles.result}>
          <div className={styles.scores}>
            <div className={styles.scoreItem}>
              <span className={styles.scoreLabel}>即时评分</span>
              <span className={styles.scoreValue}>{currentResult.rule_score}</span>
            </div>
            {currentResult.ai_feedback ? (
              <div className={styles.scoreItem}>
                <span className={styles.scoreLabel}>AI 评分</span>
                <span className={styles.scoreValue}>{currentResult.ai_feedback.ai_score}</span>
              </div>
            ) : aiPolling ? (
              <div className={styles.scoreItem}>
                <span className={styles.scoreLabel}>AI 评分中…</span>
                <Spinner size="sm" />
              </div>
            ) : null}
          </div>

          {/* Grammar explanations */}
          {currentResult.ai_feedback?.grammar_explanations && currentResult.ai_feedback.grammar_explanations.length > 0 && (
            <div className={styles.grammarSection}>
              <p className={styles.grammarLabel}>📚 涉及语法</p>
              {currentResult.ai_feedback.grammar_explanations.map((g, i) => (
                <div key={i} className={styles.grammarTag}>
                  <span className={styles.grammarPoint}>{g.grammar_point}</span>
                  <span className={styles.grammarExplanation}>{g.explanation}</span>
                </div>
              ))}
            </div>
          )}

          {/* AI commentary */}
          {currentResult.ai_feedback?.issue_description && (
            <div className={styles.commentary}>
              <p className={styles.commentaryLabel}>💬 AI 点评</p>
              <p className={styles.commentaryText}>{currentResult.ai_feedback.issue_description}</p>
            </div>
          )}

          {/* Corrected translations */}
          {currentResult.ai_feedback?.corrected_translation && (
            <div className={styles.correctedSection}>
              <p className={styles.correctedLabel}>修改建议</p>
              <p className={styles.correctedText}>{currentResult.ai_feedback.corrected_translation}</p>
            </div>
          )}
        </div>
      )}

      {/* Next button */}
      {currentResult && currentIdx < sentences.length - 1 && (
        <button className={styles.nextBtn} onClick={handleNext}>
          下一题 →
        </button>
      )}

      {error && <p className={styles.error}>{error}</p>}
    </div>
  )
}
```

- [ ] **Step 2: Write CSS module**

`TranslationPracticePage.module.css`:

```css
.page {
  padding: var(--space-4);
  max-width: 700px;
  margin: 0 auto;
}

.header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-5);
}

.backBtn {
  background: none;
  border: none;
  padding: 0;
  font-size: var(--font-size-md);
  color: var(--color-primary);
  cursor: pointer;
  font-weight: 600;
}

.directionBadge {
  font-size: var(--font-size-xs);
  font-weight: 700;
  padding: 2px var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--color-primary-light);
  color: var(--color-primary);
}

.progress {
  margin-left: auto;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  font-weight: 600;
}

.sourceText {
  padding: var(--space-5);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  margin-bottom: var(--space-4);
  font-size: var(--font-size-lg);
  line-height: 2;
  color: var(--color-text-primary);
}

.sourceText p { margin: 0; }

.translateInput {
  width: 100%;
  padding: var(--space-4);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-bg-surface);
  font-size: var(--font-size-md);
  color: var(--color-text-primary);
  resize: vertical;
  font-family: inherit;
  line-height: 1.8;
  margin-bottom: var(--space-4);
  box-sizing: border-box;
}

.translateInput:focus {
  outline: none;
  border-color: var(--color-primary);
}

.submitBtn, .nextBtn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  padding: var(--space-4);
  border: none;
  border-radius: var(--radius-lg);
  background: var(--color-primary);
  font-size: var(--font-size-md);
  font-weight: 700;
  color: #fff;
  cursor: pointer;
  margin-bottom: var(--space-4);
}

.submitBtn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.nextBtn {
  background: var(--color-accent);
}

.result {
  margin-bottom: var(--space-4);
}

.scores {
  display: flex;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}

.scoreItem {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.scoreLabel {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.scoreValue {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--color-primary);
}

.grammarSection {
  padding: var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  margin-bottom: var(--space-3);
}

.grammarLabel {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--space-2) 0;
}

.grammarTag {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-2);
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--color-border);
}

.grammarTag:last-child { border-bottom: none; }

.grammarPoint {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--color-accent);
  background: var(--color-accent-light);
  padding: 1px var(--space-2);
  border-radius: var(--radius-sm);
}

.grammarExplanation {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.commentary {
  padding: var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  margin-bottom: var(--space-3);
}

.commentaryLabel {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--space-1) 0;
}

.commentaryText {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: 1.6;
  margin: 0;
}

.correctedSection {
  padding: var(--space-4);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.correctedLabel {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  margin: 0 0 var(--space-1) 0;
}

.correctedText {
  font-size: var(--font-size-md);
  color: var(--color-primary);
  margin: 0;
  font-weight: 600;
}

.error {
  color: var(--color-error);
  font-size: var(--font-size-sm);
}

.empty {
  color: var(--color-text-secondary);
  font-size: var(--font-size-md);
  text-align: center;
  margin-top: var(--space-8);
}

.center {
  display: flex;
  justify-content: center;
  padding-top: 40px;
}
```

- [ ] **Step 3: Verify builds**

```bash
cd front/react && npm run build
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add front/react/src/pages/translation/
git commit -m "feat(translation): add TranslationPracticePage with scoring and AI feedback"
```

---

### Task 13: Frontend Routing + Navigation

**Files:**
- Modify: `front/react/src/App.tsx`
- Modify: `front/react/src/components/layout/TopNavBar.tsx`
- Modify: `front/react/src/components/layout/BottomTabBar.tsx`

- [ ] **Step 1: Add routes to App.tsx**

In `App.tsx`, add imports:
```tsx
import { TranslationListPage } from '@/pages/translation/TranslationListPage'
import { TranslationPracticePage } from '@/pages/translation/TranslationPracticePage'
```

Add routes inside `<Route element={<ProtectedLayout />}>`:
```tsx
<Route path="/translation" element={<TranslationListPage />} />
<Route path="/translation/practice" element={<TranslationPracticePage />} />
```

- [ ] **Step 2: Add nav entry in TopNavBar.tsx**

Find the nav links section and add:
```tsx
<NavLink to="/translation" className={...}>翻訳</NavLink>
```
(NavLink should use the same pattern as existing links like "写作", "口语")

- [ ] **Step 3: Add nav entry in BottomTabBar.tsx**

Find the tab items and add:
```tsx
{ icon: '🌐', label: '翻訳', path: '/translation' },
```

- [ ] **Step 4: Verify builds**

```bash
cd front/react && npm run build
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add front/react/src/App.tsx front/react/src/components/layout/TopNavBar.tsx front/react/src/components/layout/BottomTabBar.tsx
git commit -m "feat(translation): add routes and navigation for translation module"
```

---

### Task 14: Integration Test & Cleanup

**Files:**
- Modify: `internal/module/translation/service.go` — ensure `ListSources` is complete

- [ ] **Step 1: Run all tests**

```bash
go test ./... -count=1
```
Expected: all pass.

- [ ] **Step 2: Build full project**

```bash
make build
cd front/react && npm run build
```
Expected: both succeed.

- [ ] **Step 3: Commit any final fixes**

```bash
git add -A
git commit -m "chore(translation): final integration fixes"
```
