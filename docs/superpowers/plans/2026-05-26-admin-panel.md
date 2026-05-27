# Admin Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone admin panel (Go binary + React SPA) for managing words, grammar, speaking, writing, translation content, users, and learning records.

**Architecture:** Independent Go binary `backend/cmd/admin/` on `:8082` + independent React SPA `front/admin/`. Auth via `ADMIN_TOKEN` env var. Reuses `internal/data/` stores. CLI import functions refactored to accept `io.Reader`.

**Tech Stack:** Go stdlib `net/http`, React 18 + TypeScript + Vite + CSS Modules

---

## File Structure Overview

```
backend/cmd/admin/main.go            ← CREATE  admin server entrypoint
internal/module/admin/                ← CREATE  admin HTTP handlers
  handler.go                          ← shared types, ServeMux setup
  words.go                            ← word CRUD handler
  grammar.go                          ← grammar CRUD handler
  speaking.go                         ← speaking CRUD handler
  writing.go                          ← writing CRUD handler
  translation.go                      ← translation CRUD handler
  users.go                            ← users list + stats handler
  records.go                          ← records list handler
  import.go                           ← bulk JSON import handler
internal/cli/                         ← MODIFY  refactor to accept io.Reader
  import_words.go
  import_grammar.go
  import_speaking.go
  import_writing.go
internal/data/                        ← MODIFY  add Update/Delete/ListAll methods
  word_store.go
  grammar_store.go
  speaking_store.go
  writing_store.go
  translation_store.go
  user_store.go
front/admin/                           ← CREATE  React SPA
  package.json, tsconfig.json, vite.config.ts, index.html
  src/
    main.tsx, App.tsx
    api/client.ts
    pages/Login/Login.tsx, Login.module.css
    pages/Words/Words.tsx, Words.module.css
    pages/Grammar/Grammar.tsx, Grammar.module.css
    pages/Speaking/Speaking.tsx, Speaking.module.css
    pages/Writing/Writing.tsx, Writing.module.css
    pages/Translation/Translation.tsx, Translation.module.css
    pages/Users/Users.tsx, Users.module.css
    pages/Records/Records.tsx, Records.module.css
    components/Layout/Layout.tsx, Layout.module.css
    components/Modal/Modal.tsx, Modal.module.css
    components/Table/Table.tsx, Table.module.css
```

---

### Task 1: CLI Import Refactor — accept io.Reader

**Files:**
- Modify: `internal/cli/import_words.go`
- Modify: `internal/cli/import_grammar.go`
- Modify: `internal/cli/import_speaking.go`
- Modify: `internal/cli/import_writing.go`

- [ ] **Step 1: Refactor import_words.go**

In `insertWords` (or the equivalent), the file-reading logic currently does `os.ReadFile` then `json.Unmarshal`. Extract the JSON-decoding + insert portion into an exported function accepting `io.Reader`:

In `internal/cli/import_words.go`, add:

```go
// ImportWords reads a JSON array of words from r and inserts them into the database.
func ImportWords(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportWords called")
	var items []wordImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportWords decode: %w", err)
	}
	return insertWords(db, items)
}
```

Modify existing `ImportWordsFromFile`:

```go
func ImportWordsFromFile(db *sql.DB, filePath string) (int, error) {
	slog.Debug("ImportWordsFromFile called", "file", filePath)
	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("cli.ImportWordsFromFile open: %w", err)
	}
	defer f.Close()
	return ImportWords(db, f)
}
```

Replace the file reading in the current `ImportWordsFromFile` body with the delegate call above.

- [ ] **Step 2: Refactor import_grammar.go**

Same pattern:

```go
func ImportGrammar(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportGrammar called")
	var items []grammarImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportGrammar decode: %w", err)
	}
	return insertGrammarPoints(db, items)
}
```

- [ ] **Step 3: Refactor import_speaking.go**

```go
func ImportSpeaking(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportSpeaking called")
	var items []speakingImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportSpeaking decode: %w", err)
	}
	return insertSpeakingMaterials(db, items)
}
```

- [ ] **Step 4: Refactor import_writing.go**

```go
func ImportWriting(db *sql.DB, r io.Reader) (int, error) {
	slog.Debug("ImportWriting called")
	var items []writingImport
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("cli.ImportWriting decode: %w", err)
	}
	return insertWritingQuestions(db, items)
}
```

- [ ] **Step 5: Verify compilation**

```bash
cd backend && go build ./...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/cli/import_words.go internal/cli/import_grammar.go internal/cli/import_speaking.go internal/cli/import_writing.go
git commit -m "refactor(cli): extract Import* functions to accept io.Reader"
```

