---
feature: "001-core-functionality"
title: "日语学习应用 - 技术实现方案"
status: "草稿"
version: "1.0"
created: "2026-04-03"
branch: "001-japanese-learning-app"
spec: "./spec.md"
api_sketch: "./api-sketch.md"
---

# 实施计划: 日语学习应用 (japanese-learning-app)

**分支**: `001-japanese-learning-app` | **日期**: 2026-04-03 | **规范**: [spec.md](./spec.md)
**输入**: 来自 `specs/001-core-functionality/spec.md` 的功能规范 (v1.1)

---

## 摘要

本方案为边上班边学日语的职场人士构建一款 Web 优先的日语学习应用，后端采用 Go 标准库实现 RESTful API，数据层使用 SQLite（嵌入式，无需独立部署），前端采用轻量级 TypeScript + 原生 DOM 操作，服务端使用 `html/template` 渲染页面骨架。

核心模块按 MVP 渐进发布：V1 单词记忆（SM-2 间隔重复） → V2 语法学习 → V3 课文 + 口语跟读 → V4 写作 + 自由朗读 → V5 iOS。

---

## 技术背景

| 项目 | 选型 | 说明 |
|---|---|---|
| **语言/版本** | Go ≥ 1.21.0 | 使用泛型、`slog` 结构化日志（1.21 标准库新增）|
| **Web 框架** | `net/http`（标准库）| 不引入 Gin / Echo，路由使用 `http.ServeMux` |
| **数据存储** | SQLite（`database/sql` + `modernc.org/sqlite`）| 纯 Go 驱动，无 CGO 依赖，嵌入式部署 |
| **认证** | JWT，手工实现（`crypto/hmac` + `encoding/base64`）| 不引入第三方 JWT 库 |
| **日志** | `log/slog`（标准库，Go 1.21+）| 结构化日志，支持 DEBUG / INFO / ERROR 级别 |
| **前端语言** | TypeScript（编译为 ES2020）| 无框架，原生 DOM + Fetch API |
| **前端构建** | `esbuild`（单个可执行文件，极简）| 仅用于 TS → JS 编译，不引入 webpack |
| **模板渲染** | `html/template`（标准库）| 服务端渲染页面骨架，数据通过 Fetch 动态填充 |
| **测试** | `testing`（标准库）| 表格驱动测试，`data/` 层使用真实 SQLite |
| **目标平台** | Web 浏览器（桌面 + 移动响应式）| iOS 为 V5 后续迭代 |
| **性能目标** | 90% 操作 ≤ 300ms；课文加载 ≤ 3s；口语评分 ≤ 5s | 对应 SC-003/SC-005/SC-008 |
| **约束条件** | 离线可用（已缓存内容）；无网络时进度本地暂存 | 对应 NFR-004 |

---

## 合宪性审查

> 逐条对照 `constitution.md`，确认本方案无违规。

### 第一条：简单性原则 (Simplicity First)

| 子条款 | 审查结论 | 说明 |
|---|---|---|
| **1.1 YAGNI** | ✅ 合规 | 仅实现 spec.md v1.1 明确要求的 7 个模块，不预留"扩展点"抽象 |
| **1.2 标准库优先** | ✅ 合规 | HTTP: `net/http`；日志: `log/slog`；认证: `crypto/hmac`；模板: `html/template`。唯一引入的外部依赖是 `modernc.org/sqlite`（SQLite 纯 Go 驱动，无标准库替代）|
| **1.3 反过度工程** | ✅ 合规 | 每个模块 3 文件（handler/service/model），不引入 Repository 模式、DI 框架、Event Bus 等额外抽象层 |

> **唯一外部依赖豁免**：`modernc.org/sqlite` — 标准库 `database/sql` 需要驱动才能操作 SQLite，且该驱动为纯 Go 实现（无 CGO），符合"尽量不使用第三方库"的精神。

---

### 第二条：测试先行铁律 (Test-First Imperative) — 不可协商

| 子条款 | 审查结论 | 说明 |
|---|---|---|
| **2.1 TDD 循环** | ✅ 强制执行 | 所有功能开发前先写失败测试（Red），再写最小实现（Green），最后重构（Refactor）|
| **2.2 表格驱动** | ✅ 强制执行 | SM-2 算法、音频评分、语法检验评分等核心逻辑必须使用 `[]struct{ input ...; want ... }` 表格测试 |
| **2.3 拒绝 Mocks** | ✅ 合规 | `data/` 层测试使用真实 SQLite 文件（`TestMain` 中创建/销毁），`writing/` 的 AI 批改通过 `AIReviewer` 接口注入，测试时使用真实 stub 返回（不用 mock 框架）|

