---
feature: "001-core-functionality"
title: "原子化任务列表"
status: "草稿"
version: "1.0"
created: "2026-04-03"
relates_to: "plan.md"
---

# 原子化任务列表：日语学习应用

> **规则说明**
> - **粒度**：每个任务产出且仅产出 **一个文件**
> - **TDD**：测试文件任务（`*_test.go`）必须先于对应实现文件任务
> - **`[P]`**：该任务无阻塞前置依赖，可与同阶段其他 `[P]` 任务并行执行
> - **`依赖`**：列出必须在本任务开始前完成的任务编号
> - **执行约束**：所有错误必须 `fmt.Errorf("ctx: %w", err)` 包装；`return err` 前必须 `slog.Error(...)`；禁止全局变量

---

## Phase 0：项目基础设施

> 建立可编译的 Go 模块骨架和数据库迁移脚本。无任何业务逻辑。

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T001 | `go.mod` | 初始化 Go 模块（`module japanese-learning-app`，`go 1.21`），声明唯一外部依赖 `modernc.org/sqlite v1.x` | — |
| T002 | `Makefile` | 定义 `run / test / build / lint / seed / front-build / clean` 目标；`test` 目标执行 `go test ./... -v -count=1` | T001 |
| T003 | `internal/data/migrations/001_init.sql` | 建表 DDL：`words`、`grammar_points`、`lessons`、`users`、`word_records`、`grammar_records`、`speaking_records`、`writing_records`、`study_sessions`、`session_summaries`（参见 plan.md §数据库表草图） | — |
| T004 | `internal/data/migrations/002_seed.sql` | N5/N4 初始词库种子数据（至少 50 条示例，含 `kanji_form`、`reading`、`jlpt_level` 等字段） | T003 |

---

## Phase 1：数据结构定义

> 仅含 Go struct / const / type 定义，**零业务逻辑**，**零 import 外部包**。全部可并行。

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T005 [P] | `internal/config/config.go` | `Config` struct（`ListenAddr`、`DBPath`、`JWTSecret`、`JWTExpireHours`、`LogLevel`、`AIAPIKey`、`AIAPIEndpoint`、`AITimeoutSec`、`AudioStorePath`）；`Load(path string) (*Config, error)` 函数签名（实现留空，返回 `nil, nil`） | T001 |
| T006 [P] | `internal/module/word/model.go` | `JLPTLevel`、`ReviewRating` 常量；`WordExample`、`Word`、`WordRecord`、`ReviewEvent`、`WordCard` struct（含全部 JSON tag） | T001 |
| T007 [P] | `internal/module/grammar/model.go` | `QuizType`、`GrammarStatus` 常量；`QuizQuestion`、`GrammarExample`、`GrammarPoint`、`GrammarRecord`、`QuizAttempt`、`QuizSubmission`、`QuizResult`、`QuizItemResult` struct | T001 |
| T008 [P] | `internal/module/lesson/model.go` | `FuriganaToken`、`Sentence`、`LessonSummary`、`Lesson` struct；`Lesson` 嵌入 `LessonSummary` | T001 |
| T009 [P] | `internal/module/speaking/model.go` | `PracticeType` 常量；`SentenceAnnotation`、`ScoreResult`、`SpeakingRecord` struct | T001 |
| T010 [P] | `internal/module/writing/model.go` | `WritingType` 常量；`WritingQuestion`（`ExpectedAnswer` 字段 `json:"-"`）、`AIFeedback`、`WritingRecord` struct | T001 |
| T011 [P] | `internal/module/summary/model.go` | `ModuleType` 常量；`SummaryItem`、`ScoreSummary`（`map[string]any`）、`SessionSummary` struct | T001 |
| T012 [P] | `internal/module/user/model.go` | `User`、`RegisterReq`、`LoginReq`、`TokenResp`、`UserStats`、`ModuleStat` struct；复用 `word.JLPTLevel`（或在本包重声明 `JLPTLevel` 别名） | T001 |
| T013 [P] | `internal/httputil/response.go` | `APIResponse`、`APIError` struct；`WriteJSON(w, status, v)`、`WriteError(w, status, code, msg, reqID)` 工具函数 | T001 |

