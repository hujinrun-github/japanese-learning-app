# Admin Panel Design Spec

> **Goal:** Build a standalone admin panel (separate binary + SPA) for managing learning content (words, grammar, speaking, writing, translation), users, and learning records.

**Architecture:** Independent Go binary (`backend/cmd/admin/`) on port `:8082` + independent React SPA (`front/admin/`), reusing `internal/data/` stores. Token-based auth, zero coupling with the main app's JWT/user system.

**Tech Stack:** Go stdlib `net/http` + React 18 + TypeScript + Vite + CSS Modules

---

## 1. Authentication

- Admin token set via environment variable `ADMIN_TOKEN` (required, no default)
- Client sends `Authorization: Bearer <ADMIN_TOKEN>` on every request
- Middleware returns 401 on mismatch, no session/refresh mechanism
- Login page in the SPA prompts for the token and stores it in `sessionStorage`

---

## 2. API Routes

All routes share the `/api/admin/` prefix and are protected by the admin auth middleware.

### Words

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/words?level=&search=&page=&size=` | List with optional filter/search/pagination |
| POST | `/api/admin/words` | Create single word |
| PUT | `/api/admin/words/{id}` | Update existing word |
| DELETE | `/api/admin/words/{id}` | Delete word by ID |

### Grammar

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/grammar?level=&search=&page=&size=` | List |
| POST | `/api/admin/grammar` | Create grammar point |
| PUT | `/api/admin/grammar/{id}` | Update grammar point |
| DELETE | `/api/admin/grammar/{id}` | Delete grammar point |

### Speaking

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/speaking?level=&type=&page=&size=` | List |
| POST | `/api/admin/speaking` | Create speaking material |
| PUT | `/api/admin/speaking/{id}` | Update speaking material |
| DELETE | `/api/admin/speaking/{id}` | Delete speaking material |

### Writing

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/writing?level=&type=&page=&size=` | List |
| POST | `/api/admin/writing` | Create writing question |
| PUT | `/api/admin/writing/{id}` | Update writing question |
| DELETE | `/api/admin/writing/{id}` | Delete writing question |

### Translation

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/translation?page=&size=` | List sentences |
| POST | `/api/admin/translation` | Create sentence |
| PUT | `/api/admin/translation/{id}` | Update sentence |
| DELETE | `/api/admin/translation/{id}` | Delete sentence |

### Bulk Import

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/admin/import/{module}` | Upload JSON file; `module` = `words\|grammar\|speaking\|writing` |

Response: `{ "inserted": N, "skipped": M }`

### Users

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/users?page=&size=` | List all users |
| GET | `/api/admin/users/{id}/stats` | Single user's stats (per-module progress, streak) |

### Records

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/records/{module}?user_id=&page=&size=` | Learning records for a module (`word\|grammar\|speaking\|writing`) |

---

## 3. Word Entry Workflow

POST/PUT `/api/admin/words` request body:

```json
{
  "kanji_form": "食べる",
  "reading": "",
  "meaning": "吃",
  "part_of_speech": "",
  "jlpt_level": "N5",
  "examples": [{"japanese": "", "chinese": ""}],
  "auto_fill": true,
  "generate_tts": false
}
```

**Auto-fill logic** (when `auto_fill=true`):
- If `reading` or `part_of_speech` is empty, call kagome morphological analyzer to fill them
- Reuses existing `internal/cli/auto_fill.go` logic

**TTS generation** (when `generate_tts=true`):
- After INSERT, asynchronously generate WAV audio for the word's reading and each example sentence
- SHA-256 hash → `data/audio/examples/{hash}.wav`

---

## 4. Frontend Layout

Left sidebar (200px) + right content area. Seven nav items:

- Words / Grammar / Speaking / Writing / Translation
- Users / Records

Each module page: search bar + level/type filter + "Add New" button + "Bulk Import" button + paginated table + modal form for create/edit.

---

## 5. Store Changes

New methods required on existing stores (all in `internal/data/`):

| Store | New Methods |
|-------|------------|
| WordStore | `UpdateWord`, `DeleteWord`, `ListAll(level, search string, page, size int)` |
| GrammarStore | `UpdatePoint`, `DeletePoint` |
| SpeakingStore | `UpdateMaterial`, `DeleteMaterial` |
| WritingStore | `UpdateQuestion`, `DeleteQuestion`, `ListAllQuestions` |
| TranslationStore | `UpdateSentence`, `DeleteSentence`, `ListAllSentences` |
| UserStore | `ListAllUsers(offset, limit int) ([]User, error)` |

### CLI Import Refactor

Existing import functions in `internal/cli/import_*.go` read from file paths. Extract inner logic to accept `io.Reader` so HTTP handler can reuse them:

```go
// New signature
func ImportWords(db *sql.DB, r io.Reader) (int, error)
// Old signature delegates to new
func ImportWordsFromFile(db *sql.DB, filePath string) (int, error) {
    f, _ := os.Open(filePath)
    defer f.Close()
    return ImportWords(db, f)
}
```

---

## 6. Directory Structure

```
backend/cmd/admin/main.go          ← admin server entrypoint
internal/module/admin/              ← admin HTTP handlers
internal/cli/                       ← refactor: accept io.Reader
front/admin/                        ← React SPA
  src/
    pages/Login/
    pages/Words/
    pages/Grammar/
    pages/Speaking/
    pages/Writing/
    pages/Translation/
    pages/Users/
    pages/Records/
    components/Layout/              ← sidebar + topbar shell
    components/Modal/               ← reusable form modal
    components/Table/               ← reusable data table
    api/client.ts                   ← admin API client
```

---

## 7. Scope Boundaries

**In scope:**
- CRUD for words, grammar, speaking materials, writing questions, translation sentences
- Bulk JSON file import for each module
- User list and per-user stats view
- Learning records view per module
- Auto-fill (kagome) and TTS generation as optional flags on word entry

**Out of scope:**
- Admin user management (add/remove admins)
- Batch delete or bulk edit
- Content versioning or rollback
- Rich text editor for grammar examples
- Audio preview/playback in admin
- Dashboard with analytics charts