---

### Task 2: WordStore — Add UpdateWord, DeleteWord, ListAll

**Files:**
- Modify: `internal/data/word_store.go`

- [ ] **Step 1: Add ListAll method**

```go
// ListAll returns words with optional level filter, text search, and pagination.
func (s *WordStore) ListAll(level word.JLPTLevel, search string, offset, limit int) ([]word.Word, int, error) {
	slog.Debug("WordStore.ListAll called", "level", level, "search", search, "offset", offset, "limit", limit)

	var args []any
	where := "WHERE 1=1"
	if level != "" {
		where += " AND jlpt_level = ?"
		args = append(args, level)
	}
	if search != "" {
		where += " AND (kanji_form LIKE ? OR reading LIKE ? OR meaning LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s, s)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM words " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("data.WordStore.ListAll count: %w", err)
	}

	query := fmt.Sprintf(
		"SELECT id, kanji_form, reading, part_of_speech, meaning, jlpt_level, examples_json, reading_type FROM words %s ORDER BY id LIMIT ? OFFSET ?",
		where,
	)
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("data.WordStore.ListAll query: %w", err)
	}
	defer rows.Close()

	var words []word.Word
	for rows.Next() {
		var w word.Word
		var examplesJSON string
		if err := rows.Scan(&w.ID, &w.KanjiForm, &w.Reading, &w.PartOfSpeech, &w.Meaning, &w.JLPTLevel, &examplesJSON, &w.ReadingType); err != nil {
			return nil, 0, fmt.Errorf("data.WordStore.ListAll scan: %w", err)
		}
		if err := json.Unmarshal([]byte(examplesJSON), &w.Examples); err != nil {
			return nil, 0, fmt.Errorf("data.WordStore.ListAll unmarshal examples: %w", err)
		}
		words = append(words, w)
	}
	return words, total, rows.Err()
}
```

- [ ] **Step 2: Add UpdateWord method**

