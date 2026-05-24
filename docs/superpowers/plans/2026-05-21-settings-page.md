# Settings Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Settings page (account info, password change, volume control) accessible from a user icon dropdown menu in TopNavBar.

**Architecture:** Backend: migration to add `name` and rename `goal_level` → `jlpt_levels` (JSON array), two new PUT endpoints (`/api/user/profile`, `/api/user/password`). Frontend: new `SettingsPage` component, dropdown menu replacement for TopNavBar user icon, `UPDATE_USER` reducer in AuthContext, updated i18n keys.

**Tech Stack:** Go 1.24, net/http, SQLite, React 18, TypeScript, CSS Modules, react-router-dom v6

---

### Task 1: Database Migration

**Files:**
- Create: `internal/data/migrations/009_user_jlpt_levels.sql`

- [ ] **Step 1: Write the migration SQL**

```sql
-- 009_user_jlpt_levels.sql
-- Add name column, rename goal_level to jlpt_levels (JSON array for multi-select).

ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN jlpt_levels TEXT NOT NULL DEFAULT '["N5"]';
UPDATE users SET jlpt_levels = json_array(goal_level) WHERE jlpt_levels = '["N5"]';
```

Note: We cannot drop `goal_level` in SQLite without recreating the table. Leave the old column in place — it won't cause issues since no code references it after the model update.

- [ ] **Step 2: Commit**

```bash
git add internal/data/migrations/009_user_jlpt_levels.sql
git commit -m "feat(user): add migration 009 for name and jlpt_levels columns"
```

---

### Task 2: Backend Model Update

**Files:**
- Modify: `internal/module/user/model.go`

- [ ] **Step 1: Update the User struct and add request types**

Replace the `User` struct (lines 35-42) and add new request types after `ResetPasswordReq`:

```go
// User 用户账户
type User struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	JLPTLevels  []string  `json:"jlpt_levels"`
	StreakDays  int       `json:"streak_days"`
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateProfileReq 更新个人信息请求
type UpdateProfileReq struct {
	Name       string   `json:"name"`
	Email      string   `json:"email"`
	JLPTLevels []string `json:"jlpt_levels"`
}

// UpdatePasswordReq 修改密码请求
type UpdatePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
```

Also remove the `JLPTLevel` type and its `LevelN5..LevelN1` constants (lines 24-33) since we now use `[]string`.

- [ ] **Step 2: Build to check compilation**

```bash
go build ./internal/module/user/
```

Expected: compilation errors in service.go and handler.go (expected, will fix in later tasks).

- [ ] **Step 3: Commit**

```bash
git add internal/module/user/model.go
git commit -m "feat(user): update User model with name and jlpt_levels array"
```

---

### Task 3: Update UserStore Data Layer

**Files:**
- Modify: `internal/data/user_store.go`

- [ ] **Step 1: Add UpdateUser and update all queries**

Changes to `internal/data/user_store.go`:

**a) Update `Create` signature and query (line 26):**
```go
func (s *UserStore) Create(name, email, passwordHash string, jlptLevelsJSON string) (*user.User, error) {
	slog.Debug("UserStore.Create called", "email", email)

	res, err := s.db.Exec(
		`INSERT INTO users (name, email, password_hash, jlpt_levels) VALUES (?, ?, ?, ?)`,
		name, email, passwordHash, jlptLevelsJSON,
	)
```

**b) Update GetByEmail query (line 61):**
```go
row := s.db.QueryRow(
	`SELECT id, name, email, jlpt_levels, streak_days, created_at FROM users WHERE email = ?`, email,
)
```

And scan (line 66):
```go
var u user.User
var createdAt string
var jlptLevelsJSON string
err := row.Scan(&u.ID, &u.Name, &u.Email, &jlptLevelsJSON, &u.StreakDays, &createdAt)
// ...
u.JLPTLevels = parseJLPTLevels(jlptLevelsJSON)
```

**c) Update GetByID query (line 90) and scan (line 95):** — same pattern as GetByEmail.

