# Translation Learning Module Design

**Date:** 2026-05-20
**Status:** Approved

## Overview

Add a `translation` module supporting bidirectional Japanese↔Chinese translation practice, with three import pipelines (manual paste, URL scrape, API pull), dual-layer scoring (local rule-based instant + AI async), and grammar explanation linked to existing grammar points.

---

## 1. Data Model

Three new tables:

### `translation_sources` — source materials

| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | |
| title | TEXT | Source title |
| source_type | TEXT | `"manual"`, `"url"`, `"api"` |
| source_url | TEXT | Original URL (nullable) |
| api_endpoint | TEXT | API endpoint (nullable) |
| raw_content | TEXT | Original raw text |
| created_at | DATETIME | |

### `translation_sentences` — sentence-level units

| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | |
| source_id | INTEGER FK | References `translation_sources` |
| direction | TEXT | `"cn2jp"` or `"jp2cn"` |
| source_text | TEXT | Text to translate |
| reference_translation | TEXT | Reference translation (nullable) |
| position | INTEGER | Order within source |

### `translation_records` — user practice records

| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | |
| user_id | INTEGER FK | |
| sentence_id | INTEGER FK | |
| user_translation | TEXT | User's submitted translation |
| score | INTEGER | 0–100 final score |
| rule_score | INTEGER | Local rule-based score |
| ai_feedback_json | TEXT | AI scoring + grammar explanation JSON |
| practiced_at | DATETIME | |

### AI feedback JSON structure

```json
{
  "ai_score": 85,
  "grammar_explanations": [
    {
      "grammar_point": "〜てくる",
      "explanation": "Indicates directionality or gradual change...",
      "matched_db_id": 42
    }
  ],
  "issue_description": "〜てきた is more natural than plain 〜た here",
  "corrected_translation": "...",
  "reference_translation": "..."
}
```

`matched_db_id` links to the existing `grammar_points` table for navigation.

---

## 2. Backend Architecture

### Module: `internal/module/translation/`

Files:

| File | Purpose |
|---|---|
| `model.go` | Domain types: `TranslationSource`, `TranslationSentence`, `TranslationRecord`, `TranslationFeedback`, `Direction` |
| `service.go` | `TranslationStoreInterface` + `TranslationService` business logic |
| `handler.go` | HTTP handlers, route registration |
| `ai_client.go` | `TranslationReviewer` interface + `StubReviewer` + `ClaudeTranslationReviewer` |
| `*_test.go` | Table-driven tests |

### Data: `internal/data/translation_store.go`

`TranslationStore` struct with `*sql.DB`, implementing `TranslationStoreInterface`.

### Service methods

| Method | Purpose |
|---|---|
| `GetDailyQueue(userID, count)` | Select today's sentences via SM-2 spaced repetition |
| `GetFreeSentences(userID, direction, sourceID)` | Browse sentences by direction/source filter |
| `SubmitTranslation(userID, sentenceID, translation)` | Local rule scoring (sync) + AI review trigger (async) |
| `GetResult(recordID)` | Return full result including AI feedback |
| `ListRecords(userID)` | User history |
| `ImportSource(source)` | Import source material → split into sentences → insert |

### Handler endpoints

```
GET    /api/v1/translation/queue              → daily tasks
GET    /api/v1/translation/sentences           → free practice list
POST   /api/v1/translation/submit              → submit translation
GET    /api/v1/translation/records/{id}         → single result detail
GET    /api/v1/translation/records              → history
POST   /api/v1/translation/sources              → import source (manual paste)
POST   /api/v1/translation/sources/import       → import from URL
GET    /api/v1/translation/sources              → source list
```

### AI reviewer interface

```go
type TranslationReviewer interface {
    Review(direction, sourceText, userTranslation, referenceTranslation string) (TranslationFeedback, error)
}
```

Two implementations:
- `StubReviewer` — returns preset feedback for tests
- `ClaudeTranslationReviewer` — calls Claude API (reuses pattern from `writing.ClaudeClient`)

### Local rule scoring

