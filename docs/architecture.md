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
| 前端 | HTML 模板 + TypeScript（esbuild 编译）；React 18 SPA（Vite + TypeScript）|
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
│   ├── web/
│   │   ├── templates/               # HTML 模板（旧版，已弃用）
│   │   └── static/
│   │       ├── css/main.css         # 全局样式
│   │       └── js/                  # TypeScript 源文件（旧版）
│   │           ├── api.ts
│   │           ├── word.ts
│   │           ├── grammar.ts
│   │           ├── lesson.ts
│   │           ├── speaking.ts
│   │           ├── writing.ts
│   │           └── summary.ts
│   └── react/                       # React SPA（当前主前端）
│       ├── src/
│       │   ├── api/client.ts        # apiFetch<T>() 封装
│       │   ├── types/api.ts         # 全部 API 实体 TypeScript 接口
│       │   ├── i18n/                # 国际化配置 + zh/ja/en 翻译
│       │   ├── components/          # 公共组件（layout + ui）
│       │   └── pages/               # 页面组件（home/word/grammar/speaking/writing/lesson/auth）
│       ├── vite.config.ts           # Vite 配置（/api → localhost:8080 代理）
│       └── package.json
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

- Token 存储在前端 `localStorage`（key: `token`，user 信息存于 `user`）
- `api/client.ts` 的 `apiFetch<T>()` 统一在每个请求的 Header 中附加 `Authorization: Bearer <token>`
- Token 过期或无效时后端返回 `401 Unauthorized`；`apiFetch` 捕获到 401 后自动清除 `localStorage`（移除 `token` 和 `user`）并重定向至 `/login`，避免用户看到原始错误信息

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

前端存在两套实现，当前以 React SPA 为主要用户界面。

### 11.1 旧版：HTML 模板（已弃用）

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

---

### 11.2 新版：React SPA（`front/react/`）

基于 **React 18 + TypeScript + Vite** 构建的单页应用，通过 Vite 代理将 `/api` 请求转发至 Go 后端（`:8080`）。

#### 目录结构

```
front/react/src/
  api/
    client.ts          ← apiFetch<T>()：封装 fetch、注入 JWT Bearer Token、解包 {data:T} 响应体
  types/
    api.ts             ← 全部 API 实体接口（与 Go 模型 JSON 字段一一对应）
  i18n/
    config.ts          ← i18next 初始化（语言检测 + localStorage 持久化）
    locales/
      zh.ts / ja.ts / en.ts  ← 全量翻译键（zh 为默认语言）
  components/
    layout/
      TopNavBar.tsx      ← 顶部导航（Logo + LanguageSwitcher + 登出按钮）
      BottomTabBar.tsx   ← 底部标签栏（Home / 单词 / 语法 / 口语 / 写作 / 课文）
    ui/
      Badge.tsx          ← JLPT 等级标签（N5~N1 不同色）
      Button.tsx         ← 通用按钮（primary / outline / ghost）
      Card.tsx           ← 内容卡片容器
      Spinner.tsx        ← 加载转圈
      EmptyState.tsx     ← 空状态占位（icon + title + description）
      ProgressBar.tsx    ← 进度条
      LanguageSwitcher.tsx ← 🌐 语言切换按钮组
  pages/
    home/
      HomePage.tsx       ← 首页（打招呼、连续天数、今日任务、学习模块入口）
    word/
      WordReviewPage.tsx       ← 单词闪卡复习
      WordReviewPage.module.css
    grammar/
      GrammarListPage.tsx      ← 语法列表（JLPT 筛选）
      GrammarListPage.module.css
      GrammarDetailPage.tsx    ← 语法详情 + 测验（新建）
      GrammarDetailPage.module.css
    speaking/
      SpeakingPage.tsx         ← 口语练习（功能介绍 + 历史记录）
      SpeakingPage.module.css
    writing/
      WritingQueuePage.tsx     ← 写作练习（逐题提交 + AI 反馈）
      WritingQueuePage.module.css
    lesson/
      LessonPage.tsx           ← 课文阅读（列表 + 内嵌详情，含振り仮名）
      LessonPage.module.css
    auth/
      LoginPage.tsx / RegisterPage.tsx / ForgotPasswordPage.tsx / ResetPasswordPage.tsx
  App.tsx              ← 路由配置（react-router-dom v6）
```

