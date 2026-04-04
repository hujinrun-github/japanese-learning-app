---
feature: "001-core-functionality"
title: "API 草图 & 包结构设计"
status: "草稿"
version: "1.0"
created: "2026-04-03"
relates_to: "spec.md"
---

# API 草图 & 包结构设计

本文档描述：
1. 项目整体包结构及各包的职责边界
2. 每个模块对外暴露的 RESTful API 端点草图
3. 关键设计决策及其与 `constitution.md` 的对应关系

---

## 一、包结构总览

```
japanese-learning-app/
│
├── backend/
│   └── cmd/
│       └── server/
│           └── main.go          # 程序入口：组装依赖，启动 net/http 服务
│
├── internal/                    # 所有业务代码；Go 编译器保证不被外部导入
│   │
│   ├── config/                  # 配置管理
│   │   ├── config.go            # Config 结构体定义、Load() 函数
│   │   └── config_test.go
│   │
│   ├── data/                    # 数据访问层（仅 SQL / 文件 IO，无业务逻辑）
│   │   ├── db.go                # *sql.DB 初始化与 migration
│   │   ├── word_store.go        # WordStore: 词库 CRUD
│   │   ├── grammar_store.go     # GrammarStore: 语法点 CRUD
│   │   ├── lesson_store.go      # LessonStore: 课文 CRUD
│   │   ├── user_store.go        # UserStore: 用户账户 CRUD
│   │   ├── session_store.go     # StudySession / SessionSummary 存取
│   │   └── *_test.go            # 集成测试，使用真实数据库（参见宪法 2.3）
│   │
│   ├── module/
│   │   ├── word/                # 单词记忆模块
│   │   │   ├── handler.go       # HTTP handler（注册路由、解析请求、写响应）
│   │   │   ├── service.go       # 业务逻辑：SM-2 调度、卡片队列计算
│   │   │   ├── model.go         # Word, WordRecord 结构体
│   │   │   └── service_test.go  # 表格驱动单元测试（SM-2 算法验证）
│   │   │
│   │   ├── grammar/             # 语法学习模块
│   │   │   ├── handler.go
│   │   │   ├── service.go       # 语法点调度、检验题评分
│   │   │   ├── model.go         # GrammarPoint, GrammarRecord, QuizQuestion
│   │   │   └── service_test.go
│   │   │
│   │   ├── lesson/              # 课文学习模块
│   │   │   ├── handler.go
│   │   │   ├── service.go       # 振り仮名解析、句子分割、音频时间戳关联
│   │   │   ├── model.go         # Lesson, Sentence, FuriganaToken
│   │   │   └── service_test.go
│   │   │
│   │   ├── speaking/            # 口语练习模块（跟读 + 自由朗读）
│   │   │   ├── handler.go
│   │   │   ├── service.go       # 录音接收、音频相似度评分调用
│   │   │   ├── scorer.go        # 音频波形特征比对（纯函数，便于测试）
│   │   │   ├── model.go         # SpeakingRecord, ScoreResult
│   │   │   └── scorer_test.go   # 表格驱动：各种录音场景的评分预期
│   │   │
│   │   ├── writing/             # 写作练习模块（键盘输入 + 造句 + AI 批改）
│   │   │   ├── handler.go
│   │   │   ├── service.go       # 输入判题、AI 批改请求封装
│   │   │   ├── ai_client.go     # LLM API 调用（接口隔离，便于替换）
│   │   │   ├── model.go         # WritingRecord, AIFeedback
│   │   │   └── service_test.go
│   │   │
│   │   ├── summary/             # 练习总结模块（跨模块聚合）
│   │   │   ├── handler.go
│   │   │   ├── service.go       # 从各模块会话数据计算 strengths/weaknesses
│   │   │   ├── model.go         # SessionSummary
│   │   │   └── service_test.go  # 表格驱动：不同会话数据 → 预期总结输出
│   │   │
│   │   └── user/                # 用户账户 & 认证模块
│   │       ├── handler.go       # 注册/登录/统计 handler
│   │       ├── service.go       # 密码哈希、JWT 签发/验证
│   │       ├── middleware.go    # AuthMiddleware：JWT 校验，注入 user_id
│   │       ├── model.go         # User, LoginRequest, TokenResponse
│   │       └── service_test.go
│   │
│   └── cli/                     # 命令行工具（管理员用：导入词库、迁移数据等）
│       ├── root.go              # CLI 入口，使用标准 flag 包
│       ├── import_words.go      # 从 CSV/JSON 批量导入单词
│       └── import_words_test.go
│
├── front/
│   ├── web/
│   │   ├── templates/           # Go html/template 模板文件（服务端渲染）
│   │   │   ├── base.html        # 公共布局（导航、页脚）
│   │   │   ├── word/            # 单词模块页面
│   │   │   ├── grammar/         # 语法模块页面
│   │   │   ├── lesson/          # 课文模块页面
│   │   │   ├── speaking/        # 口语模块页面
│   │   │   ├── writing/         # 写作模块页面
│   │   │   └── summary/         # 练习总结页面
│   │   └── static/
│   │       ├── css/             # 样式文件
│   │       └── js/              # 客户端脚本（录音、音频播放、卡片翻转）
│   └── ios/                     # iOS 原生 App（V5 迭代，当前仅占位）
│
└── specs/                       # 规范文档（不参与编译）
    └── 001-core-functionality/
        ├── spec.md
        └── api-sketch.md        # 本文件
```