---

### 第三条：明确性原则 (Clarity and Explicitness)

| 子条款 | 审查结论 | 说明 |
|---|---|---|
| **3.1 错误处理** | ✅ 强制执行 | 所有错误必须 `fmt.Errorf("context: %w", err)` 包装；handler 层统一转为 JSON 错误响应；**不允许** `_ = err` 或裸 `return` |
| **3.2 无全局变量** | ✅ 强制执行 | `*sql.DB`、`Config`、`slog.Logger` 均通过结构体成员注入；路由注册在 `main.go` 中显式完成 |

---

### 第四条：方便调试的原则

| 子条款 | 审查结论 | 说明 |
|---|---|---|
| **4.1 错误日志** | ✅ 强制执行 | 所有 `return err` 前必须 `slog.Error("msg", "err", err, "context", ...)` |
| **4.2 Debug 日志** | ✅ 强制执行 | 关键函数入口/出口记录 `slog.Debug`，包含请求参数和返回值摘要；生产环境通过 `LOG_LEVEL=INFO` 关闭 |

---

### 复杂度跟踪（违规豁免申请）

| 违规项 | 为什么需要 | 拒绝更简单替代方案的原因 |
|---|---|---|
| 引入 `modernc.org/sqlite` | SQLite 驱动无标准库实现 | `database/sql` 是标准库，但必须配合驱动才能工作。内存 map 无法满足 SM-2 算法需要的持久化排序查询 |
| `writing/ai_client.go` 接口抽象 | LLM API（Claude/GPT）属外部不可控依赖 | 不做接口隔离则单元测试必须网络通信；接口仅含 1 个方法，不属于过度设计 |

---

## 项目结构

### 文档（本功能）

```
specs/001-core-functionality/
├── spec.md              # 功能规范 v1.1
├── api-sketch.md        # API 端点草图 & 包结构设计
└── plan.md              # 本文件 — 技术实现方案
```

### 源代码（仓库根目录）