---

## Phase 2：数据访问层（TDD）

> `data/` 包只做 SQL IO，**零业务逻辑**。每个 Store 先写测试（Red），再写实现（Green）。
> 测试使用 `TestMain` 创建真实 SQLite 文件，测试完成后删除；**不使用 Mock**。

### 2.1 数据库初始化

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T014 | `internal/data/db.go` | `OpenDB(path string) (*sql.DB, error)`：打开 SQLite 连接，执行 WAL 模式设置；`RunMigrations(db *sql.DB, migrationsDir string) error`：按文件名顺序执行 SQL 文件 | T001, T003 |

### 2.2 Config 测试

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T015 [P] | `internal/config/config_test.go` | 表格驱动测试：①缺失必填字段返回 error；②默认值填充正确；③所有字段正常加载 | T005 |

> **注**：`config.go` 的完整实现在 T015 之后（修改 T005 的空实现），不单独建任务，因为属于同一文件。

### 2.3 WordStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T016 | `internal/data/word_store_test.go` | 集成测试：`GetByID` 返回正确单词；`ListByLevel` 过滤正确；`ListDueRecords` 按 `next_review_at` 排序；`UpsertRecord` 幂等；`BookmarkWord` 不重复插入 | T014, T006 |
| T017 | `internal/data/word_store.go` | `WordStore` struct（持有 `*sql.DB`）；实现 `WordStore` 接口全部方法（`GetByID`、`ListByLevel`、`GetRecord`、`ListDueRecords`、`UpsertRecord`、`BookmarkWord`） | T016 |

### 2.4 GrammarStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T018 [P] | `internal/data/grammar_store_test.go` | 集成测试：`GetByID` 返回含 `QuizQuestions` 的完整语法点；`ListByLevel` 过滤；`UpsertRecord` 更新 `status` 和 `next_review_at`；`ListDueRecords` 只返回到期记录 | T014, T007 |
| T019 | `internal/data/grammar_store.go` | `GrammarStore` struct；实现 `GrammarStore` 接口全部方法 | T018 |

### 2.5 LessonStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T020 [P] | `internal/data/lesson_store_test.go` | 集成测试：`ListSummaries` 按 level+tag 过滤；`GetDetail` 返回含 `Sentences` 的完整课文；`GetSentences` 返回按 index 排序的句子列表 | T014, T008 |
| T021 | `internal/data/lesson_store.go` | `LessonStore` struct；实现 `LessonStore` 接口全部方法 | T020 |

### 2.6 UserStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T022 [P] | `internal/data/user_store_test.go` | 集成测试：`Create` 成功插入；重复 email 返回 error；`GetByEmail` 返回含 `password_hash` 的用户；`UpdateStreak` 正确更新 | T014, T012 |
| T023 | `internal/data/user_store.go` | `UserStore` struct；实现 `UserStore` 接口全部方法 | T022 |

### 2.7 SessionStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T024 [P] | `internal/data/session_store_test.go` | 集成测试：`CreateSession` 返回唯一 session_id；`GetSessionData` 正确反序列化；`SaveSummary` + `GetSummary` 往返一致 | T014, T011 |
| T025 | `internal/data/session_store.go` | `SessionStore` struct；实现 `SessionStore` 接口全部方法 | T024 |

### 2.8 SpeakingStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T026 [P] | `internal/data/speaking_store_test.go` | 集成测试：`SaveRecord` 持久化；`ListRecords` 按 `practiced_at` 倒序；`GetRecord` 返回正确记录 | T014, T009 |
| T027 | `internal/data/speaking_store.go` | `SpeakingStore` struct；实现 `SpeakingStore` 接口全部方法 | T026 |

### 2.9 WritingStore（TDD）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T028 [P] | `internal/data/writing_store_test.go` | 集成测试：`GetDailyQueue` 返回 3~5 道题；`SaveRecord` 持久化 AI 反馈；`ListRecords` 按时间倒序 | T014, T010 |
| T029 | `internal/data/writing_store.go` | `WritingStore` struct；实现 `WritingStore` 接口全部方法 | T028 |