| Dimension | JP→CN | CN→JP |
|---|---|---|
| Keyword match | Reference keywords present | Source keywords translated |
| Grammar markers | Particle coverage (は/が/を/に/で) | N5-N3 pattern match |
| Length ratio | Output/input ratio 0.5–2.0 | Same |

Returns 0–100 `rule_score`. AI review triggered asynchronously after submission.

---

## 3. Import Pipeline

Three entry points, shared processing:

```
Manual paste ──┐
URL scrape ────┼──→ Sentence splitting ──→ Direction detection ──→ DB insert
API pull ──────┘     (by punctuation)       (char ratio heuristic)
```

### A — Manual paste
- Frontend form: paste text → select direction or auto-detect → preview split sentences → confirm
- POST `/api/v1/translation/sources` with `{source_type: "manual", content: "...", direction: "auto"}`

### C — URL scrape
- POST `/api/v1/translation/sources/import` with `{url: "https://...", direction: "auto"}`
- Backend fetches HTML via `net/http` → extracts text from `<article>/<main>/<p>` tags → strips scripts/styles → merges paragraphs → splits sentences → inserts

### D — API pull
- CLI subcommand: `import-translation-api --file config.json`
- Config specifies: API endpoint, HTTP method, JSONPath extraction rule, direction
- Manually run or via system cron (no built-in scheduler)

### Sentence splitting rules
- Japanese: split on `。！？…`
- Chinese: split on `。！？…`
- Min sentence length: 3 chars, max: 200 chars
- Consecutive sentences preserve `position` order for paragraph context

---

## 4. Frontend

### Routes (inside `<ProtectedLayout>`)

```
/translation           → TranslationListPage
/translation/practice   → TranslationPracticePage
```

### TranslationListPage

Two card entries at top:
- **Daily tasks** with progress indicator (e.g., "3/5 done")
- **Free practice** with source browser

Direction filter tabs: [All] [CN→JP] [JP→CN]
Source list with sentence count
"+ Import source" button → import dialog (tab: paste text / input URL / API config)

### TranslationPracticePage

- Header: back button, direction badge, progress (e.g., "#3/12")
- Source text display (read-only)
- Translation input (textarea)
- Submit button
- Results area (shown after submit):
  - Instant rule score
  - AI score (appears asynchronously)
  - Grammar explanation tags (clickable → opens grammar detail page)
  - AI commentary text

### Interaction flow

```
User submits translation
  → POST /submit
  → Backend: local scoring + return record_id
  → Frontend: show rule_score immediately
  → Backend: async Claude AI review → write back to record
  → Frontend: poll GET /records/{id} until AI feedback available
```

---

## 5. Error Handling

| Scenario | Handling |
|---|---|
| URL fetch timeout (>10s) | Return error, suggest user check URL or paste manually |
| URL content has no JP/CN text | Direction detection returns `"unknown"`, frontend shows "No valid JP/CN content detected" |
| Sentence splitting yields 0 sentences | Return error, do not create empty source |
| AI review timeout (>30s) | Rule score already returned; AI score remains `null`, UI shows "AI commentary unavailable" |
| AI API key not configured | Degrade to local scoring only; AI fields `null` |
| Duplicate sentence submission | `INSERT OR REPLACE` — overwrites previous record |
| Empty translation submitted | Reject with 400 |

---

## 6. Testing Strategy

Per project constitution (TDD + table-driven tests):

| Layer | Content |
|---|---|
| Unit (`*_test.go`) | Sentence splitting (JP/CN punctuation), direction detection (char ratio heuristic), local rule scoring algorithm |
| Integration | `TranslationStore` CRUD with `:memory:` SQLite |
| Stub | `StubReviewer` returns preset AI feedback for full Service flow testing |
| Frontend | Page render, submit/result display, import dialog switching |

---

## 7. Implementation Order

1. **DB migration** — `008_translation.sql` (3 tables)
2. **Backend Store + Service + Handler** — TDD, Stub first then Claude
3. **CLI import command** — `import-translation-api`
4. **Frontend pages** — TranslationListPage + TranslationPracticePage
5. **Navigation entries** — TopNavBar + BottomTabBar