```
japanese-learning-app/
│
├── backend/
│   └── cmd/
│       └── server/
│           └── main.go          # 程序唯一入口
│                                # 职责：读 Config → 初始化 DB → 组装所有 Service/Handler
│                                #       → 注册路由 → 启动 net/http 服务器
│
├── internal/                    # Go 编译器强制：不可被外部包 import
│   │
│   ├── config/
│   │   ├── config.go            # Config struct + Load(path string) (*Config, error)
│   │   └── config_test.go       # 表格驱动：各种配置文件场景
│   │
│   ├── data/                    # 数据访问层（仅 SQL IO，零业务逻辑）
│   │   ├── db.go                # OpenDB(path) (*sql.DB, error) + RunMigrations()
│   │   ├── word_store.go        # WordStore struct，实现 WordReader/WordWriter 接口
│   │   ├── grammar_store.go     # GrammarStore struct
│   │   ├── lesson_store.go      # LessonStore struct
│   │   ├── user_store.go        # UserStore struct
│   │   ├── session_store.go     # SessionStore struct（StudySession + SessionSummary）
│   │   ├── migrations/
│   │   │   ├── 001_init.sql     # 建表 DDL
│   │   │   └── 002_seed.sql     # 初始词库种子数据（N5/N4 基础词汇）
│   │   └── *_test.go            # 集成测试（TestMain 创建真实 SQLite 文件）
│   │
│   ├── module/
│   │   │
│   │   ├── word/                         # ── 单词记忆 ──
│   │   │   ├── model.go                  # Word, WordRecord, ReviewRating 类型定义
│   │   │   ├── service.go                # WordService struct
│   │   │   │                             #   ReviewQueue(userID) ([]WordCard, error)
│   │   │   │                             #   SubmitRating(userID, wordID, rating) error
│   │   │   │                             #   Bookmark(userID, wordID) error
│   │   │   ├── sm2.go                    # SM-2 算法纯函数（无副作用，便于测试）
│   │   │   │                             #   CalcNextReview(record WordRecord, rating) WordRecord
│   │   │   ├── handler.go                # 注册路由，调用 WordService，写 JSON 响应
│   │   │   ├── sm2_test.go               # ← 表格驱动：SM-2 算法各评分场景
│   │   │   └── service_test.go           # ← 集成测试：ReviewQueue 排序逻辑
│   │   │
│   │   ├── grammar/                      # ── 语法学习 ──
│   │   │   ├── model.go                  # GrammarPoint, GrammarRecord, QuizQuestion, QuizResult
│   │   │   ├── service.go                # GrammarService struct
│   │   │   │                             #   ListByLevel(level) ([]GrammarPoint, error)
│   │   │   │                             #   GetDetail(id) (*GrammarPoint, error)
│   │   │   │                             #   SubmitQuiz(userID, pointID, answers) (QuizResult, error)
│   │   │   │                             #   Enqueue(userID, pointID) error
│   │   │   ├── handler.go
│   │   │   └── service_test.go           # ← 表格驱动：检验题评分逻辑
│   │   │
│   │   ├── lesson/                       # ── 课文学习 ──
│   │   │   ├── model.go                  # Lesson, Sentence, FuriganaToken
│   │   │   ├── service.go                # LessonService struct
│   │   │   │                             #   ListByLevel(level, tag) ([]LessonSummary, error)
│   │   │   │                             #   GetDetail(id) (*Lesson, error)
│   │   │   │                             #   GetSentences(id) ([]Sentence, error)
│   │   │   │                             #   BookmarkWords(userID, lessonID, wordIDs) error
│   │   │   ├── handler.go
│   │   │   └── service_test.go
│   │   │
│   │   ├── speaking/                     # ── 口语练习 ──
│   │   │   ├── model.go                  # SpeakingMaterial, SpeakingRecord, ScoreResult, PracticeType
│   │   │   ├── scorer.go                 # 音频相似度纯函数（波形特征比对）
│   │   │   │                             #   Score(reference, userAudio []byte) (ScoreResult, error)
│   │   │   ├── service.go                # SpeakingService struct
│   │   │   │                             #   GetMaterials(ptype, level) ([]SpeakingMaterial, error)
│   │   │   │                             #   SubmitRecording(userID, materialID, ptype, audio) (ScoreResult, error)
│   │   │   │                             #   GetHistory(userID) ([]SpeakingRecord, error)
│   │   │   ├── handler.go                # 处理 multipart/form-data 上传
│   │   │   └── scorer_test.go            # ← 表格驱动：不同相似度场景
│   │   │
│   │   ├── writing/                      # ── 写作练习 ──
│   │   │   ├── model.go                  # WritingQuestion, WritingRecord, AIFeedback, WritingType
│   │   │   ├── ai_client.go              # AIReviewer 接口 + ClaudeClient 实现
│   │   │   │                             #   type AIReviewer interface {
│   │   │   │                             #     Review(question, answer string) (AIFeedback, error)
│   │   │   │                             #   }
│   │   │   ├── service.go                # WritingService struct（注入 AIReviewer）
│   │   │   │                             #   DailyQueue(userID) ([]WritingQuestion, error)
│   │   │   │                             #   SubmitInput(userID, qID, answer) (bool, error)
│   │   │   │                             #   SubmitSentence(userID, qID, answer) (AIFeedback, error)
│   │   │   ├── handler.go
│   │   │   └── service_test.go           # ← stub AIReviewer，不发真实网络请求
│   │   │
│   │   ├── summary/                      # ── 练习总结 ──
│   │   │   ├── model.go                  # SessionSummary, SummaryItem, ScoreSummary
│   │   │   ├── service.go                # SummaryService struct
│   │   │   │                             #   Generate(userID, sessionID, module) (SessionSummary, error)
│   │   │   │                             #   GetBySession(sessionID) (*SessionSummary, error)
│   │   │   │                             # 只通过 data/session_store 读数据，不 import 其他模块
│   │   │   ├── handler.go
│   │   │   └── service_test.go           # ← 表格驱动：不同会话数据 → 预期亮点/待改进
│   │   │
│   │   └── user/                         # ── 用户账户 & 认证 ──
│   │       ├── model.go                  # User, RegisterReq, LoginReq, TokenResp, UserStats
│   │       ├── service.go                # UserService struct
│   │       │                             #   Register(req RegisterReq) (*User, error)
│   │       │                             #   Login(req LoginReq) (TokenResp, error)
│   │       │                             #   GetStats(userID) (UserStats, error)
│   │       ├── jwt.go                    # SignToken / VerifyToken（crypto/hmac，无外部库）
│   │       ├── middleware.go             # AuthMiddleware(next http.Handler) http.Handler
│   │       ├── handler.go
│   │       └── service_test.go           # ← 表格驱动：密码哈希、JWT 签发/验证
│   │
│   └── cli/                             # 管理员命令行工具
│       ├── root.go                      # flag 包解析子命令
│       ├── import_words.go              # 从 JSON/CSV 批量导入词库
│       └── import_words_test.go
│
├── front/
│   ├── web/
│   │   ├── templates/                   # html/template 模板（服务端骨架渲染）
│   │   │   ├── base.html                # 公共布局（极简：顶部导航 + 内容区 + 底部状态栏）
│   │   │   ├── word/
│   │   │   │   ├── index.html           # 单词复习主页
│   │   │   │   └── stats.html           # 学习统计
│   │   │   ├── grammar/
│   │   │   │   ├── index.html           # 语法点列表
│   │   │   │   └── detail.html          # 语法点详情 + 检验题
│   │   │   ├── lesson/
│   │   │   │   ├── index.html           # 课文列表
│   │   │   │   └── detail.html          # 课文阅读页（振り仮名渲染）
│   │   │   ├── speaking/
│   │   │   │   └── index.html           # 口语练习（影子跟读 + 自由朗读）
│   │   │   ├── writing/
│   │   │   │   └── index.html           # 写作练习
│   │   │   └── summary/
│   │   │       └── index.html           # 练习总结
│   │   └── static/
│   │       ├── css/
│   │       │   └── main.css             # 极简样式（无框架，仅 CSS 变量 + 系统字体栈）
│   │       └── js/
│   │           ├── word.ts              # 单词卡片翻转、评分提交
│   │           ├── grammar.ts           # 检验题交互
│   │           ├── lesson.ts            # 音频同步高亮、释义弹窗
│   │           ├── speaking.ts          # 录音 API（MediaRecorder）、音频上传
│   │           ├── writing.ts           # 输入法联动、造句提交
│   │           ├── summary.ts           # 总结页渲染
│   │           └── api.ts              # 统一 Fetch 封装（含 token 注入、错误处理）
│   └── ios/                             # V5 占位目录
│
├── Makefile                             # 标准化操作入口
├── go.mod
├── go.sum
└── specs/
    └── 001-core-functionality/
        ├── spec.md
        ├── api-sketch.md
        └── plan.md                      # 本文件
```