---

## Phase 3：模块业务逻辑（TDD）

> 每个模块的 service 先写测试（依赖注入 Store 接口），再写实现。handler 在 service 完成后编写，不单独写测试（handler 逻辑在集成测试中覆盖）。

### 3.1 单词模块（word）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T030 [P] | `internal/module/word/sm2_test.go` | 表格驱动测试（≥10 组）：①首次学习 EF=2.5, interval=1；②`easy` 评分后 interval 扩大、EF 提升；③`hard` 评分后 interval 重置为1、EF 降低但不低于 1.3；④`normal` 不改变 EF | T006 |
| T031 | `internal/module/word/sm2.go` | 纯函数 `CalcNextReview(record WordRecord, rating ReviewRating) WordRecord`；无副作用，无 DB 依赖；完整实现 SM-2 算法 | T030 |
| T032 | `internal/module/word/service_test.go` | 集成测试：`ReviewQueue` 返回 `next_review_at ≤ now` 的卡片且按时间升序；`SubmitRating` 调用 SM-2 后更新 DB；`Bookmark` 幂等 | T017, T031 |
| T033 | `internal/module/word/service.go` | `WordService` struct（注入 `WordStore`）；实现 `ReviewQueue`、`SubmitRating`、`Bookmark` 方法；所有 DB 错误 `fmt.Errorf("word.ReviewQueue: %w", err)` 包装 | T032 |
| T034 | `internal/module/word/handler.go` | 注册路由：`GET /api/v1/words`、`GET /api/v1/words/:id`、`GET /api/v1/words/review/queue`、`POST /api/v1/words/review/:id`、`GET /api/v1/words/review/stats`、`POST /api/v1/words/:id/bookmark`；使用 `httputil.WriteJSON/WriteError` | T033, T013 |

### 3.2 语法模块（grammar）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T035 [P] | `internal/module/grammar/service_test.go` | 表格驱动测试（评分逻辑）：①全对 score=100；②半对 score≈50；③答案大小写/前后空格不影响正确性；集成测试：`Enqueue` 创建 GrammarRecord；`ReviewQueue` 只返回到期记录 | T019 |
| T036 | `internal/module/grammar/service.go` | `GrammarService` struct（注入 `GrammarStore`、`WordStore`）；实现 `ListByLevel`、`GetDetail`、`SubmitQuiz`（含评分计算）、`Enqueue`、`ReviewQueue`、`SubmitReview` | T035 |
| T037 | `internal/module/grammar/handler.go` | 注册路由：`GET /api/v1/grammar`、`GET /api/v1/grammar/:id`、`POST /api/v1/grammar/:id/quiz`、`POST /api/v1/grammar/:id/enqueue`、`GET /api/v1/grammar/review/queue`、`POST /api/v1/grammar/review/:id` | T036, T013 |

### 3.3 课文模块（lesson）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T038 [P] | `internal/module/lesson/service_test.go` | 集成测试：`ListByLevel` 过滤正确；`GetDetail` 含完整 Sentences；`GetSentences` 顺序正确；`BookmarkWords` 调用 `WordStore.BookmarkWord` 逐条执行 | T021, T017 |
| T039 | `internal/module/lesson/service.go` | `LessonService` struct（注入 `LessonStore`、`WordStore`）；实现 `ListByLevel`、`GetDetail`、`GetSentences`、`BookmarkWords` | T038 |
| T040 | `internal/module/lesson/handler.go` | 注册路由：`GET /api/v1/lessons`、`GET /api/v1/lessons/:id`、`GET /api/v1/lessons/:id/sentences`、`POST /api/v1/lessons/:id/words/bookmark` | T039, T013 |