---

## 二、设计决策 & 宪法对应

| 决策 | 宪法条款 | 说明 |
|---|---|---|
| 每个模块包含 `handler` / `service` / `model` 三个文件，不拆更细 | 第一条 1.3（反过度工程）| 在当前规模下，目录内三文件已足够清晰，不引入 repository/usecase 等额外层 |
| `data/` 包只做数据存取，不含业务逻辑 | 第一条 1.3 | 单一职责，`service.go` 依赖注入 Store 接口，而非直接调用 `database/sql` |
| `summary/` 独立成包，不在各模块内部实现 | 第一条 1.1（YAGNI）+ NFR-002（模块独立）| 总结需要跨模块聚合数据，独立包避免循环依赖 |
| HTTP 服务使用标准库 `net/http`，不引入框架 | 第一条 1.2（标准库优先）| Go 标准库路由在当前端点数量下完全够用 |
| `ai_client.go` 独立文件，通过接口与 `service.go` 解耦 | 第三条 3.2（无全局变量）| LLM API 客户端作为依赖注入，便于测试时替换为 stub |
| 所有错误用 `fmt.Errorf("...: %w", err)` 包装后返回 | 第三条 3.1（错误处理）| `handler.go` 统一将业务错误转换为结构化 JSON 响应 |
| 测试文件与被测文件同包，优先集成测试 | 第二条 2.3（拒绝 Mocks）| `data/` 层测试使用真实 SQLite/PostgreSQL，不 Mock |

---

## 三、API 端点草图

> **约定**：
> - 所有响应体均为 JSON
> - 认证接口使用 `Authorization: Bearer <jwt>` header
> - 错误响应格式：`{"code": "ERR_XXX", "message": "...", "request_id": "..."}`
> - 路径前缀：`/api/v1`

---

### 3.1 用户账户 (`/api/v1/auth`, `/api/v1/users`)

```
POST   /api/v1/auth/register          # 注册：{ email, password, goal_level }
POST   /api/v1/auth/login             # 登录：{ email, password } → { token, user }
GET    /api/v1/users/me               # 获取当前用户信息（需认证）
GET    /api/v1/users/me/stats         # 学习统计看板（连续天数、总时长、模块频率）
```

---

### 3.2 单词记忆 (`/api/v1/words`)

```
# 内容接口（词库，管理员维护）
GET    /api/v1/words                  # 词库列表，支持 ?level=N5&page=1&size=20
GET    /api/v1/words/:id              # 单词详情

# 学习接口（需认证）
GET    /api/v1/words/review/queue     # 今日复习队列（SM-2 调度后的有序列表）
POST   /api/v1/words/review/:id       # 提交评分：{ rating: "easy"|"normal"|"hard" }
GET    /api/v1/words/review/stats     # 复习统计（掌握程度分布、历史评分）
POST   /api/v1/words/:id/bookmark     # 将单词加入个人单词本
```

---

### 3.3 语法学习 (`/api/v1/grammar`)

```
# 内容接口
GET    /api/v1/grammar                # 语法点列表，支持 ?level=N4
GET    /api/v1/grammar/:id            # 语法点详情（含例句、检验题）

# 学习接口（需认证）
POST   /api/v1/grammar/:id/quiz       # 提交检验答案：{ answers: [{q_id, answer}] }
                                      # 响应：{ score, results: [{correct, explanation}] }
POST   /api/v1/grammar/:id/enqueue    # 将语法点加入间隔重复队列
GET    /api/v1/grammar/review/queue   # 今日待复习语法点队列
POST   /api/v1/grammar/review/:id     # 提交复习评分：{ rating: "easy"|"normal"|"hard" }
```

---

### 3.4 课文学习 (`/api/v1/lessons`)

```
GET    /api/v1/lessons                # 课文列表，支持 ?level=N3&tag=日常会话
GET    /api/v1/lessons/:id            # 课文详情（含振り仮名结构、翻译、音频 URL）
GET    /api/v1/lessons/:id/sentences  # 按句分割的文本列表（含时间戳，用于音频同步）
POST   /api/v1/lessons/:id/words/bookmark  # 批量将课文生词加入单词本：{ word_ids: [] }
```

---

### 3.5 口语练习 (`/api/v1/speaking`)