#### 路由表

| 路径 | 组件 | 说明 |
|---|---|---|
| `/login` | `LoginPage` | 无需认证 |
| `/register` | `RegisterPage` | 无需认证 |
| `/forgot-password` | `ForgotPasswordPage` | 无需认证 |
| `/reset-password` | `ResetPasswordPage` | 无需认证 |
| `/` | `HomePage` | 需认证 |
| `/words/review` | `WordReviewPage` | 需认证 |
| `/grammar` | `GrammarListPage` | 需认证 |
| `/grammar/:id` | `GrammarDetailPage` | 需认证 |
| `/speaking` | `SpeakingPage` | 需认证 |
| `/writing` | `WritingQueuePage` | 需认证 |
| `/lesson` | `LessonPage` | 需认证 |

#### 各页面功能详述

**WordReviewPage（单词闪卡复习）**
1. JLPT 等级 tabs（N5/N4/N3/N2/N1）切换
2. `GET /api/v1/words/queue?level=N5` 拉取今日队列
3. 闪卡正面：汉字（大号）+ 词性 + "新词" badge
4. 点击卡片：CSS 3D 翻转（`perspective: 1000px` + `rotateY(180deg)`）
5. 背面：假名读音 + 中文释义 + 例句
6. 翻转后显示 3 档评分按钮（😓困难 / 🙂普通 / 😄简单）
7. `POST /api/v1/words/{id}/rate` 提交评分，自动切换下一张
8. 队列耗尽 → `EmptyState` 展示"今日复习完成🎉"

**GrammarListPage（语法列表）**
1. JLPT 等级 tabs 筛选
2. `GET /api/v1/grammar?level=N5` 拉取列表
3. 每条展示：语法名 + 含义 + JLPT Badge + 箭头
4. 点击 → `navigate('/grammar/:id')`

**GrammarDetailPage（语法详情，新建页面）**
1. `GET /api/v1/grammar/{id}` 拉取详情
2. 展示：名称、JLPT Badge、含义、接续规则（代码块）、用法说明、例句列表
3. 测验区域（可折叠）：
   - `fill_blank` 题型 → `<input type="text">`
   - `multi_choice` 题型 → 单选 `<label>` 按钮组
4. "提交全部" → `POST /api/v1/grammar/{id}/quiz` 发送 `QuizSubmission[]`
5. 结果：总分卡片 + 逐题正误标识 + 解释说明

**SpeakingPage（口语练习）**
- 展示功能介绍卡片（影子跟读 + 自由朗读说明 + 麦克风权限提示）
- `GET /api/v1/speaking/records` 拉取历史练习记录
- 每条记录显示：日期 + `overall_score` badge
- 无记录时：`EmptyState`

**WritingQueuePage（写作练习）**
1. `GET /api/v1/writing/queue` 拉取题目列表（`sentence` / `input` 两种类型）
2. 逐题展示：提示（prompt）+ `<textarea>` 输入框
3. 提交逻辑（按类型分流）：
   - `sentence` → `POST /api/v1/writing/sentence` `{ question, user_answer }`
   - `input` → `POST /api/v1/writing/input` `{ question, user_answer, expected: '' }`
4. AI 反馈卡：score 圆圈 + grammar/vocab 标签 + 修正句 + 问题描述 + 参考表达 + 参考答案
5. "下一题" 继续；全部完成 → `EmptyState`

**LessonPage（课文阅读）**
1. `GET /api/v1/lessons` 拉取课文列表
2. 列表显示：标题 + JLPT Badge + 句数 + tags
3. 点击课文 → 同页面切换为详情视图（不新增路由）
4. 详情：逐句渲染
   - 有 `reading` 的 token → `<ruby>surface<rt>reading</rt></ruby>`（振り仮名）
   - 无 `reading` → 直接显示 `surface`
5. 翻译切换按钮：一键显示/隐藏全部中文翻译
6. 返回按钮回到列表

#### 通用约定

