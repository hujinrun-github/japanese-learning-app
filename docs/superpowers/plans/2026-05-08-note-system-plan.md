# Note System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a user-private note system with Markdown content, FTS5 search, SRS review, note linking, and a unified word+note review queue.

**Architecture:** Follows the existing Handler → Service → Adapter → Store → SQLite pattern. Notes are independent entities with optional `reference_id` links to system words/grammar/lessons. SM-2 algorithm extracted to `internal/sm2/` for reuse between word and note modules.

**Tech Stack:** Go 1.24+, net/http, modernc.org/sqlite, FTS5, standard library only.

**Spec:** docs/superpowers/specs/2026-05-08-note-system-design.md

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/sm2/sm2.go` | Create | Extracted SM-2 algorithm |
| `internal/sm2/sm2_test.go` | Create | SM-2 tests (moved from word) |
| `internal/data/migrations/006_notes.sql` | Create | DDL for notes, note_links, FTS5, triggers |
| `internal/module/note/model.go` | Create | Note, NoteLink, NoteDetail, NoteDigest types |
| `internal/module/note/service.go` | Create | NoteService + NoteStoreInterface |
| `internal/module/note/handler.go` | Create | HTTP handlers + RegisterRoutes |
| `internal/module/note/note_test.go` | Create | Handler + Service tests |
| `internal/data/note_store.go` | Create | SQLite NoteStore implementation |
| `internal/data/note_store_test.go` | Create | Integration tests for NoteStore |
| `internal/data/adapters.go` | Modify | Add NoteStoreAdapter |
| `internal/module/word/sm2.go` | Delete | Moved to internal/sm2/ |
| `internal/module/word/model.go` | Modify | Remove Rating types, import from sm2 |
| `internal/module/word/service.go` | Modify | Import sm2, update CalcNextReview call |
| `backend/cmd/server/main.go` | Modify | Wire note module + register routes |
| `internal/module/word/handler.go` | Modify | Add word detail endpoint |
| `internal/module/grammar/handler.go` | Modify | Inject related_notes in detail |
| `internal/module/grammar/service.go` | Modify | Accept NoteDigestProvider interface |

---

### Task 1: Extract SM-2 algorithm to internal/sm2/

**Files:**
- Create: `internal/sm2/sm2.go`
- Create: `internal/sm2/sm2_test.go`
- Delete: `internal/module/word/sm2.go`
- Modify: `internal/module/word/model.go` (remove Rating types, import from sm2)
- Modify: `internal/module/word/service.go` (update import)
- Modify: `internal/module/word/handler.go` (update Rating references)

- [ ] **Step 1: Create internal/sm2/sm2.go with extracted pure-function CalcNextReview**

```go
package sm2

import (
	"math"
	"time"
)

// Rating is a self-assessment rating for a review.
type Rating string

const (
	RatingEasy   Rating = "easy"
	RatingNormal Rating = "normal"
	RatingHard   Rating = "hard"
)