**结构决策**：采用「选项 2（Web 应用）」模式——后端 Go 代码在 `internal/`，前端资源在 `front/web/`，通过 API 松耦合。`backend/cmd/server/main.go` 作为唯一组装点，负责依赖注入和路由注册。

---

## 核心数据结构

> 以下 Go struct 定义是模块间流转的标准数据契约。JSON tag 即为 API 响应字段名。

### `internal/module/word/model.go`

```go
package word

import "time"

// JLPTLevel 表示 JLPT 等级
type JLPTLevel string

const (
    LevelN5 JLPTLevel = "N5"
    LevelN4 JLPTLevel = "N4"
    LevelN3 JLPTLevel = "N3"
    LevelN2 JLPTLevel = "N2"
    LevelN1 JLPTLevel = "N1"
)

// ReviewRating 用户对单词的三级评分
type ReviewRating string

const (
    RatingEasy   ReviewRating = "easy"
    RatingNormal ReviewRating = "normal"
    RatingHard   ReviewRating = "hard"
)

// WordExample 单词例句
type WordExample struct {
    Japanese string `json:"japanese"`
    Chinese  string `json:"chinese"`
}

// Word 表示词库中的一个日语单词（内容库，只读）
type Word struct {
    ID           int64        `json:"id"`
    KanjiForm    string       `json:"kanji_form"`    // 汉字写法，如「勉強」
    Reading      string       `json:"reading"`       // 假名读音，如「べんきょう」
    PartOfSpeech string       `json:"part_of_speech"` // 词性，如「名詞」
    Meaning      string       `json:"meaning"`       // 中文释义
    Examples     []WordExample `json:"examples"`
    JLPTLevel    JLPTLevel    `json:"jlpt_level"`
}

// WordRecord 用户与某个单词的学习关系（用户数据，读写）
type WordRecord struct {
    ID           int64        `json:"id"`
    UserID       int64        `json:"user_id"`
    WordID       int64        `json:"word_id"`
    MasteryLevel int          `json:"mastery_level"` // 0~5，SM-2 重复次数
    NextReviewAt time.Time    `json:"next_review_at"`
    EaseFactor   float64      `json:"ease_factor"`   // SM-2 EF，初始 2.5
    Interval     int          `json:"interval"`      // 距下次复习的天数
    ReviewHistory []ReviewEvent `json:"review_history"`
    UpdatedAt    time.Time    `json:"updated_at"`
}

// ReviewEvent 一次评分事件记录
type ReviewEvent struct {
    Rating    ReviewRating `json:"rating"`
    ReviewedAt time.Time  `json:"reviewed_at"`
}

// WordCard 复习队列中的单张卡片（聚合 Word + WordRecord）
type WordCard struct {
    Word       Word       `json:"word"`
    Record     WordRecord `json:"record"`
    IsNew      bool       `json:"is_new"` // true 表示首次学习
}
```