### 3.4 口语模块（speaking）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T041 [P] | `internal/module/speaking/scorer_test.go` | 表格驱动测试（≥6 组）：①相同音频 score=100；②完全不同音频 score<30；③部分相似 30≤score≤80；④空音频返回 error；⑤极短音频（<0.1s）返回 error | T009 |
| T042 | `internal/module/speaking/scorer.go` | `AudioScorer` 接口；`WaveformScorer` struct 实现：基于音频波形特征（RMS 能量序列、过零率）计算余弦相似度；`Score(ref, user []byte) (ScoreResult, error)` | T041 |
| T043 | `internal/module/speaking/service_test.go` | 集成测试：`SubmitRecording` 调用 scorer 并持久化 record；`GetHistory` 按时间倒序；stub scorer 返回固定 ScoreResult | T027, T042 |
| T044 | `internal/module/speaking/service.go` | `SpeakingService` struct（注入 `SpeakingStore`、`AudioScorer`、`audioStorePath string`）；实现 `GetMaterials`、`SubmitRecording`（含音频文件写入磁盘）、`GetHistory`、`GetRecord` | T043 |
| T045 | `internal/module/speaking/handler.go` | 注册路由：`GET /api/v1/speaking/materials`、`GET /api/v1/speaking/materials/:id`、`POST /api/v1/speaking/records`（解析 `multipart/form-data`）、`GET /api/v1/speaking/records`、`GET /api/v1/speaking/records/:id` | T044, T013 |

### 3.5 写作模块（writing）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T046 [P] | `internal/module/writing/ai_client.go` | `AIReviewer` 接口（`Review(question, answer string) (AIFeedback, error)`）；`ClaudeClient` struct 实现（HTTP POST 到 `AIAPIEndpoint`，超时 `AITimeoutSec`）；`StubReviewer` struct（测试用 stub，直接返回预设 AIFeedback） | T010 |
| T047 | `internal/module/writing/service_test.go` | 使用 `StubReviewer`：`SubmitInput` 对/错判断正确；`SubmitSentence` 调用 reviewer 并持久化；`DailyQueue` 返回 3~5 道题；表格驱动测试输入判题逻辑（大小写、假名规范化） | T029, T046 |
| T048 | `internal/module/writing/service.go` | `WritingService` struct（注入 `WritingStore`、`AIReviewer`）；实现 `DailyQueue`、`SubmitInput`（布尔判题）、`SubmitSentence`（调用 AI 批改） | T047 |
| T049 | `internal/module/writing/handler.go` | 注册路由：`GET /api/v1/writing/input/queue`、`POST /api/v1/writing/input/submit`、`GET /api/v1/writing/sentences/daily`、`POST /api/v1/writing/sentences/submit`、`GET /api/v1/writing/records` | T048, T013 |

### 3.6 总结模块（summary）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T050 | `internal/module/summary/service_test.go` | 表格驱动测试（≥4 组）：①单词模块会话含 3 个 easy、2 个 hard → strengths/weaknesses 正确；②语法模块 score=100 → 无 weaknesses；③空会话 → 返回空总结不报错；④模块类型未知 → error | T025 |
| T051 | `internal/module/summary/service.go` | `SummaryService` struct（注入 `SessionStore`，**不 import 其他模块包**）；实现 `Generate`（从 session 数据计算 strengths/weaknesses/suggestions）、`GetBySession` | T050 |
| T052 | `internal/module/summary/handler.go` | 注册路由：`POST /api/v1/summary`、`GET /api/v1/summary/:session_id` | T051, T013 |

### 3.7 用户认证模块（user）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T053 [P] | `internal/module/user/jwt_test.go` | 表格驱动测试：①`SignToken` 产出可被 `VerifyToken` 验证的 token；②过期 token 返回 error；③篡改 payload 后验证失败；④不同 secret 验证失败 | T012 |
| T054 | `internal/module/user/jwt.go` | `SignToken(userID int64, secret string, expireHours int) (string, error)`；`VerifyToken(token, secret string) (int64, error)` 返回 `userID`；手工实现 HMAC-SHA256 签名（`crypto/hmac` + `encoding/base64`），**不引入第三方 JWT 库** | T053 |
| T055 | `internal/module/user/service_test.go` | 集成测试：`Register` 成功创建用户、密码不持久化明文；重复 email 返回 error；`Login` 正确密码返回 token、错误密码返回 error；`GetStats` 返回各模块统计 | T023, T054 |
| T056 | `internal/module/user/service.go` | `UserService` struct（注入 `UserStore`、`jwtSecret string`、`jwtExpireHours int`）；实现 `Register`（`bcrypt` 或 `crypto/sha256` 哈希密码）、`Login`、`GetMe`、`GetStats` | T055 |
| T057 | `internal/module/user/middleware.go` | `AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler`：从 `Authorization: Bearer <token>` 提取并验证 JWT，注入 `userID` 到 `context.Context`；验证失败返回 `401` JSON 错误 | T054, T013 |
| T058 | `internal/module/user/handler.go` | 注册路由：`POST /api/v1/auth/register`、`POST /api/v1/auth/login`、`GET /api/v1/users/me`（需认证）、`GET /api/v1/users/me/stats`（需认证） | T056, T057, T013 |

