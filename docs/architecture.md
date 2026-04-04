# Japanese Learning App — 架构文档

> Go 1.22 · SQLite · 纯标准库 HTTP · TypeScript 前端

---

## 目录

1. [项目概述](#1-项目概述)
2. [目录结构](#2-目录结构)
3. [启动方式](#3-启动方式)
4. [测试方式](#4-测试方式)
5. [整体架构](#5-整体架构)
6. [数据库设计](#6-数据库设计)
7. [各模块说明](#7-各模块说明)
8. [适配器层](#8-适配器层)
9. [认证机制](#9-认证机制)
10. [AI 集成](#10-ai-集成)
11. [前端结构](#11-前端结构)
12. [一次请求的完整链路](#12-一次请求的完整链路)

---

## 1. 项目概述

本项目是一款面向「边上班边学日语」人群的 Web 应用，目标覆盖 N5～N1 全级别。

| 维度 | 选型 |
|---|---|
| 后端语言 | Go 1.22 |
| Web 框架 | 标准库 `net/http` |
| 数据库 | SQLite（`modernc.org/sqlite`，纯 Go 实现） |
| 前端 | HTML 模板 + TypeScript（esbuild 编译） |
| 认证 | JWT（HMAC-SHA256，自实现） |
| AI | Anthropic Claude API（写作批改，可选） |

---

## 2. 目录结构

```
japanese-learning-app/
├── backend/
│   └── cmd/server/
│       └── main.go                  # 服务器启动入口
├── internal/
│   ├── cli/                         # 命令行工具
│   │   ├── root.go                  # 子命令分发器
│   │   ├── import_words.go          # import-words 子命令
│   │   └── import_words_test.go
│   ├── config/                      # 环境变量配置
│   │   ├── config.go
│   │   └── config_test.go
│   ├── httputil/                    # HTTP 响应工具
│   │   └── response.go              # WriteJSON / WriteError / APIResponse
│   ├── data/                        # 数据访问层
│   │   ├── db.go                    # 打开 DB、运行迁移
│   │   ├── adapters.go              # 接口适配器（见第 8 节）
│   │   ├── timeutil.go              # SQLite 时间解析工具
│   │   ├── word_store.go
│   │   ├── grammar_store.go
│   │   ├── lesson_store.go
│   │   ├── speaking_store.go
│   │   ├── writing_store.go
│   │   ├── user_store.go
│   │   ├── session_store.go
│   │   ├── *_store_test.go          # 集成测试（使用真实 SQLite）
│   │   ├── main_test.go             # 测试 DB 初始化 fixture
│   │   └── migrations/
│   │       ├── 001_init.sql         # 完整 Schema
│   │       ├── 002_seed.sql         # N5/N4 词汇种子数据（各 30 条）
│   │       ├── 003_fix_writing_questions.sql
│   │       └── 004_words_unique_kanji_reading.sql
│   └── module/                      # 业务模块（每个模块自包含）
│       ├── word/                    # 单词记忆
│       ├── grammar/                 # 语法学习
│       ├── lesson/                  # 课文学习
│       ├── speaking/                # 口语练习
│       ├── writing/                 # 写作练习
│       ├── user/                    # 用户账户 & 认证
│       └── summary/                 # 学习会话总结
├── front/
│   └── web/
│       ├── templates/               # HTML 模板（服务端静态托管）
│       │   ├── base.html
│       │   ├── word/
│       │   ├── grammar/
│       │   ├── lesson/
│       │   ├── speaking/
│       │   ├── writing/
│       │   └── summary/
│       └── static/
│           ├── css/main.css         # 全局样式
│           └── js/                  # TypeScript 源文件
│               ├── api.ts           # 公共 HTTP 客户端
│               ├── word.ts
│               ├── grammar.ts
│               ├── lesson.ts
│               ├── speaking.ts      # MediaRecorder + 上传
│               ├── writing.ts
│               └── summary.ts
├── specs/                           # 需求文档
├── Makefile
├── go.mod
├── CLAUDE.md
└── constitution.md                  # 开发原则宪法
```

---

## 3. 启动方式

### 3.1 环境要求

| 工具 | 版本 |
|---|---|
| Go | ≥ 1.22 |
| Node.js（可选，编译 TS） | ≥ 18 |

> 本机安装了多版本 Go 时，需通过 gvm 激活：
> ```bash
> source ~/.gvm/scripts/gvm && gvm use go1.22.1
> ```

### 3.2 Makefile 速查

```bash
make run          # 启动开发服务器（默认监听 :8080）
make build        # 编译为 bin/server 可执行文件
make test         # 运行所有测试（含集成测试）
make lint         # 静态分析 go vet ./...
make seed         # 导入初始词库（N5/N4 JSON → SQLite）
make front-build  # 编译 TypeScript → front/web/static/js/dist/
make clean        # 清理 bin/ 和 dist/
```

### 3.3 手动启动

```bash
# 最简启动（使用默认配置）
go run ./backend/cmd/server/

# 指定配置
DB_PATH=./data/app.db \
LISTEN_ADDR=:8080 \
JWT_SECRET=your-secret-here \
LOG_LEVEL=DEBUG \
AI_API_KEY=sk-ant-... \
go run ./backend/cmd/server/
```

### 3.4 环境变量参考

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP 监听地址 |
| `DB_PATH` | `./data/app.db` | SQLite 数据库路径 |
| `JWT_SECRET` | `change-me-in-production` | **生产环境必须修改** |
| `LOG_LEVEL` | `INFO` | `DEBUG / INFO / WARN / ERROR` |
| `AI_API_KEY` | `""` | Claude API 密钥（空则使用 StubReviewer） |
| `AI_API_ENDPOINT` | `https://api.anthropic.com/v1/messages` | AI 接口地址 |
| `STATIC_DIR` | `./front/web/static` | 静态文件目录 |
| `TEMPLATE_DIR` | `./front/web/templates` | HTML 模板目录 |

### 3.5 启动流程（main.go）

```
os.Args[1] != "serve"?
    └─ YES → cli.Run(args) 然后退出
    └─ NO  → 继续启动 HTTP 服务器

setupLogger(logLevel)
    ↓
data.OpenDB(dbPath)          // 打开 SQLite，开启 WAL + 外键
    ↓
data.RunMigrations(db)       // 按序执行 migrations/*.sql
    ↓
NewXxxStore(db) × 7          // 创建所有 Store 实例
    ↓
NewXxxAdapter(store) × 4     // 接口适配（见第 8 节）
    ↓
NewXxxService(adapter) × 7   // 创建所有 Service
    ↓
NewXxxHandler(svc) × 7       // 创建所有 Handler
    ↓
注册路由（公开 + 受保护 + 静态）
    ↓
http.ListenAndServe(addr, mux)
```

### 3.6 CLI 子命令

程序支持以 CLI 模式运行，不启动 HTTP 服务器：

```bash
# 导入单词 JSON 文件（幂等，重复运行安全）
./bin/server import-words --file ./data/seed/words_n5.json
./bin/server import-words --file ./data/seed/words_n4.json --db ./data/app.db
```

---

## 4. 测试方式

### 4.1 运行测试

```bash
# 运行所有测试
make test
# 等价于
go test ./... -v -count=1

# 只跑某个包
go test ./internal/module/word/... -v

# 只跑特定测试函数
go test ./internal/data/... -run TestWordStore_GetByID -v
```

### 4.2 测试策略

| 原则 | 说明 |
|---|---|
| **表格驱动测试** | 所有单元测试使用 `[]struct{ name, input, want }` 格式 |
| **集成优先** | 数据层测试使用真实 SQLite（`data/main_test.go` 初始化内存 DB） |
| **不使用 Mock** | 避免 Mock 库，接口通过真实依赖验证 |
| **`-count=1`** | 禁用测试缓存，每次都重新运行 |

### 4.3 测试文件分布

| 包 | 测试文件 | 说明 |
|---|---|---|
| `internal/data` | `*_store_test.go` + `main_test.go` | Store 集成测试，使用真实 DB |
| `internal/module/word` | `service_test.go`, `sm2_test.go` | SM-2 算法 + 服务逻辑 |
| `internal/module/grammar` | `service_test.go` | 语法服务逻辑 |
| `internal/module/lesson` | `service_test.go` | 课文服务逻辑 |
| `internal/module/speaking` | `service_test.go`, `scorer_test.go` | 评分算法 |
| `internal/module/writing` | `service_test.go` | AI 批改（使用 StubReviewer） |
| `internal/module/user` | `service_test.go`, `jwt_test.go` | 注册/登录/JWT |
| `internal/module/summary` | `service_test.go` | 会话总结逻辑 |
| `internal/cli` | `import_words_test.go` | CLI 幂等导入测试 |
| `internal/config` | `config_test.go` | 配置加载 |

### 4.4 数据层测试 fixture

`internal/data/main_test.go` 提供共享帮助函数，每个 Store 测试用例：

1. 创建内存 SQLite（`file::memory:?mode=memory&cache=shared`）
2. 执行所有迁移（`RunMigrations`）
3. 插入最小必要数据
4. 运行断言
5. 测试结束自动关闭连接

---

## 5. 整体架构

### 5.1 分层示意

```
┌────────────────────────────────────────┐
│              HTTP Client               │
│         (浏览器 / iOS App)             │
└───────────────────┬────────────────────┘
                    │ HTTP/JSON
┌───────────────────▼────────────────────┐
│           AuthMiddleware               │  ← JWT 验证，注入 userID
├────────────────────────────────────────┤
│  Handler Layer  (module/*/handler.go)  │  ← 解析请求、写响应
├────────────────────────────────────────┤
│  Service Layer  (module/*/service.go)  │  ← 业务逻辑、SM-2、评分
├────────────────────────────────────────┤
│  Adapter Layer  (data/adapters.go)     │  ← 接口适配（见第 8 节）
├────────────────────────────────────────┤
│  Store Layer    (data/*_store.go)      │  ← SQL 查询
├────────────────────────────────────────┤
│           SQLite (WAL mode)            │
└────────────────────────────────────────┘
```

### 5.2 HTTP 路由表

#### 公开路由（无需认证）
```
POST /api/v1/auth/register        注册
POST /api/v1/auth/login           登录，返回 JWT
```

#### 受保护路由（需 Bearer Token）
```
# 单词
GET  /api/v1/words/queue          获取复习队列
POST /api/v1/words/review         提交评分（SM-2）
POST /api/v1/words/{id}/bookmark  收藏单词
GET  /api/v1/words/{id}           获取单词详情

# 语法
GET  /api/v1/grammar              语法列表
GET  /api/v1/grammar/{id}         语法详情 + 例题
POST /api/v1/grammar/{id}/quiz    提交测验答案

# 课文
GET  /api/v1/lessons              课文列表
GET  /api/v1/lessons/{id}         课文详情（含句子）

# 口语
GET  /api/v1/speaking/materials   练习素材列表
POST /api/v1/speaking/score       上传录音，返回评分

# 写作
GET  /api/v1/writing/queue        获取题目队列
POST /api/v1/writing/input        提交输入练习答案
POST /api/v1/writing/sentence     提交造句（AI 批改）

# 总结
POST /api/v1/summary/sessions     记录学习会话
POST /api/v1/summary/generate     生成会话总结
GET  /api/v1/summary              获取历史总结列表

# 用户
GET  /api/v1/users/me             获取个人信息
```

#### 静态资源
```
/static/*    → front/web/static/
/*           → front/web/templates/   （HTML 页面）
```

---

## 6. 数据库设计

### 6.1 内容库（只读）

```sql
words               -- 单词（kanji_form, reading, meaning, examples_json, jlpt_level）
grammar_points      -- 语法点（name, meaning, conjunction_rule, examples_json, quiz_questions_json）
lessons             -- 课文（content_furigana_json, sentence_timestamps_json, audio_url）
speaking_materials  -- 口语素材（type: shadow|free, text, audio_url）
writing_questions   -- 写作题目（type: input|sentence, prompt, expected_answer）
```

### 6.2 用户数据

```sql
users               -- 账号（email UNIQUE, password_hash, goal_level, streak_days）
```

### 6.3 学习记录

```sql
word_records        -- SM-2 复习状态（mastery_level, next_review_at, ease_factor, interval）
word_bookmarks      -- 单词收藏（user_id, word_id）
grammar_records     -- 语法学习状态（status: unlearned|learning|mastered, quiz_history_json）
speaking_records    -- 口语练习记录（score, audio_ref）
writing_records     -- 写作记录（user_answer, ai_feedback_json, score）
study_sessions      -- 学习会话（session_id UUID, module, duration_seconds, completed_count）
session_summaries   -- 会话总结（strengths_json, weaknesses_json, suggestions_json）
```

### 6.4 关键迁移

| 文件 | 内容 |
|---|---|
| `001_init.sql` | 完整 Schema，WAL 模式，外键约束 |
| `002_seed.sql` | N5×30 + N4×30 词汇种子（`INSERT OR IGNORE`） |
| `003_fix_writing_questions.sql` | `grammar_point_id` 改为软引用（0=无关联） |
| `004_words_unique_kanji_reading.sql` | `UNIQUE(kanji_form, reading)`，支持幂等导入 |

---

## 7. 各模块说明

每个模块位于 `internal/module/<name>/`，结构统一：

```
model.go     数据结构（Entity + DTO + 枚举）
service.go   业务逻辑 + StoreInterface 定义
handler.go   HTTP Handler + RegisterRoutes()
*_test.go    服务层测试
```

### word — 单词记忆

- 核心算法：SM-2 间隔重复（`sm2.go`）
  - Rating `easy` → ease_factor +0.3，interval ×2.5
  - Rating `normal` → ease_factor 不变，interval ×ease_factor
  - Rating `hard` → ease_factor -0.2，interval 重置为 1 天
- 复习队列：先返回到期卡片，再补充该等级新单词

### grammar — 语法学习

- 每个语法点内嵌 2~3 道即时测验（`quiz_questions_json`）
- 题型：`fill_blank`（填空）/ `multi_choice`（选择）
- 学习状态：`unlearned → learning → mastered`（答题分数驱动）

### lesson — 课文学习

- Furigana Token：每个汉字附带假名注音（`content_furigana_json`）
- 音频同步：`sentence_timestamps_json` 记录每句的起止毫秒数
- 单词联动：课文中出现的单词 ID 存入 `word_ids_json`，可一键加入词库

### speaking — 口语练习

- 两种模式：`shadow`（跟读，有参考音频）/ `free`（自由朗读）
- 评分：`WaveformScorer` 对比音频波形，返回 0~100 分 + 逐句标注
- 录音上传：`multipart/form-data`，字段 `audio`（webm）+ `material_id`

### writing — 写作练习

- 输入模式：提交答案后与 `expected_answer` 精确比对
- 造句模式：答案发送给 Claude API，返回 `AIFeedback{corrected, explanation, score}`
- 若未配置 `AI_API_KEY`，自动降级为 `StubReviewer`（返回固定分数）

### user — 用户认证

- 密码：SHA-256 哈希（无 salt，简化实现；生产建议改 bcrypt）
- JWT：HMAC-SHA256，Payload 包含 `user_id` + `exp`，有效期 24 小时
- Middleware：从 `Authorization: Bearer <token>` 提取 token，将 `userID` 注入 `context.Context`

### summary — 学习总结

- 记录每次学习会话（模块、时长、完成数）
- 聚合生成总结（强项/弱项/建议），以 JSON 字段灵活存储各模块的得分格式
- 前端展示：列表按时间倒序，点击展开详情

---

## 8. 适配器层

**文件：** `internal/data/adapters.go`

### 为什么需要适配器？

数据层（`data.*Store`）的方法签名比服务层接口更丰富（带分页、带过滤参数），
为了让数据层可独立演化，中间插入一层薄适配器：

```
module.XxxStoreInterface  ←  XxxStoreAdapter  →  data.XxxStore
（服务层依赖的接口）              （适配器）            （具体实现）
```

### 适配器清单

| 适配器 | 解决的不匹配 |
|---|---|
| `WordStoreAdapter` | `ListByLevel(level, page, size)` → 服务层无分页版本；`GetRecord` 把 `sql.ErrNoRows` 转为 `(nil, nil)` |
| `LessonStoreAdapter` | `ListSummaries(level, tag)` → 服务层无 tag 参数（透传空字符串） |
| `UserStoreAdapter` | `GetByEmail` + `GetPasswordHash` 合并为接口要求的 `GetUserByEmail(...) (*User, string, error)` |
| `SessionStoreAdapter` | `CreateSession`（返回 sessionID）→ `SaveSession`（忽略 ID）；补充 `ListSummaries` |

---

## 9. 认证机制

```
客户端                        服务端
  │                              │
  │  POST /api/v1/auth/login     │
  │  {email, password}  ─────►  │  SHA-256(password) == stored_hash?
  │                              │  YES → SignToken(userID, secret, 24h)
  │  ◄─────────────────────────  │  {token: "eyJ..."}
  │                              │
  │  GET /api/v1/words/queue     │
  │  Authorization: Bearer eyJ..►│  AuthMiddleware:
  │                              │    VerifyToken(token, secret)
  │                              │    → userID 注入 context
  │                              │    → 传给 handler
  │  ◄─────────────────────────  │  JSON 响应
```

- Token 存储在前端 `localStorage`（key: `jla_token`）
- `api.ts` 统一在每个请求的 Header 中附加 Token
- Token 过期或无效时返回 `401 Unauthorized`

---

## 10. AI 集成

**接口：**
```go
type AIReviewer interface {
    Review(prompt, userAnswer string) (AIFeedback, error)
}
```

**两种实现：**

| 实现 | 条件 | 行为 |
|---|---|---|
| `ClaudeClient` | `AI_API_KEY` 非空 | 调用 `https://api.anthropic.com/v1/messages`，返回真实批改 |
| `StubReviewer` | `AI_API_KEY` 为空 | 返回固定分数（score=80），用于开发/测试 |

**批改流程（写作·造句模式）：**

```
用户提交 answer
  → writing.Service.SubmitSentence()
  → AIReviewer.Review(question.Prompt, answer)
  → Claude API（或 Stub）
  → AIFeedback{corrected, explanation, score}
  → 保存 writing_records
  → 返回给前端展示
```

---

## 11. 前端结构

### 模板（服务端静态托管）

HTML 模板由 Go 的 `http.FileServer` 直接托管，不做服务端渲染，
所有数据由页面加载后的 TypeScript 通过 API 获取（SPA 交互模式）。

| 模板 | 对应功能 |
|---|---|
| `base.html` | 全局导航（単語 / 文法 / 課文 / 口語 / ライティング）|
| `word/index.html` | 闪卡翻转复习 |
| `word/stats.html` | 学习统计（总数/到期/掌握/连续天数）|
| `grammar/index.html` | 语法列表 + JLPT 筛选 |
| `grammar/detail.html` | 语法详情 + 内嵌测验 |
| `lesson/index.html` | 课文列表 |
| `lesson/detail.html` | 课文阅读器（音频同步高亮）|
| `speaking/index.html` | 素材选择 + 录音控件 |
| `writing/index.html` | 输入/造句切换 + AI 反馈面板 |
| `summary/index.html` | 历史总结列表 + 详情 |

### TypeScript 模块

| 文件 | 关键逻辑 |
|---|---|
| `api.ts` | `apiFetch<T>()` 封装 fetch，自动注入 Bearer Token；`getToken/setToken/clearToken` |
| `word.ts` | 闪卡翻转（CSS 3D）、评分按钮、空队列状态 |
| `grammar.ts` | 列表/详情页判断（`document.getElementById`）、测验选项评分 |
| `lesson.ts` | `audio.timeupdate` 事件驱动句子高亮；单词弹窗 |
| `speaking.ts` | `navigator.mediaDevices.getUserMedia`、`MediaRecorder`、`FormData` 上传 |
| `writing.ts` | 输入模式精确比对（1.2s 延迟翻页）；造句模式 AI 反馈面板 |
| `summary.ts` | 列表渲染；`showDetail()` 展示强项/弱项/建议 |

### 编译

```bash
make front-build
# 等价于：
npx esbuild front/web/static/js/*.ts --bundle --outdir=front/web/static/js/dist
```

编译产物输出到 `front/web/static/js/dist/`，HTML 模板中以 `<script src="/static/js/dist/xxx.js">` 引用。

---

## 12. 一次请求的完整链路

以「单词复习：提交评分」为例：

```
前端 word.ts
  → POST /api/v1/words/review
    { word_id: 42, rating: "easy" }
    Authorization: Bearer <jwt>

  ↓ AuthMiddleware
    VerifyToken(token, secret)
    → 提取 userID=7，注入 context

  ↓ WordHandler.handleSubmitReview
    解析 JSON body → wordID=42, rating="easy"

  ↓ WordService.SubmitRating(userID=7, wordID=42, rating="easy")
    1. store.GetByID(42)          验证单词存在
    2. store.GetRecord(7, 42)     获取当前学习记录（无则初始化）
    3. CalcNextReview(record, "easy")
       → ease_factor: 2.5 → 2.8
       → interval:    1   → 3 天
       → next_review_at: now + 3 days
    4. store.UpsertRecord(updated)
       INSERT INTO word_records ... ON CONFLICT DO UPDATE

  ↓ httputil.WriteJSON(w, 200, APIResponse{Data: updated})
    {"data": {"word_id":42, "next_review_at":"2026-04-07T..."}}

前端收到 200 → 显示下一张卡片
```

---

## 附录：关键设计决策

| 决策 | 原因 |
|---|---|
| 纯标准库 `net/http` | 遵守宪法「简单性原则」，避免引入 Gin/Echo 等框架依赖 |
| SQLite 而非 PostgreSQL | 单文件部署，无需独立数据库服务，适合 MVP 阶段 |
| `modernc.org/sqlite`（纯 Go） | 无需 CGO，交叉编译友好 |
| 适配器层（adapters.go） | 数据层可独立扩展（如加分页），服务层接口保持稳定 |
| SHA-256 密码哈希 | 项目宪法要求「避免第三方依赖」，生产升级建议改 bcrypt |
| SM-2 自实现 | 算法简单，避免引入外部依赖 |
| `StubReviewer` | 开发/测试无需真实 API Key，与 `ClaudeClient` 同接口可无缝切换 |
| WAL 模式 | SQLite 并发读不阻塞写，适合多标签页用户 |