```go
// UpdateWord updates all fields of an existing word by ID.
func (s *WordStore) UpdateWord(w word.Word) error {
	slog.Debug("WordStore.UpdateWord called", "word_id", w.ID)
	examplesJSON, err := json.Marshal(w.Examples)
	if err != nil {
		return fmt.Errorf("data.WordStore.UpdateWord marshal examples: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE words SET kanji_form=?, reading=?, part_of_speech=?, meaning=?, jlpt_level=?, examples_json=?, reading_type=? WHERE id=?`,
		w.KanjiForm, w.Reading, w.PartOfSpeech, w.Meaning, w.JLPTLevel, string(examplesJSON), w.ReadingType, w.ID,
	)
	if err != nil {
		return fmt.Errorf("data.WordStore.UpdateWord exec: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Add DeleteWord method**

```go
// DeleteWord deletes a word by ID.
func (s *WordStore) DeleteWord(id int64) error {
	slog.Debug("WordStore.DeleteWord called", "word_id", id)
	_, err := s.db.Exec("DELETE FROM words WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("data.WordStore.DeleteWord exec: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd backend && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/data/word_store.go
git commit -m "feat(data): add ListAll, UpdateWord, DeleteWord to WordStore"
```

---

### Task 3: GrammarStore — Add UpdatePoint, DeletePoint

**Files:**
- Modify: `internal/data/grammar_store.go`

- [ ] **Step 1: Add UpdatePoint**

```go
// UpdatePoint updates all fields of an existing grammar point by ID.
func (s *GrammarStore) UpdatePoint(gp grammar.GrammarPoint) error {
	slog.Debug("GrammarStore.UpdatePoint called", "grammar_point_id", gp.ID)
	examplesJSON, err := json.Marshal(gp.Examples)
	if err != nil {
		return fmt.Errorf("data.GrammarStore.UpdatePoint marshal examples: %w", err)
	}
	quizJSON, err := json.Marshal(gp.QuizQuestions)
	if err != nil {
		return fmt.Errorf("data.GrammarStore.UpdatePoint marshal quiz: %w", err)
	}
	_, err = s.db.Exec(
		"UPDATE grammar_points SET name=?, meaning=?, conjunction_rule=?, usage_note=?, examples_json=?, quiz_questions_json=?, jlpt_level=? WHERE id=?",
		gp.Name, gp.Meaning, gp.ConjunctionRule, gp.UsageNote, string(examplesJSON), string(quizJSON), gp.JLPTLevel, gp.ID,
	)
	if err != nil {
		return fmt.Errorf("data.GrammarStore.UpdatePoint exec: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Add DeletePoint**

```go
// DeletePoint deletes a grammar point by ID.
func (s *GrammarStore) DeletePoint(id int64) error {
	slog.Debug("GrammarStore.DeletePoint called", "grammar_point_id", id)
	_, err := s.db.Exec("DELETE FROM grammar_points WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("data.GrammarStore.DeletePoint exec: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Verify compilation and commit**

```bash
cd backend && go build ./... && git add internal/data/grammar_store.go && git commit -m "feat(data): add UpdatePoint, DeletePoint to GrammarStore"
```

---

### Task 4: SpeakingStore + WritingStore + TranslationStore + UserStore — Add CRUD methods

**Files:**
- Modify: `internal/data/speaking_store.go`
- Modify: `internal/data/writing_store.go`
- Modify: `internal/data/translation_store.go`
- Modify: `internal/data/user_store.go`

- [ ] **Step 1: SpeakingStore — UpdateMaterial, DeleteMaterial**

In `internal/data/speaking_store.go`:

```go
func (s *SpeakingStore) UpdateMaterial(m speaking.SpeakingMaterial) error {
	slog.Debug("SpeakingStore.UpdateMaterial called", "id", m.ID)
	_, err := s.db.Exec(
		"UPDATE speaking_materials SET type=?, title=?, text=?, audio_url=?, jlpt_level=? WHERE id=?",
		m.Type, m.Title, m.Text, m.AudioURL, m.JLPTLevel, m.ID,
	)
	if err != nil {
		return fmt.Errorf("data.SpeakingStore.UpdateMaterial exec: %w", err)
	}
	return nil
}

func (s *SpeakingStore) DeleteMaterial(id int64) error {
	slog.Debug("SpeakingStore.DeleteMaterial called", "id", id)
	_, err := s.db.Exec("DELETE FROM speaking_materials WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("data.SpeakingStore.DeleteMaterial exec: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: WritingStore — UpdateQuestion, DeleteQuestion, ListAllQuestions**

In `internal/data/writing_store.go`:

```go
func (s *WritingStore) UpdateQuestion(q writing.WritingQuestion) error {
	slog.Debug("WritingStore.UpdateQuestion called", "id", q.ID)
	_, err := s.db.Exec(
		"UPDATE writing_questions SET type=?, prompt=?, expected_answer=?, grammar_point_id=?, jlpt_level=? WHERE id=?",
		q.Type, q.Prompt, q.ExpectedAnswer, q.GrammarPointID, q.JLPTLevel, q.ID,
	)
	if err != nil {
		return fmt.Errorf("data.WritingStore.UpdateQuestion exec: %w", err)
	}
	return nil
}

func (s *WritingStore) DeleteQuestion(id int64) error {
	slog.Debug("WritingStore.DeleteQuestion called", "id", id)
	_, err := s.db.Exec("DELETE FROM writing_questions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("data.WritingStore.DeleteQuestion exec: %w", err)
	}
	return nil
}

func (s *WritingStore) ListAllQuestions(level, qtype string, offset, limit int) ([]writing.WritingQuestion, int, error) {
	where := "WHERE 1=1"
	var args []any
	if level != "" {
		where += " AND jlpt_level = ?"
		args = append(args, level)
	}
	if qtype != "" {
		where += " AND type = ?"
		args = append(args, qtype)
	}
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM writing_questions "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("data.WritingStore.ListAllQuestions count: %w", err)
	}
	query := fmt.Sprintf("SELECT id, type, prompt, grammar_point_id, jlpt_level, expected_answer FROM writing_questions %s ORDER BY id LIMIT ? OFFSET ?", where)
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("data.WritingStore.ListAllQuestions query: %w", err)
	}
	defer rows.Close()
	var questions []writing.WritingQuestion
	for rows.Next() {
		var q writing.WritingQuestion
		var gpid sql.NullInt64
		if err := rows.Scan(&q.ID, &q.Type, &q.Prompt, &gpid, &q.JLPTLevel, &q.ExpectedAnswer); err != nil {
			return nil, 0, fmt.Errorf("data.WritingStore.ListAllQuestions scan: %w", err)
		}
		if gpid.Valid {
			q.GrammarPointID = gpid.Int64
		}
		questions = append(questions, q)
	}
	return questions, total, rows.Err()
}
```

- [ ] **Step 3: TranslationStore — UpdateSentence, DeleteSentence, ListAllSentences**

Read the existing translation_store.go, add:

```go
func (s *TranslationStore) UpdateSentence(ts translation.TranslationSentence) error {
	_, err := s.db.Exec(
		"UPDATE translation_sentences SET source_text=?, reference_translation=?, direction=?, position=? WHERE id=?",
		ts.SourceText, ts.ReferenceTranslation, ts.Direction, ts.Position, ts.ID,
	)
	if err != nil {
		return fmt.Errorf("data.TranslationStore.UpdateSentence exec: %w", err)
	}
	return nil
}

func (s *TranslationStore) DeleteSentence(id int64) error {
	_, err := s.db.Exec("DELETE FROM translation_sentences WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("data.TranslationStore.DeleteSentence exec: %w", err)
	}
	return nil
}

func (s *TranslationStore) ListAllSentences(offset, limit int) ([]translation.TranslationSentence, int, error) {
	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM translation_sentences").Scan(&total)
	rows, err := s.db.Query(
		"SELECT id, source_id, direction, source_text, reference_translation, position FROM translation_sentences ORDER BY id LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("data.TranslationStore.ListAllSentences query: %w", err)
	}
	defer rows.Close()
	var items []translation.TranslationSentence
	for rows.Next() {
		var ts translation.TranslationSentence
		if err := rows.Scan(&ts.ID, &ts.SourceID, &ts.Direction, &ts.SourceText, &ts.ReferenceTranslation, &ts.Position); err != nil {
			return nil, 0, fmt.Errorf("data.TranslationStore.ListAllSentences scan: %w", err)
		}
		items = append(items, ts)
	}
	return items, total, rows.Err()
}
```

- [ ] **Step 4: UserStore — ListAllUsers**

In `internal/data/user_store.go`:

```go
func (s *UserStore) ListAllUsers(offset, limit int) ([]user.User, int, error) {
	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total)
	rows, err := s.db.Query(
		"SELECT id, name, email, jlpt_levels, streak_days, created_at FROM users ORDER BY id LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("data.UserStore.ListAllUsers query: %w", err)
	}
	defer rows.Close()
	var users []user.User
	for rows.Next() {
		var u user.User
		var jlptJSON string
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &jlptJSON, &u.StreakDays, &u.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("data.UserStore.ListAllUsers scan: %w", err)
		}
		json.Unmarshal([]byte(jlptJSON), &u.JLPTLevels)
		users = append(users, u)
	}
	return users, total, rows.Err()
}
```

- [ ] **Step 5: Verify compilation and commit**

```bash
cd backend && go build ./...
```

```bash
git add internal/data/speaking_store.go internal/data/writing_store.go internal/data/translation_store.go internal/data/user_store.go
git commit -m "feat(data): add CRUD methods to Speaking/Writing/Translation/User stores"
```

---

### Task 5: Admin Backend — Binary skeleton + auth middleware + routing

**Files:**
- Create: `backend/cmd/admin/main.go`
- Create: `internal/module/admin/handler.go`

- [ ] **Step 1: Create main.go**

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/admin"
)

func main() {
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		slog.Error("ADMIN_TOKEN environment variable is required")
		os.Exit(1)
	}

	dbPath := envOrDefault("DB_PATH", "./data/app.db")
	db, err := data.OpenDB(dbPath)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := data.RunMigrations(db); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}

	// Initialize stores
	wordStore := data.NewWordStore(db)
	grammarStore := data.NewGrammarStore(db)
	speakingStore := data.NewSpeakingStore(db)
	writingStore := data.NewWritingStore(db)
	translationStore := data.NewTranslationStore(db)
	userStore := data.NewUserStore(db)

	h := admin.NewHandler(admin.HandlerConfig{
		AdminToken:        adminToken,
		WordStore:         wordStore,
		GrammarStore:      grammarStore,
		SpeakingStore:     speakingStore,
		WritingStore:      writingStore,
		TranslationStore:  translationStore,
		UserStore:         userStore,
		DB:                db,
	})

	mux := h.RegisterRoutes()

	addr := envOrDefault("LISTEN_ADDR", ":8082")
	slog.Info("admin server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 2: Create handler.go — config, middleware, route registration**

```go
package admin

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"japanese-learning-app/internal/data"
)