| 约定 | 说明 |
|---|---|
| 样式隔离 | 每个页面配套 `*.module.css`，使用 CSS 变量（`--color-*`、`--space-*`、`--radius-*`）|
| 国际化 | 所有 UI 字符串通过 `useTranslation()` + `t()` 输出，支持 zh / ja / en 三语言 |
| 加载状态 | API 请求期间显示 `<Spinner size="lg" />`，详情加载使用局部 spinner |
| 错误处理 | `try/catch` 包裹所有 API 调用，错误文本以 `--color-error` 颜色内联显示 |
| 组件复用 | `Badge`、`Button`、`Card`、`Spinner`、`EmptyState`、`ProgressBar` |

#### TypeScript 类型（`types/api.ts`）

所有接口字段与 Go 模型 JSON tag 严格对应：

```ts
// 单词
interface WordCard { word: Word; record: WordRecord; is_new: boolean }
interface Word { id: number; kanji_form: string; reading: string; meaning: string; part_of_speech: string; jlpt_level: string; examples: WordExample[] }
interface WordExample { japanese: string; chinese: string }

// 语法
interface GrammarPoint { id: number; name: string; meaning: string; jlpt_level: string; conjunction_rule: string; usage_note: string; examples: GrammarExample[]; quiz_questions: QuizQuestion[] }
interface QuizQuestion { id: number; type: 'fill_blank' | 'multi_choice'; prompt: string; options?: string[]; explanation: string }
interface QuizResult { score: number; results: QuizItemResult[] }

// 写作 AI 反馈
interface AIFeedback { score: number; grammar_correct: boolean; vocab_correct: boolean; issue_description: string; corrected_sentence: string; alternative_phrases: string[]; reference_answer: string }
interface WritingRecord { id: number; score: number; ai_feedback?: AIFeedback; practiced_at: string }

// 课文
interface LessonSummary { id: number; title: string; jlpt_level: string; sentence_count: number; tags: string[] }
interface Lesson { id: number; title: string; jlpt_level: string; sentence_count: number; tags: string[]; sentences: Sentence[] }
interface Sentence { index: number; translation: string; tokens: Token[] }
interface Token { surface: string; reading: string; pos: string }
```

#### 开发启动

```bash
cd front/react
npm install
npm run dev      # Vite dev server，http://localhost:5173
                 # /api/* → 代理至 http://localhost:8080
```

---

## 11.x 多语言支持（i18n）

### 技术选型

| 库 | 用途 |
|---|---|
| `i18next` | 核心 i18n 引擎，管理翻译资源与语言切换 |
| `react-i18next` | React 绑定，提供 `useTranslation()` hook |

### 目录结构

```
front/react/src/
  i18n/
    config.ts            ← i18next 初始化（语言检测 + localStorage 持久化）
    locales/
      zh.ts              ← 中文翻译（默认 fallback 语言）
      ja.ts              ← 日语翻译
      en.ts              ← 英文翻译
  components/ui/
    LanguageSwitcher.tsx         ← 语言切换按钮组件
    LanguageSwitcher.module.css  ← 组件样式
```

### 语言检测逻辑

```typescript
// 优先级：localStorage > 浏览器语言 > 默认 'zh'
function detectLanguage(): string {
  const stored = localStorage.getItem('preferred_language')
  if (stored && SUPPORTED.includes(stored)) return stored
  const browser = navigator.language.split('-')[0]
  return SUPPORTED.includes(browser) ? browser : 'zh'
}
```

切换语言时自动写入 `localStorage['preferred_language']`，刷新后保持用户选择。

### 翻译键结构

```
common.*          通用（loading、appName）
nav.*             顶部导航 + 底部标签（home、words、grammar、speaking、writing、lesson、logout）
auth.login.*      登录页
auth.register.*   注册页
auth.forgot.*     忘记密码页
auth.reset.*      重置密码页
password.*        密码显示/隐藏
home.*            首页（greeting、streakDays、todaysTasks、mastered、tips、modules.*）
word.queue.*      单词复习（title、done、doneDesc、progress、isNew、flip、masteryLevel、rating.*）
grammar.list.*    语法列表（title、empty）
grammar.detail.*  语法详情（conjunction、usage、examples、quiz.*）
speaking.*        口语练习（title、desc、howto、shadow、free、micNote、records.*、score）
writing.*         写作练习（title、placeholder、submit、next、done、doneDesc、feedback.*）
lesson.*          课文阅读（title、chars、back、translation.*）
```