---

### `internal/module/grammar/model.go`

```go
package grammar

import "time"

// QuizType 检验题类型
type QuizType string

const (
    QuizFillBlank  QuizType = "fill_blank"  // 填空
    QuizMultiChoice QuizType = "multi_choice" // 选择
)

// QuizQuestion 语法检验题
type QuizQuestion struct {
    ID          int64    `json:"id"`
    Type        QuizType `json:"type"`
    Prompt      string   `json:"prompt"`      // 题目（含空格标记，如「___てもいい」）
    Options     []string `json:"options,omitempty"` // 选择题选项
    Answer      string   `json:"answer"`      // 正确答案（服务端存储，响应时不返回）
    Explanation string   `json:"explanation"` // 解析（答错后展示）
}

// GrammarExample 语法例句
type GrammarExample struct {
    Japanese   string `json:"japanese"`
    Chinese    string `json:"chinese"`
    LinkedWords []int64 `json:"linked_word_ids,omitempty"` // 可一键加入单词本的词汇
}

// GrammarPoint 语法点（内容库）
type GrammarPoint struct {
    ID              int64            `json:"id"`
    Name            string           `json:"name"`             // 如「〜てもいい」
    Meaning         string           `json:"meaning"`          // 中文意思
    ConjunctionRule string           `json:"conjunction_rule"` // 接续方式
    UsageNote       string           `json:"usage_note"`       // 使用场景说明
    Examples        []GrammarExample `json:"examples"`
    QuizQuestions   []QuizQuestion   `json:"quiz_questions"`
    JLPTLevel       JLPTLevel        `json:"jlpt_level"`
}

// GrammarStatus 用户对语法点的学习状态
type GrammarStatus string

const (
    StatusUnlearned  GrammarStatus = "unlearned"   // 未学
    StatusLearning   GrammarStatus = "learning"    // 学习中
    StatusMastered   GrammarStatus = "mastered"    // 已掌握
)

// GrammarRecord 用户语法学习记录
type GrammarRecord struct {
    ID             int64         `json:"id"`
    UserID         int64         `json:"user_id"`
    GrammarPointID int64         `json:"grammar_point_id"`
    Status         GrammarStatus `json:"status"`
    NextReviewAt   time.Time     `json:"next_review_at"`
    QuizHistory    []QuizAttempt `json:"quiz_history"`
}

// QuizAttempt 一次检验记录
type QuizAttempt struct {
    Score       int       `json:"score"`       // 本次得分（0~100）
    AttemptedAt time.Time `json:"attempted_at"`
}

// QuizSubmission 用户提交的检验答案
type QuizSubmission struct {
    QuestionID int64  `json:"question_id"`
    Answer     string `json:"answer"`
}

// QuizResult 检验结果
type QuizResult struct {
    Score   int           `json:"score"` // 0~100
    Results []QuizItemResult `json:"results"`
}

// QuizItemResult 单题结果
type QuizItemResult struct {
    QuestionID  int64  `json:"question_id"`
    Correct     bool   `json:"correct"`
    UserAnswer  string `json:"user_answer"`
    Expected    string `json:"expected"`
    Explanation string `json:"explanation"` // 答错时返回
}
```

---

### `internal/module/lesson/model.go`

```go
package lesson

// FuriganaToken 振り仮名标注单元
// 对于有汉字的词：{ Surface: "勉強", Reading: "べんきょう" }
// 对于假名直接：  { Surface: "です", Reading: "" }
type FuriganaToken struct {
    Surface string `json:"surface"` // 显示文字
    Reading string `json:"reading"` // 假名读音（空字符串表示无需标注）
}

// Sentence 课文中的一个句子
type Sentence struct {
    Index      int             `json:"index"`
    Tokens     []FuriganaToken `json:"tokens"`      // 振り仮名分词结果
    Chinese    string          `json:"chinese"`     // 中文翻译
    StartMS    int64           `json:"start_ms"`    // 音频开始时间（毫秒）
    EndMS      int64           `json:"end_ms"`      // 音频结束时间（毫秒）
}

// LessonSummary 课文列表项（不含全文内容，减少传输量）
type LessonSummary struct {
    ID        int64     `json:"id"`
    Title     string    `json:"title"`
    JLPTLevel JLPTLevel `json:"jlpt_level"`
    Tags      []string  `json:"tags"`
    CharCount int       `json:"char_count"`
    AudioURL  string    `json:"audio_url"`
}

// Lesson 课文详情
type Lesson struct {
    LessonSummary
    Sentences []Sentence `json:"sentences"`
    WordIDs   []int64    `json:"word_ids"` // 课文中可加入单词本的词汇 ID 列表
}
```

