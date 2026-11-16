# 单词复习：重复卡片与加载慢修复（2026-11-16）

## 概述

本次修复单词复习页的两个问题：

1. **学过的词反复出现** —— 卡片答过之后仍会在刷新/重新进入时被当作新词或到期词再次发出。
2. **页面加载慢** —— PostgreSQL 部署下，一次复习队列请求会产生上千次串行数据库往返。

两次修复均按项目宪法走 Red-Green-Refactor，单元测试采用表格驱动风格；**无数据库 schema 变更、无迁移**。

---

## 一、学过的词反复出现

### 根因

| # | 问题 | 位置 |
|---|---|---|
| 1 | 认读模式答对只翻面，不写学习记录；必须再点「容易/一般/困难」才落库。用户看完答案直接离开，该词就没有 `word_record`，下次仍被当作新词 | `front/react/src/pages/word/WordReviewPage.tsx` |
| 2 | `/rate` 保存失败被 `catch` 吞掉后仍前进到下一张，界面看起来「学过了」但数据库没变 | 同上 |
| 3 | 队列每次请求重新拼「到期词（按 `next_review_at` 升序、全量）+ 无记录的新词」，无「本轮已答」状态；且到期词不限量，历史逾期记录会把整轮占满，新词排不进来 | `internal/module/word/service.go`、`handler.go` |

排查时 `data/app.db` 中 91 条 `word_records` 的 `next_review_at` 全部停在 2026-05~06，即全部逾期；当前用户 12 条记录中最老的一条正是截图首卡「電車」，与队列顺序完全吻合。

### 修复

- 新增幂等接口 `POST /api/v1/words/{id}/seen`：仅当该用户对此词没有记录时，写入一条保底记录（`mastery_level=0`、`ease_factor=2.5`、`interval=1`、`next_review_at=+24h`、`review_history` 为空）。正式评分基于这条记录计算，不会重复计数；已有记录一律不动。
- 前端在认读答对/答错、听写翻面时调用该接口（fire-and-forget，失败只记日志、不阻塞答题），保证「看过答案」这件事一定落库。
- `/rate` 改为**保存成功才进入下一张**，失败显示可见错误并允许重试；两处自动提交 `hard` 的失败也补了日志与提示。
- `GetReviewQueue` 增加 `limit` 参数并加两条规则：
  - `UpdatedAt` 是当天的到期词跳过（刷新页面不重放同一批）；
  - 到期词最多占 `limit/2`（至少 1 张），其余用新词补足，凑够即停。
- `GET /api/v1/words/queue` 先算好 `limit` 再下推给 service，去掉 handler 层的二次截断；统一复习队列 `/api/v1/review/queue` 的词侧上限设为 50。

### 效果（真实数据模拟）

user 8 / N5 / limit=20：从「12 张全是 5–6 月逾期老词」变为「10 张到期词 + 10 张新词（先生、友達、家族…）」。答过一轮后，这些词当天不会再出现；按 SM-2 排期在之后的日子回来属于预期行为。

---

## 二、加载慢：PostgreSQL N+1

### 根因

- `internal/data/postgres/word_store.go` 的 `scanWords` 对 `ListByLevel` 返回的**每一个词**单独查一次 `word_examples`：N5 114 次、N4 691 次远程往返；`GetByID` 又是每词 2 次查询。
- service 对每条到期记录单独 `GetByID`、对等级内每个词单独 `GetRecord`。

本地 SQLite 把例句存在 `examples_json` 列里，一次查询即可，所以本地实测队列仅 ~1.1ms（N5）/ ~6.0ms（N4），问题只在 PG 部署暴露。

### 修复

- `scanWords` 改为「先扫完词表 → 一次批量查询 `loadExamplesByWordIDs` → 按 `word_id` 分组回填」；`GetByID` 复用同一批量函数，逐词版本删除。批量查询用 `IN ($1,…)` 动态占位符而非 `ANY($1::bigint[])`，避免 `database/sql` 的 pgx 数组编码兼容问题；`ORDER BY word_id, position` 保持词内例句顺序。
- `GetReviewQueue` 用一次 `ListByLevel` 的结果建 `map[int64]Word`，到期词直接从 map 取，不再逐条 `GetByID`。
- store 接口未改动，SQLite / PG 两套实现与 adapter 无需同步修改。

### 效果（估算）

`GET /api/v1/words/queue?level=N4`，limit=20：

| | 修复前 | 修复后 |
|---|---|---|
| `ListByLevel` 例句查询 | 691 | 1 |
| `GetByID`（到期词） | ~24 | 0 |
| `GetRecord`（新词判定） | 691 | ≤10 |
| **合计远程往返** | **≈1409** | **≈14** |

其中「凑够即停」已把 `GetRecord` 从 691 降到 ≤10；本次消掉了例句与 `GetByID` 的 N+1。

---

## API 变更

| 变更 | 说明 |
|---|---|
| 新增 `POST /api/v1/words/{id}/seen` | 幂等；返回 `{"data":{"status":"ok"}}`；需登录 |
| `GET /api/v1/words/queue` | 行为变化：当天已复习的词不再出现，到期词最多占半轮；`limit` 下推到查询层 |
| `GET /api/v1/review/queue` | 词侧最多 50 张 |

`WordService.GetReviewQueue` 签名由 `(userID, level)` 变为 `(userID, level, limit)`（内部方法，非 HTTP 契约）。

---

## 改动文件

| 文件 | 内容 |
|---|---|
| `internal/module/word/service.go` | `MarkSeen`、队列 limit/去重/配比规则、等级词表 map |
| `internal/module/word/handler.go` | `/seen` 路由、limit 下推、去掉二次截断 |
| `internal/module/word/service_test.go` | `MarkSeen` 4 例、队列去重 5 例、无逐词查询 3 例（表格驱动） |
| `internal/module/word/handler_test.go` | `/seen` 创建记录与幂等回归测试 |
| `internal/module/review/handler.go` | 统一队列传 limit |
| `internal/data/postgres/word_store.go` | 批量加载例句 |
| `internal/data/postgres/word_store_test.go` | 批量例句 PG 集成测试 |
| `front/react/src/pages/word/WordReviewPage.tsx` | `/seen` 调用、评分失败不静默前进 |

---

## 验证

- `go test ./... -count=1`、`go vet ./...`、`go build ./...` 全部通过。
- 前端 `tsc --noEmit`、`vitest`（19 文件 / 168 测试）、`vite build` 全部通过。
- `TestWordStoreListByLevelLoadsExamplesInBatch` 依赖真实 PostgreSQL，本地缺少 `DATABASE_URL_TEST` 时为 SKIP，需在配了该变量的 CI 环境执行。

---

## 部署与遗留

- **需重启后端**（新增 `/seen` 路由）；前端由 HMR / 重新构建生效。重启会重跑现有迁移，注意 AGENTS.md 记录的迁移缺陷（002 重复插入种子词、003 重建 `writing_questions`）。
- 历史 91 条逾期 `word_records` 未清洗，用户答过一次即按新规则重新排期。
- 未处理：「今日 0/20」不增长（`/rate` 未写 `study_sessions`）、`GetRecord` 仍逐词（最多 limit 次）、LIMIT 未下推到 SQL（PG 仍传输整等级词表，但仅 2 次查询）、前端 StrictMode 在 dev 下双发队列请求。
