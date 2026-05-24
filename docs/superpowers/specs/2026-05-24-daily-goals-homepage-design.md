# 每日目标与首页任务展示

## 目标
在首页为单词/语法/口语/写作四个模块展示"今日完成 vs 每日目标"的进度，目标值在设置页可配置。

## 数据模型

### 存储
`users` 表新增 `daily_goals_json TEXT` 列，存储 JSON：
```json
{"word": 20, "grammar": 5, "speaking": 3, "writing": 3}
```
默认值（用户未设置时）：word=20, grammar=5, speaking=3, writing=3。

### API 响应变化
`GET /api/v1/users/stats` 的 `ModuleStat` 新增两个字段：
- `today_completed: int` — 今日已完成数，从 `study_sessions` 按当天 `started_at` 汇总 `completed_count`
- `daily_goal: int` — 每日目标值

### 新 API
`PUT /api/v1/users/me/daily-goals` — 更新每日目标
- 请求体：`{"word": 20, "grammar": 5, "speaking": 3, "writing": 3}`
- 响应：更新后的 `User` 对象

## 后端改动

| 文件 | 改动 |
|------|------|
| `internal/data/migrations/010_daily_goals.sql` | 新增 migration：`ALTER TABLE users ADD COLUMN daily_goals_json TEXT NOT NULL DEFAULT '{}'` |
| `internal/module/user/model.go` | `ModuleStat` 加 `TodayCompleted int`、`DailyGoal int`；新增 `DailyGoalsReq` 结构体 |
| `internal/data/user_store.go` | `GetStats` 查询今日完成数；新增 `UpdateDailyGoals`、`GetDailyGoals` 方法 |
| `internal/module/user/service.go` | 新增 `UpdateDailyGoals` 方法 |
| `internal/module/user/handler.go` | 新增 `PUT /api/v1/users/me/daily-goals` 路由和处理函数 |

## 前端改动

| 文件 | 改动 |
|------|------|
| `front/react/src/types/api.ts` | `ModuleStat` 加 `today_completed`、`daily_goal` |
| `front/react/src/api/user.ts` | 新增 `updateDailyGoals()` |
| `front/react/src/pages/home/HomePage.tsx` | 模块卡片加"今日完成 X/Y"进度条 |
| `front/react/src/pages/settings/SettingsPage.tsx` | 新增"每日目标"设置区 |
| `front/react/src/i18n/locales/zh.ts` | 新增 i18n key |
| `front/react/src/pages/home/HomePage.module.css` | 新增今日进度条样式 |

## 首页布局
每个模块卡片从上到下：
1. 图标 + 标签 + due 角标（保持不变）
2. **新增**：今日进度条 `━━━━━ 8/20 今日`（绿色主题）
3. 总进度条 `已掌握 120/500`（保持不变）

## 边界情况
- 未设置目标：使用默认值 20/5/3/3
- 今日无学习记录：today_completed = 0
- 已完成超过目标：进度条满格，显示如 `25/20 ✓`