// HandlerConfig holds all dependencies for admin handlers.
type HandlerConfig struct {
	AdminToken        string
	WordStore         *data.WordStore
	GrammarStore      *data.GrammarStore
	SpeakingStore     *data.SpeakingStore
	WritingStore      *data.WritingStore
	TranslationStore  *data.TranslationStore
	UserStore         *data.UserStore
	DB                *sql.DB
}

// Handler groups all admin HTTP handlers.
type Handler struct {
	cfg HandlerConfig
}

// NewHandler creates a Handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{cfg: cfg}
}

// RegisterRoutes sets up all admin routes under /api/admin/.
func (h *Handler) RegisterRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/words", h.auth(h.listWords))
	mux.HandleFunc("POST /api/admin/words", h.auth(h.createWord))
	mux.HandleFunc("PUT /api/admin/words/{id}", h.auth(h.updateWord))
	mux.HandleFunc("DELETE /api/admin/words/{id}", h.auth(h.deleteWord))
	// ... similar for grammar, speaking, writing, translation, users, records, import
	return mux
}

// auth wraps a handler with admin token verification.
func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != h.cfg.AdminToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd backend && go build ./cmd/admin/
```

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/admin/main.go internal/module/admin/handler.go
git commit -m "feat(admin): add admin server skeleton with auth middleware"
```