**d) Add UpdateUser method (after UpdatePassword, before GetStats):**
```go
func (s *UserStore) UpdateUser(id int64, name, email, jlptLevelsJSON string) error {
	slog.Debug("UserStore.UpdateUser called", "user_id", id)

	_, err := s.db.Exec(
		`UPDATE users SET name = ?, email = ?, jlpt_levels = ? WHERE id = ?`,
		name, email, jlptLevelsJSON, id,
	)
	if err != nil {
		slog.Error("failed to update user", "err", err, "user_id", id)
		if isUniqueConstraintError(err) {
			return fmt.Errorf("data.UserStore.UpdateUser: %w", user.ErrEmailTaken)
		}
		return fmt.Errorf("data.UserStore.UpdateUser: %w", err)
	}

	slog.Debug("UserStore.UpdateUser done", "user_id", id)
	return nil
}
```

**e) Add helper at bottom of file:**
```go
func parseJLPTLevels(jsonStr string) []string {
	if jsonStr == "" {
		return []string{"N5"}
	}
	var levels []string
	if err := json.Unmarshal([]byte(jsonStr), &levels); err != nil {
		return []string{"N5"}
	}
	if len(levels) == 0 {
		return []string{"N5"}
	}
	return levels
}
```

Add `"encoding/json"` to imports.

- [ ] **Step 2: Build to check compilation**

```bash
go build ./internal/data/
```

Expected: compilation errors in adapters.go (expected).

- [ ] **Step 3: Commit**

```bash
git add internal/data/user_store.go
git commit -m "feat(user): add UpdateUser and jlpt_levels JSON support to user store"
```

---

### Task 4: Update Adapter and Service Interface

**Files:**
- Modify: `internal/data/adapters.go`
- Modify: `internal/module/user/service.go`

- [ ] **Step 1: Update UserStoreAdapter.CreateUser**

```go
func (a *UserStoreAdapter) CreateUser(u user.User, passwordHash string) (*user.User, error) {
	slog.Debug("UserStoreAdapter.CreateUser called", "email", u.Email)
	jlptJSON, _ := json.Marshal(u.JLPTLevels)
	created, err := a.s.Create(u.Name, u.Email, passwordHash, string(jlptJSON))
	if err != nil {
		return nil, fmt.Errorf("UserStoreAdapter.CreateUser: %w", err)
	}
	return created, nil
}
```

Add `"encoding/json"` to imports in adapters.go.

**Step 1b: Add UpdateUser to adapter:**
```go
// UpdateUser updates user profile fields.
func (a *UserStoreAdapter) UpdateUser(id int64, name, email string, jlptLevels []string) error {
	jlptJSON, _ := json.Marshal(jlptLevels)
	return a.s.UpdateUser(id, name, email, string(jlptJSON))
}
```

- [ ] **Step 2: Update service.go register method**

In `UserService.Register` (line 54):
```go
u := User{
	Name:       req.Name,  // add this
	Email:      req.Email,
	JLPTLevels: []string{string(req.GoalLevel)}, // was GoalLevel
	CreatedAt:  time.Now(),
}
```

Add `Name` field to `RegisterReq`:
```go
type RegisterReq struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`
	GoalLevel JLPTLevel `json:"goal_level"`
}
```

**Step 2b: Add UpdateProfile and UpdatePassword to service interface (line 20):**
Add to `UserStoreInterface`:
```go
UpdateUser(id int64, name, email string, jlptLevels []string) error
```

Add `ErrWrongPassword`:
```go
var ErrWrongPassword = errors.New("current password is incorrect")
```

**Step 2c: Add service methods:**
```go
// UpdateProfile updates the user's profile fields.
func (s *UserService) UpdateProfile(userID int64, req UpdateProfileReq) (*User, error) {
	slog.Debug("UserService.UpdateProfile called", "user_id", userID)

	if err := s.store.UpdateUser(userID, req.Name, req.Email, req.JLPTLevels); err != nil {
		slog.Error("UserService.UpdateProfile: UpdateUser failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("user.UserService.UpdateProfile UpdateUser: %w", err)
	}

	u, err := s.store.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user.UserService.UpdateProfile GetUserByID: %w", err)
	}

	slog.Debug("UserService.UpdateProfile done", "user_id", userID)
	return u, nil
}