---

### `internal/module/speaking/model.go`

```go
package speaking

import "time"

// PracticeType 口语练习类型
type PracticeType string

const (
    PracticeTypeShadow PracticeType = "shadow" // 影子跟读
    PracticeTypeFree   PracticeType = "free"   // 自由朗读
)

// SentenceAnnotation 评分后对单个句子的标注
type SentenceAnnotation struct {
    SentenceIndex int    `json:"sentence_index"`
    Score         int    `json:"score"`         // 0~100，该句得分
    NeedsAttention bool  `json:"needs_attention"` // 是否需要注意
    Note          string `json:"note,omitempty"`  // 提示说明
}

// ScoreResult 口语评分结果
type ScoreResult struct {
    OverallScore  int                  `json:"overall_score"` // 0~100
    Annotations   []SentenceAnnotation `json:"annotations"`
    FeedbackMS    int64                `json:"feedback_ms"` // 评分耗时（毫秒，用于监控 SC-005）
}

// SpeakingRecord 一次口语练习记录
type SpeakingRecord struct {
    ID          int64        `json:"id"`
    UserID      int64        `json:"user_id"`
    Type        PracticeType `json:"type"`
    MaterialID  int64        `json:"material_id"`
    Score       int          `json:"score"`
    AudioRef    string       `json:"audio_ref"`   // 录音文件存储路径
    PracticedAt time.Time    `json:"practiced_at"`
}
```

---

### `internal/module/writing/model.go`

```go
package writing

import "time"

// WritingType 写作练习类型
type WritingType string

const (
    WritingTypeInput    WritingType = "input"    // 键盘输入练习
    WritingTypeSentence WritingType = "sentence" // 造句练习
)

// WritingQuestion 写作题目
type WritingQuestion struct {
    ID              int64       `json:"id"`
    Type            WritingType `json:"type"`
    Prompt          string      `json:"prompt"`           // 题目提示（中文或假名）
    GrammarPointID  int64       `json:"grammar_point_id,omitempty"` // 造句题关联的语法点
    ExpectedAnswer  string      `json:"-"`                // 仅后端存储，不返回前端
}

// AIFeedback AI 批改结果
type AIFeedback struct {
    Score              int      `json:"score"`               // 0~100
    GrammarCorrect     bool     `json:"grammar_correct"`
    VocabCorrect       bool     `json:"vocab_correct"`
    IssueDescription   string   `json:"issue_description"`   // 问题说明（空表示全对）
    CorrectedSentence  string   `json:"corrected_sentence"`  // 修改后的句子
    AlternativePhrases []string `json:"alternative_phrases"` // 其他地道表达
    ReferenceAnswer    string   `json:"reference_answer"`
}

// WritingRecord 一次写作练习记录
type WritingRecord struct {
    ID          int64       `json:"id"`
    UserID      int64       `json:"user_id"`
    Type        WritingType `json:"type"`
    Question    string      `json:"question"`
    UserAnswer  string      `json:"user_answer"`
    AIFeedback  *AIFeedback `json:"ai_feedback,omitempty"` // 输入练习无 AI 反馈
    Score       int         `json:"score"`
    PracticedAt time.Time   `json:"practiced_at"`
}
```

---

### `internal/module/summary/model.go`

```go
package summary

import "time"

// ModuleType 学习模块类型
type ModuleType string

const (
    ModuleWord     ModuleType = "word"
    ModuleGrammar  ModuleType = "grammar"
    ModuleLesson   ModuleType = "lesson"
    ModuleSpeaking ModuleType = "speaking"
    ModuleWriting  ModuleType = "writing"
)

// SummaryItem 总结中的单条亮点或待改进项
type SummaryItem struct {
    Label string `json:"label"` // 对象名称（如单词、语法点名称）
    Note  string `json:"note"`  // 说明（如「连续3次评为容易」）
}

// ScoreSummary 得分概要（各模块字段不同，使用灵活 map）
type ScoreSummary map[string]any
// 单词：{ "reviewed": 15, "easy_rate": 0.47, "hard_count": 4 }
// 语法：{ "score": 80, "correct": 2, "total": 3 }
// 口语：{ "score": 78, "history_avg": 71 }
// 写作：{ "completed": 4, "avg_score": 82 }

// SessionSummary 一次练习会话的总结
type SessionSummary struct {
    ID                     int64        `json:"id"`
    UserID                 int64        `json:"user_id"`
    SessionID              string       `json:"session_id"`
    Module                 ModuleType   `json:"module"`
    ScoreSummary           ScoreSummary `json:"score_summary"`
    Strengths              []SummaryItem `json:"strengths"`              // 亮点
    Weaknesses             []SummaryItem `json:"weaknesses"`             // 待改进
    ImprovementSuggestions []string      `json:"improvement_suggestions"` // 1~3 条建议
    GeneratedAt            time.Time     `json:"generated_at"`
}
```