---

## Phase 4：CLI 工具 & 前端 & 入口集成

> 所有后端业务逻辑已完成，本阶段完成程序入口组装和前端资源。

### 4.1 CLI 管理工具

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T059 | `internal/cli/import_words_test.go` | 测试：从合法 JSON 文件导入 N 条单词后 DB 中存在 N 条；格式错误文件返回 error；重复导入（含相同 id）不重复插入 | T017 |
| T060 | `internal/cli/import_words.go` | `ImportWords(db *sql.DB, filePath string) (int, error)`：读取 JSON 数组，批量插入 `words` 表（`INSERT OR IGNORE`） | T059 |
| T061 | `internal/cli/root.go` | `flag` 包解析子命令：`import-words --file <path>`；调用 `ImportWords`；打印导入数量 | T060 |

### 4.2 HTML 模板（服务端骨架）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T062 [P] | `front/web/templates/base.html` | 公共布局：顶部导航（5 个模块链接）+ `{{block "content" .}}` 插槽 + 底部状态栏；引入 `main.css`；纯 HTML/CSS，无 JS 框架 | — |
| T063 [P] | `front/web/templates/word/index.html` | 单词复习主页骨架：卡片容器 div、评分按钮组、进度条占位；引入 `word.js` | T062 |
| T064 [P] | `front/web/templates/word/stats.html` | 学习统计骨架：连续天数、掌握程度分布图占位 | T062 |
| T065 [P] | `front/web/templates/grammar/index.html` | 语法点列表骨架：按 JLPT 级别分组的列表容器 | T062 |
| T066 [P] | `front/web/templates/grammar/detail.html` | 语法点详情骨架：讲解区、例句区、检验题区；引入 `grammar.js` | T062 |
| T067 [P] | `front/web/templates/lesson/index.html` | 课文列表骨架：筛选器 + 课文卡片列表容器 | T062 |
| T068 [P] | `front/web/templates/lesson/detail.html` | 课文阅读骨架：振り仮名渲染区（`<ruby>` 标签）、音频播放控制、翻译切换；引入 `lesson.js` | T062 |
| T069 [P] | `front/web/templates/speaking/index.html` | 口语练习骨架：材料列表、录音控制区（录音按钮、波形可视化占位）、评分结果区；引入 `speaking.js` | T062 |
| T070 [P] | `front/web/templates/writing/index.html` | 写作练习骨架：输入练习区 + 造句练习区 + AI 反馈展示区；引入 `writing.js` | T062 |
| T071 [P] | `front/web/templates/summary/index.html` | 练习总结骨架：得分概要卡片、亮点列表、待改进列表、建议文本；引入 `summary.js` | T062 |