---

### Task 6: Admin Backend — Words handler

**Files:**
- Create: `internal/module/admin/words.go`

- [ ] **Step 1: Implement word CRUD handlers**

```go
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/word"
)

type wordRequest struct {
	KanjiForm    string            `json:"kanji_form"`
	Reading      string            `json:"reading"`
	Meaning      string            `json:"meaning"`
	PartOfSpeech string            `json:"part_of_speech"`
	JLPTLevel    word.JLPTLevel    `json:"jlpt_level"`
	Examples     []word.WordExample `json:"examples"`
	ReadingType  string            `json:"reading_type"`
	AutoFill     bool              `json:"auto_fill"`
}

func (h *Handler) listWords(w http.ResponseWriter, r *http.Request) {
	level := word.JLPTLevel(r.URL.Query().Get("level"))
	search := r.URL.Query().Get("search")
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	words, total, err := h.cfg.WordStore.ListAll(level, search, offset, size)
	if err != nil {
		slog.Error("listWords failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": words, "total": total})
}

func (h *Handler) createWord(w http.ResponseWriter, r *http.Request) {
	var req wordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.KanjiForm == "" || req.Meaning == "" || req.JLPTLevel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kanji_form, meaning, jlpt_level are required"})
		return
	}
	// auto-fill reading/pos via kagome if requested (reuse cli.AutoFill logic)
	if req.AutoFill && (req.Reading == "" || req.PartOfSpeech == "") {
		// call kagome auto-fill — see Task 6 Step 2
	}
	w := word.Word{
		KanjiForm:    req.KanjiForm,
		Reading:      req.Reading,
		Meaning:      req.Meaning,
		PartOfSpeech: req.PartOfSpeech,
		JLPTLevel:    req.JLPTLevel,
		Examples:     req.Examples,
		ReadingType:  req.ReadingType,
	}
	if err := h.cfg.WordStore.InsertWord(w); err != nil {
		slog.Error("createWord failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, w)
}

func (h *Handler) updateWord(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req wordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	w := word.Word{
		ID:           id,
		KanjiForm:    req.KanjiForm,
		Reading:      req.Reading,
		Meaning:      req.Meaning,
		PartOfSpeech: req.PartOfSpeech,
		JLPTLevel:    req.JLPTLevel,
		Examples:     req.Examples,
		ReadingType:  req.ReadingType,
	}
	if err := h.cfg.WordStore.UpdateWord(w); err != nil {
		slog.Error("updateWord failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, w)
}

func (h *Handler) deleteWord(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.cfg.WordStore.DeleteWord(id); err != nil {
		slog.Error("deleteWord failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
```

- [ ] **Step 2: Add route registration for words in handler.go**

In `handler.go` RegisterRoutes, the word routes (already sketched in Task 5) should reference these methods. Verify they match.

- [ ] **Step 3: Build and commit**

```bash
cd backend && go build ./cmd/admin/ && git add internal/module/admin/words.go internal/module/admin/handler.go && git commit -m "feat(admin): add word CRUD handler"
```

---

### Task 7: Admin Backend — Grammar, Speaking, Writing handlers

**Files:**
- Create: `internal/module/admin/grammar.go`
- Create: `internal/module/admin/speaking.go`
- Create: `internal/module/admin/writing.go`

Each follows the same CRUD pattern as Task 6 (List/Create/Update/Delete), adapted for each module's fields.

- [ ] **Step 1: Grammar handler**

Route registration (in handler.go):
```go
mux.HandleFunc("GET /api/admin/grammar", h.auth(h.listGrammar))
mux.HandleFunc("POST /api/admin/grammar", h.auth(h.createGrammar))
mux.HandleFunc("PUT /api/admin/grammar/{id}", h.auth(h.updateGrammar))
mux.HandleFunc("DELETE /api/admin/grammar/{id}", h.auth(h.deleteGrammar))
```