// ChangePassword validates current password and sets a new one.
func (s *UserService) ChangePassword(userID int64, req UpdatePasswordReq) error {
	slog.Debug("UserService.ChangePassword called", "user_id", userID)

	_, storedHash, err := s.store.GetUserByEmail(/* need email */)
	// We need the user's email. Get it from GetUserByID.
	u, err := s.store.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("user.UserService.ChangePassword GetUserByID: %w", err)
	}

	u2, storedHash, err := s.store.GetUserByEmail(u.Email)
	if err != nil {
		return fmt.Errorf("user.UserService.ChangePassword GetUserByEmail: %w", err)
	}

	if hashPassword(req.CurrentPassword) != storedHash {
		return ErrWrongPassword
	}

	if err := s.store.UpdatePassword(userID, hashPassword(req.NewPassword)); err != nil {
		return fmt.Errorf("user.UserService.ChangePassword UpdatePassword: %w", err)
	}

	slog.Info("UserService.ChangePassword done", "user_id", userID)
	return nil
}
```

- [ ] **Step 3: Build**

```bash
go build ./internal/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/data/adapters.go internal/module/user/service.go
git commit -m "feat(user): add UpdateProfile and ChangePassword service methods"
```

---

### Task 5: Add Handler Endpoints

**Files:**
- Modify: `internal/module/user/handler.go`

- [ ] **Step 1: Add handler methods and register routes**

Add `"strings"` to imports.

Add to `RegisterProtectedRoutes` (after line 33):
```go
mux.HandleFunc("PUT /api/v1/users/me/profile", h.handleUpdateProfile)
mux.HandleFunc("PUT /api/v1/users/me/password", h.handleChangePassword)
```

Add handler methods before `handleGetStats`:

```go
// handleUpdateProfile handles PUT /api/v1/users/me/profile
func (h *UserHandler) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req UpdateProfileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Email) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "name and email are required", "")
		return
	}

	u, err := h.svc.UpdateProfile(userID, req)
	if err != nil {
		slog.Error("handleUpdateProfile failed", "err", err, "user_id", userID)
		if errors.Is(err, ErrEmailTaken) {
			httputil.WriteError(w, http.StatusConflict, "ERR_EMAIL_TAKEN", "email already registered", "")
		} else {
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: u})
}

