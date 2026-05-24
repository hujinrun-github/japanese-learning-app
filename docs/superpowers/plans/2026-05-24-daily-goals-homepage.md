# Daily Goals & Homepage Task Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-module daily goals (configurable in settings) and show today's completion progress on the home page.

**Architecture:** A new `daily_goals_json` TEXT column on `users` stores per-module goals. The `GET /api/v1/users/stats` endpoint returns `today_completed` (from `study_sessions` aggregated by today's date) and `daily_goal` per module. A new `PUT /api/v1/users/me/daily-goals` endpoint handles updates. The settings page gets a daily-goals input section, and the home page module cards gain a today-progress bar.

**Tech Stack:** Go stdlib `net/http`, SQLite, React 18 + TypeScript + CSS Modules, react-i18next

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/data/migrations/010_daily_goals.sql` | Add `daily_goals_json` column |
| `internal/module/user/model.go` | `ModuleStat` + `DailyGoalsReq` types |
| `internal/data/user_store.go` | DB queries for daily goals + today completions |
| `internal/module/user/service.go` | `UpdateDailyGoals` + updated `GetStats` |
| `internal/module/user/handler.go` | `PUT /api/v1/users/me/daily-goals` route |
| `internal/data/adapters.go` | Adapter methods for new store methods |
| `front/react/src/types/api.ts` | `ModuleStat` + `DailyGoals` TS types |
| `front/react/src/api/user.ts` | `updateDailyGoals` API function |
| `front/react/src/i18n/locales/zh.ts` | New i18n keys |
| `front/react/src/i18n/locales/en.ts` | English translations |
| `front/react/src/pages/settings/SettingsPage.tsx` | Daily goals input section |
| `front/react/src/pages/home/HomePage.tsx` | Today-progress bars per module |
| `front/react/src/pages/home/HomePage.module.css` | Styles for today-progress |

---

### Task 1: Database Migration

**Files:**
- Create: `internal/data/migrations/010_daily_goals.sql`

- [ ] **Step 1: Write the migration**

```sql
-- 010_daily_goals.sql
-- Add daily_goals_json column to users table for per-module daily goal tracking.

ALTER TABLE users ADD COLUMN daily_goals_json TEXT NOT NULL DEFAULT '{}';
```

- [ ] **Step 2: Run the app to verify migration applies**

Run: `make web`
Expected: Server starts without errors, "migration applied: 010_daily_goals.sql" in logs.
Stop the server after verification.

- [ ] **Step 3: Commit**

```bash
git add internal/data/migrations/010_daily_goals.sql
git commit -m "feat(data): add daily_goals_json column to users table"
```

---

### Task 2: Go Model Changes

**Files:**
- Modify: `internal/module/user/model.go`

- [ ] **Step 1: Add TodayCompleted and DailyGoal to ModuleStat, add DailyGoalsReq**

Replace `ModuleStat` with:
```go
// ModuleStat 单个模块的进度统计
type ModuleStat struct {
	DueCount       int `json:"due_count"`
	MasteredCount  int `json:"mastered_count"`
	TotalCount     int `json:"total_count"`
	TodayCompleted int `json:"today_completed"`
	DailyGoal      int `json:"daily_goal"`
}
```

Add after `UpdatePasswordReq`:
```go
// DailyGoalsReq 更新每日目标请求
type DailyGoalsReq struct {
	Word     int `json:"word"`
	Grammar  int `json:"grammar"`
	Speaking int `json:"speaking"`
	Writing  int `json:"writing"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /home/tylerhu/github_project/japanese-learning-app && go build ./...`
Expected: Compilation succeeds (may have unused field warnings from store, which is fine — we'll fill them next).

- [ ] **Step 3: Commit**

```bash
git add internal/module/user/model.go
git commit -m "feat(user): add TodayCompleted, DailyGoal to ModuleStat; add DailyGoalsReq"
```

---

### Task 3: UserStore — Daily Goals Methods + GetStats Update

**Files:**
- Modify: `internal/data/user_store.go`

- [ ] **Step 1: Add today-completed query to GetStats and parse daily_goals_json**

Update `GetStats` in `user_store.go`. The method currently reads `streak_days` from users. Change it to also read `daily_goals_json` in the same query, and for each module add a query against `study_sessions` for today's completed count. Also add `GetDailyGoals` and `UpdateDailyGoals` methods.

Replace the `GetStats` function body (lines 249-281) with:

```go
// defaultDailyGoals returns the default daily goals map.
func defaultDailyGoals() map[string]int {
	return map[string]int{"word": 20, "grammar": 5, "speaking": 3, "writing": 3}
}

// GetStats returns the user's learning stats across all modules.
func (s *UserStore) GetStats(userID int64) (*user.UserStats, error) {
	slog.Debug("UserStore.GetStats called", "user_id", userID)

	stats := &user.UserStats{
		ModuleStats: make(map[string]user.ModuleStat),
	}

	var streakDays int
	var dailyGoalsJSON string
	if err := s.db.QueryRow(
		`SELECT streak_days, daily_goals_json FROM users WHERE id = ?`, userID,
	).Scan(&streakDays, &dailyGoalsJSON); err != nil {
		return nil, fmt.Errorf("data.UserStore.GetStats streak_days: %w", err)
	}
	stats.StreakDays = streakDays

	goals := parseDailyGoals(dailyGoalsJSON)

	// word
	wordTotal, wordDue, wordMastered := s.countWords(userID)
	stats.ModuleStats["word"] = user.ModuleStat{
		DueCount: wordDue, MasteredCount: wordMastered, TotalCount: wordTotal,
		TodayCompleted: s.countTodayCompleted(userID, "word"),
		DailyGoal:      goals["word"],
	}

	// grammar
	grammarTotal, grammarDue, grammarMastered := s.countGrammar(userID)
	stats.ModuleStats["grammar"] = user.ModuleStat{
		DueCount: grammarDue, MasteredCount: grammarMastered, TotalCount: grammarTotal,
		TodayCompleted: s.countTodayCompleted(userID, "grammar"),
		DailyGoal:      goals["grammar"],
	}

	// speaking
	speakingTotal, speakingMastered := s.countSpeaking(userID)
	stats.ModuleStats["speaking"] = user.ModuleStat{
		DueCount: 0, MasteredCount: speakingMastered, TotalCount: speakingTotal,
		TodayCompleted: s.countTodayCompleted(userID, "speaking"),
		DailyGoal:      goals["speaking"],
	}

	// writing
	writingTotal, writingMastered := s.countWriting(userID)
	stats.ModuleStats["writing"] = user.ModuleStat{
		DueCount: 0, MasteredCount: writingMastered, TotalCount: writingTotal,
		TodayCompleted: s.countTodayCompleted(userID, "writing"),
		DailyGoal:      goals["writing"],
	}

	slog.Debug("UserStore.GetStats done", "user_id", userID)
	return stats, nil
}
```

- [ ] **Step 2: Add helper methods after countWriting (before isUniqueConstraintError)**

```go
func (s *UserStore) countTodayCompleted(userID int64, module string) int {
	var count int
	s.db.QueryRow(
		`SELECT COALESCE(SUM(completed_count), 0)
		 FROM study_sessions
		 WHERE user_id = ? AND module = ? AND date(started_at) = date('now')`,
		userID, module,
	).Scan(&count)
	return count
}

// GetDailyGoals returns the user's daily goals map.
func (s *UserStore) GetDailyGoals(userID int64) (map[string]int, error) {
	slog.Debug("UserStore.GetDailyGoals called", "user_id", userID)

	var jsonStr string
	if err := s.db.QueryRow(
		`SELECT daily_goals_json FROM users WHERE id = ?`, userID,
	).Scan(&jsonStr); err != nil {
		slog.Error("failed to query daily_goals_json", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.UserStore.GetDailyGoals: %w", err)
	}

	goals := parseDailyGoals(jsonStr)
	slog.Debug("UserStore.GetDailyGoals done", "user_id", userID)
	return goals, nil
}

// UpdateDailyGoals updates the user's daily goals JSON.
func (s *UserStore) UpdateDailyGoals(userID int64, goals map[string]int) error {
	slog.Debug("UserStore.UpdateDailyGoals called", "user_id", userID)

	jsonBytes, err := json.Marshal(goals)
	if err != nil {
		slog.Error("failed to marshal daily goals", "err", err)
		return fmt.Errorf("data.UserStore.UpdateDailyGoals marshal: %w", err)
	}

	_, err = s.db.Exec(
		`UPDATE users SET daily_goals_json = ? WHERE id = ?`,
		string(jsonBytes), userID,
	)
	if err != nil {
		slog.Error("failed to update daily_goals_json", "err", err, "user_id", userID)
		return fmt.Errorf("data.UserStore.UpdateDailyGoals exec: %w", err)
	}

	slog.Debug("UserStore.UpdateDailyGoals done", "user_id", userID)
	return nil
}

// parseDailyGoals parses the daily_goals_json string, filling missing keys with defaults.
func parseDailyGoals(jsonStr string) map[string]int {
	goals := defaultDailyGoals()
	if jsonStr == "" || jsonStr == "{}" {
		return goals
	}
	var stored map[string]int
	if err := json.Unmarshal([]byte(jsonStr), &stored); err != nil {
		slog.Warn("failed to parse daily_goals_json, using defaults", "err", err)
		return goals
	}
	for k, v := range stored {
		goals[k] = v
	}
	return goals
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /home/tylerhu/github_project/japanese-learning-app && go build ./...`
Expected: Compilation succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/data/user_store.go
git commit -m "feat(data): add today-completed queries, daily goals methods to UserStore"
```

---

### Task 4: Service Layer — Daily Goals Method

**Files:**
- Modify: `internal/module/user/service.go`

- [ ] **Step 1: Add UpdateDailyGoals to service**

Add after `GetStats` method (line 252):

```go
// UpdateDailyGoals updates the user's per-module daily goals.
func (s *UserService) UpdateDailyGoals(userID int64, req DailyGoalsReq) error {
	slog.Debug("UserService.UpdateDailyGoals called", "user_id", userID)

	goals := map[string]int{
		"word":     req.Word,
		"grammar":  req.Grammar,
		"speaking": req.Speaking,
		"writing":  req.Writing,
	}

	if err := s.store.UpdateDailyGoals(userID, goals); err != nil {
		slog.Error("UserService.UpdateDailyGoals failed", "err", err, "user_id", userID)
		return fmt.Errorf("user.UserService.UpdateDailyGoals: %w", err)
	}

	slog.Debug("UserService.UpdateDailyGoals done", "user_id", userID)
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /home/tylerhu/github_project/japanese-learning-app && go build ./...`
Expected: Compilation fails — `UserStoreInterface` doesn't have `UpdateDailyGoals` yet. That's expected; we'll add it in Task 5.

- [ ] **Step 3: Commit**

```bash
git add internal/module/user/service.go
git commit -m "feat(user): add UpdateDailyGoals to UserService"
```

---

### Task 5: Interface + Adapter + Handler

**Files:**
- Modify: `internal/module/user/service.go` (UserStoreInterface)
- Modify: `internal/data/adapters.go`
- Modify: `internal/module/user/handler.go`

- [ ] **Step 1: Add UpdateDailyGoals and GetDailyGoals to UserStoreInterface**

In `service.go`, add to the `UserStoreInterface`:

```go
	UpdateDailyGoals(userID int64, goals map[string]int) error
	GetDailyGoals(userID int64) (map[string]int, error)
```

Insert after the `GetStats` line and before `// Password reset methods`.

- [ ] **Step 2: Add adapter methods in adapters.go**

Add after the `UpdateUser` adapter method (line 192):

```go
// UpdateDailyGoals delegates to UserStore.UpdateDailyGoals.
func (a *UserStoreAdapter) UpdateDailyGoals(userID int64, goals map[string]int) error {
	return a.s.UpdateDailyGoals(userID, goals)
}

// GetDailyGoals delegates to UserStore.GetDailyGoals.
func (a *UserStoreAdapter) GetDailyGoals(userID int64) (map[string]int, error) {
	return a.s.GetDailyGoals(userID)
}
```

- [ ] **Step 3: Add handler route and function**

In `handler.go`, add the route in `RegisterProtectedRoutes`:

```go
	mux.HandleFunc("PUT /api/v1/users/me/daily-goals", h.handleUpdateDailyGoals)
```

Add after `handleChangePassword` (line 193):

```go
// handleUpdateDailyGoals handles PUT /api/v1/users/me/daily-goals
func (h *UserHandler) handleUpdateDailyGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req DailyGoalsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	if err := h.svc.UpdateDailyGoals(userID, req); err != nil {
		slog.Error("handleUpdateDailyGoals failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{
		"message": "Daily goals updated.",
	}})
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /home/tylerhu/github_project/japanese-learning-app && go build ./...`
Expected: Compilation succeeds.

- [ ] **Step 5: Start server and smoke-test the new endpoint**

Run: `cd /home/tylerhu/github_project/japanese-learning-app && go run ./backend/cmd/server/ &`
Wait for server to start, then:

```bash
# Login to get a token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}' | jq -r '.data.token')

# Update daily goals
curl -s -X PUT http://localhost:8080/api/v1/users/me/daily-goals \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"word":30,"grammar":10,"speaking":5,"writing":5}' | jq .

# Check stats include daily_goal and today_completed
curl -s http://localhost:8080/api/v1/users/stats \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected: The stats response includes `daily_goal` and `today_completed` in each module stat.

- [ ] **Step 6: Stop the server and commit**

```bash
kill %1 2>/dev/null
git add internal/module/user/service.go internal/data/adapters.go internal/module/user/handler.go
git commit -m "feat(user): add PUT /api/v1/users/me/daily-goals endpoint with adapter"
```

---

### Task 6: Frontend Types

**Files:**
- Modify: `front/react/src/types/api.ts`

- [ ] **Step 1: Update ModuleStat and add DailyGoals type**

```typescript
export interface ModuleStat {
  due_count: number
  mastered_count: number
  total_count: number
  today_completed: number
  daily_goal: number
}

export interface DailyGoals {
  word: number
  grammar: number
  speaking: number
  writing: number
}
```

- [ ] **Step 2: Commit**

```bash
git add front/react/src/types/api.ts
git commit -m "feat(frontend): add today_completed, daily_goal to ModuleStat; add DailyGoals type"
```

---

### Task 7: Frontend API Client

**Files:**
- Modify: `front/react/src/api/user.ts`

- [ ] **Step 1: Add updateDailyGoals and getStats functions**

Add after the `changePassword` function:

```typescript
import type { UserStats } from '../types/api'

export async function getStats(): Promise<UserStats> {
  return apiFetch<UserStats>('GET', '/api/v1/users/stats')
}

export async function updateDailyGoals(goals: { word: number; grammar: number; speaking: number; writing: number }): Promise<void> {
  return apiFetch<void>('PUT', '/api/v1/users/me/daily-goals', goals)
}
```

- [ ] **Step 2: Commit**

```bash
git add front/react/src/api/user.ts
git commit -m "feat(frontend): add getStats and updateDailyGoals API functions"
```

---

### Task 8: i18n Translations

**Files:**
- Modify: `front/react/src/i18n/locales/zh.ts`
- Modify: `front/react/src/i18n/locales/en.ts`

- [ ] **Step 1: Add Chinese translations**

In `zh.ts`, add to `settings` section:
```typescript
dailyGoals: '每日目标',
dailyGoalWord: '单词',
dailyGoalGrammar: '语法',
dailyGoalSpeaking: '口语',
dailyGoalWriting: '写作',
saveDailyGoals: '保存目标',
saveDailyGoalsSuccess: '每日目标已更新',
```

Add to `home` section:
```typescript
todayProgress: '今日 {{completed}}/{{goal}}',
```

- [ ] **Step 2: Add English translations**

In `en.ts`, add corresponding keys in the same locations.

- [ ] **Step 3: Commit**

```bash
git add front/react/src/i18n/locales/zh.ts front/react/src/i18n/locales/en.ts
git commit -m "feat(frontend): add i18n keys for daily goals settings and home progress"
```

---

### Task 9: Settings Page — Daily Goals Section

**Files:**
- Modify: `front/react/src/pages/settings/SettingsPage.tsx`
- Modify: `front/react/src/pages/settings/SettingsPage.module.css`

- [ ] **Step 1: Add daily goals state and save logic**

After the volume state line:
```typescript
const [volume, setVol] = useState(getVolume() * 100)
```

Add:
```typescript
// Daily goals
const [dailyWord, setDailyWord] = useState(20)
const [dailyGrammar, setDailyGrammar] = useState(5)
const [dailySpeaking, setDailySpeaking] = useState(3)
const [dailyWriting, setDailyWriting] = useState(3)
const [dailyGoalMsg, setDailyGoalMsg] = useState<{ text: string; ok: boolean } | null>(null)

// Load current daily goals from stats on mount
useEffect(() => {
  import('../../api/user').then(({ getStats }) => {
    getStats().then(stats => {
      setDailyWord(stats.modules.word?.daily_goal ?? 20)
      setDailyGrammar(stats.modules.grammar?.daily_goal ?? 5)
      setDailySpeaking(stats.modules.speaking?.daily_goal ?? 3)
      setDailyWriting(stats.modules.writing?.daily_goal ?? 5)
    }).catch(() => {})
  })
}, [])
```

Add the save handler before `const handleSaveProfile`:
```typescript
const handleSaveDailyGoals = async () => {
  try {
    await updateDailyGoals({ word: dailyWord, grammar: dailyGrammar, speaking: dailySpeaking, writing: dailyWriting })
    setDailyGoalMsg({ text: t('settings.saveDailyGoalsSuccess'), ok: true })
  } catch {
    setDailyGoalMsg({ text: 'Error', ok: false })
  }
}
```

Add the `useEffect` import at the top:
```typescript
import { useState, useEffect } from 'react'
```

And import `updateDailyGoals`:
```typescript
import { updateProfile, changePassword, updateDailyGoals } from '../../api/user'
```

Wait, `updateDailyGoals` is already imported from the api/user module — update the import line to include it.

- [ ] **Step 2: Add daily goals card UI in the JSX**

After the audio settings card, add:

```tsx
<div className={styles.card}>
  <h2 className={styles.cardTitle}>{t('settings.dailyGoals')}</h2>
  <div className={styles.goalGrid}>
    <div className={styles.field}>
      <label className={styles.label}>{t('settings.dailyGoalWord')}</label>
      <input className={styles.input} type="number" min={0} max={200} value={dailyWord} onChange={e => setDailyWord(Number(e.target.value))} />
    </div>
    <div className={styles.field}>
      <label className={styles.label}>{t('settings.dailyGoalGrammar')}</label>
      <input className={styles.input} type="number" min={0} max={50} value={dailyGrammar} onChange={e => setDailyGrammar(Number(e.target.value))} />
    </div>
    <div className={styles.field}>
      <label className={styles.label}>{t('settings.dailyGoalSpeaking')}</label>
      <input className={styles.input} type="number" min={0} max={20} value={dailySpeaking} onChange={e => setDailySpeaking(Number(e.target.value))} />
    </div>
    <div className={styles.field}>
      <label className={styles.label}>{t('settings.dailyGoalWriting')}</label>
      <input className={styles.input} type="number" min={0} max={20} value={dailyWriting} onChange={e => setDailyWriting(Number(e.target.value))} />
    </div>
  </div>
  <button className={styles.saveBtn} onClick={handleSaveDailyGoals}>
    {t('settings.saveDailyGoals')}
  </button>
  {dailyGoalMsg && (
    <div className={`${styles.msg} ${dailyGoalMsg.ok ? styles.msgSuccess : styles.msgError}`}>
      {dailyGoalMsg.text}
    </div>
  )}
</div>
```

- [ ] **Step 3: Add CSS for goal grid**

In `SettingsPage.module.css`, add:
```css
.goalGrid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
}
```

- [ ] **Step 4: Build frontend and verify**

Run: `cd /home/tylerhu/github_project/japanese-learning-app/front && npm run build`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add front/react/src/pages/settings/SettingsPage.tsx front/react/src/pages/settings/SettingsPage.module.css
git commit -m "feat(frontend): add daily goals configuration section to settings page"
```

---

### Task 10: Home Page — Today Progress Display

**Files:**
- Modify: `front/react/src/pages/home/HomePage.tsx`
- Modify: `front/react/src/pages/home/HomePage.module.css`

- [ ] **Step 1: Update module card rendering to show today progress**

In `HomePage.tsx`, replace the module card content inside the `MODULE_CONFIG.map` callback (lines 69-82) with:

```tsx
<Link key={mod.key} to={mod.to} className={styles.moduleLink}>
  <Card hoverable padding="md" className={styles.moduleCard}>
    <div className={styles.moduleHeader}>
      <span className={styles.moduleIcon}>{mod.icon}</span>
      <span className={styles.moduleLabel}>{t(mod.labelKey)}</span>
      {s.due_count > 0 && (
        <span className={styles.dueBadge}>{s.due_count}</span>
      )}
    </div>
    {/* Today's progress */}
    <div className={styles.todayRow}>
      <span className={styles.todayLabel}>{t('home.todayProgress', { completed: s.today_completed, goal: s.daily_goal })}</span>
      {s.today_completed >= s.daily_goal && s.daily_goal > 0 && (
        <span className={styles.todayDone}>✓</span>
      )}
    </div>
    <ProgressBar value={s.daily_goal > 0 ? Math.min(100, Math.round((s.today_completed / s.daily_goal) * 100)) : 0} />
    {/* Overall progress */}
    <ProgressBar value={s.total_count > 0 ? Math.round((s.mastered_count / s.total_count) * 100) : 0} label={t('home.mastered', { mastered: s.mastered_count, total: s.total_count })} />
  </Card>
</Link>
```

- [ ] **Step 2: Add CSS for today progress row**

In `HomePage.module.css`, add after `.dueBadge`:

```css
.todayRow {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.todayLabel {
  font-size: var(--font-size-sm);
  color: var(--color-success);
  font-weight: var(--font-weight-medium);
}

.todayDone {
  font-size: var(--font-size-sm);
  color: var(--color-success);
}
```

- [ ] **Step 3: Build frontend and verify**

Run: `cd /home/tylerhu/github_project/japanese-learning-app/front && npm run build`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add front/react/src/pages/home/HomePage.tsx front/react/src/pages/home/HomePage.module.css
git commit -m "feat(frontend): add today progress display per module on home page"
```

---

### Task 11: End-to-End Verification

- [ ] **Step 1: Start the full app**

Run: `cd /home/tylerhu/github_project/japanese-learning-app && make web`
Open the app in a browser and log in.

- [ ] **Step 2: Verify settings page**

Navigate to Settings. Verify:
- "每日目标" section appears with 4 number inputs
- Change values and click "保存目标"
- Success message appears
- Reload page — values persist

- [ ] **Step 3: Verify home page**

Navigate to Home. Verify:
- Each module card shows "今日 X/Y" progress
- Green progress bar fills proportionally
- When completed >= goal, checkmark (✓) appears

- [ ] **Step 4: Verify stats API**

Run:
```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}' | jq -r '.data.token')
curl -s http://localhost:8080/api/v1/users/stats \
  -H "Authorization: Bearer $TOKEN" | jq '.data.modules.word'
```

Expected: Response includes `today_completed` and `daily_goal` fields.