grammar.go — uses `GrammarStore.ListByLevelWithStatus` for listing (adapt to `ListAll` pattern), `UpdatePoint` and `DeletePoint` for mutations, `GetByID` + existing `InsertPoint` or the store's insert method.

- [ ] **Step 2: Speaking handler**

Route registration:
```go
mux.HandleFunc("GET /api/admin/speaking", h.auth(h.listSpeaking))
mux.HandleFunc("POST /api/admin/speaking", h.auth(h.createSpeaking))
mux.HandleFunc("PUT /api/admin/speaking/{id}", h.auth(h.updateSpeaking))
mux.HandleFunc("DELETE /api/admin/speaking/{id}", h.auth(h.deleteSpeaking))
```

speaking.go — uses `SpeakingStore.GetMaterials` for list, `UpdateMaterial`/`DeleteMaterial` for mutations.

- [ ] **Step 3: Writing handler**

Route registration:
```go
mux.HandleFunc("GET /api/admin/writing", h.auth(h.listWriting))
mux.HandleFunc("POST /api/admin/writing", h.auth(h.createWriting))
mux.HandleFunc("PUT /api/admin/writing/{id}", h.auth(h.updateWriting))
mux.HandleFunc("DELETE /api/admin/writing/{id}", h.auth(h.deleteWriting))
```

writing.go — uses `ListAllQuestions`, `UpdateQuestion`, `DeleteQuestion`.

- [ ] **Step 4: Build and commit**

```bash
cd backend && go build ./cmd/admin/ && git add internal/module/admin/grammar.go internal/module/admin/speaking.go internal/module/admin/writing.go internal/module/admin/handler.go && git commit -m "feat(admin): add grammar, speaking, writing CRUD handlers"
```

---

### Task 8: Admin Backend — Translation, Users, Records, Import handlers

**Files:**
- Create: `internal/module/admin/translation.go`
- Create: `internal/module/admin/users.go`
- Create: `internal/module/admin/records.go`
- Create: `internal/module/admin/import.go`

- [ ] **Step 1: Translation handler**

Route registration:
```go
mux.HandleFunc("GET /api/admin/translation", h.auth(h.listTranslation))
mux.HandleFunc("POST /api/admin/translation", h.auth(h.createTranslation))
mux.HandleFunc("PUT /api/admin/translation/{id}", h.auth(h.updateTranslation))
mux.HandleFunc("DELETE /api/admin/translation/{id}", h.auth(h.deleteTranslation))
```

translation.go — uses `ListAllSentences`, `UpdateSentence`, `DeleteSentence`.

- [ ] **Step 2: Users handler**

```go
mux.HandleFunc("GET /api/admin/users", h.auth(h.listUsers))
mux.HandleFunc("GET /api/admin/users/{id}/stats", h.auth(h.getUserStats))
```

users.go — uses `UserStore.ListAllUsers` and `UserStore.GetStats(userID)`.

- [ ] **Step 3: Records handler**

```go
mux.HandleFunc("GET /api/admin/records/{module}", h.auth(h.listRecords))
```

records.go — reads `module` path param, delegates to appropriate store's `ListRecords(userID)` or list-all query.

- [ ] **Step 4: Import handler**

```go
mux.HandleFunc("POST /api/admin/import/{module}", h.auth(h.bulkImport))
```

import.go:
```go
func (h *Handler) bulkImport(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	var inserted int
	switch module {
	case "words":
		inserted, err = cli.ImportWords(h.cfg.DB, file)
	case "grammar":
		inserted, err = cli.ImportGrammar(h.cfg.DB, file)
	case "speaking":
		inserted, err = cli.ImportSpeaking(h.cfg.DB, file)
	case "writing":
		inserted, err = cli.ImportWriting(h.cfg.DB, file)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown module"})
		return
	}
	if err != nil {
		slog.Error("bulkImport failed", "module", module, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"inserted": inserted})
}
```

- [ ] **Step 5: Build and commit**

```bash
cd backend && go build ./cmd/admin/ && git add internal/module/admin/ && git commit -m "feat(admin): add translation, users, records, import handlers"
```

---

### Task 9: Frontend — Project setup + Layout + Login

**Files:**
- Create: `front/admin/package.json`
- Create: `front/admin/tsconfig.json`
- Create: `front/admin/vite.config.ts`
- Create: `front/admin/index.html`
- Create: `front/admin/src/main.tsx`
- Create: `front/admin/src/App.tsx`
- Create: `front/admin/src/api/client.ts`
- Create: `front/admin/src/components/Layout/Layout.tsx`
- Create: `front/admin/src/components/Layout/Layout.module.css`
- Create: `front/admin/src/components/Modal/Modal.tsx`
- Create: `front/admin/src/components/Modal/Modal.module.css`
- Create: `front/admin/src/components/Table/Table.tsx`
- Create: `front/admin/src/components/Table/Table.module.css`
- Create: `front/admin/src/pages/Login/Login.tsx`
- Create: `front/admin/src/pages/Login/Login.module.css`