// handleChangePassword handles PUT /api/v1/users/me/password
func (h *UserHandler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req UpdatePasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "current_password and new_password are required", "")
		return
	}

	if err := h.svc.ChangePassword(userID, req); err != nil {
		slog.Error("handleChangePassword failed", "err", err, "user_id", userID)
		if errors.Is(err, ErrWrongPassword) {
			httputil.WriteError(w, http.StatusBadRequest, "ERR_WRONG_PASSWORD", "current password is incorrect", "")
		} else {
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{
		"message": "Password changed successfully.",
	}})
}
```

- [ ] **Step 2: Update RegisterReq validation in handleRegister**

Add name check (line 43):
```go
if req.Name == "" || req.Email == "" || req.Password == "" {
	httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "name, email and password are required", "")
	return
}
```

- [ ] **Step 3: Build and verify**

```bash
go build ./internal/module/user/
go build ./backend/cmd/server/
```

- [ ] **Step 4: Commit**

```bash
git add internal/module/user/handler.go
git commit -m "feat(user): add PUT /api/v1/users/me/profile and /api/v1/users/me/password endpoints"
```

---

### Task 6: Backend Test

**Files:**
- Create: `internal/module/user/handler_test.go` (or update existing test file)

- [ ] **Step 1: Write table-driven test for UpdateProfile**

Check if a test file exists:
```bash
ls internal/module/user/*_test.go
```

If exists, add tests. If not, create a basic integration test.

For now, since this project prefers integration tests per CLAUDE.md, verify via `make test`:
```bash
make test
```

- [ ] **Step 2: Fix any test compilation errors from model changes**

Search for references to old `GoalLevel` or `JLPTLevel`:
```bash
grep -r "GoalLevel\|JLPTLevel" internal/ --include="*.go" | grep -v "_test.go" | grep -v ".git"
```

Fix any remaining references in non-test files (e.g., middleware, other modules).

- [ ] **Step 3: Run tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix(user): fix compilation after model changes"
```

---

### Task 7: Frontend Types and API Update

**Files:**
- Modify: `front/react/src/types/api.ts`
- Modify: `front/react/src/api/client.ts` (no changes needed — verify)

- [ ] **Step 1: Update User interface**

Change lines 15-22 in `types/api.ts`:
```ts
export interface User {
  id: number
  name: string
  email: string
  jlpt_levels: JLPTLevel[]
  streak_days: number
  created_at: string
}
```

- [ ] **Step 2: Add API functions for new endpoints**

Create a new file or add to an existing API file. Check what exists:

```bash
ls front/react/src/api/
```

If there's a user API file, add there. Otherwise, add to `client.ts` or create `front/react/src/api/user.ts`:

```ts
import { apiFetch } from './client'
import type { User } from '../types/api'

export async function updateProfile(name: string, email: string, jlptLevels: string[]): Promise<User> {
  return apiFetch<User>('PUT', '/api/v1/users/me/profile', { name, email, jlpt_levels: jlptLevels })
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  return apiFetch<void>('PUT', '/api/v1/users/me/password', { current_password: currentPassword, new_password: newPassword })
}
```

- [ ] **Step 3: Build frontend to check for type errors**

```bash
cd front/react && npx tsc --noEmit 2>&1 | head -30
```

Expected: errors from files still using `jlpt_level` (singular) — will fix in Task 9.

- [ ] **Step 4: Commit**

```bash
git add front/react/src/types/api.ts front/react/src/api/user.ts
git commit -m "feat(frontend): update User type to jlpt_levels array, add profile API functions"
```

---

### Task 8: i18n Keys

**Files:**
- Modify: `front/react/src/i18n/locales/zh.ts`
- Modify: `front/react/src/i18n/locales/en.ts`
- Modify: `front/react/src/i18n/locales/ja.ts`

- [ ] **Step 1: Add settings keys to zh.ts**

Add after the existing `nav` block (find the closing `},` of nav):

```ts
settings: {
  title: '设置',
  account: '账号信息',
  name: '用户名',
  email: '邮箱',
  jlptLevels: 'JLPT 等级',
  saveProfile: '保存',
  saveProfileSuccess: '个人信息已更新',
  password: '修改密码',
  currentPassword: '当前密码',
  newPassword: '新密码',
  confirmPassword: '确认新密码',
  savePassword: '修改密码',
  savePasswordSuccess: '密码已修改',
  passwordMismatch: '两次输入的密码不一致',
  wrongPassword: '当前密码错误',
  audio: '音频设置',
  volume: '音量',
},
```

Also add to the `nav` block:
```ts
settings: '设置',
```

- [ ] **Step 2: Add English keys to en.ts**

Same structure:
```ts
nav: {
  // ... existing keys ...
  settings: 'Settings',
},
settings: {
  title: 'Settings',
  account: 'Account Information',
  name: 'Username',
  email: 'Email',
  jlptLevels: 'JLPT Levels',
  saveProfile: 'Save',
  saveProfileSuccess: 'Profile updated',
  password: 'Change Password',
  currentPassword: 'Current Password',
  newPassword: 'New Password',
  confirmPassword: 'Confirm New Password',
  savePassword: 'Change Password',
  savePasswordSuccess: 'Password changed',
  passwordMismatch: 'Passwords do not match',
  wrongPassword: 'Current password is incorrect',
  audio: 'Audio Settings',
  volume: 'Volume',
},
```

- [ ] **Step 3: Add Japanese keys to ja.ts**

```ts
nav: {
  // ... existing keys ...
  settings: '設定',
},
settings: {
  title: '設定',
  account: 'アカウント情報',
  name: 'ユーザー名',
  email: 'メールアドレス',
  jlptLevels: 'JLPT レベル',
  saveProfile: '保存',
  saveProfileSuccess: 'プロフィールを更新しました',
  password: 'パスワード変更',
  currentPassword: '現在のパスワード',
  newPassword: '新しいパスワード',
  confirmPassword: '新しいパスワード（確認）',
  savePassword: '変更',
  savePasswordSuccess: 'パスワードを変更しました',
  passwordMismatch: 'パスワードが一致しません',
  wrongPassword: '現在のパスワードが正しくありません',
  audio: 'オーディオ設定',
  volume: '音量',
},
```

- [ ] **Step 4: Commit**

```bash
git add front/react/src/i18n/locales/zh.ts front/react/src/i18n/locales/en.ts front/react/src/i18n/locales/ja.ts
git commit -m "feat(frontend): add settings i18n keys"
```

---

### Task 9: AuthContext Update and jlpt_level → jlpt_levels Migration

**Files:**
- Modify: `front/react/src/contexts/AuthContext.tsx`
- Modify: all files using `user.jlpt_level`

- [ ] **Step 1: Add UPDATE_USER action to AuthContext**

In `AuthContext.tsx`, add to `AuthAction` type (line 17):
```ts
| { type: 'UPDATE_USER'; user: User }
```

Add to `authReducer` (line 22, add before default or return):
```ts
case 'UPDATE_USER':
  return { ...state, user: action.user }
```

Add to `AuthContextValue` (line 36):
```ts
updateUser: (user: User) => void
```

In `AuthProvider`, add `updateUser` callback (after `logout`):
```ts
const updateUser = useCallback((user: User) => {
  dispatch({ type: 'UPDATE_USER', user })
  localStorage.setItem('user', JSON.stringify(user))
}, [])
```

Add `updateUser` to the context value (line 79).

In `useAuth` hook, update the return type and error message if needed.

- [ ] **Step 2: Fix all jlpt_level references**

Run to find all files:
```bash
grep -r "jlpt_level" front/react/src/ --include="*.ts" --include="*.tsx" -l
```

Update each file:
- `jlpt_level` → `jlpt_levels` (type definition — already done in Task 7)
- `user.jlpt_level` → `user.jlpt_levels` 
- Any JLPT level display needs to render an array (join with commas or show tags)

Key files likely affected:
- `HomePage.tsx` — may show user's JLPT level
- `RegisterPage.tsx` — sends `goal_level` on registration, may show JLPT select
- Any component displaying `user.jlpt_level`

Search for exact usage:
```bash
grep -rn "jlpt_level" front/react/src/ --include="*.ts" --include="*.tsx" | grep -v node_modules
```

For each usage, update to handle `string[]` instead of `string`. For display: `user.jlpt_levels.join(', ')`. For registration, keep sending a single `goal_level` to the existing endpoint (backend converts it).

- [ ] **Step 3: Verify type check**

```bash
cd front/react && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add front/react/src/contexts/AuthContext.tsx
git add <all modified files from Step 2>
git commit -m "fix(frontend): migrate jlpt_level to jlpt_levels array, add updateUser to auth context"
```

---

### Task 10: Settings Page Component

**Files:**
- Create: `front/react/src/pages/settings/SettingsPage.tsx`
- Create: `front/react/src/pages/settings/SettingsPage.module.css`

- [ ] **Step 1: Check CSS variables available**

```bash
grep -E "^  --" front/react/src/styles/variables.css | head -20
```

(Use existing design tokens for consistency.)

- [ ] **Step 2: Create SettingsPage.module.css**

```css
.page {
  max-width: 600px;
  margin: 0 auto;
  padding: var(--space-6) var(--space-4);
}

.title {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: var(--space-6);
}

.card {
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-5);
  margin-bottom: var(--space-5);
}

.cardTitle {
  font-size: 1.1rem;
  font-weight: 600;
  margin-bottom: var(--space-4);
}

.field {
  margin-bottom: var(--space-3);
}

.label {
  display: block;
  font-size: 0.875rem;
  color: var(--color-text-secondary);
  margin-bottom: var(--space-1);
}

.input {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: 1rem;
  background: var(--color-bg);
  color: var(--color-text);
}

.input:focus {
  outline: none;
  border-color: var(--color-primary);
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.tag {
  padding: var(--space-1) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  font-size: 0.875rem;
  cursor: pointer;
  background: var(--color-bg);
  color: var(--color-text-secondary);
  transition: all 0.15s;
}

.tagSelected {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
}

.saveBtn {
  margin-top: var(--space-4);
  padding: var(--space-2) var(--space-5);
  background: var(--color-primary);
  color: white;
  border: none;
  border-radius: var(--radius-sm);
  font-size: 0.95rem;
  cursor: pointer;
}

.saveBtn:hover {
  opacity: 0.9;
}

.slider {
  width: 100%;
  accent-color: var(--color-primary);
}

.msg {
  margin-top: var(--space-2);
  font-size: 0.875rem;
}

.msgSuccess {
  color: var(--color-success);
}

.msgError {
  color: var(--color-error);
}
```

- [ ] **Step 3: Create SettingsPage.tsx**

```tsx
import { useState } from 'react'
import { useTranslation } from '../../hooks/useTranslation'
import { useAuth } from '../../contexts/AuthContext'
import { getVolume, setVolume } from '../../util/audioVolume'
import { updateProfile, changePassword } from '../../api/user'
import type { JLPTLevel } from '../../types/api'
import styles from './SettingsPage.module.css'

const ALL_LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

export default function SettingsPage() {
  const { t } = useTranslation()
  const { user, updateUser } = useAuth()

  // Account form
  const [name, setName] = useState(user?.name ?? '')
  const [email, setEmail] = useState(user?.email ?? '')
  const [selectedLevels, setSelectedLevels] = useState<JLPTLevel[]>(user?.jlpt_levels ?? ['N5'])
  const [profileMsg, setProfileMsg] = useState<{ text: string; ok: boolean } | null>(null)

  // Password form
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwMsg, setPwMsg] = useState<{ text: string; ok: boolean } | null>(null)

  // Volume
  const [volume, setVol] = useState(getVolume() * 100)

  const toggleLevel = (level: JLPTLevel) => {
    setSelectedLevels(prev =>
      prev.includes(level) ? prev.filter(l => l !== level) : [...prev, level]
    )
  }

  const handleSaveProfile = async () => {
    try {
      const updated = await updateProfile(name.trim(), email.trim(), selectedLevels)
      updateUser(updated)
      setProfileMsg({ text: t('settings.saveProfileSuccess'), ok: true })
    } catch {
      setProfileMsg({ text: 'Error', ok: false })
    }
  }

  const handleSavePassword = async () => {
    if (newPw !== confirmPw) {
      setPwMsg({ text: t('settings.passwordMismatch'), ok: false })
      return
    }
    try {
      await changePassword(currentPw, newPw)
      setPwMsg({ text: t('settings.savePasswordSuccess'), ok: true })
      setCurrentPw(''); setNewPw(''); setConfirmPw('')
    } catch (e: any) {
      const msg = e?.code === 'ERR_WRONG_PASSWORD' ? t('settings.wrongPassword') : 'Error'
      setPwMsg({ text: msg, ok: false })
    }
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{t('settings.title')}</h1>

      {/* Account Info Card */}
      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.account')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.name')}</label>
          <input className={styles.input} value={name} onChange={e => setName(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.email')}</label>
          <input className={styles.input} value={email} onChange={e => setEmail(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.jlptLevels')}</label>
          <div className={styles.tags}>
            {ALL_LEVELS.map(l => (
              <button
                key={l}
                className={`${styles.tag} ${selectedLevels.includes(l) ? styles.tagSelected : ''}`}
                onClick={() => toggleLevel(l)}
              >
                {l}
              </button>
            ))}
          </div>
        </div>
        <button className={styles.saveBtn} onClick={handleSaveProfile}>
          {t('settings.saveProfile')}
        </button>
        {profileMsg && (
          <div className={`${styles.msg} ${profileMsg.ok ? styles.msgSuccess : styles.msgError}`}>
            {profileMsg.text}
          </div>
        )}
      </div>

      {/* Password Card */}
      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.password')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.currentPassword')}</label>
          <input className={styles.input} type="password" value={currentPw} onChange={e => setCurrentPw(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.newPassword')}</label>
          <input className={styles.input} type="password" value={newPw} onChange={e => setNewPw(e.target.value)} />
        </div>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.confirmPassword')}</label>
          <input className={styles.input} type="password" value={confirmPw} onChange={e => setConfirmPw(e.target.value)} />
        </div>
        <button className={styles.saveBtn} onClick={handleSavePassword}>
          {t('settings.savePassword')}
        </button>
        {pwMsg && (
          <div className={`${styles.msg} ${pwMsg.ok ? styles.msgSuccess : styles.msgError}`}>
            {pwMsg.text}
          </div>
        )}
      </div>

      {/* Volume Card */}
      <div className={styles.card}>
        <h2 className={styles.cardTitle}>{t('settings.audio')}</h2>
        <div className={styles.field}>
          <label className={styles.label}>{t('settings.volume')}: {Math.round(volume)}%</label>
          <input
            type="range"
            className={styles.slider}
            min={0}
            max={100}
            value={volume}
            onChange={e => {
              const v = Number(e.target.value)
              setVol(v)
              setVolume(v / 100)
            }}
          />
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Commit**

```bash
git add front/react/src/pages/settings/
git commit -m "feat(frontend): add SettingsPage component"
```

---

### Task 11: TopNavBar Dropdown Menu

**Files:**
- Modify: `front/react/src/components/layout/TopNavBar.tsx`
- Modify: `front/react/src/components/layout/TopNavBar.module.css`

- [ ] **Step 1: Update TopNavBar.tsx**

Replace the user section (lines 39-45) with a dropdown:

```tsx
import { useState, useRef, useEffect } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
// ... keep other imports, add:
import { useTranslation } from '../../hooks/useTranslation'
import { useAuth } from '../../contexts/AuthContext'
import LanguageSwitcher from '../LanguageSwitcher'
import styles from './TopNavBar.module.css'

export default function TopNavBar() {
  const { t } = useTranslation()
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  // ... keep the logo and nav links ...

  return (
    <header className={styles.header}>
      <div className={styles.inner}>
        {/* existing logo + nav links — keep unchanged */}
        <div className={styles.user}>
          <LanguageSwitcher />
          <div className={styles.userMenu} ref={menuRef}>
            <button className={styles.userIcon} onClick={() => setMenuOpen(!menuOpen)} title={t('nav.settings')}>
              👤
            </button>
            {menuOpen && (
              <div className={styles.dropdown}>
                <button className={styles.dropdownItem} onClick={() => { navigate('/settings'); setMenuOpen(false) }}>
                  ⚙ {t('nav.settings')}
                </button>
                <button className={styles.dropdownItem} onClick={() => { logout(); setMenuOpen(false) }}>
                  ➡ {t('nav.logout')}
                </button>
              </div>
            )}
          </div>
          {user && <span className={styles.userName}>{user.name}</span>}
        </div>
      </div>
    </header>
  )
}
```

- [ ] **Step 2: Update TopNavBar.module.css**

Remove old `.logoutBtn` styles. Update `.userIcon` to be a button. Add dropdown styles:

```css
.userMenu {
  position: relative;
}

.userIcon {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 1px solid var(--color-border);
  background: transparent;
  cursor: pointer;
  font-size: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}

.userIcon:hover {
  background: var(--color-bg-hover);
}

.dropdown {
  position: absolute;
  top: 100%;
  right: 0;
  margin-top: var(--space-2);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
  min-width: 160px;
  z-index: var(--z-dropdown);
  overflow: hidden;
}

.dropdownItem {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-4);
  border: none;
  background: transparent;
  color: var(--color-text);
  font-size: 0.9rem;
  cursor: pointer;
  text-align: left;
}

.dropdownItem:hover {
  background: var(--color-bg-hover);
}
```

Remove the `.logoutBtn` class block entirely.

- [ ] **Step 3: Build and check**

```bash
cd front/react && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 4: Commit**

```bash
git add front/react/src/components/layout/TopNavBar.tsx front/react/src/components/layout/TopNavBar.module.css
git commit -m "feat(frontend): replace user icon with dropdown menu"
```

---

### Task 12: Add Route to App.tsx

**Files:**
- Modify: `front/react/src/App.tsx`

- [ ] **Step 1: Add import and route**

Add import:
```tsx
import SettingsPage from './pages/settings/SettingsPage'
```

Add route inside ProtectedLayout (after the notes routes, line ~42):
```tsx
<Route path="/settings" element={<SettingsPage />} />
```

- [ ] **Step 2: Build and type check**

```bash
cd front/react && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add front/react/src/App.tsx
git commit -m "feat(frontend): add /settings route"
```

---

### Task 13: Integration and Final Verification

- [ ] **Step 1: Run backend tests**

```bash
make test
```

- [ ] **Step 2: Build backend**

```bash
make web
```

- [ ] **Step 3: Build frontend**

```bash
cd front/react && npm run build
```

- [ ] **Step 4: Start server and verify**

```bash
make run
```

Manual verification checklist:
1. Login, click user icon — dropdown appears with "设置" and "退出登录"
2. Click "设置" — navigates to `/settings`
3. Change name, email, JLPT levels — save, page refreshes with new data
4. Change password — verify with wrong current password (error), correct (success), mismatch (error)
5. Adjust volume slider — value changes in real-time
6. Click "退出登录" — logs out

- [ ] **Step 5: Commit any final fixes**

```bash
git add -A
git commit -m "chore: final integration fixes"
```