---

### `internal/module/user/model.go`

```go
package user

import "time"

// User 用户账户
type User struct {
    ID          int64     `json:"id"`
    Email       string    `json:"email"`
    GoalLevel   JLPTLevel `json:"goal_level"` // 学习目标等级
    StreakDays  int       `json:"streak_days"` // 连续学习天数
    CreatedAt   time.Time `json:"created_at"`
}

// RegisterReq 注册请求
type RegisterReq struct {
    Email     string    `json:"email"`
    Password  string    `json:"password"`  // 明文，服务端立即哈希，不持久化
    GoalLevel JLPTLevel `json:"goal_level"`
}

// LoginReq 登录请求
type LoginReq struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

// TokenResp 登录成功响应
type TokenResp struct {
    Token     string    `json:"token"`
    ExpiresAt time.Time `json:"expires_at"`
    User      User      `json:"user"`
}

// UserStats 学习统计看板数据
type UserStats struct {
    StreakDays     int                    `json:"streak_days"`
    TotalMinutes   int                    `json:"total_minutes"`
    ModuleStats    map[string]ModuleStat  `json:"module_stats"`
}

// ModuleStat 单个模块的使用统计
type ModuleStat struct {
    SessionCount   int `json:"session_count"`
    TotalMinutes   int `json:"total_minutes"`
    LastPracticedAt string `json:"last_practiced_at,omitempty"`
}
```

---

### `internal/config/config.go`

```go
package config

// Config 应用配置（从环境变量或配置文件加载）
type Config struct {
    // 服务器
    ListenAddr string // 默认 ":8080"

    // 数据库
    DBPath string // SQLite 文件路径，默认 "./data/app.db"

    // 认证
    JWTSecret     string // HMAC 签名密钥，生产环境必须设置
    JWTExpireHours int   // Token 有效期（小时），默认 72

    // 日志
    LogLevel string // "DEBUG" | "INFO" | "WARN" | "ERROR"，默认 "INFO"

    // AI 批改（写作模块）
    AIAPIKey      string // LLM API Key（Claude 或 OpenAI）
    AIAPIEndpoint string // API 端点 URL
    AITimeoutSec  int    // 请求超时秒数，默认 15

    // 文件存储（口语录音）
    AudioStorePath string // 录音文件存储目录，默认 "./data/audio"
}
```

---

## 接口设计

> `internal` 包对外暴露的关键接口，Service 层通过构造函数注入。

### 数据存储接口（`internal/data/`）

```go
// WordStore 单词数据访问接口
type WordStore interface {
    GetByID(id int64) (*word.Word, error)
    ListByLevel(level word.JLPTLevel, page, size int) ([]word.Word, int, error)

    GetRecord(userID, wordID int64) (*word.WordRecord, error)
    ListDueRecords(userID int64, limit int) ([]word.WordRecord, error) // SM-2 到期队列
    UpsertRecord(record word.WordRecord) error
    BookmarkWord(userID, wordID int64) error
}

// GrammarStore 语法点数据访问接口
type GrammarStore interface {
    GetByID(id int64) (*grammar.GrammarPoint, error)
    ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error)

    GetRecord(userID, pointID int64) (*grammar.GrammarRecord, error)
    UpsertRecord(record grammar.GrammarRecord) error
    ListDueRecords(userID int64) ([]grammar.GrammarRecord, error)
}

// LessonStore 课文数据访问接口
type LessonStore interface {
    ListSummaries(level lesson.JLPTLevel, tag string) ([]lesson.LessonSummary, error)
    GetDetail(id int64) (*lesson.Lesson, error)
    GetSentences(lessonID int64) ([]lesson.Sentence, error)
}

// UserStore 用户账户数据访问接口
type UserStore interface {
    Create(email, passwordHash string, goalLevel user.JLPTLevel) (*user.User, error)
    GetByEmail(email string) (*user.User, error)
    GetByID(id int64) (*user.User, error)
    UpdateStreak(userID int64, streakDays int) error
}

// SessionStore 会话与总结数据访问接口
type SessionStore interface {
    CreateSession(session study.StudySession) (string, error) // 返回 session_id
    GetSessionData(sessionID string) (*study.StudySession, error)
    SaveSummary(s summary.SessionSummary) error
    GetSummary(sessionID string) (*summary.SessionSummary, error)
}

// SpeakingStore 口语记录访问接口
type SpeakingStore interface {
    SaveRecord(record speaking.SpeakingRecord) error
    ListRecords(userID int64) ([]speaking.SpeakingRecord, error)
    GetRecord(id int64) (*speaking.SpeakingRecord, error)
}

// WritingStore 写作记录访问接口
type WritingStore interface {
    GetDailyQueue(userID int64) ([]writing.WritingQuestion, error)
    SaveRecord(record writing.WritingRecord) error
    ListRecords(userID int64) ([]writing.WritingRecord, error)
}
```