- [ ] **Step 1: package.json**

```json
{
  "name": "japanese-learning-admin",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.0"
  },
  "devDependencies": {
    "@types/react": "^18.3.11",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.2",
    "typescript": "^5.6.0",
    "vite": "^5.4.0"
  }
}
```

- [ ] **Step 2: vite.config.ts**

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    proxy: {
      '/api/admin': 'http://localhost:8082',
    },
  },
  build: {
    outDir: 'dist',
  },
})
```

- [ ] **Step 3: API client (`src/api/client.ts`)**

```typescript
const BASE = '/api/admin'

function token(): string {
  return sessionStorage.getItem('admin_token') || ''
}

export async function adminFetch<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    'Authorization': `Bearer ${token()}`,
  }
  let bodyInit: BodyInit | undefined
  if (body instanceof FormData) {
    bodyInit = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    bodyInit = JSON.stringify(body)
  }
  const res = await fetch(BASE + path, { method, headers, body: bodyInit })
  if (res.status === 204) return undefined as T
  const json = await res.json()
  if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
  return json as T
}
```

- [ ] **Step 4: Layout component**

Layout.tsx — Sidebar with 7 nav items (Words, Grammar, Speaking, Writing, Translation, Users, Records) + top bar with "Admin Panel" title and logout button.

Layout.module.css — Fixed sidebar (200px), flex layout, highlight active nav item.

- [ ] **Step 5: Login page**

Login.tsx — Simple form with single input for admin token. On submit, store in sessionStorage, redirect to `/words`.

- [ ] **Step 6: App.tsx** — React Router with Login route (public) and Layout-protected routes.

- [ ] **Step 7: Modal and Table reusable components**

Modal.tsx — Generic modal with overlay, close button, title, children.
Table.tsx — Generic table accepting `columns` and `data` props.

- [ ] **Step 8: npm install and verify dev build**

```bash
cd front/admin && npm install && npx vite build
```

- [ ] **Step 9: Commit**

```bash
git add front/admin/
git commit -m "feat(admin): add frontend project setup, layout, login"
```

---

### Task 10: Frontend — CRUD pages (Words, Grammar, Speaking, Writing)

**Files:**
- Create: `front/admin/src/pages/Words/Words.tsx`
- Create: `front/admin/src/pages/Words/Words.module.css`
- Create: `front/admin/src/pages/Grammar/Grammar.tsx`
- Create: `front/admin/src/pages/Grammar/Grammar.module.css`
- Create: `front/admin/src/pages/Speaking/Speaking.tsx`
- Create: `front/admin/src/pages/Speaking/Speaking.module.css`
- Create: `front/admin/src/pages/Writing/Writing.tsx`
- Create: `front/admin/src/pages/Writing/Writing.module.css`

Each page follows the same pattern. Using Words as the template:

- [ ] **Step 1: Words page (Words.tsx)**

```typescript
import { useState, useEffect } from 'react'
import { adminFetch } from '@/api/client'
import { Layout } from '@/components/Layout/Layout'
import { Modal } from '@/components/Modal/Modal'
import styles from './Words.module.css'

