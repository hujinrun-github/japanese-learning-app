# Settings Page Design

Date: 2026-05-21

## Overview

Add a Settings page accessible via a dropdown menu on the user icon in TopNavBar. The settings page includes account info editing, password change, and global audio volume control.

## 1. Dropdown Menu (TopNavBar)

- Replace user icon `NavLink` (currently → `/`) with a `<button>` that toggles a dropdown
- Dropdown items:
  - "设置" → navigates to `/settings`
  - "退出登录" → calls `logout()` (moved from standalone button)
- Close on click outside
- Username still displayed next to the icon

## 2. Routing

- New protected route: `/settings` → `SettingsPage`
- Added inside `ProtectedLayout` in `App.tsx`

## 3. Settings Page Layout

Three card sections:

**Card 1 — Account Info:**
- Username (editable, default: current name)
- Email (editable, default: current email)
- JLPT Levels (multi-select tags: N5/N4/N3/N2/N1, any combination)
- Save button → calls `PUT /api/user/profile`

**Card 2 — Change Password:**
- Current password
- New password
- Confirm new password
- Save button → calls `PUT /api/user/password`

**Card 3 — Audio Settings:**
- Volume slider (0% – 100%, default 100%)
- Writes to localStorage in real-time, no save button

## 4. Backend API

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/api/user/profile` | Update name, email, jlpt_levels |
| `PUT` | `/api/user/password` | Change password (requires current password) |

**PUT /api/user/profile request:**
```json
{ "name": "string", "email": "string", "jlpt_levels": ["N5", "N4"] }
```

**PUT /api/user/password request:**
```json
{ "current_password": "string", "new_password": "string" }
```

## 5. Database Migration (009)

- Rename `users.jlpt_level` to `users.jlpt_levels`
- Change type to TEXT (JSON array)
- Migrate old data: `'N5'` → `'["N5"]'`

## 6. i18n

New keys: `nav.settings`, `settings.title`, `settings.account`, `settings.password`, `settings.audio`, `settings.volume`, `settings.save`, plus error/success messages.

## 7. Frontend Files

| File | Change |
|------|--------|
| `TopNavBar.tsx` | Dropdown menu replacing icon link + logout button |
| `TopNavBar.module.css` | Dropdown styles |
| `pages/settings/SettingsPage.tsx` | New |
| `pages/settings/SettingsPage.module.css` | New |
| `App.tsx` | Add `/settings` route |
| `types/api.ts` | Update `User.jlpt_level` → `jlpt_levels: string[]` |
| `i18n/locales/*.ts` | New translation keys |

## 8. Backend Files

| File | Change |
|------|--------|
| `internal/module/user/handler.go` | New: PUT profile, PUT password |
| `internal/module/user/model.go` | Update User struct |
| `internal/data/user_store.go` | Update user queries |
| `internal/data/migrations/009_user_jlpt_levels.sql` | New |

## 9. AuthContext Changes

- Add `UPDATE_USER` reducer action to refresh user in state + localStorage
- Expose `updateUser(user: User)` via context so SettingsPage can sync after profile save
- All places consuming `user.jlpt_level` need updating for `jlpt_levels: string[]`