```
GET    /api/v1/speaking/materials     # 跟读/朗读材料列表，支持 ?type=shadow|free&level=N4
GET    /api/v1/speaking/materials/:id # 材料详情（文本、音频 URL）

# 练习接口（需认证）
POST   /api/v1/speaking/records       # 提交录音并请求评分
                                      # 请求：multipart/form-data
                                      #   material_id, type(shadow|free), audio(文件)
                                      # 响应：{ score, annotated_sentences, record_id }
GET    /api/v1/speaking/records       # 历史练习记录列表（含得分趋势）
GET    /api/v1/speaking/records/:id   # 单次练习详情（含录音回放 URL）
```

---

### 3.6 写作练习 (`/api/v1/writing`)

```
GET    /api/v1/writing/input/queue    # 今日键盘输入练习题目列表
POST   /api/v1/writing/input/submit   # 提交输入答案：{ question_id, answer }
                                      # 响应：{ correct: bool, expected }

GET    /api/v1/writing/sentences/daily  # 今日造句题目列表（3~5 道）
POST   /api/v1/writing/sentences/submit # 提交造句：{ question_id, answer }
                                        # 响应（≤10s）：{ score, feedback, reference_answer }
GET    /api/v1/writing/records          # 写作历史记录
```

---

### 3.7 练习总结 (`/api/v1/summary`)

```
# 总结由服务端在会话结束时自动生成，也支持客户端主动触发
POST   /api/v1/summary                # 生成本次会话总结
                                      # 请求：{ session_id, module }
                                      # 响应（≤2s）：SessionSummary 对象
GET    /api/v1/summary/:session_id    # 获取已生成的总结（复查用）

# SessionSummary 响应结构示例：
# {
#   "session_id": "sess_abc123",
#   "module": "word",
#   "score_summary": { "reviewed": 15, "easy_rate": 0.47 },
#   "strengths": [
#     { "type": "word", "label": "日本語", "note": "连续3次评为容易" }
#   ],
#   "weaknesses": [
#     { "type": "word", "label": "覚える", "note": "本次评为困难，明日优先复习" }
#   ],
#   "improvement_suggestions": [
#     "「覚える」「忘れる」属同类动词，建议对比记忆"
#   ],
#   "generated_at": "2026-04-03T10:30:00Z"
# }
```

---

## 四、模块间依赖关系

```
                    ┌─────────────┐
                    │   user/     │  认证中间件（所有模块共用）
                    └──────┬──────┘
                           │ AuthMiddleware
          ┌────────────────┼────────────────┐
          │                │                │
     ┌────▼────┐      ┌────▼────┐     ┌────▼────┐
     │  word/  │      │grammar/ │     │ lesson/ │
     └────┬────┘      └────┬────┘     └─────────┘
          │  生词加入单词本  │  例句生词加入单词本
          └────────────────┘
                           │ 会话数据
                    ┌──────▼──────┐
                    │  summary/   │  跨模块聚合（读取各模块 session 数据）
                    └─────────────┘
                           ↑
               speaking/ writing/ 也输出会话数据

     所有模块 → data/ 层（Store 接口）→ 数据库
```

**关键约束**：
- `summary/` 只通过 `data/session_store` 读取各模块数据，**不直接 import** 其他模块包，避免循环依赖
- `word/`、`grammar/` 之间的「生词加入单词本」功能，通过共同调用 `data/word_store` 实现，不相互 import

---

## 五、数据库表草图

```sql
-- 内容库（管理员维护，只读）
words          (id, kanji_form, reading, pos, meaning, examples_json, jlpt_level)
grammar_points (id, name, meaning, conjunction_rule, usage_note, examples_json,
                quiz_questions_json, jlpt_level)
lessons        (id, title, content_furigana_json, translation_json, jlpt_level,
                tags_json, audio_url, sentence_timestamps_json)

-- 用户学习数据（读写频繁）
users          (id, email, password_hash, goal_level, streak_days, created_at)
word_records   (id, user_id, word_id, mastery_level, next_review_at,
                review_history_json, ease_factor, interval)
grammar_records(id, user_id, grammar_point_id, status, next_review_at,
                quiz_history_json)
speaking_records(id, user_id, type, material_id, similarity_score,
                 audio_ref, practiced_at)
writing_records (id, user_id, type, question, user_answer, ai_feedback_json,
                 score, practiced_at)
study_sessions  (id, user_id, module, duration_seconds, completed_count, started_at)
session_summaries(id, user_id, session_id, module, score_summary_json,
                  strengths_json, weaknesses_json, suggestions_json, generated_at)
```

> **分离原则（对应 NFR-003）**：内容库与用户学习数据虽在同一数据库实例，但通过命名约定（前缀）和访问控制逻辑严格隔离。管理员通过 CLI 工具维护内容库，用户 API 只能写入学习数据表。