### 4.3 TypeScript 前端脚本

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T072 [P] | `front/web/static/js/api.ts` | 统一 Fetch 封装：`apiFetch<T>(method, path, body?) → Promise<T>`；自动注入 `Authorization: Bearer` header（从 `localStorage` 读取 token）；统一处理 4xx/5xx 错误，抛出含 `code` 字段的 `APIError` | — |
| T073 | `front/web/static/js/word.ts` | 单词卡片翻转动画（CSS class toggle）；调用 `apiFetch` 获取复习队列；提交评分（easy/normal/hard）并自动加载下一张；今日完成时显示总结入口 | T072 |
| T074 | `front/web/static/js/grammar.ts` | 语法点检验题交互：选择/填空答案收集；提交后展示逐题结果和解析；答错题目高亮 | T072 |
| T075 | `front/web/static/js/lesson.ts` | 音频播放同步高亮（`timeupdate` 事件对比 sentence 时间戳）；点击单词触发释义弹窗（Fetch `/api/v1/words/:id`）；翻译显示切换 | T072 |
| T076 | `front/web/static/js/speaking.ts` | `MediaRecorder` 录音启动/停止；录音数据转 `Blob` 后 `multipart/form-data` 上传；接收评分结果并渲染句子级标注；录音回放 | T072 |
| T077 | `front/web/static/js/writing.ts` | 输入练习：即时判题，正确绿色/错误红色高亮；造句练习：提交后轮询或直接等待 AI 反馈（≤10s），展示批改结果 | T072 |
| T078 | `front/web/static/js/summary.ts` | 总结页渲染：从 URL 参数读取 `session_id`，Fetch `/api/v1/summary/:session_id`，动态填充得分、亮点、建议 | T072 |

### 4.4 CSS 样式

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T079 [P] | `front/web/static/css/main.css` | 极简样式：CSS 变量定义（颜色、间距、字体）；系统字体栈（含日文字体回退：`"Noto Sans JP", "Hiragino Sans", sans-serif`）；卡片组件、按钮组件、进度条基础样式；响应式布局（移动优先，单列 → 双列） | — |

### 4.5 程序入口（组装所有依赖）

| 编号 | 文件 | 职责说明 | 依赖 |
|---|---|---|---|
| T080 | `backend/cmd/server/main.go` | **程序唯一入口**：①`config.Load()` 读取配置；②`data.OpenDB` + `data.RunMigrations` 初始化 DB；③构造所有 Store（`WordStore`、`GrammarStore`、...）；④构造所有 Service（注入 Store + 配置参数）；⑤构造所有 Handler，调用各自 `RegisterRoutes(mux)`；⑥注册静态文件路由（`/static/`）和模板路由；⑦`http.ListenAndServe(cfg.ListenAddr, mux)`；所有错误导致 `log.Fatal` | T034, T037, T040, T045, T049, T052, T058, T061 |

---

## 任务依赖关系总览

```
Phase 0: T001 → T002, T003 → T004
         T001 → T003

Phase 1: T001 → T005~T013 [全部 P，可并行]

Phase 2: T001,T003 → T014
         T014,T006 → T016 → T017
         T014,T007 → T018 → T019
         T014,T008 → T020 → T021
         T014,T012 → T022 → T023
         T014,T011 → T024 → T025
         T014,T009 → T026 → T027
         T014,T010 → T028 → T029
         T005 → T015 [P with T016~T028]

Phase 3:
  word:    T006 → T030[P] → T031; T017,T031 → T032 → T033; T033,T013 → T034
  grammar: T019 → T035[P] → T036; T036,T013 → T037
  lesson:  T021,T017 → T038[P] → T039; T039,T013 → T040
  speaking:T009 → T041[P] → T042; T027,T042 → T043 → T044; T044,T013 → T045
  writing: T010 → T046[P]; T029,T046 → T047 → T048; T048,T013 → T049
  summary: T025 → T050 → T051; T051,T013 → T052
  user:    T012 → T053[P] → T054; T023,T054 → T055 → T056; T054,T013 → T057; T056,T057,T013 → T058

Phase 4:
  CLI:     T017 → T059 → T060 → T061
  template:T062[P] → T063~T071[P]
  TS:      T072[P] → T073~T078
  CSS:     T079[P]
  main:    T034,T037,T040,T045,T049,T052,T058,T061 → T080
```

---

## 统计

| 阶段 | 任务数 | 可并行任务数 |
|---|---|---|
| Phase 0 | 4 | 0 |
| Phase 1 | 9 | 9 |
| Phase 2 | 16 | 7（T016/T018/T020/T022/T024/T026/T028 与 T015 互相独立）|
| Phase 3 | 30 | 7（T030/T035/T038/T041/T046/T053 + T050 可与3.1~3.5同期推进）|
| Phase 4 | 21 | 12（模板×10 + CSS + TS基础层 可并行）|
| **合计** | **80** | — |