插值示例：
- `t('home.streakDays', { count: 7 })` → `"7 天连续"` / `"7 日連続"` / `"7 day streak"`
- `t('lesson.chars', { count: 42 })` → `"42 字"` / `"42 文字"` / `"42 chars"`
- `t('word.queue.progress', { current: 3, total: 10 })` → `"第 3 / 共 10 张"`

### UI 入口

`<LanguageSwitcher />` 放置在 `TopNavBar` 右侧用户区域（`styles.user`），
显示为 `🌐 中 | 日 | EN` 按钮组，当前激活语言高亮（`--color-primary`）。

### 扩展新语言

1. 在 `front/react/src/i18n/locales/` 新建 `<code>.ts`，照 `en.ts` 结构填写翻译
2. 在 `config.ts` 的 `SUPPORTED` 数组和 `resources` 对象中添加该语言代码
3. 在 `LanguageSwitcher.tsx` 的 `LANGS` 数组中添加 `{ code, label }`
4. 无需修改任何业务组件

---

## 12. 一次请求的完整链路

以「单词复习：提交评分」为例：

```
前端 WordReviewPage.tsx（React）
  → POST /api/v1/words/{id}/rate
    { rating: "easy" }
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

## 13. 学习进度与完成状态标记设计

> 本节描述各模块子项「已完成/已掌握」的视觉呈现规范与数据来源。

---

### 13.1 现状分析

| 模块 | 列表项完成标记 | 可用数据字段 | 实现状态 |
|---|---|---|---|
| 单词复习 | ✅ 已实现 | `word_records.mastery_level`（0–5） | ✅ 卡片背面掌握度进度条 |
| 语法列表 | ✅ 已实现 | `grammar_records.status`（`unlearned/learning/mastered`） | ✅ StatusBadge + 后端 LEFT JOIN |
| 语法详情 | ✅ 已实现 | `QuizResult.score` + `user_status` | ✅ StatusBadge 显示于 nameRow |
| 口语练习 | ✅ 已实现 | `SpeakingRecord.score.overall_score` | ✅ Pass/需改善 StatusBadge |
| 写作练习 | ✅ 已实现 | `WritingRecord.score` | ✅ 反馈卡 score 颜色区分 |
| 课文阅读 | ✅ 已实现 | `localStorage`（前端本地） | ✅ 已读 ✓ 图标 |

---

### 13.2 设计方案

各模块列表项在右侧统一增加状态徽章，遵循三档状态语义：

| 状态值 | 视觉表现 | 语义 |
|---|---|---|
| `unlearned` / 未学 | 灰色边框徽章 + "未学习" | 从未接触 |
| `learning` / 学习中 | 🔵 黄色徽章 + "学习中" | 已开始但未达标 |
| `mastered` / 已掌握 | ✅ 绿色徽章 + "已掌握" | 达到掌握标准 |
| `pass` / 通过 | ✅ 绿色徽章 + "通过" | 口语/写作单次合格 |
| `needs_work` / 需改善 | 🔶 黄色徽章 + "需改善" | 口语/写作单次不合格 |

---

### 13.3 各模块具体规则

#### 单词复习（WordReviewPage）

**数据来源**：`WordCard.record.mastery_level`（整数 0–5）

- 卡片翻转后背面底部显示 `<ProgressBar value={mastery_level * 20} label={t('word.queue.masteryLevel', { level })} />`
- 进度条颜色：≥80% → success（绿），≥40% → mid（蓝），其余 → low（红）

#### 语法列表（GrammarListPage）

**数据来源**：`GET /api/v1/grammar` 响应中的 `user_status` 字段（后端 LEFT JOIN `grammar_records`）

- 列表项右侧：`<StatusBadge status={p.user_status} />`（在 JLPT Badge 左侧）
- 后端实现：`GrammarStore.ListByLevelWithStatus(userID, level)` → SQL LEFT JOIN + COALESCE('unlearned')

**前端类型**：
```ts
interface GrammarPointWithStatus extends GrammarPoint {
  user_status: 'unlearned' | 'learning' | 'mastered'
}
```

#### 语法详情（GrammarDetailPage）

- 顶部 nameRow 区域：`{point.user_status && <StatusBadge status={point.user_status} />}`
- 详情 API `GET /api/v1/grammar/{id}` 暂不含 `user_status`（单点查询无 JOIN），前端使用 optional 渲染

#### 课文阅读（LessonPage）

**数据来源**：前端 `localStorage` 本地记录（后端无已读记录表，采用轻量本地方案）

- key：`lesson_read_<userId>_<lessonId>`，value：`"1"`
- 用户点击课文详情后自动写入 localStorage 并更新 `readSet` state
- 列表项标题旁显示：已读 → 绿色 ✓ 图标（`.readMark` CSS class）；未读 → 无标记

**方案选择依据**：课文「已读」不影响算法调度，无需持久化到后端；localStorage 方案零 API 改动，符合简单性原则。

#### 口语练习（SpeakingPage）

**数据来源**：`SpeakingRecord.score.overall_score`

- 历史记录每条末尾：score ≥ 80 → `<StatusBadge status="pass" />`；< 80 → `<StatusBadge status="needs_work" />`

#### 写作练习（WritingQueuePage）

**数据来源**：`WritingRecord.score`

- 反馈卡 scoreCircle 边框 + 数字颜色：
  - ≥ 80 → `var(--color-success)`
  - 60–79 → `var(--color-warning)`
  - < 60 → `var(--color-error)`

---

### 13.4 实现优先级（已完成）

| 优先级 | 模块 | 改动范围 | 状态 |
|---|---|---|---|
| P1 | 单词复习 – 掌握进度条 | 前端纯UI，数据已有 | ✅ 完成 |
| P1 | 课文阅读 – 已读标记 | 前端 localStorage，零后端改动 | ✅ 完成 |
| P2 | 语法列表 – 学习状态 | 前端UI + 后端API扩展（ListByLevelWithStatus） | ✅ 完成 |
| P2 | 口语练习 – Pass/需改善 | 前端纯UI，数据已有 | ✅ 完成 |
| P3 | 写作练习 – score 颜色 | 前端纯UI，数据已有 | ✅ 完成 |

---

### 13.5 通用 StatusBadge 组件规范

`src/components/ui/StatusBadge.tsx` — 统一各模块的状态徽章

```tsx
export type StatusType = 'unlearned' | 'learning' | 'mastered' | 'pass' | 'needs_work'