interface Word {
  id: number; kanji_form: string; reading: string; meaning: string
  part_of_speech: string; jlpt_level: string
  examples: { japanese: string; chinese: string }[]
  reading_type: string
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']

export function WordsPage() {
  const [words, setWords] = useState<Word[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Word | null>(null)
  const [form, setForm] = useState({ kanji_form: '', reading: '', meaning: '', part_of_speech: '', jlpt_level: 'N5', examples: [] as any[], reading_type: '', auto_fill: true })

  useEffect(() => { loadWords() }, [page, level])

  async function loadWords() {
    setLoading(true)
    const params = new URLSearchParams({ page: String(page), size: '20' })
    if (level) params.set('level', level)
    if (search) params.set('search', search)
    const data = await adminFetch<{items: Word[]; total: number}>('GET', `/words?${params}`)
    setWords(data.items)
    setTotal(data.total)
    setLoading(false)
  }

  // handleCreate, handleUpdate, handleDelete — call adminFetch POST/PUT/DELETE
  // handleBulkImport — FormData with file, POST /import/words
  // render: search bar, level select, Add New button, Bulk Import button
  // table rows with Edit/Delete buttons
  // Modal with form fields for create/edit
}
```

- [ ] **Step 2: Words page CSS**

Standard admin CRUD layout: top toolbar (search + filter + buttons), data table, modal form with label+input pairs.

- [ ] **Step 3: Repeat for Grammar, Speaking, Writing pages**

Same structure, different fields per module. Grammar includes `conjunction_rule`, `usage_note`, `examples_json`, `quiz_questions_json`. Speaking includes `type`, `title`, `text`, `audio_url`, `jlpt_level`. Writing includes `type`, `prompt`, `expected_answer`, `jlpt_level`, `grammar_point_id`.

- [ ] **Step 4: Verify build and commit**

```bash
cd front/admin && npx vite build
```

```bash
git add front/admin/src/pages/
git commit -m "feat(admin): add words, grammar, speaking, writing CRUD pages"
```

---

### Task 11: Frontend — Translation, Users, Records pages

**Files:**
- Create: `front/admin/src/pages/Translation/Translation.tsx`
- Create: `front/admin/src/pages/Translation/Translation.module.css`
- Create: `front/admin/src/pages/Users/Users.tsx`
- Create: `front/admin/src/pages/Users/Users.module.css`
- Create: `front/admin/src/pages/Records/Records.tsx`
- Create: `front/admin/src/pages/Records/Records.module.css`

- [ ] **Step 1: Translation page** — CRUD table for translation sentences (source_text, reference_translation, direction, position).

- [ ] **Step 2: Users page** — Read-only table listing users with ID, name, email, jlpt_levels, streak. Click a row to expand per-module stats.

- [ ] **Step 3: Records page** — Dropdown to select module, optional user_id filter, paginated table of records (date, question, answer, score).

- [ ] **Step 4: Verify build and commit**

```bash
cd front/admin && npx vite build && git add front/admin/src/pages/ && git commit -m "feat(admin): add translation, users, records pages"
```

---

### Task 12: Integration Test — End-to-end admin flow

**Files:**
- Create: `internal/module/admin/handler_test.go`

- [ ] **Step 1: Write integration test for auth middleware**

```go
func TestAuthMiddleware(t *testing.T) {
	// Setup: create Handler with test token
	// Test: request without token → 401
	// Test: request with wrong token → 401
	// Test: request with correct token → passes through
}
```

- [ ] **Step 2: Write CRUD integration tests**

Test word CRUD:
```go
func TestWordCRUD(t *testing.T) {
	// Setup test DB, create word via POST /api/admin/words
	// GET /api/admin/words → list includes it
	// PUT /api/admin/words/{id} → update reading
	// DELETE /api/admin/words/{id} → gone from list
}
```

- [ ] **Step 3: Run tests**

```bash
cd backend && go test ./internal/module/admin/ -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/module/admin/handler_test.go && git commit -m "test(admin): add integration tests for auth and CRUD"
```

---

### Task 13: Makefile + Documentation

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add admin targets to Makefile**

```makefile
admin-run:     ## 启动管理后台
	ADMIN_TOKEN=change-me go run ./backend/cmd/admin/

admin-build:   ## 编译管理后台
	go build -o bin/admin ./backend/cmd/admin/

admin-front-build: ## 构建管理后台前端
	cd front/admin && npm install && npm run build

admin-front-dev: ## 启动管理后台前端开发服务器
	cd front/admin && npm run dev
```

- [ ] **Step 2: Commit**

```bash
git add Makefile && git commit -m "chore: add admin panel targets to Makefile"
```

---

## Summary

13 tasks covering:
1. **Task 1:** CLI import refactor (io.Reader)
2. **Task 2:** WordStore CRUD methods
3. **Task 3:** GrammarStore CRUD methods
4. **Task 4:** Speaking/Writing/Translation/User store CRUD methods
5. **Task 5:** Admin backend skeleton + auth
6. **Task 6:** Words API handler
7. **Task 7:** Grammar/Speaking/Writing API handlers
8. **Task 8:** Translation/Users/Records/Import API handlers
9. **Task 9:** Frontend project setup + Layout + Login
10. **Task 10:** Frontend CRUD pages (Words/Grammar/Speaking/Writing)
11. **Task 11:** Frontend Translation/Users/Records pages
12. **Task 12:** Integration tests
13. **Task 13:** Makefile targets

Dependencies: Tasks 1-4 are foundational (no deps). Task 5 depends on Tasks 1-4. Tasks 6-8 depend on Task 5. Tasks 9-11 depend on Tasks 5-8. Task 12 depends on Tasks 5-8. Task 13 is independent.