---

### 核心业务接口

```go
// AIReviewer 写作 AI 批改接口（internal/module/writing/ai_client.go）
// 通过接口隔离 LLM API 调用，测试时可替换为 stub
type AIReviewer interface {
    Review(question, userAnswer string) (writing.AIFeedback, error)
}

// AudioScorer 音频相似度评分接口（internal/module/speaking/scorer.go）
// 纯函数形式，接口用于未来扩展（如接入商业 ASR API）
type AudioScorer interface {
    Score(referenceAudio, userAudio []byte) (speaking.ScoreResult, error)
}
```

---

### HTTP 响应标准格式

所有 handler 返回统一的 JSON 结构：

```go
// internal/httputil/response.go（供所有 handler 使用的工具函数）

// APIResponse 成功响应包装
type APIResponse struct {
    Data      any    `json:"data"`
    RequestID string `json:"request_id"`
}

// APIError 错误响应
type APIError struct {
    Code      string `json:"code"`       // 业务错误码，如 "ERR_WORD_NOT_FOUND"
    Message   string `json:"message"`    // 用户可读消息
    RequestID string `json:"request_id"` // 用于日志追踪
}

// 使用示例（handler 层）：
// WriteJSON(w, http.StatusOK, APIResponse{Data: cards, RequestID: reqID})
// WriteError(w, http.StatusNotFound, "ERR_WORD_NOT_FOUND", "单词不存在", reqID)
```

---

## 日志规范

遵循宪法第四条，使用 `log/slog` 结构化日志：

```go
// 错误发生处（必须）
slog.Error("failed to get review queue",
    "err", err,
    "user_id", userID,
    "request_id", reqID,
)

// 关键业务操作（INFO 级别）
slog.Info("word review submitted",
    "user_id", userID,
    "word_id", wordID,
    "rating", rating,
    "next_review_at", newRecord.NextReviewAt,
)

// 函数入口/出口（DEBUG 级别，生产环境关闭）
slog.Debug("ReviewQueue called",
    "user_id", userID,
    "due_count", len(cards),
)
```

---

## Makefile 标准操作

```makefile
.PHONY: run test build lint seed

run:        ## 启动开发服务器
	go run ./backend/cmd/server/

test:       ## 运行所有测试（含集成测试）
	go test ./... -v -count=1

build:      ## 编译服务端二进制
	go build -o bin/server ./backend/cmd/server/

lint:       ## 静态检查
	go vet ./...

seed:       ## 导入初始词库（N5/N4）
	go run ./internal/cli/ import-words --file ./data/seed/words_n5.json

front-build: ## 编译前端 TypeScript
	npx esbuild front/web/static/js/*.ts --bundle --outdir=front/web/static/js/dist

clean:
	rm -rf bin/ front/web/static/js/dist/
```

---

## MVP 阶段对应工作量

| 版本 | 核心工作 | 需实现的 FR | 预估复杂度 |
|---|---|---|---|
| **V1** | 单词记忆（SM-2）+ 用户账户 + SQLite | FR-001~007, FR-033~037 | 高（算法核心）|
| **V2** | 语法学习 + 检验题评分 | FR-008~014 | 中 |
| **V3** | 课文学习（振り仮名）+ 影子跟读 | FR-015~020, FR-021~023, FR-027 | 中高（音频处理）|
| **V4** | 写作练习（AI 批改）+ 自由朗读 + 总结 | FR-024~026, FR-028~032, FR-038~046 | 高（外部 API）|
| **V5** | iOS App（复用后端 API）| — | 后续单独规划 |