interface StatusBadgeProps {
  status: StatusType
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const { t } = useTranslation()
  return (
    <span className={`${styles.badge} ${styles[status]}`} data-status={status}>
      {t(STATUS_I18N_KEY[status])}
    </span>
  )
}
```

| status | 背景色 | 文字色 | i18n key |
|---|---|---|---|
| `unlearned` | `--color-text-secondary` 10% | `--color-text-secondary` | `status.unlearned` |
| `learning` | `--color-warning` 15% | `--color-warning` 80% | `status.learning` |
| `mastered` | `--color-success` 15% | `--color-success` | `status.mastered` |
| `pass` | `--color-success` 15% | `--color-success` | `status.pass` |
| `needs_work` | `--color-warning` 15% | `--color-warning` 80% | `status.needsWork` |

**配套 CSS**：`StatusBadge.module.css`，使用 `color-mix()` 实现透明背景，无硬编码颜色值。

**i18n 键**（zh / ja / en 均已添加）：
```
status.masteryLevel, status.mastered, status.learning, status.unlearned, status.pass, status.needsWork
```

**localStorage key 命名规范**：`lesson_read_<userId>_<lessonId>`
- `userId`：从 `localStorage.getItem('user')` 解析 `.id` 字段
- 用于标记某用户是否曾打开该课文详情

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
| `i18next + react-i18next` | 业界标准 React i18n 方案；翻译资源打包进 bundle，零运行时网络请求；localStorage 持久化语言偏好 |
| `fallbackLng: 'zh'` | 中文作为默认语言，覆盖无法检测到偏好语言的场景 |