// ReviewEvent records a single review attempt.
type ReviewEvent struct {
	Rating     Rating    `json:"rating"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// CalcNextReview applies the SM-2 spaced repetition algorithm.
// Returns: newMastery, newInterval, newEaseFactor, nextReviewAt, newHistory.
//
// SM-2 rules:
//   - hard  → reset mastery to 0, interval = 1, EF -= 0.2 (min 1.3)
//   - normal→ advance mastery, EF unchanged
//   - easy  → advance mastery, EF += 0.1 (max 3.0)
//
// Interval schedule:
//   - mastery 0 (first learn): interval = 1
//   - mastery 1: interval = 6
//   - mastery >= 2: interval = floor(prev_interval * EF)
func CalcNextReview(mastery int, interval int, easeFactor float64, rating Rating, history []ReviewEvent) (int, int, float64, time.Time, []ReviewEvent) {
	switch rating {
	case RatingHard:
		mastery = 0
		interval = 1
		easeFactor = math.Max(1.3, easeFactor-0.2)
	case RatingNormal:
		interval = nextInterval(mastery, interval, easeFactor)
		mastery++
	case RatingEasy:
		interval = nextInterval(mastery, interval, easeFactor)
		mastery++
		easeFactor = math.Min(3.0, easeFactor+0.1)
	}

	nextReviewAt := time.Now().Add(time.Duration(interval) * 24 * time.Hour)
	history = append(history, ReviewEvent{Rating: rating, ReviewedAt: time.Now()})

	return mastery, interval, easeFactor, nextReviewAt, history
}

func nextInterval(mastery, prevInterval int, ef float64) int {
	switch mastery {
	case 0:
		return 1
	case 1:
		return 6
	default:
		return int(math.Floor(float64(prevInterval) * ef))
	}
}
```

- [ ] **Step 2: Create internal/sm2/sm2_test.go with moved tests**

```go
package sm2_test

import (
	"testing"
	"time"

	"japanese-learning-app/internal/sm2"
)

func TestCalcNextReview(t *testing.T) {
	tests := []struct {
		name            string
		mastery         int
		interval        int
		easeFactor      float64
		rating          sm2.Rating
		wantMastery     int
		wantInterval    int
		wantEFMin       float64
		wantEFMax       float64
	}{
		{
			name:         "first learning easy",
			mastery:      0,
			interval:     0,
			easeFactor:   2.5,
			rating:       sm2.RatingEasy,
			wantMastery:  1,
			wantInterval: 1,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "first learning normal",
			mastery:      0,
			interval:     0,
			easeFactor:   2.5,
			rating:       sm2.RatingNormal,
			wantMastery:  1,
			wantInterval: 1,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "first learning hard",
			mastery:      0,
			interval:     0,
			easeFactor:   2.5,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 1 easy -> interval 6",
			mastery:      1,
			interval:     1,
			easeFactor:   2.5,
			rating:       sm2.RatingEasy,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "mastery 1 normal -> interval 6",
			mastery:      1,
			interval:     1,
			easeFactor:   2.5,
			rating:       sm2.RatingNormal,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 1 hard -> reset",
			mastery:      1,
			interval:     1,
			easeFactor:   2.5,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 2 normal -> interval = prev*EF",
			mastery:      2,
			interval:     6,
			easeFactor:   2.5,
			rating:       sm2.RatingNormal,
			wantMastery:  3,
			wantInterval: 15,
			wantEFMin:    2.5,
			wantEFMax:    2.5,
		},
		{
			name:         "mastery 2 easy -> ef increases",
			mastery:      2,
			interval:     6,
			easeFactor:   2.5,
			rating:       sm2.RatingEasy,
			wantMastery:  3,
			wantInterval: 15,
			wantEFMin:    2.6,
			wantEFMax:    3.0,
		},
		{
			name:         "mastery 3 hard -> reset regardless of mastery",
			mastery:      3,
			interval:     15,
			easeFactor:   2.5,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    2.49,
		},
		{
			name:         "ef already min 1.3 stays at 1.3 on hard",
			mastery:      1,
			interval:     1,
			easeFactor:   1.3,
			rating:       sm2.RatingHard,
			wantMastery:  0,
			wantInterval: 1,
			wantEFMin:    1.3,
			wantEFMax:    1.3,
		},
		{
			name:         "ef cap at 3.0 on easy",
			mastery:      1,
			interval:     1,
			easeFactor:   2.9,
			rating:       sm2.RatingEasy,
			wantMastery:  2,
			wantInterval: 6,
			wantEFMin:    3.0,
			wantEFMax:    3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			mastery, interval, ef, nextReview, history := sm2.CalcNextReview(tt.mastery, tt.interval, tt.easeFactor, tt.rating, nil)
			after := time.Now()

			if mastery != tt.wantMastery {
				t.Errorf("mastery = %d, want %d", mastery, tt.wantMastery)
			}
			if interval != tt.wantInterval {
				t.Errorf("interval = %d, want %d", interval, tt.wantInterval)
			}
			if ef < tt.wantEFMin-0.001 || ef > tt.wantEFMax+0.001 {
				t.Errorf("easeFactor = %.4f, want [%.4f, %.4f]", ef, tt.wantEFMin, tt.wantEFMax)
			}
			wantLo := before.Add(time.Duration(interval)*24*time.Hour - 5*time.Second)
			wantHi := after.Add(time.Duration(interval)*24*time.Hour + 5*time.Second)
			if nextReview.Before(wantLo) || nextReview.After(wantHi) {
				t.Errorf("nextReviewAt = %v, want between %v and %v", nextReview, wantLo, wantHi)
			}
			if len(history) != 1 {
				t.Errorf("history len = %d, want 1", len(history))
			}
			if history[0].Rating != tt.rating {
				t.Errorf("history[0].Rating = %s, want %s", history[0].Rating, tt.rating)
			}
		})
	}
}
```

- [ ] **Step 3: Run SM-2 tests to verify new package works**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/sm2/ -v
```
Expected: PASS

- [ ] **Step 4: Update word model to use sm2 types, remove Rating constants**

In `internal/module/word/model.go`:
- Remove `ReviewRating` type and `RatingEasy`/`RatingNormal`/`RatingHard` constants
- Remove `ReviewEvent` type
- Add `import "japanese-learning-app/internal/sm2"`
- Change `ReviewRating` references to `sm2.Rating`
- Change `ReviewEvent` references to `sm2.ReviewEvent`

```go
// In model.go, replace:
// type ReviewRating string
// const (RatingEasy/RatingNormal/RatingHard)
// type ReviewEvent struct { ... }

// With:
import "japanese-learning-app/internal/sm2"

// Update WordRecord.ReviewHistory type:
ReviewHistory []sm2.ReviewEvent `json:"review_history"`
```

- [ ] **Step 5: Update word handler to use sm2.Rating**

In `internal/module/word/handler.go`:
- Add import `"japanese-learning-app/internal/sm2"`
- Change `ReviewRating` references to `sm2.Rating`
- Change `req.Rating` type assertion to `sm2.Rating`

- [ ] **Step 6: Update word service to use sm2.CalcNextReview**

In `internal/module/word/service.go`:
- Add import `"japanese-learning-app/internal/sm2"`
- In `SubmitRating`, replace the `CalcNextReview` call:

```go
// Before:
updated := CalcNextReview(base, rating)

// After:
newMastery, newInterval, newEF, nextReview, newHistory := sm2.CalcNextReview(
    base.MasteryLevel, base.Interval, base.EaseFactor,
    sm2.Rating(rating), base.ReviewHistory,
)
updated := WordRecord{
    UserID:        userID,
    WordID:        wordID,
    MasteryLevel:  newMastery,
    Interval:      newInterval,
    EaseFactor:    newEF,
    NextReviewAt:  nextReview,
    ReviewHistory: newHistory,
}
```

- [ ] **Step 7: Delete internal/module/word/sm2.go**

```bash
rm /home/tylerhu/github_project/japanese-learning-app/internal/module/word/sm2.go
```

- [ ] **Step 8: Run full test suite to verify nothing is broken**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/... -v 2>&1 | tail -30
```
Expected: All tests PASS

- [ ] **Step 9: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/sm2/ internal/module/word/ && \
git rm internal/module/word/sm2.go && \
git commit -m "$(cat <<'EOF'
refactor(sm2): extract SM-2 algorithm to internal/sm2/ for reuse

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Database migration for notes

**Files:**
- Create: `internal/data/migrations/006_notes.sql`

- [ ] **Step 1: Create migration file**

```sql
-- 006_notes.sql
-- Notes system: user-private notes with Markdown content, FTS5 search, SRS review, and linking.

CREATE TABLE IF NOT EXISTS notes (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL REFERENCES users(id),
    type                TEXT NOT NULL CHECK(type IN ('word', 'grammar', 'sentence')),
    title               TEXT NOT NULL,
    content             TEXT NOT NULL DEFAULT '',
    source_text         TEXT NOT NULL DEFAULT '',
    reference_id        INTEGER,
    reference_type      TEXT,
    tags_json           TEXT NOT NULL DEFAULT '[]',
    mastery_level       INTEGER NOT NULL DEFAULT 0,
    next_review_at      DATETIME,
    ease_factor         REAL NOT NULL DEFAULT 2.5,
    interval            INTEGER NOT NULL DEFAULT 0,
    review_history_json TEXT NOT NULL DEFAULT '[]',
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    deleted_at          DATETIME
);

CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes(user_id);
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(user_id, type);
CREATE INDEX IF NOT EXISTS idx_notes_review ON notes(user_id, next_review_at)
    WHERE next_review_at IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notes_reference ON notes(reference_type, reference_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS note_links (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    note_id         INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_note_id  INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    relation        TEXT NOT NULL DEFAULT 'related',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(note_id, target_note_id)
);

CREATE INDEX IF NOT EXISTS idx_note_links_note_id ON note_links(note_id);
CREATE INDEX IF NOT EXISTS idx_note_links_target ON note_links(target_note_id);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
    title, content, source_text,
    content=notes, content_rowid=id
);

CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, title, content, source_text)
    VALUES (new.id, new.title, new.content, new.source_text);
END;

CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, content, source_text)
    VALUES ('delete', old.id, old.title, old.content, old.source_text);
END;

CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, content, source_text)
    VALUES ('delete', old.id, old.title, old.content, old.source_text);
    INSERT INTO notes_fts(rowid, title, content, source_text)
    VALUES (new.id, new.title, new.content, new.source_text);
END;
```

- [ ] **Step 2: Verify migration applies cleanly**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestMain -v 2>&1 | head -20
```
Expected: migrations applied without errors

- [ ] **Step 3: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/migrations/006_notes.sql && \
git commit -m "$(cat <<'EOF'
feat(data): add notes, note_links, and FTS5 migration

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Note model

**Files:**
- Create: `internal/module/note/model.go`

- [ ] **Step 1: Create model.go**

```go
package note

import (
	"time"

	"japanese-learning-app/internal/sm2"
)

// NoteType classifies the kind of content a note captures.
type NoteType string

const (
	TypeWord     NoteType = "word"
	TypeGrammar  NoteType = "grammar"
	TypeSentence NoteType = "sentence"
)

// LinkRelation describes how two notes are related.
type LinkRelation string

const (
	RelationRelated     LinkRelation = "related"
	RelationUsesWord    LinkRelation = "uses_word"
	RelationUsesGrammar LinkRelation = "uses_grammar"
	RelationContext     LinkRelation = "context"
)

// Note is a user-private record of a word, grammar point, or sentence.
type Note struct {
	ID               int64            `json:"id"`
	UserID           int64            `json:"-"`
	Type             NoteType         `json:"type"`
	Title            string           `json:"title"`
	Content          string           `json:"content"`
	SourceText       string           `json:"source_text"`
	ReferenceID      *int64           `json:"reference_id,omitempty"`
	ReferenceType    *string          `json:"reference_type,omitempty"`
	Tags             []string         `json:"tags"`
	MasteryLevel     int              `json:"mastery_level"`
	NextReviewAt     *time.Time       `json:"next_review_at,omitempty"`
	EaseFactor       float64          `json:"ease_factor"`
	Interval         int              `json:"interval"`
	ReviewHistory    []sm2.ReviewEvent `json:"review_history"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// NoteLink connects two notes with a typed relationship.
type NoteLink struct {
	ID           int64        `json:"id"`
	NoteID       int64        `json:"note_id"`
	TargetNoteID int64        `json:"target_note_id"`
	Relation     LinkRelation `json:"relation"`
	TargetNote   *NoteDigest  `json:"target_note,omitempty"`
}

// NoteDetail is the full view of a note including all inbound and outbound links.
type NoteDetail struct {
	Note
	OutgoingLinks []NoteLink `json:"links"`
	IncomingLinks []NoteLink `json:"backlinks"`
}

// NoteDigest is a lightweight reference to another note, used in link enrichment and cross-module lists.
type NoteDigest struct {
	ID    int64    `json:"id"`
	Title string   `json:"title"`
	Type  NoteType `json:"type"`
}

// NoteListParams filters and sorts note list queries.
type NoteListParams struct {
	Type   NoteType
	Tag    string
	Sort   string // "created_at" | "updated_at" | "next_review_at"
	Order  string // "asc" | "desc"
	Offset int
	Limit  int
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./internal/module/note/
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/module/note/model.go && \
git commit -m "$(cat <<'EOF'
feat(note): add Note, NoteLink, NoteDetail model types

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: NoteStore — Create and GetByID

**Files:**
- Create: `internal/data/note_store.go`
- Create: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing test for Create and GetByID**

```go
package data_test

import (
	"testing"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/note"
)

func TestNoteStore_CreateAndGetByID(t *testing.T) {
	db := testDB // from main_test.go

	store := data.NewNoteStore(db)
	n := &note.Note{
		UserID:     1,
		Type:       note.TypeWord,
		Title:      "雨",
		Content:    "あめ、雨。**音读**：う。",
		SourceText: "雨が降っている",
		Tags:       []string{"N5", "天气"},
	}

	err := store.Create(n)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if n.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	got, err := store.GetByID(1, n.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Title != "雨" {
		t.Errorf("Title = %q, want %q", got.Title, "雨")
	}
	if got.Type != note.TypeWord {
		t.Errorf("Type = %q, want %q", got.Type, note.TypeWord)
	}
	if got.Content != "あめ、雨。**音读**：う。" {
		t.Errorf("Content = %q", got.Content)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(got.Tags))
	}
	if got.MasteryLevel != 0 {
		t.Errorf("MasteryLevel = %d, want 0", got.MasteryLevel)
	}
	if got.NextReviewAt != nil {
		t.Error("NextReviewAt should be nil for new note")
	}
	if got.EaseFactor != 2.5 {
		t.Errorf("EaseFactor = %f, want 2.5", got.EaseFactor)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_CreateAndGetByID -v
```
Expected: FAIL — "undefined: data.NewNoteStore"

- [ ] **Step 3: Implement Create and GetByID in note_store.go**

```go
package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/sm2"
)

// NoteStore implements persistence for notes, links, and FTS5 search.
type NoteStore struct {
	db *sql.DB
}

// NewNoteStore creates a NoteStore.
func NewNoteStore(db *sql.DB) *NoteStore {
	return &NoteStore{db: db}
}

// Create inserts a new note and sets its ID, CreatedAt, and UpdatedAt.
func (s *NoteStore) Create(n *note.Note) error {
	slog.Debug("NoteStore.Create called", "user_id", n.UserID, "type", n.Type)

	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return fmt.Errorf("NoteStore.Create marshal tags: %w", err)
	}

	result, err := s.db.Exec(
		`INSERT INTO notes (user_id, type, title, content, source_text, reference_id, reference_type, tags_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Type, n.Title, n.Content, n.SourceText, n.ReferenceID, n.ReferenceType, string(tagsJSON),
	)
	if err != nil {
		slog.Error("NoteStore.Create failed", "err", err)
		return fmt.Errorf("NoteStore.Create exec: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("NoteStore.Create LastInsertId: %w", err)
	}

	// Read back the created row to get server-generated timestamps
	created, err := s.GetByID(n.UserID, id)
	if err != nil {
		return fmt.Errorf("NoteStore.Create readback: %w", err)
	}
	*n = *created
	return nil
}

// GetByID returns a note by ID, filtering by user. Returns error if not found.
func (s *NoteStore) GetByID(userID, noteID int64) (*note.Note, error) {
	slog.Debug("NoteStore.GetByID called", "user_id", userID, "note_id", noteID)

	row := s.db.QueryRow(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)

	n, err := scanNote(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("NoteStore.GetByID note=%d user=%d: %w", noteID, userID, sql.ErrNoRows)
		}
		return nil, fmt.Errorf("NoteStore.GetByID: %w", err)
	}

	return n, nil
}

// scanNote scans a single note from a row scanner.
func scanNote(scanner interface{ Scan(...interface{}) error }) (*note.Note, error) {
	var n note.Note
	var tagsJSON, historyJSON string
	var nextReviewAt sql.NullString
	var createdAt, updatedAt string
	var refID sql.NullInt64
	var refType sql.NullString

	err := scanner.Scan(
		&n.ID, &n.UserID, &n.Type, &n.Title, &n.Content, &n.SourceText,
		&refID, &refType, &tagsJSON, &n.MasteryLevel, &nextReviewAt, &n.EaseFactor,
		&n.Interval, &historyJSON, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if refID.Valid {
		n.ReferenceID = &refID.Int64
	}
	if refType.Valid {
		n.ReferenceType = &refType.String
	}

	if err := json.Unmarshal([]byte(tagsJSON), &n.Tags); err != nil {
		return nil, fmt.Errorf("scanNote unmarshal tags: %w", err)
	}

	if nextReviewAt.Valid {
		t, err := parseSQLiteTime(nextReviewAt.String)
		if err != nil {
			return nil, fmt.Errorf("scanNote parse next_review_at: %w", err)
		}
		n.NextReviewAt = &t
	}

	if err := json.Unmarshal([]byte(historyJSON), &n.ReviewHistory); err != nil {
		return nil, fmt.Errorf("scanNote unmarshal review_history: %w", err)
	}

	n.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("scanNote parse created_at: %w", err)
	}
	n.UpdatedAt, err = parseSQLiteTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanNote parse updated_at: %w", err)
	}

	return &n, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_CreateAndGetByID -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore Create and GetByID with tests

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: NoteStore — List

**Files:**
- Modify: `internal/data/note_store.go`
- Modify: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing test for List**

Add to `internal/data/note_store_test.go`:

```go
func TestNoteStore_List(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	// Create test notes
	n1 := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "ame", Tags: []string{"N5", "天气"}}
	n2 := &note.Note{UserID: 1, Type: note.TypeGrammar, Title: "～ている", Content: "持续体", Tags: []string{"N5"}}
	n3 := &note.Note{UserID: 2, Type: note.TypeWord, Title: "other user", Content: "should not appear"}
	for _, n := range []*note.Note{n1, n2, n3} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List all for user 1
	t.Run("list all", func(t *testing.T) {
		notes, total, err := store.List(1, note.NoteListParams{Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
		if len(notes) != 2 {
			t.Fatalf("len = %d, want 2", len(notes))
		}
	})

	// Filter by type
	t.Run("filter by type", func(t *testing.T) {
		notes, total, err := store.List(1, note.NoteListParams{Type: note.TypeWord, Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if notes[0].Type != note.TypeWord {
			t.Errorf("type = %q, want word", notes[0].Type)
		}
	})

	// Filter by tag
	t.Run("filter by tag", func(t *testing.T) {
		notes, total, err := store.List(1, note.NoteListParams{Tag: "天气", Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if notes[0].Title != "雨" {
			t.Errorf("title = %q, want 雨", notes[0].Title)
		}
	})

	// Pagination
	t.Run("pagination", func(t *testing.T) {
		notes, total, err := store.List(1, note.NoteListParams{Offset: 0, Limit: 1, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2 (total ignores pagination)", total)
		}
		if len(notes) != 1 {
			t.Errorf("len = %d, want 1", len(notes))
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_List -v
```
Expected: FAIL — "List not defined"

- [ ] **Step 3: Implement List in note_store.go**

```go
// List returns paginated notes for a user, with optional type/tag filtering and sorting.
func (s *NoteStore) List(userID int64, params NoteListParams) ([]note.Note, int, error) {
	slog.Debug("NoteStore.List called", "user_id", userID)

	sortCol := "updated_at"
	switch params.Sort {
	case "created_at":
		sortCol = "created_at"
	case "next_review_at":
		sortCol = "next_review_at"
	case "updated_at":
		sortCol = "updated_at"
	}
	order := "DESC"
	if params.Order == "asc" {
		order = "ASC"
	}

	where := "WHERE user_id = ? AND deleted_at IS NULL"
	args := []interface{}{userID}

	if params.Type != "" {
		where += " AND type = ?"
		args = append(args, string(params.Type))
	}
	if params.Tag != "" {
		where += " AND tags_json LIKE ?"
		args = append(args, fmt.Sprintf(`%%"%s"%%`, params.Tag))
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notes %s", where)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		slog.Error("NoteStore.List count failed", "err", err)
		return nil, 0, fmt.Errorf("NoteStore.List count: %w", err)
	}

	// Query with sorting and pagination
	query := fmt.Sprintf(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortCol, order,
	)
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		slog.Error("NoteStore.List query failed", "err", err)
		return nil, 0, fmt.Errorf("NoteStore.List query: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("NoteStore.List scan: %w", err)
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("NoteStore.List rows: %w", err)
	}

	slog.Debug("NoteStore.List done", "user_id", userID, "count", len(notes), "total", total)
	return notes, total, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_List -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore List with type/tag filtering and pagination

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: NoteStore — Update and SoftDelete

**Files:**
- Modify: `internal/data/note_store.go`
- Modify: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/data/note_store_test.go`:

```go
func TestNoteStore_Update(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	n := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "old content", Tags: []string{"N5"}}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	n.Title = "雨（更新）"
	n.Content = "new content"
	n.Tags = []string{"N5", "天气"}

	if err := store.Update(n); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := store.GetByID(1, n.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Title != "雨（更新）" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Content != "new content" {
		t.Errorf("Content = %q", got.Content)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(got.Tags))
	}
}

func TestNoteStore_SoftDelete(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	n := &note.Note{UserID: 1, Type: note.TypeWord, Title: "to delete", Content: "x"}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := store.SoftDelete(1, n.ID); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// GetByID should return error for soft-deleted note
	_, err := store.GetByID(1, n.ID)
	if err == nil {
		t.Error("expected error for soft-deleted note")
	}

	// List should not include soft-deleted note
	notes, total, err := store.List(1, note.NoteListParams{Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	_ = notes
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run "TestNoteStore_Update|TestNoteStore_SoftDelete" -v
```
Expected: FAIL

- [ ] **Step 3: Implement Update and SoftDelete**

Add to `internal/data/note_store.go`:

```go
// Update updates all editable fields of a note. The updated_at timestamp is refreshed.
func (s *NoteStore) Update(n *note.Note) error {
	slog.Debug("NoteStore.Update called", "user_id", n.UserID, "note_id", n.ID)

	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return fmt.Errorf("NoteStore.Update marshal tags: %w", err)
	}

	historyJSON, err := json.Marshal(n.ReviewHistory)
	if err != nil {
		return fmt.Errorf("NoteStore.Update marshal review_history: %w", err)
	}

	_, err = s.db.Exec(
		`UPDATE notes SET
		    type = ?, title = ?, content = ?, source_text = ?,
		    reference_id = ?, reference_type = ?, tags_json = ?,
		    mastery_level = ?, next_review_at = ?, ease_factor = ?,
		    interval = ?, review_history_json = ?, updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		n.Type, n.Title, n.Content, n.SourceText,
		n.ReferenceID, n.ReferenceType, string(tagsJSON),
		n.MasteryLevel, formatSQLiteTimePtr(n.NextReviewAt), n.EaseFactor,
		n.Interval, string(historyJSON),
		n.ID, n.UserID,
	)
	if err != nil {
		slog.Error("NoteStore.Update failed", "err", err)
		return fmt.Errorf("NoteStore.Update exec: %w", err)
	}

	return nil
}

// SoftDelete marks a note as deleted by setting deleted_at.
func (s *NoteStore) SoftDelete(userID, noteID int64) error {
	slog.Debug("NoteStore.SoftDelete called", "user_id", userID, "note_id", noteID)

	result, err := s.db.Exec(
		`UPDATE notes SET deleted_at = datetime('now') WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.SoftDelete failed", "err", err)
		return fmt.Errorf("NoteStore.SoftDelete exec: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.SoftDelete: note %d not found or already deleted", noteID)
	}

	return nil
}

// formatSQLiteTimePtr formats a *time.Time for SQLite, returning NULL if nil.
func formatSQLiteTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return formatSQLiteTime(*t)
}
```

Add `"time"` to imports in note_store.go.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run "TestNoteStore_Update|TestNoteStore_SoftDelete" -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore Update and SoftDelete with tests

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: NoteStore — Search (FTS5)

**Files:**
- Modify: `internal/data/note_store.go`
- Modify: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestNoteStore_Search(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	n1 := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "あめ、ame", SourceText: "雨が降っている"}
	n2 := &note.Note{UserID: 1, Type: note.TypeGrammar, Title: "～ている", Content: "持续体", SourceText: "降っている"}
	n3 := &note.Note{UserID: 1, Type: note.TypeSentence, Title: "hello", Content: "not japanese"}
	for _, n := range []*note.Note{n1, n2, n3} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	t.Run("search by title", func(t *testing.T) {
		results, err := store.Search(1, "雨", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len = %d, want 1", len(results))
		}
		if results[0].Title != "雨" {
			t.Errorf("Title = %q", results[0].Title)
		}
	})

	t.Run("search by content", func(t *testing.T) {
		results, err := store.Search(1, "持续体", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len = %d, want 1", len(results))
		}
	})

	t.Run("search by source_text", func(t *testing.T) {
		results, err := store.Search(1, "降っている", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("len = %d, want 2 (matches both 雨 and ～ている source_text)", len(results))
		}
	})

	t.Run("no results", func(t *testing.T) {
		results, err := store.Search(1, "nonexistent", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("len = %d, want 0", len(results))
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_Search -v
```
Expected: FAIL

- [ ] **Step 3: Implement Search**

```go
// Search performs FTS5 full-text search on title, content, and source_text.
// Results are joined with notes table to filter by user and soft-delete.
func (s *NoteStore) Search(userID int64, query string, limit int) ([]note.Note, error) {
	slog.Debug("NoteStore.Search called", "user_id", userID, "query", query)

	rows, err := s.db.Query(
		`SELECT n.id, n.user_id, n.type, n.title, n.content, n.source_text,
		        n.reference_id, n.reference_type, n.tags_json,
		        n.mastery_level, n.next_review_at, n.ease_factor, n.interval,
		        n.review_history_json, n.created_at, n.updated_at
		 FROM notes n
		 JOIN notes_fts fts ON n.id = fts.rowid
		 WHERE notes_fts MATCH ? AND n.user_id = ? AND n.deleted_at IS NULL
		 ORDER BY rank
		 LIMIT ?`,
		query, userID, limit,
	)
	if err != nil {
		slog.Error("NoteStore.Search query failed", "err", err)
		return nil, fmt.Errorf("NoteStore.Search: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("NoteStore.Search scan: %w", err)
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("NoteStore.Search rows: %w", err)
	}

	slog.Debug("NoteStore.Search done", "user_id", userID, "count", len(notes))
	return notes, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_Search -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore Search with FTS5 full-text search

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: NoteStore — Links (Add, Remove, GetOutgoing, GetIncoming)

**Files:**
- Modify: `internal/data/note_store.go`
- Modify: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestNoteStore_Links(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	wordNote := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "ame"}
	grammarNote := &note.Note{UserID: 1, Type: note.TypeGrammar, Title: "～ている", Content: "持续体"}
	sentenceNote := &note.Note{UserID: 1, Type: note.TypeSentence, Title: "雨が降っている", Content: "正在下雨"}
	for _, n := range []*note.Note{wordNote, grammarNote, sentenceNote} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Add links
	t.Run("add link", func(t *testing.T) {
		link, err := store.AddLink(1, grammarNote.ID, wordNote.ID, note.RelationUsesWord)
		if err != nil {
			t.Fatalf("AddLink failed: %v", err)
		}
		if link.ID == 0 {
			t.Error("link ID not set")
		}
		if link.Relation != note.RelationUsesWord {
			t.Errorf("relation = %q", link.Relation)
		}
	})

	// Duplicate should fail (UNIQUE constraint)
	t.Run("duplicate link", func(t *testing.T) {
		_, err := store.AddLink(1, grammarNote.ID, wordNote.ID, note.RelationUsesWord)
		if err == nil {
			t.Error("expected error for duplicate link")
		}
	})

	store.AddLink(1, sentenceNote.ID, wordNote.ID, note.RelationContext)
	store.AddLink(1, sentenceNote.ID, grammarNote.ID, note.RelationUsesGrammar)

	// Get outgoing links
	t.Run("outgoing links", func(t *testing.T) {
		links, err := store.GetOutgoingLinks(1, sentenceNote.ID)
		if err != nil {
			t.Fatalf("GetOutgoingLinks failed: %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("len = %d, want 2", len(links))
		}
	})

	// Get incoming links
	t.Run("incoming links", func(t *testing.T) {
		links, err := store.GetIncomingLinks(1, wordNote.ID)
		if err != nil {
			t.Fatalf("GetIncomingLinks failed: %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("len = %d, want 2 (grammar uses_word + sentence context)", len(links))
		}
	})

	// Remove link
	t.Run("remove link", func(t *testing.T) {
		links, _ := store.GetOutgoingLinks(1, sentenceNote.ID)
		if err := store.RemoveLink(1, links[0].ID); err != nil {
			t.Fatalf("RemoveLink failed: %v", err)
		}
		remaining, _ := store.GetOutgoingLinks(1, sentenceNote.ID)
		if len(remaining) != 1 {
			t.Errorf("len = %d, want 1 after removal", len(remaining))
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_Links -v
```
Expected: FAIL

- [ ] **Step 3: Implement link operations**

```go
// AddLink creates a link between two notes.
func (s *NoteStore) AddLink(userID, noteID, targetNoteID int64, relation note.LinkRelation) (*note.NoteLink, error) {
	slog.Debug("NoteStore.AddLink called", "user_id", userID)

	result, err := s.db.Exec(
		`INSERT INTO note_links (user_id, note_id, target_note_id, relation)
		 VALUES (?, ?, ?, ?)`,
		userID, noteID, targetNoteID, string(relation),
	)
	if err != nil {
		slog.Error("NoteStore.AddLink failed", "err", err)
		return nil, fmt.Errorf("NoteStore.AddLink: %w", err)
	}

	id, _ := result.LastInsertId()
	return &note.NoteLink{
		ID:           id,
		NoteID:       noteID,
		TargetNoteID: targetNoteID,
		Relation:     relation,
	}, nil
}

// RemoveLink deletes a link by ID.
func (s *NoteStore) RemoveLink(userID, linkID int64) error {
	slog.Debug("NoteStore.RemoveLink called", "user_id", userID, "link_id", linkID)

	result, err := s.db.Exec(
		`DELETE FROM note_links WHERE id = ? AND user_id = ?`,
		linkID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.RemoveLink failed", "err", err)
		return fmt.Errorf("NoteStore.RemoveLink: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.RemoveLink: link %d not found", linkID)
	}
	return nil
}

// GetOutgoingLinks returns all links from a note to others, with target note digests populated.
func (s *NoteStore) GetOutgoingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	slog.Debug("NoteStore.GetOutgoingLinks called", "user_id", userID, "note_id", noteID)

	rows, err := s.db.Query(
		`SELECT nl.id, nl.note_id, nl.target_note_id, nl.relation,
		        n.id, n.title, n.type
		 FROM note_links nl
		 JOIN notes n ON nl.target_note_id = n.id
		 WHERE nl.user_id = ? AND nl.note_id = ? AND n.deleted_at IS NULL`,
		userID, noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.GetOutgoingLinks: %w", err)
	}
	defer rows.Close()

	return scanNoteLinks(rows)
}

// GetIncomingLinks returns all links from other notes to this note (backlinks).
func (s *NoteStore) GetIncomingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	slog.Debug("NoteStore.GetIncomingLinks called", "user_id", userID, "note_id", noteID)

	rows, err := s.db.Query(
		`SELECT nl.id, nl.note_id, nl.target_note_id, nl.relation,
		        n.id, n.title, n.type
		 FROM note_links nl
		 JOIN notes n ON nl.note_id = n.id
		 WHERE nl.user_id = ? AND nl.target_note_id = ? AND n.deleted_at IS NULL`,
		userID, noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.GetIncomingLinks: %w", err)
	}
	defer rows.Close()

	return scanNoteLinks(rows)
}

func scanNoteLinks(rows *sql.Rows) ([]note.NoteLink, error) {
	var links []note.NoteLink
	for rows.Next() {
		var l note.NoteLink
		var digest note.NoteDigest
		if err := rows.Scan(&l.ID, &l.NoteID, &l.TargetNoteID, &l.Relation,
			&digest.ID, &digest.Title, &digest.Type); err != nil {
			return nil, fmt.Errorf("scanNoteLinks: %w", err)
		}
		l.TargetNote = &digest
		links = append(links, l)
	}
	return links, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_Links -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore link operations (add, remove, get)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: NoteStore — SRS operations

**Files:**
- Modify: `internal/data/note_store.go`
- Modify: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestNoteStore_SRS(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	// Create a note and promote it
	n := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "ame"}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	t.Run("promote", func(t *testing.T) {
		if err := store.Promote(1, n.ID); err != nil {
			t.Fatalf("Promote failed: %v", err)
		}
		got, _ := store.GetByID(1, n.ID)
		if got.NextReviewAt == nil {
			t.Error("NextReviewAt should be set after promote")
		}
		if got.MasteryLevel != 0 {
			t.Errorf("MasteryLevel = %d, want 0", got.MasteryLevel)
		}
	})

	t.Run("demote", func(t *testing.T) {
		if err := store.Demote(1, n.ID); err != nil {
			t.Fatalf("Demote failed: %v", err)
		}
		got, _ := store.GetByID(1, n.ID)
		if got.NextReviewAt != nil {
			t.Error("NextReviewAt should be nil after demote")
		}
	})

	t.Run("save review", func(t *testing.T) {
		store.Promote(1, n.ID)
		got, _ := store.GetByID(1, n.ID)
		got.MasteryLevel = 2
		got.EaseFactor = 2.5
		got.Interval = 6
		now := time.Now()
		got.NextReviewAt = &now

		if err := store.SaveReview(1, n.ID, *got); err != nil {
			t.Fatalf("SaveReview failed: %v", err)
		}
		updated, _ := store.GetByID(1, n.ID)
		if updated.MasteryLevel != 2 {
			t.Errorf("MasteryLevel = %d, want 2", updated.MasteryLevel)
		}
	})

	t.Run("list due notes", func(t *testing.T) {
		// Promote note to make it due now
		store.Promote(1, n.ID)
		due, err := store.ListDueNotes(1)
		if err != nil {
			t.Fatalf("ListDueNotes failed: %v", err)
		}
		found := false
		for _, dn := range due {
			if dn.ID == n.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("promoted note should appear in due notes")
		}
	})

	t.Run("list archived", func(t *testing.T) {
		// Set note to mastery >= 5, next_review_at = nil (graduated)
		n2 := &note.Note{UserID: 1, Type: note.TypeWord, Title: "graduated", Content: "x"}
		store.Create(n2)
		// Directly update to simulate graduation
		db.Exec(`UPDATE notes SET mastery_level = 5, next_review_at = NULL WHERE id = ?`, n2.ID)

		archived, total, err := store.ListArchived(1, note.NoteListParams{Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("ListArchived failed: %v", err)
		}
		if total < 1 {
			t.Errorf("total = %d, want >= 1", total)
		}
		_ = archived
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_SRS -v
```
Expected: FAIL

- [ ] **Step 3: Implement SRS operations**

```go
// Promote sets next_review_at to now, adding the note to the review queue.
func (s *NoteStore) Promote(userID, noteID int64) error {
	slog.Debug("NoteStore.Promote called", "user_id", userID, "note_id", noteID)

	result, err := s.db.Exec(
		`UPDATE notes SET next_review_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)
	if err != nil {
		return fmt.Errorf("NoteStore.Promote: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.Promote: note %d not found", noteID)
	}
	return nil
}

// Demote removes the note from the review queue by setting next_review_at to NULL.
func (s *NoteStore) Demote(userID, noteID int64) error {
	slog.Debug("NoteStore.Demote called", "user_id", userID, "note_id", noteID)

	result, err := s.db.Exec(
		`UPDATE notes SET next_review_at = NULL, updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)
	if err != nil {
		return fmt.Errorf("NoteStore.Demote: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.Demote: note %d not found", noteID)
	}
	return nil
}

// SaveReview persists SM-2 review results (mastery, interval, EF, next_review_at, history).
func (s *NoteStore) SaveReview(userID, noteID int64, n note.Note) error {
	slog.Debug("NoteStore.SaveReview called", "user_id", userID, "note_id", noteID)

	historyJSON, err := json.Marshal(n.ReviewHistory)
	if err != nil {
		return fmt.Errorf("NoteStore.SaveReview marshal history: %w", err)
	}

	_, err = s.db.Exec(
		`UPDATE notes SET
		    mastery_level = ?, next_review_at = ?, ease_factor = ?, interval = ?,
		    review_history_json = ?, updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		n.MasteryLevel, formatSQLiteTimePtr(n.NextReviewAt), n.EaseFactor,
		n.Interval, string(historyJSON),
		noteID, userID,
	)
	if err != nil {
		return fmt.Errorf("NoteStore.SaveReview: %w", err)
	}
	return nil
}

// ListDueNotes returns notes due for review (next_review_at <= now, not deleted).
func (s *NoteStore) ListDueNotes(userID int64) ([]note.Note, error) {
	slog.Debug("NoteStore.ListDueNotes called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes
		 WHERE user_id = ? AND next_review_at IS NOT NULL AND next_review_at <= datetime('now')
		       AND deleted_at IS NULL
		 ORDER BY next_review_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.ListDueNotes: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("NoteStore.ListDueNotes scan: %w", err)
		}
		notes = append(notes, *n)
	}
	return notes, rows.Err()
}

// ListArchived returns graduated notes (mastery >= 5, next_review_at IS NULL, not deleted).
func (s *NoteStore) ListArchived(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	slog.Debug("NoteStore.ListArchived called", "user_id", userID)

	where := "WHERE user_id = ? AND mastery_level >= 5 AND next_review_at IS NULL AND deleted_at IS NULL"
	args := []interface{}{userID}

	sortCol := "updated_at"
	if params.Sort == "created_at" {
		sortCol = "created_at"
	}
	order := "DESC"
	if params.Order == "asc" {
		order = "ASC"
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notes %s", where)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("NoteStore.ListArchived count: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortCol, order,
	)
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("NoteStore.ListArchived query: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("NoteStore.ListArchived scan: %w", err)
		}
		notes = append(notes, *n)
	}
	return notes, total, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run TestNoteStore_SRS -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore SRS operations (promote, demote, review, archive)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: NoteStore — ListByReference and ListTags

**Files:**
- Modify: `internal/data/note_store.go`
- Modify: `internal/data/note_store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestNoteStore_ListByReference(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	wordID := int64(42)
	refType := "word"

	n1 := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨 note", Content: "x",
		ReferenceID: &wordID, ReferenceType: &refType}
	n2 := &note.Note{UserID: 1, Type: note.TypeSentence, Title: "sentence about 雨", Content: "y",
		ReferenceID: &wordID, ReferenceType: &refType}
	n3 := &note.Note{UserID: 1, Type: note.TypeWord, Title: "other", Content: "z"}
	for _, n := range []*note.Note{n1, n2, n3} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	digests, err := store.ListByReference(1, "word", wordID, 10)
	if err != nil {
		t.Fatalf("ListByReference failed: %v", err)
	}
	if len(digests) != 2 {
		t.Fatalf("len = %d, want 2", len(digests))
	}
	if digests[0].Type != note.TypeWord && digests[1].Type != note.TypeWord {
		t.Error("should contain the word-type note")
	}
}

func TestNoteStore_ListTags(t *testing.T) {
	db := testDB
	store := data.NewNoteStore(db)

	n1 := &note.Note{UserID: 1, Type: note.TypeWord, Title: "a", Content: "x", Tags: []string{"N5", "动词"}}
	n2 := &note.Note{UserID: 1, Type: note.TypeGrammar, Title: "b", Content: "y", Tags: []string{"N5", "易错"}}
	if err := store.Create(n1); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := store.Create(n2); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tags, err := store.ListTags(1)
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("len = %d, want 3", len(tags))
	}
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	for _, want := range []string{"N5", "动词", "易错"} {
		if !tagSet[want] {
			t.Errorf("missing tag %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run "TestNoteStore_ListByReference|TestNoteStore_ListTags" -v
```
Expected: FAIL

- [ ] **Step 3: Implement ListByReference and ListTags**

```go
// ListByReference returns note digests that reference a specific system entity.
func (s *NoteStore) ListByReference(userID int64, refType string, refID int64, limit int) ([]note.NoteDigest, error) {
	slog.Debug("NoteStore.ListByReference called", "user_id", userID, "ref_type", refType, "ref_id", refID)

	rows, err := s.db.Query(
		`SELECT id, title, type FROM notes
		 WHERE user_id = ? AND reference_type = ? AND reference_id = ? AND deleted_at IS NULL
		 ORDER BY updated_at DESC LIMIT ?`,
		userID, refType, refID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.ListByReference: %w", err)
	}
	defer rows.Close()

	var digests []note.NoteDigest
	for rows.Next() {
		var d note.NoteDigest
		if err := rows.Scan(&d.ID, &d.Title, &d.Type); err != nil {
			return nil, fmt.Errorf("NoteStore.ListByReference scan: %w", err)
		}
		digests = append(digests, d)
	}
	return digests, rows.Err()
}

// ListTags returns all distinct tags used by a user across their notes.
func (s *NoteStore) ListTags(userID int64) ([]string, error) {
	slog.Debug("NoteStore.ListTags called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT DISTINCT tags_json FROM notes WHERE user_id = ? AND deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.ListTags: %w", err)
	}
	defer rows.Close()

	tagSet := make(map[string]bool)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			return nil, fmt.Errorf("NoteStore.ListTags scan: %w", err)
		}
		var tags []string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, t := range tags {
			tagSet[t] = true
		}
	}

	result := make([]string, 0, len(tagSet))
	for t := range tagSet {
		result = append(result, t)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/data/ -run "TestNoteStore_ListByReference|TestNoteStore_ListTags" -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/note_store.go internal/data/note_store_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStore ListByReference and ListTags

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: NoteStoreAdapter

**Files:**
- Modify: `internal/data/adapters.go`

- [ ] **Step 1: Add NoteStoreAdapter to adapters.go**

```go
// ── NoteStoreAdapter ──────────────────────────────────────────────────────────

// NoteStoreAdapter wraps NoteStore to satisfy note.NoteStoreInterface.
type NoteStoreAdapter struct {
	s *NoteStore
}

// NewNoteStoreAdapter creates a NoteStoreAdapter.
func NewNoteStoreAdapter(s *NoteStore) *NoteStoreAdapter {
	return &NoteStoreAdapter{s: s}
}

func (a *NoteStoreAdapter) Create(n *note.Note) error {
	return a.s.Create(n)
}

func (a *NoteStoreAdapter) GetByID(userID, noteID int64) (*note.Note, error) {
	return a.s.GetByID(userID, noteID)
}

func (a *NoteStoreAdapter) List(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	return a.s.List(userID, params)
}

func (a *NoteStoreAdapter) Update(n *note.Note) error {
	return a.s.Update(n)
}

func (a *NoteStoreAdapter) SoftDelete(userID, noteID int64) error {
	return a.s.SoftDelete(userID, noteID)
}

func (a *NoteStoreAdapter) Search(userID int64, query string, limit int) ([]note.Note, error) {
	return a.s.Search(userID, query, limit)
}

func (a *NoteStoreAdapter) AddLink(userID, noteID, targetNoteID int64, relation note.LinkRelation) (*note.NoteLink, error) {
	return a.s.AddLink(userID, noteID, targetNoteID, relation)
}

func (a *NoteStoreAdapter) RemoveLink(userID, linkID int64) error {
	return a.s.RemoveLink(userID, linkID)
}

func (a *NoteStoreAdapter) GetOutgoingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	return a.s.GetOutgoingLinks(userID, noteID)
}

func (a *NoteStoreAdapter) GetIncomingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	return a.s.GetIncomingLinks(userID, noteID)
}

func (a *NoteStoreAdapter) Promote(userID, noteID int64) error {
	return a.s.Promote(userID, noteID)
}

func (a *NoteStoreAdapter) Demote(userID, noteID int64) error {
	return a.s.Demote(userID, noteID)
}

func (a *NoteStoreAdapter) SaveReview(userID, noteID int64, n note.Note) error {
	return a.s.SaveReview(userID, noteID, n)
}

func (a *NoteStoreAdapter) ListDueNotes(userID int64) ([]note.Note, error) {
	return a.s.ListDueNotes(userID)
}

func (a *NoteStoreAdapter) ListArchived(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	return a.s.ListArchived(userID, params)
}

func (a *NoteStoreAdapter) ListByReference(userID int64, refType string, refID int64, limit int) ([]note.NoteDigest, error) {
	return a.s.ListByReference(userID, refType, refID, limit)
}

func (a *NoteStoreAdapter) ListTags(userID int64) ([]string, error) {
	return a.s.ListTags(userID)
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./internal/data/
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/data/adapters.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteStoreAdapter bridging NoteStore to NoteStoreInterface

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: NoteService

**Files:**
- Create: `internal/module/note/service.go`
- Create: `internal/module/note/note_test.go` (service tests)

- [ ] **Step 1: Create NoteStoreInterface and NoteService in service.go**

```go
package note

import (
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/sm2"
)

// NoteStoreInterface defines data access methods required by NoteService.
type NoteStoreInterface interface {
	Create(note *Note) error
	GetByID(userID, noteID int64) (*Note, error)
	List(userID int64, params NoteListParams) ([]Note, int, error)
	Update(note *Note) error
	SoftDelete(userID, noteID int64) error
	Search(userID int64, query string, limit int) ([]Note, error)
	AddLink(userID, noteID, targetNoteID int64, relation LinkRelation) (*NoteLink, error)
	RemoveLink(userID, linkID int64) error
	GetOutgoingLinks(userID, noteID int64) ([]NoteLink, error)
	GetIncomingLinks(userID, noteID int64) ([]NoteLink, error)
	Promote(userID, noteID int64) error
	Demote(userID, noteID int64) error
	SaveReview(userID, noteID int64, n Note) error
	ListDueNotes(userID int64) ([]Note, error)
	ListArchived(userID int64, params NoteListParams) ([]Note, int, error)
	ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
	ListTags(userID int64) ([]string, error)
}

// NoteService handles business logic for notes.
type NoteService struct {
	store NoteStoreInterface
}

// NewNoteService creates a NoteService.
func NewNoteService(store NoteStoreInterface) *NoteService {
	return &NoteService{store: store}
}

// Create creates a new note.
func (s *NoteService) Create(n *Note) error {
	slog.Debug("NoteService.Create called", "user_id", n.UserID, "type", n.Type)
	if err := s.store.Create(n); err != nil {
		slog.Error("NoteService.Create failed", "err", err)
		return fmt.Errorf("NoteService.Create: %w", err)
	}
	return nil
}

// GetDetail returns a note with its outgoing and incoming links.
func (s *NoteService) GetDetail(userID, noteID int64) (*NoteDetail, error) {
	slog.Debug("NoteService.GetDetail called", "user_id", userID, "note_id", noteID)

	n, err := s.store.GetByID(userID, noteID)
	if err != nil {
		slog.Error("NoteService.GetDetail: GetByID failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetDetail: %w", err)
	}

	outgoing, err := s.store.GetOutgoingLinks(userID, noteID)
	if err != nil {
		slog.Error("NoteService.GetDetail: GetOutgoingLinks failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetDetail outgoing: %w", err)
	}

	incoming, err := s.store.GetIncomingLinks(userID, noteID)
	if err != nil {
		slog.Error("NoteService.GetDetail: GetIncomingLinks failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetDetail incoming: %w", err)
	}

	return &NoteDetail{
		Note:          *n,
		OutgoingLinks: outgoing,
		IncomingLinks: incoming,
	}, nil
}

// List returns paginated notes with optional filtering.
func (s *NoteService) List(userID int64, params NoteListParams) ([]Note, int, error) {
	slog.Debug("NoteService.List called", "user_id", userID)
	notes, total, err := s.store.List(userID, params)
	if err != nil {
		slog.Error("NoteService.List failed", "err", err)
		return nil, 0, fmt.Errorf("NoteService.List: %w", err)
	}
	return notes, total, nil
}

// Update updates a note.
func (s *NoteService) Update(n *Note) error {
	slog.Debug("NoteService.Update called", "note_id", n.ID)
	if err := s.store.Update(n); err != nil {
		slog.Error("NoteService.Update failed", "err", err)
		return fmt.Errorf("NoteService.Update: %w", err)
	}
	return nil
}

// Delete soft-deletes a note.
func (s *NoteService) Delete(userID, noteID int64) error {
	slog.Debug("NoteService.Delete called", "user_id", userID, "note_id", noteID)
	if err := s.store.SoftDelete(userID, noteID); err != nil {
		slog.Error("NoteService.Delete failed", "err", err)
		return fmt.Errorf("NoteService.Delete: %w", err)
	}
	return nil
}

// Search performs FTS5 search.
func (s *NoteService) Search(userID int64, query string, limit int) ([]Note, error) {
	slog.Debug("NoteService.Search called", "user_id", userID, "query", query)
	notes, err := s.store.Search(userID, query, limit)
	if err != nil {
		slog.Error("NoteService.Search failed", "err", err)
		return nil, fmt.Errorf("NoteService.Search: %w", err)
	}
	return notes, nil
}

// AddLink creates a link between notes.
func (s *NoteService) AddLink(userID, noteID, targetNoteID int64, relation LinkRelation) (*NoteLink, error) {
	slog.Debug("NoteService.AddLink called", "user_id", userID)
	link, err := s.store.AddLink(userID, noteID, targetNoteID, relation)
	if err != nil {
		slog.Error("NoteService.AddLink failed", "err", err)
		return nil, fmt.Errorf("NoteService.AddLink: %w", err)
	}
	return link, nil
}

// RemoveLink deletes a link.
func (s *NoteService) RemoveLink(userID, linkID int64) error {
	slog.Debug("NoteService.RemoveLink called", "user_id", userID)
	if err := s.store.RemoveLink(userID, linkID); err != nil {
		slog.Error("NoteService.RemoveLink failed", "err", err)
		return fmt.Errorf("NoteService.RemoveLink: %w", err)
	}
	return nil
}

// Promote adds the note to the review queue.
func (s *NoteService) Promote(userID, noteID int64) error {
	slog.Debug("NoteService.Promote called", "user_id", userID, "note_id", noteID)
	if err := s.store.Promote(userID, noteID); err != nil {
		slog.Error("NoteService.Promote failed", "err", err)
		return fmt.Errorf("NoteService.Promote: %w", err)
	}
	return nil
}

// Demote removes the note from the review queue.
func (s *NoteService) Demote(userID, noteID int64) error {
	slog.Debug("NoteService.Demote called", "user_id", userID, "note_id", noteID)
	if err := s.store.Demote(userID, noteID); err != nil {
		slog.Error("NoteService.Demote failed", "err", err)
		return fmt.Errorf("NoteService.Demote: %w", err)
	}
	return nil
}

// SubmitRating records a review rating and applies SM-2 scheduling.
// Mastery >= 5 automatically graduates the note (next_review_at = NULL).
func (s *NoteService) SubmitRating(userID, noteID int64, rating sm2.Rating) error {
	slog.Debug("NoteService.SubmitRating called", "user_id", userID, "note_id", noteID, "rating", rating)

	n, err := s.store.GetByID(userID, noteID)
	if err != nil {
		slog.Error("NoteService.SubmitRating: note not found", "err", err)
		return fmt.Errorf("NoteService.SubmitRating: %w", err)
	}

	newMastery, newInterval, newEF, nextReview, newHistory := sm2.CalcNextReview(
		n.MasteryLevel, n.Interval, n.EaseFactor, rating, n.ReviewHistory,
	)

	n.MasteryLevel = newMastery
	n.Interval = newInterval
	n.EaseFactor = newEF
	n.ReviewHistory = newHistory

	if newMastery >= 5 {
		// Graduate: remove from review queue
		n.NextReviewAt = nil
	} else {
		n.NextReviewAt = &nextReview
	}

	if err := s.store.SaveReview(userID, noteID, *n); err != nil {
		slog.Error("NoteService.SubmitRating: SaveReview failed", "err", err)
		return fmt.Errorf("NoteService.SubmitRating: %w", err)
	}

	return nil
}

// GetReviewQueue returns notes due for review.
func (s *NoteService) GetReviewQueue(userID int64) ([]Note, error) {
	slog.Debug("NoteService.GetReviewQueue called", "user_id", userID)
	notes, err := s.store.ListDueNotes(userID)
	if err != nil {
		slog.Error("NoteService.GetReviewQueue failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetReviewQueue: %w", err)
	}
	return notes, nil
}

// ListArchived returns graduated notes.
func (s *NoteService) ListArchived(userID int64, params NoteListParams) ([]Note, int, error) {
	slog.Debug("NoteService.ListArchived called", "user_id", userID)
	return s.store.ListArchived(userID, params)
}

// ListTags returns all tags used by the user.
func (s *NoteService) ListTags(userID int64) ([]string, error) {
	slog.Debug("NoteService.ListTags called", "user_id", userID)
	tags, err := s.store.ListTags(userID)
	if err != nil {
		slog.Error("NoteService.ListTags failed", "err", err)
		return nil, fmt.Errorf("NoteService.ListTags: %w", err)
	}
	return tags, nil
}

// ListByReference returns note digests referencing a system entity.
func (s *NoteService) ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error) {
	slog.Debug("NoteService.ListByReference called", "user_id", userID, "ref_type", refType, "ref_id", refID)
	return s.store.ListByReference(userID, refType, refID, limit)
}

// Recycle brings an archived note back into the review queue (same as Promote).
func (s *NoteService) Recycle(userID, noteID int64) error {
	return s.Promote(userID, noteID)
}
```

- [ ] **Step 2: Write service tests**

```go
package note_test

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/module/note"
)

type fakeNoteStore struct {
	notes   map[int64]*note.Note
	links   []note.NoteLink
	nextID  int64
	linkID  int64
}

func newFakeNoteStore() *fakeNoteStore {
	return &fakeNoteStore{notes: make(map[int64]*note.Note)}
}

func (f *fakeNoteStore) Create(n *note.Note) error {
	f.nextID++
	n.ID = f.nextID
	f.notes[n.ID] = n
	return nil
}

func (f *fakeNoteStore) GetByID(userID, noteID int64) (*note.Note, error) {
	n, ok := f.notes[noteID]
	if !ok || n.UserID != userID {
		return nil, errors.New("not found")
	}
	return n, nil
}

func (f *fakeNoteStore) List(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	var result []note.Note
	for _, n := range f.notes {
		if n.UserID == userID {
			result = append(result, *n)
		}
	}
	return result, len(result), nil
}

func (f *fakeNoteStore) Update(n *note.Note) error {
	if _, ok := f.notes[n.ID]; !ok {
		return errors.New("not found")
	}
	f.notes[n.ID] = n
	return nil
}

func (f *fakeNoteStore) SoftDelete(userID, noteID int64) error {
	delete(f.notes, noteID)
	return nil
}

func (f *fakeNoteStore) Search(userID int64, query string, limit int) ([]note.Note, error) {
	return nil, nil
}

func (f *fakeNoteStore) AddLink(userID, noteID, targetNoteID int64, relation note.LinkRelation) (*note.NoteLink, error) {
	f.linkID++
	l := &note.NoteLink{ID: f.linkID, NoteID: noteID, TargetNoteID: targetNoteID, Relation: relation}
	f.links = append(f.links, *l)
	return l, nil
}

func (f *fakeNoteStore) RemoveLink(userID, linkID int64) error {
	for i, l := range f.links {
		if l.ID == linkID {
			f.links = append(f.links[:i], f.links[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (f *fakeNoteStore) GetOutgoingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	var result []note.NoteLink
	for _, l := range f.links {
		if l.NoteID == noteID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (f *fakeNoteStore) GetIncomingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	var result []note.NoteLink
	for _, l := range f.links {
		if l.TargetNoteID == noteID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (f *fakeNoteStore) Promote(userID, noteID int64) error  { return nil }
func (f *fakeNoteStore) Demote(userID, noteID int64) error   { return nil }
func (f *fakeNoteStore) SaveReview(userID, noteID int64, n note.Note) error { return nil }
func (f *fakeNoteStore) ListDueNotes(userID int64) ([]note.Note, error)      { return nil, nil }
func (f *fakeNoteStore) ListArchived(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	return nil, 0, nil
}
func (f *fakeNoteStore) ListByReference(userID int64, refType string, refID int64, limit int) ([]note.NoteDigest, error) {
	return nil, nil
}
func (f *fakeNoteStore) ListTags(userID int64) ([]string, error) { return nil, nil }

func TestNoteService_CreateAndGetDetail(t *testing.T) {
	store := newFakeNoteStore()
	svc := note.NewNoteService(store)

	n := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "ame"}
	if err := svc.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if n.ID == 0 {
		t.Error("ID not set after create")
	}

	// Add some links
	svc.AddLink(1, n.ID, 99, note.RelationRelated)
	svc.AddLink(1, 99, n.ID, note.RelationContext)

	detail, err := svc.GetDetail(1, n.ID)
	if err != nil {
		t.Fatalf("GetDetail failed: %v", err)
	}
	if detail.Title != "雨" {
		t.Errorf("Title = %q", detail.Title)
	}
	if len(detail.OutgoingLinks) != 1 {
		t.Errorf("OutgoingLinks len = %d, want 1", len(detail.OutgoingLinks))
	}
	if len(detail.IncomingLinks) != 1 {
		t.Errorf("IncomingLinks len = %d, want 1", len(detail.IncomingLinks))
	}
}
```

- [ ] **Step 3: Run service tests**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/module/note/ -v
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/module/note/service.go internal/module/note/note_test.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteService with business logic and interface

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: NoteHandler

**Files:**
- Create: `internal/module/note/handler.go`

- [ ] **Step 1: Create handler.go with all route handlers**

```go
package note

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/sm2"
)

// NoteHandler handles HTTP requests for the note module.
type NoteHandler struct {
	svc *NoteService
}

// NewNoteHandler creates a NoteHandler.
func NewNoteHandler(svc *NoteService) *NoteHandler {
	return &NoteHandler{svc: svc}
}

// RegisterRoutes registers note routes onto the provided mux.
func (h *NoteHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/notes", h.handleList)
	mux.HandleFunc("POST /api/v1/notes", h.handleCreate)
	mux.HandleFunc("GET /api/v1/notes/search", h.handleSearch)
	mux.HandleFunc("GET /api/v1/notes/tags", h.handleListTags)
	mux.HandleFunc("GET /api/v1/notes/archive", h.handleListArchived)
	mux.HandleFunc("GET /api/v1/notes/review-queue", h.handleGetReviewQueue)
	mux.HandleFunc("GET /api/v1/notes/{id}", h.handleGetDetail)
	mux.HandleFunc("PUT /api/v1/notes/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/notes/{id}", h.handleDelete)
	mux.HandleFunc("POST /api/v1/notes/{id}/links", h.handleAddLink)
	mux.HandleFunc("DELETE /api/v1/notes/{id}/links/{linkId}", h.handleRemoveLink)
	mux.HandleFunc("POST /api/v1/notes/{id}/promote", h.handlePromote)
	mux.HandleFunc("DELETE /api/v1/notes/{id}/promote", h.handleDemote)
	mux.HandleFunc("POST /api/v1/notes/{id}/review", h.handleReview)
	mux.HandleFunc("POST /api/v1/notes/{id}/recycle", h.handleRecycle)
}

func (h *NoteHandler) userID(r *http.Request) (int64, bool) {
	return user.UserIDFromContext(r.Context())
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handleList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	params := NoteListParams{
		Type:  NoteType(r.URL.Query().Get("type")),
		Tag:   r.URL.Query().Get("tag"),
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}
	if params.Sort == "" {
		params.Sort = "updated_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	params.Offset = (page - 1) * size
	params.Limit = size

	notes, total, err := h.svc.List(userID, params)
	if err != nil {
		slog.Error("handleList failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list notes", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]interface{}{
		"items": notes, "total": total, "page": page, "size": size,
	}})
}

func (h *NoteHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var n Note
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	n.UserID = userID

	if err := h.svc.Create(&n); err != nil {
		slog.Error("handleCreate failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to create note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: n})
}

func (h *NoteHandler) handleGetDetail(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	detail, err := h.svc.GetDetail(userID, noteID)
	if err != nil {
		slog.Error("handleGetDetail failed", "err", err)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "note not found", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: detail})
}

func (h *NoteHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	var n Note
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	n.ID = noteID
	n.UserID = userID

	if err := h.svc.Update(&n); err != nil {
		slog.Error("handleUpdate failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to update note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Delete(userID, noteID); err != nil {
		slog.Error("handleDelete failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to delete note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// ── Search ────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "missing q parameter", "")
		return
	}

	notes, err := h.svc.Search(userID, query, 50)
	if err != nil {
		slog.Error("handleSearch failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "search failed", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: notes})
}

// ── Links ─────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handleAddLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	var req struct {
		TargetNoteID int64        `json:"target_note_id"`
		Relation     LinkRelation `json:"relation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	link, err := h.svc.AddLink(userID, noteID, req.TargetNoteID, req.Relation)
	if err != nil {
		slog.Error("handleAddLink failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to create link", "")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: link})
}

func (h *NoteHandler) handleRemoveLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	linkID, err := strconv.ParseInt(r.PathValue("linkId"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid link id", "")
		return
	}

	if err := h.svc.RemoveLink(userID, linkID); err != nil {
		slog.Error("handleRemoveLink failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to remove link", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// ── SRS ───────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handlePromote(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Promote(userID, noteID); err != nil {
		slog.Error("handlePromote failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to promote note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleDemote(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Demote(userID, noteID); err != nil {
		slog.Error("handleDemote failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to demote note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleReview(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	var req struct {
		Rating sm2.Rating `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	if err := h.svc.SubmitRating(userID, noteID, req.Rating); err != nil {
		slog.Error("handleReview failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to record review", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleRecycle(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Recycle(userID, noteID); err != nil {
		slog.Error("handleRecycle failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to recycle note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleGetReviewQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	notes, err := h.svc.GetReviewQueue(userID)
	if err != nil {
		slog.Error("handleGetReviewQueue failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load review queue", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: notes})
}

func (h *NoteHandler) handleListArchived(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	params := NoteListParams{
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}
	if params.Sort == "" {
		params.Sort = "updated_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	params.Offset = (page - 1) * size
	params.Limit = size

	notes, total, err := h.svc.ListArchived(userID, params)
	if err != nil {
		slog.Error("handleListArchived failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list archived notes", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]interface{}{
		"items": notes, "total": total, "page": page, "size": size,
	}})
}

func (h *NoteHandler) handleListTags(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	tags, err := h.svc.ListTags(userID)
	if err != nil {
		slog.Error("handleListTags failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list tags", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: tags})
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./internal/module/note/
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/module/note/handler.go && \
git commit -m "$(cat <<'EOF'
feat(note): add NoteHandler with all CRUD, link, SRS, and search routes

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 14: Route registration in main.go

**Files:**
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Wire note module in main.go**

Add import:
```go
"japanese-learning-app/internal/module/note"
```

Add store + adapter + service + handler (after the existing setups, around line 103):
```go
noteStore    := data.NewNoteStore(db)
noteAdapter  := data.NewNoteStoreAdapter(noteStore)
noteSvc      := note.NewNoteService(noteAdapter)
noteH        := note.NewNoteHandler(noteSvc)
```

Register routes in `protectedMux` (after the existing registrations, around line 128):
```go
noteH.RegisterRoutes(protectedMux)
```

Add auth middleware wrappers for notes routes (after the existing mux.Handle lines, around line 139):
```go
mux.Handle("/api/v1/notes", user.AuthMiddleware(jwtSecret, protectedMux))
mux.Handle("/api/v1/notes/", user.AuthMiddleware(jwtSecret, protectedMux))
```

- [ ] **Step 2: Build the server binary to verify wiring**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./backend/cmd/server/
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add backend/cmd/server/main.go && \
git commit -m "$(cat <<'EOF'
feat(server): wire note module routes in main.go

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 15: Word detail endpoint with related_notes

**Files:**
- Modify: `internal/module/word/handler.go`
- Modify: `internal/module/word/service.go`

- [ ] **Step 1: Add NoteDigestProvider interface to word service**

In `internal/module/word/service.go`, add below `WordStoreInterface`:

```go
// NoteDigestProvider is the subset of note store needed for cross-module queries.
type NoteDigestProvider interface {
	ListByReference(userID int64, refType string, refID int64, limit int) ([]word.NoteDigest, error)
}
```

Hmm, we can't import `word` from `note` without a circular dependency. Instead, define a local type.

Actually, let me check: the cross-module injection adds related_notes to word/grammar detail responses. The cleanest approach is to add a generic interface.

Actually, the simplest approach following the project's pattern: define a small interface in the word service package that the note adapter can satisfy. Since the return type `NoteDigest` is in the `note` package, we need to handle the circular dep.

The cleanest way: the word service doesn't need to know about NoteDigest at all. The handler calls note service directly.

Let me reconsider. The word handler calls `wordSvc.GetByID(wordID)` which doesn't currently exist. Let me add a word detail endpoint that the HANDLER enriches with note data:

```go
// In word handler:
func (h *WordHandler) handleGetWord(w http.ResponseWriter, r *http.Request) {
    wordID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
    word, err := h.svc.GetByID(wordID)
    // ...
    // Then enrich with related notes if a noteService is available
    notes, _ := h.noteSvc.ListByReference(userID, "word", wordID, 5)
    // Return enriched response
}
```

But this requires the word handler to have a reference to the note service. That's coupling the handler. Let me use a cleaner approach.

Alternative: Use an optional `NoteEnricher` interface in the word handler. Or just keep it simple and have the handler accept an optional note service reference.

Actually, looking at the project's simplicity focus: let me just pass the noteService to the word handler as an optional dependency. If nil, no enrichment. Simple and direct.

```go
type WordHandler struct {
    svc     *WordService
    noteSvc NoteDigestProvider // optional, nil = no enrichment
}
```

Let me define a minimal interface:

```go
// NoteDigestProvider is satisfied by *note.NoteService for cross-module enrichment.
type NoteDigestProvider interface {
    ListByReference(userID int64, refType string, refID int64, limit int) ([]note.NoteDigest, error)
}
```

Wait, word handler can't import note types. Let me use a generic approach:

```go
// In word handler:
type NoteDigestProvider interface {
    ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
}

// And define a local NoteDigest that matches note.NoteDigest
type NoteDigest struct {
    ID    int64  `json:"id"`
    Title string `json:"title"`
    Type  string `json:"type"`
}
```

But this duplicates the type. For the project's simplicity, let me just use `interface{}` or define a struct in the word package.

OK, I think the simplest approach:

1. Define `NoteDigest` struct and `NoteDigestProvider` interface in the word handler
2. The handler calls it directly to enrich the response
3. In main.go, we cast noteSvc to this interface and pass it

Let me update the plan accordingly.

- [ ] **Step 1: Update word handler to add detail endpoint with optional note enrichment**

In `internal/module/word/handler.go`:

Add a field and constructor change:
```go
// NoteDigestProvider is the optional interface for cross-module note enrichment.
type NoteDigestProvider interface {
	ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
}

// NoteDigest is a lightweight note reference for cross-module responses.
type NoteDigest struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type WordHandler struct {
	svc     *WordService
	noteSvc NoteDigestProvider // optional
}

func NewWordHandler(svc *WordService) *WordHandler {
	return &WordHandler{svc: svc}
}

func NewWordHandlerWithNotes(svc *WordService, noteSvc NoteDigestProvider) *WordHandler {
	return &WordHandler{svc: svc, noteSvc: noteSvc}
}
```

Add route and handler:
```go
// In RegisterRoutes:
mux.HandleFunc("GET /api/v1/words/{id}", h.handleGetWord)

func (h *WordHandler) handleGetWord(w http.ResponseWriter, r *http.Request) {
	wordID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid word id", "")
		return
	}

	wrd, err := h.svc.GetByID(wordID)
	if err != nil {
		slog.Error("handleGetWord failed", "err", err, "word_id", wordID)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "word not found", "")
		return
	}

	type enriched struct {
		Word
		RelatedNotes []NoteDigest `json:"related_notes"`
	}
	response := enriched{Word: *wrd}

	if h.noteSvc != nil {
		userID, _ := user.UserIDFromContext(r.Context())
		notes, err := h.noteSvc.ListByReference(userID, "word", wordID, 5)
		if err == nil {
			response.RelatedNotes = notes
		}
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: response})
}
```

- [ ] **Step 2: Add GetByID to WordService if not already exposed**

Check if `wordSvc.GetByID(id)` is already public. Looking at the service, there's no direct `GetByID` on WordService — it's only on the store. Add it:

```go
// In word service.go:
func (s *WordService) GetByID(id int64) (*Word, error) {
	return s.store.GetByID(id)
}
```

- [ ] **Step 3: Add needed NoteStoreAdapter method to satisfy word.NoteDigestProvider**

The word handler's `NoteDigestProvider` interface requires:
```go
ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
```

The NoteStoreAdapter already has this method, but it returns `[]note.NoteDigest`. We need the word handler to use a local `NoteDigest` type. Create an adapter in main.go or in the word handler.

Actually, the simplest approach: make the word handler's `NoteDigest` and `NoteDigestProvider` match what the note service returns exactly. But note.NoteDigest has `Type NoteType` (string), while word's NoteDigest has `Type string`. These are compatible at the JSON level.

The cleanest way: have note.NoteService implement a method that returns a type-compatible slice. But Go doesn't have structural typing for slices of structs.

Let me take the simplest approach: create a tiny adapter in main.go:

```go
// In main.go, after creating noteSvc:
type wordNoteProvider struct {
	svc *note.NoteService
}

func (p *wordNoteProvider) ListByReference(userID int64, refType string, refID int64, limit int) ([]word.NoteDigest, error) {
	digests, err := p.svc.ListByReference(userID, refType, refID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]word.NoteDigest, len(digests))
	for i, d := range digests {
		result[i] = word.NoteDigest{ID: d.ID, Title: d.Title, Type: string(d.Type)}
	}
	return result, nil
}

// Then:
wordH := word.NewWordHandlerWithNotes(wordSvc, &wordNoteProvider{svc: noteSvc})
```

This is clean and keeps the word package from depending on the note package.

- [ ] **Step 4: Verify compilation**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./backend/cmd/server/
```
Expected: no errors

- [ ] **Step 5: Run all tests**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/... -v 2>&1 | tail -30
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/module/word/handler.go internal/module/word/service.go backend/cmd/server/main.go && \
git commit -m "$(cat <<'EOF'
feat(word): add word detail endpoint with related notes enrichment

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 16: Grammar detail enrichment

**Files:**
- Modify: `internal/module/grammar/handler.go`

- [ ] **Step 1: Add optional note enrichment to grammar handler**

Same pattern as word handler. Add NoteDigestProvider interface and optional constructor:

```go
// NoteDigestProvider is the optional interface for cross-module note enrichment.
type NoteDigestProvider interface {
	ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
}

// NoteDigest is a lightweight note reference.
type NoteDigest struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type GrammarHandler struct {
	svc     *GrammarService
	noteSvc NoteDigestProvider
}

func NewGrammarHandler(svc *GrammarService) *GrammarHandler {
	return &GrammarHandler{svc: svc}
}

func NewGrammarHandlerWithNotes(svc *GrammarService, noteSvc NoteDigestProvider) *GrammarHandler {
	return &GrammarHandler{svc: svc, noteSvc: noteSvc}
}
```

Update `handleGetPoint` to enrich response:
```go
func (h *GrammarHandler) handleGetPoint(w http.ResponseWriter, r *http.Request) {
	// ... existing code to load grammar point ...

	type enriched struct {
		GrammarPoint
		RelatedNotes []NoteDigest `json:"related_notes"`
	}
	response := enriched{GrammarPoint: *p}

	if h.noteSvc != nil {
		userID, _ := user.UserIDFromContext(r.Context())
		notes, err := h.noteSvc.ListByReference(userID, "grammar", id, 5)
		if err == nil {
			response.RelatedNotes = notes
		}
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: response})
}
```

- [ ] **Step 2: Add grammarNoteProvider adapter in main.go**

```go
type grammarNoteProvider struct {
	svc *note.NoteService
}

func (p *grammarNoteProvider) ListByReference(userID int64, refType string, refID int64, limit int) ([]grammar.NoteDigest, error) {
	digests, err := p.svc.ListByReference(userID, refType, refID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]grammar.NoteDigest, len(digests))
	for i, d := range digests {
		result[i] = grammar.NoteDigest{ID: d.ID, Title: d.Title, Type: string(d.Type)}
	}
	return result, nil
}

// Update constructor:
grammarH := grammar.NewGrammarHandlerWithNotes(grammarSvc, &grammarNoteProvider{svc: noteSvc})
```

- [ ] **Step 3: Verify compilation and tests**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./backend/cmd/server/ && go test ./internal/... -v 2>&1 | tail -30
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/module/grammar/handler.go backend/cmd/server/main.go && \
git commit -m "$(cat <<'EOF'
feat(grammar): add related notes enrichment to grammar detail endpoint

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 17: Unified review queue

**Files:**
- Create: `internal/module/review/handler.go` (or add to note handler)
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Define ReviewCard and related types**

Since the unified queue merges word cards and note cards, define the types in the review handler:

```go
package review

import (
	"net/http"
	"sort"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

// ReviewCard is a unified review queue card that can be either a word or a note.
type ReviewCard struct {
	CardType string          `json:"card_type"` // "word" | "note"
	WordCard *word.WordCard  `json:"word_card,omitempty"`
	NoteCard *note.Note      `json:"note_card,omitempty"`
	IsNew    bool            `json:"is_new"`
}

// ReviewHandler serves the unified review queue.
type ReviewHandler struct {
	wordSvc *word.WordService
	noteSvc *note.NoteService
}

func NewReviewHandler(wordSvc *word.WordService, noteSvc *note.NoteService) *ReviewHandler {
	return &ReviewHandler{wordSvc: wordSvc, noteSvc: noteSvc}
}

func (h *ReviewHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/review/queue", h.handleGetQueue)
}

func (h *ReviewHandler) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var cards []ReviewCard

	// Word cards from existing review queue
	level := word.JLPTLevel(r.URL.Query().Get("level"))
	if level == "" {
		level = word.LevelN5
	}
	wordCards, err := h.wordSvc.GetReviewQueue(userID, level)
	if err == nil {
		for _, wc := range wordCards {
			cards = append(cards, ReviewCard{
				CardType: "word",
				WordCard: &wc,
				IsNew:    wc.IsNew,
			})
		}
	}

	// Note cards due for review
	noteCards, err := h.noteSvc.GetReviewQueue(userID)
	if err == nil {
		for _, nc := range noteCards {
			cards = append(cards, ReviewCard{
				CardType: "note",
				NoteCard: &nc,
				IsNew:    nc.NextReviewAt == nil,
			})
		}
	}

	// Sort: non-nil next_review_at first (ascending), nil (new) cards at end
	sort.Slice(cards, func(i, j int) bool {
		ti := reviewTime(cards[i])
		tj := reviewTime(cards[j])
		if ti == nil && tj == nil {
			return false
		}
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.Before(*tj)
	})

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: cards})
}

func reviewTime(c ReviewCard) *time.Time {
	if c.CardType == "word" && c.WordCard != nil {
		return &c.WordCard.Record.NextReviewAt
	}
	if c.CardType == "note" && c.NoteCard != nil {
		return c.NoteCard.NextReviewAt
	}
	return nil
}
```

- [ ] **Step 2: Wire review handler in main.go**

```go
import "japanese-learning-app/internal/module/review"

reviewH := review.NewReviewHandler(wordSvc, noteSvc)
reviewH.RegisterRoutes(protectedMux)

// Add middleware:
mux.Handle("/api/v1/review/", user.AuthMiddleware(jwtSecret, protectedMux))
```

- [ ] **Step 3: Build and verify**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build ./backend/cmd/server/ && go vet ./internal/...
```
Expected: no errors

- [ ] **Step 4: Commit**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && \
git add internal/module/review/ backend/cmd/server/main.go && \
git commit -m "$(cat <<'EOF'
feat(review): add unified review queue merging word and note cards

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Final Verification

- [ ] **Run full test suite**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go test ./internal/... -v 2>&1 | grep -E "PASS|FAIL|ok"
```
Expected: all PASS

- [ ] **Build the server binary**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go build -o /dev/null ./backend/cmd/server/
```
Expected: no errors

- [ ] **Run go vet**

```bash
cd /home/tylerhu/github_project/japanese-learning-app && go vet ./internal/...
```
Expected: no warnings
