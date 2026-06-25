# 影子跟读重设计方案（方案 1：Lesson-Centered）

> **Status:** Draft design document for rebuilding shadowing around `lesson` content.

**目标：** 参考 [docs/shadowing.md](../../shadowing.md) 的交互形态，在当前项目已有的数据结构和前后端边界上，重新设计一套可持续演进的影子跟读能力。

**核心决策：** 影子跟读的内容主数据归 `lesson`，练习态和进度态归新的 `shadowing` 子能力；前端最终路由层级为 `/lesson`、`/lesson/:id`、`/lesson/:id/shadowing`，后端新增 `/api/v1/lessons/{id}/shadowing*` 接口；现有 `speaking` 模块继续承接自由口语和轻量朗读，不升级为视频化影子跟读主系统。

**上线策略：** 第一阶段优先做 `audio-first, video-ready, SQLite-first`。也就是先基于课文现有句子切分、时间戳和音频跑通逐句跟读；视频字段、管理台和自动评分按阶段递进，不把第一版做成大而全工程，同时明确 PostgreSQL 适配不是首批阻塞项。

---

## 1. 当前项目现状

当前仓库里已经有两套与“跟读”相关的数据来源，但成熟度差异很大：

- `lesson` 已经具备句子级结构化内容：`tokens`、中文释义、`start_ms`、`end_ms`。
- `speaking_materials` 仍然是扁平文本模型，现有页面主要是 TTS + 录音 + 自评。
- `docs/shadowing.md` 目标的是“视频/音频 + 字幕时间轴 + 当前句练习卡”的重交互学习页。
- `front/react/src/App.tsx` 当前只有 `/lesson` 路由，课文详情由 `LessonPage.tsx` 内部状态切换，不支持深链接详情页。
- `front/react/src/pages/lesson/LessonPage.tsx` 目前只是课文阅读页，还没有播放器、同步字幕、进度恢复和逐句练习状态。
- `front/react/src/pages/speaking/SpeakingPage.tsx` 没有句子时间轴，也没有和课文统一的数据边界。
- 当前 Lesson API 契约已经存在前后端错位：Go 侧是 `char_count`、`chinese`、`start_ms`、`end_ms`，前端 TypeScript 仍在使用 `sentence_count`、`translation`。
- SQLite 里虽然有 `speaking_materials.lines`，但当前运行时模型、store、admin 都没有真正使用这列。
- Lesson 目前没有管理台编辑能力，课文主要靠种子 JSON 和导入脚本维护。
- `internal/cli/import_lessons.go` 当前只支持 `title/jlpt_level/tags/audio_url/word_ids/sentences`，并且使用 `INSERT OR IGNORE`；SQLite `lessons` 表本身也还没有 `(title, jlpt_level)` 唯一约束，因此“重新导入覆盖 shadowing 元数据”在现状下并不成立。
- PostgreSQL 的 lesson schema 目前使用 `audio_object_id`，而不是 SQLite 风格的 `audio_url` 文本列；现有 PG lesson store 也还没有把对象存储映射成应用层 `audio_url`。
- 现有 `lessons_n5.json` 已有句子时间戳，但 `audio_url` 普遍为空，说明影子跟读第一步不是“改页面”，而是“选一批真正有媒体资源的 lesson 做试点”。

换句话说，项目已经有“适合承载影子跟读的内容骨架”，但还没有“可训练的影子跟读产品层”。

---

## 2. 为什么选择方案 1

本次采用“以课文为中心”的方案，而不是继续扩张 `speaking`，原因有四个：

1. `lesson` 已经有句子级时间轴。
   影子跟读最核心的是“当前播放到了哪一句”。这个前提在 `lesson.sentences[].start_ms/end_ms` 里已经存在，而 `speaking_materials` 目前没有可靠的句子级时间模型。

2. `lesson` 已经有更适合学习页的文本结构。
   课文句子天然带 `tokens`、振假名和中文释义，和 [docs/shadowing.md](../../shadowing.md) 里的“当前句卡片”高度匹配。

3. 如果继续往 `speaking` 塞视频化能力，会出现内容双写。
   同一份课文如果既要在 `lesson` 维护，又要在 `speaking_materials` 再存一份时间轴、字幕、译文和媒体链接，后续一定会漂移。

4. `speaking` 的职责应该保持轻量。
   `speaking` 更适合自由表达、朗读打卡、自评和未来开放式口语练习；影子跟读属于“围绕课文内容的精细化训练”，不应把两个域混成一个。

因此，本方案的边界是：

- `lesson` 负责“学什么”。
- `shadowing` 负责“怎么练”。
- `speaking` 负责“泛口语练习”。

---

## 3. 产品定位

### 3.1 功能目标

新影子跟读页要解决四个问题：

1. 用户始终知道当前媒体播放到哪一句。
2. 用户可以围绕当前句做重播、慢速、循环、录音、回放和自评。
3. 用户离开后可以从上次句子和时间继续。
4. 一份课文只维护一份字幕和切句真相。

### 3.2 非目标

第一版不做下面这些事情：

- 不替换现有 `/speaking` 页面。
- 不在第一版接入服务端自动评分。
- 不在第一版持久化保存每一条用户录音文件。
- 不在第一版建设完整 lesson 管理后台。
- 不要求所有 lesson 立刻支持 shadowing，只对 `shadowing_enabled=true` 的课文开放。

---

## 4. 页面与路由设计

### 4.1 前端路由

最终推荐路由层级：

```text
/lesson
/lesson/:id
/lesson/:id/shadowing
```

而不是新开一个完全独立的顶层 `/shadowing/:id`，原因是：

- 用户认知上“影子跟读”属于课文的一种学习方式。
- 这样可以复用当前 `lesson` 列表和详情入口，不必新增全局导航域。
- 回退路径更清晰：`影子跟读页 -> 课文详情 -> 课文列表`。

这里要明确一个前置改造：当前 `LessonPage` 通过内部 state 切换详情，不适合作为 shadowing 的承载入口。原因不是命名，而是行为模型不兼容：

- 刷新 `/lesson/:id/shadowing` 时需要能独立恢复 lesson 详情和 shadowing 数据。
- 分享链接需要可直接打开详情页或跟读页。
- 浏览器返回应该从 shadowing 回到 `/lesson/:id`，而不是被迫跳回 `/lesson` 列表。

因此，`/lesson/:id` 的详情路由化是 shadowing 上线前的明确前置条件。若这一前置条件还没落地，不建议在当前内部 state 详情模型上直接叠加 shadowing。

### 4.2 入口策略

推荐入口：

- 课文列表卡片：点击进入 `/lesson/:id`。
- 课文详情页：在标题区增加“开始影子跟读” CTA，跳转 `/lesson/:id/shadowing`。
- 首页后续可加一个“继续上次跟读”的快捷卡片。

返回逻辑要求：

- 从 `/lesson/:id/shadowing` 返回，应落回 `/lesson/:id`。
- 从 `/lesson/:id` 返回，应落回 `/lesson`。
- 详情页和跟读页都必须支持刷新后独立恢复，不依赖前一页 state。

不建议第一版新增独立底部 Tab。当前阶段它仍是 `lesson` 的子能力，不是一个独立模块。

### 4.3 页面结构

页面结构沿用 [docs/shadowing.md](../../shadowing.md) 的训练思路，但按当前项目能力做收敛：

桌面端：

1. 顶部栏：返回、课文标题、完成度、字幕设置、继续学习。
2. 左侧媒体区：Phase 1 只渲染音频播放器；即使 lesson 已有 `video_url` 预留字段，前端也先忽略，不实现视频分支。
3. 右侧当前句卡：日文、振假名、中文、重播、慢速、循环、录音、播放录音、自评。
4. 下方字幕时间轴：按句渲染，可点击跳转，可高亮当前句。

移动端：

1. 顶部保留标题和进度。
2. 媒体区在上，当前句卡紧随其后。
3. 字幕时间轴在下。
4. 固定底部练习栏保留“上句 / 重播 / 录音 / 下句”。

---

## 5. 域模型与边界

### 5.1 设计原则

为了避免把 `lesson` handler 做成巨石接口，本方案采用“内容归 lesson，练习态归 shadowing 子接口”的结构：

- `lesson` 继续负责课文列表和课文详情。
- `shadowing` 负责播放器态、逐句训练态、进度恢复和练习记录。
- `shadowing` 不复制课文文本，只引用 `lesson_id + sentence_index`。

### 5.2 推荐的模块边界

后端建议新增：

```text
internal/module/shadowing/
```

它不拥有独立课文内容表，而是组合：

- `lesson.Store` 读取课文和句子。
- `shadowing.Store` 读写用户进度和句子练习记录。

这样做的好处是：

- 不会污染现有 `lesson` 的简单阅读接口。
- 后续接入自动评分、录音上传、难句复习时，不需要把复杂逻辑塞进 `lesson` 服务。
- 仍然保持“内容主数据只有一份”的原则。

### 5.3 句子标识策略

对外 API 第一版统一使用：

```text
lesson_id + sentence_index
```

原因是当前 SQLite lesson 句子来自 JSON，天然有 `index`；PostgreSQL 虽然已有 `lesson_sentences.id`，但现阶段没必要把前端 API 绑死到双存储实现差异上。

为了处理未来“重切句导致历史漂移”的问题，建议新增：

```text
shadowing_version
```

当 lesson 的时间轴或切句发生破坏性变更时，版本号递增；新旧版本的进度和练习记录不混算。

---

## 6. 数据模型设计

### 6.1 Lesson 扩展字段

建议在 lesson 维度新增以下字段：

- `shadowing_enabled`
  是否开放影子跟读入口。
- `video_url`
  可选；若为空则使用 `audio_url` 进入音频模式。
- `shadowing_version`
  影子跟读内容版本号，默认 `1`。
- `shadowing_config_json`
  影子跟读配置，保留后续可扩展空间。

推荐的 `shadowing_config_json` 结构：

```json
{
  "media_type": "audio",
  "poster_url": "",
  "media_duration_ms": 0,
  "default_playback_rate": 1.0,
  "allow_recording": true,
  "allow_loop": true,
  "subtitle_layout": "ja-kana-zh"
}
```

这里不建议第一版就引入一组分散的小字段，因为播放器行为和展示设置未来很可能继续扩展，配置型字段更稳。

### 6.2 进度表

新增逻辑表：

```text
lesson_shadowing_progress
```

建议字段：

- `id`
- `user_id`
- `lesson_id`
- `shadowing_version`
- `last_sentence_index`
- `last_position_ms`
- `last_practice_mode`
- `updated_at`

唯一约束：

```text
UNIQUE (user_id, lesson_id, shadowing_version)
```

用途：

- 恢复上次学习位置
- 首页“继续跟读”快捷入口

这张表只承载“可恢复的会话快照”，不承载句子完成状态真相。句子是否完成、完成了哪些句子，统一从 `lesson_shadowing_attempts` 派生。

### 6.3 练习记录表

新增逻辑表：

```text
lesson_shadowing_attempts
```

建议字段：

- `id`
- `user_id`
- `lesson_id`
- `shadowing_version`
- `sentence_index`
- `practice_mode`
  `normal | slow | loop | record`
- `playback_rate`
- `loop_count`
- `self_score`
  可为空；只有用户显式自评时才写入，范围沿用 `0..100`
- `recognition_text`
  预留给未来 ASR
- `audio_ref`
  第一版允许为空；若未来持久化录音，可存对象地址或对象 ID
- `created_at`

这张表保持 append-only，不做覆盖更新。它既能支持“最近练过哪些句子”，也能支撑后续自动评分和复盘分析。

第一阶段明确规定：

- `lesson_shadowing_attempts` 是“句子完成状态”的唯一真相源。
- `lesson_shadowing_progress` 只保存恢复播放所需的会话快照。
- `GET /shadowing` 返回给前端的 `completed_sentence_indexes`、`completed_sentence_count` 都由 attempts 聚合得出。
- 完成统计按 `DISTINCT sentence_index` 计算，且只统计当前 `shadowing_version` 下的有效 attempt。

这样即使 progress 节流写失败，只要 attempt 已成功落库，用户的已练句子状态仍然一致。

推荐索引：

- `UNIQUE (user_id, lesson_id, shadowing_version)` on `lesson_shadowing_progress`
- `INDEX (user_id, lesson_id, shadowing_version, sentence_index, created_at DESC)` on `lesson_shadowing_attempts`
- `INDEX (user_id, lesson_id, shadowing_version, created_at DESC)` on `lesson_shadowing_attempts`

### 6.4 为什么不复用 `speaking_records`

不建议把影子跟读记录塞进现有 `speaking_records`：

- `speaking_records` 目前是“素材级”记录，不是“句子级”记录。
- 它的 `material_id` 对应 `speaking_materials`，与 lesson 内容域不一致。
- 影子跟读需要携带 `sentence_index`、`playback_rate`、`loop_count`、`shadowing_version`，强行复用会让 speaking 语义变脏。

### 6.5 当前仓库的存储落点建议

为了减少实现时的二次分歧，第一阶段要明确区分“应用层响应字段”和“底层存储实现”：

- SQLite：
  在 `lessons` 表新增 `shadowing_enabled INTEGER NOT NULL DEFAULT 0`、`video_url TEXT NOT NULL DEFAULT ''`、`shadowing_version INTEGER NOT NULL DEFAULT 1`、`shadowing_config_json TEXT NOT NULL DEFAULT '{}'`，并新增 `lesson_shadowing_progress`、`lesson_shadowing_attempts` 两张表。
- PostgreSQL：
  在 schema 文件中为 `lessons` 新增 `shadowing_enabled BOOLEAN NOT NULL DEFAULT FALSE`、`shadowing_version INTEGER NOT NULL DEFAULT 1`、`shadowing_config_json JSONB NOT NULL DEFAULT '{}'::jsonb`，并补齐同名 progress/attempts 表定义；音频仍复用现有 `audio_object_id`，不在 Phase 0/1 引入新的 shadowing PG runtime store。

推荐约束：

- `lessons.shadowing_version >= 1`
- `lesson_shadowing_progress.shadowing_version >= 1`
- `lesson_shadowing_progress.last_sentence_index >= 0`
- `lesson_shadowing_progress.last_position_ms >= 0`
- `lesson_shadowing_attempts.shadowing_version >= 1`
- `lesson_shadowing_attempts.sentence_index >= 0`
- `lesson_shadowing_attempts.loop_count >= 0`
- `lesson_shadowing_attempts.playback_rate > 0`
- `lesson_shadowing_attempts.self_score IS NULL OR (self_score BETWEEN 0 AND 100)`

SQLite 和 PostgreSQL 都应尽量在 schema 层表达这些约束；若 SQLite `ALTER TABLE ADD COLUMN` 的兼容性限制某一条约束难以下推，也至少要在 service 层做同等校验。

`practice_mode` 的 Phase 限制不建议写成过窄的 schema CHECK。schema 层可以接受完整枚举 `normal | slow | loop | record`，而“Phase 1 只允许 `loop` / `record` 持久化”为 service 层校验，避免后续阶段扩展 mode 时被迁移反向卡住。

第一阶段不要求 PostgreSQL 立即支持视频。原因是现有 PG schema 只有 `audio_objects`，还没有成型的视频媒体模型。Phase 0/1 应明确为 `SQLite-first`：

- SQLite 先跑通 `audio_url + 逐句跟读`；`video_url` 只作为预留字段，不进入 Phase 1 播放器实现。
- PostgreSQL 只维护 schema 兼容，避免 `sqlite` / `postgres` 迁移定义继续分叉。
- PostgreSQL 在 Phase 0/1 不实现 shadowing runtime store、不接 service/router、不做 `audio_object_id -> audio_url` 的 shadowing 运行时映射。
- 若后续 PG 需要视频支持，优先设计 `video_object_id` 或统一媒体引用层，而不是镜像 SQLite 的 `video_url` 文本字段。
- 在 PG 运行时，Phase 0/1 只要求 migration 通过、代码可编译、类型/接口签名兼容；不注册 shadowing 路由，不暴露前端入口。

### 6.6 Lesson 导入与 Upsert 策略

影子跟读不能继续依赖当前 `internal/cli/import_lessons.go` 的 `INSERT OR IGNORE`。

Phase 0 需要明确做三件事：

1. 扩展 lesson import JSON 结构。
   至少支持 `shadowing_enabled`、`shadowing_version`、`shadowing_config`，并允许 `video_url` 作为可空预留字段导入；SQLite 试点不依赖 `video_url`。
2. 让 lesson 导入变成可更新。
   当前 importer 只插入不更新，重新导入不会覆盖 shadowing 元数据，也不会刷新课文内容。
3. 分两步给 SQLite lesson 导入建立稳定冲突键。
   当前 `lessons` 表没有 `(title, jlpt_level)` 唯一约束，`INSERT OR IGNORE` 实际上不能提供可靠幂等。Phase 0 不能把“建唯一索引”作为默认自动迁移步骤：先实现重复报表和 cleanup CLI；确认目标数据库已无重复后，再执行单独的唯一索引迁移或管理命令，并把 importer 切到基于该冲突键的 upsert。

建议迁移顺序：

1. 迁移前先备份 `lessons` 表，或至少导出重复候选清单。
2. 自动迁移阶段只负责扫描 `(title, jlpt_level)` 重复并输出报表/日志。
3. 若存在重复，自动迁移不得静默删数据，也不应直接建立唯一索引。
4. 真正的去重操作通过单独 CLI 命令手动执行。
5. 重复数据清理完成后，再单独执行“建立唯一索引/唯一约束”的迁移或管理命令。
6. 最后将 importer 改为基于该冲突键的 upsert。

建议重复清单至少打印：

- `title`
- `jlpt_level`
- `duplicate_ids`
- `kept_id`

删除动作必须在操作者已看到重复清单后执行，避免静默删数据。

单独 CLI 去重时，默认保留最小 `id` 的 lesson 作为 canonical 行。当前 SQLite schema 里没有其他持久化子表通过外键引用 `lessons`，因此在 Phase 0 做这一步的成本是可控的。若执行清理时仓库状态已经引入了新的 `lesson_id` 子表，则必须先把这些子表的 `lesson_id` 重映射到保留行，再删除重复 lesson。

对试点阶段，推荐 upsert 行为是“整条 lesson 内容覆盖更新”，即当冲突键命中时，统一更新：

- `content_furigana_json`
- `translation_json`
- `tags_json`
- `audio_url`
- `video_url`（SQLite）
- `sentence_timestamps_json`
- `word_ids_json`
- `char_count`
- `shadowing_enabled`
- `shadowing_version`
- `shadowing_config_json`

如果后续需要支持“改标题但保持同一导入身份”，再单独引入 `source_key`；第一阶段先不把这个问题和 shadowing 首页版本耦合在一起。

---

## 7. API 设计

### 7.0 Lesson API / TypeScript 契约对齐前置项

影子跟读依赖 lesson 句子的时间戳和中文释义，因此必须先修正当前 Lesson API 的前后端契约错位。

当前已知错位包括：

- 后端 `LessonSummary` 返回 `char_count`，前端类型仍写成 `sentence_count`
- 后端 `Sentence` 返回 `chinese`，前端类型仍写成 `translation`
- 后端 `Sentence` 已有 `start_ms/end_ms`，前端类型缺失

Phase 0 需要先统一成一套正式契约，再让 lesson 阅读页和 shadowing 共用。推荐以当前后端命名为准：

- `LessonSummary.char_count`
- `Lesson.audio_url`
- `Sentence.chinese`
- `Sentence.start_ms`
- `Sentence.end_ms`

如果短期内需要兼容旧前端，可以在服务端临时补别名字段，但 canonical contract 应只保留一套，避免 shadowing 页面再踩第二遍类型坑。

### 7.1 鉴权与用户身份来源

所有 shadowing 接口都必须要求登录。

具体要求：

- handler 运行在现有认证中间件之后。
- 用户身份从 `user.UserIDFromContext(r.Context())` 获取。
- `user_id` 绝不从请求体、query 或 path 读取。
- 若 context 中取不到合法用户，直接返回 `401 ERR_UNAUTHORIZED`。

这条规则适用于：

- `GET /api/v1/lessons/{id}/shadowing`
- `POST /api/v1/lessons/{id}/shadowing/progress`
- `POST /api/v1/lessons/{id}/shadowing/attempts`
- 后续任何 shadowing history / analytics 接口

### 7.2 标准错误结构与影子跟读错误码

当前项目的标准错误结构只有：

```json
{
  "code": "ERR_XXX",
  "message": "human readable message",
  "request_id": ""
}
```

为了支持 `ERR_SHADOWING_VERSION_STALE` 返回当前版本号，本方案明确扩展共享错误结构为“可选 `details` 字段”：

```json
{
  "code": "ERR_XXX",
  "message": "human readable message",
  "request_id": "",
  "details": {}
}
```

落地要求：

- 后端 `httputil.APIError` 增加可选 `details`
- `front/react/src/types/api.ts` 的 `APIError` 类型增加可选 `details`
- `front/react/src/api/client.ts` 的 `APIError` 类和解析逻辑增加可选 `details`
- 未使用 `details` 的现有接口保持兼容，不需要逐个改造

shadowing 接口错误契约建议如下：

- `401 ERR_UNAUTHORIZED`
  未登录或 context 中无合法用户
- `404 ERR_NOT_FOUND`
  lesson 不存在
- `403 ERR_SHADOWING_DISABLED`
  lesson 存在但未开启 shadowing
- `422 ERR_SHADOWING_MEDIA_MISSING`
  lesson 已开启 shadowing，但缺少可播放媒体
- `422 ERR_SHADOWING_CONTENT_INVALID`
  lesson 句子时间轴或字幕数据不合法
- `409 ERR_SHADOWING_VERSION_STALE`
  写接口请求版本落后于服务端当前版本
- `422 ERR_SHADOWING_ATTEMPT_MODE_INVALID`
  Phase 1 中向 `POST /attempts` 提交了不允许持久化的 mode

### 7.3 获取影子跟读详情

```http
GET /api/v1/lessons/{id}/shadowing
```

响应建议：

```json
{
  "data": {
    "lesson": {
      "id": 12,
      "title": "第3课：コンビニで買い物",
      "jlpt_level": "N5",
      "audio_url": "/audio/lessons/12.wav",
      "video_url": "",
      "shadowing_enabled": true,
      "shadowing_version": 1
    },
    "config": {
      "media_type": "audio",
      "default_playback_rate": 1.0,
      "allow_recording": true
    },
    "progress": {
      "last_sentence_index": 3,
      "last_position_ms": 18200,
      "updated_at": "2026-06-18T10:00:00Z"
    },
    "completion": {
      "completed_sentence_indexes": [0, 1, 2, 3],
      "completed_sentence_count": 4
    },
    "sentences": [
      {
        "index": 0,
        "start_ms": 0,
        "end_ms": 2400,
        "tokens": [
          { "surface": "すみません", "reading": "" }
        ],
        "chinese": "不好意思",
        "attempt_summary": {
          "attempt_count": 2,
          "best_score": 80,
          "last_score": 60
        }
      }
    ]
  }
}
```

说明：

- `sentences` 继续直接复用 lesson 句子结构。
- `attempt_summary` 由后端按 lesson + sentence 聚合，不要求前端自己扫全量历史。
- `completion` 是读模型，由 attempts 聚合生成，不要求前端单独维护第二份完成状态。
- `attempt_summary.attempt_count` 统计当前 `shadowing_version` 下该句子的所有有效 attempt。
- `attempt_summary.best_score`、`attempt_summary.last_score` 只看 `self_score IS NOT NULL` 的 attempt；若该句从未出现自评，则返回 `null`。

### 7.4 版本一致性校验

所有写接口都必须校验 `shadowing_version`，客户端提交版本号不等于服务端当前 lesson 版本时，不能静默覆盖。

服务端处理规则：

1. 先读取 lesson 当前 `shadowing_version`
2. 比较请求里的 `shadowing_version`
3. 若不一致，返回 `409 ERR_SHADOWING_VERSION_STALE`
4. 响应里携带当前版本号
5. 前端收到后必须重新拉取 `GET /shadowing`，不能继续写旧版本进度或 attempt

建议错误响应：

```json
{
  "code": "ERR_SHADOWING_VERSION_STALE",
  "message": "shadowing content version is stale",
  "request_id": "",
  "details": {
    "current_shadowing_version": 2
  }
}
```

### 7.5 保存进度

```http
POST /api/v1/lessons/{id}/shadowing/progress
```

请求：

```json
{
  "shadowing_version": 1,
  "last_sentence_index": 3,
  "last_position_ms": 18200,
  "last_practice_mode": "loop"
}
```

触发时机：

- 用户暂停离开
- 切换句子
- 页面退出
- 练习完成

这个接口只更新“恢复播放所需的会话快照”，不负责写句子完成状态。

### 7.6 记录一次练习

```http
POST /api/v1/lessons/{id}/shadowing/attempts
```

请求：

```json
{
  "shadowing_version": 1,
  "sentence_index": 3,
  "practice_mode": "record",
  "playback_rate": 0.75,
  "loop_count": 2,
  "self_score": 80,
  "recognition_text": "",
  "audio_ref": ""
}
```

第一版建议只在以下行为时记 attempt：

- 用户完成一次录音并给出自评
- 用户显式完成某句循环练习

单纯“播放中经过一句”不写库，避免噪音数据爆炸。

补充规则：

- `self_score` 允许为空，例如用户完成一次循环练习但没有做自评。
- 只要某次 attempt 被定义为“完成了一次有效练习”，它就参与 completed sentence 的聚合。
- Phase 1 中“有效完成 attempt”只包含 `loop` 和 `record`。
- `normal` 和 `slow` 是共享状态枚举，第一阶段可出现在前端状态机和 `progress.last_practice_mode`，但不应通过 `POST /attempts` 持久化。
- 若客户端在 Phase 1 向 `POST /attempts` 提交 `normal` 或 `slow`，服务端返回 `422 ERR_SHADOWING_ATTEMPT_MODE_INVALID`。

### 7.7 历史接口

可选新增：

```http
GET /api/v1/lessons/{id}/shadowing/attempts?sentence_index=3&limit=20
```

它不是第一版阻塞项，但如果后面要做“本句最近表现”或“难句复盘”，这个接口会很有用。

---

## 8. 前端实现设计

### 8.1 页面组件拆分

推荐拆分：

- `ShadowingPage`
- `ShadowingMediaPlayer`
- `CurrentSentenceCard`
- `SubtitleTimeline`
- `ShadowingBottomDock`（移动端）
- `useShadowingSession` hook

`useShadowingSession` 负责：

- 当前播放时间
- 当前句定位
- 当前练习模式
- 录音状态
- 进度写回节流

### 8.2 当前句判定规则

当前句判定必须由：

```text
媒体 currentTime + sentence.start_ms/end_ms
```

直接驱动。

不应依赖浏览器 TTS 的 `onboundary`。当前项目里的 `exampleAudio` 在本地 WAV 场景下并不提供稳定的逐词边界事件，不适合作为 shadowing 主同步机制。

建议规则：

1. 若当前时间落在某句 `[start_ms, end_ms)` 内，则该句为当前句。
2. 若时间落在两句间空隙，则使用最近一个已开始但未被下一句覆盖的句子。
3. 用户点击字幕句子时，播放器直接 seek 到 `start_ms`。

这部分建议两端各自实现纯函数，并共享同一组 JSON fixtures 做一致性测试，而不是假设 Go 和 TS 直接共享一份函数实现。

边界行为需要明确：

1. 句子列表为空时，返回“无当前句”，播放器相关训练控件禁用，并记录内容错误。
2. `currentTime < first.start_ms` 时，当前句返回第一句，作为预览态。
3. `currentTime >= last.end_ms` 时，当前句保持为最后一句，避免播放结束后卡片闪空。
4. 两句之间存在空隙时，当前句保持为最近一个已开始的句子，直到下一句开始。
5. `end_ms <= start_ms` 的句子视为无效数据，导入校验应直接失败；运行时若仍遇到，判定逻辑应跳过该句并打日志，而不是参与高亮。

### 8.3 练习模式

第一版保留四种模式就够：

- `normal`
  正常连续播放
- `slow`
  当前句按 `0.75x` 播放
- `loop`
  当前句循环播放，默认 2 次
- `record`
  用户录音并本地回放

`normal | slow | loop | record` 这组枚举需要贯穿前端状态机、progress 记录和 attempts 记录，避免接口存的是一套、组件状态里跑的是另一套。

不建议第一版加入过多状态机，比如 AB 区间、跟读队列、自定义片段拼接。先把“围绕一句反复练”做顺。

### 8.4 录音策略

第一版延续现有浏览器录音能力：

- 使用当前已有的 `useAudioRecorder`
- 录音结果先只保存在本地 session 内供立即回放
- 后端先只记 metadata，不强制上传音频文件
- `useAudioRecorder` 生成的 object URL 需要在“开始新录音”或“页面卸载”时释放，避免 blob URL 泄漏

这样可以明显降低第一版复杂度：

- 不需要引入大文件上传与对象存储写路径
- 不会让一次普通跟读练习生成大量音频对象
- 后续若要做“录音历史回放”，再把 `audio_ref` 真正落到对象存储

实现要求建议写进前端任务：

- hook 自己管理最近一次 object URL 的生命周期
- 生成新 URL 前先 revoke 旧 URL
- 组件卸载时 revoke 当前 URL

### 8.5 UI 状态

句子级展示建议至少体现：

- 当前句
- 已练过句子
- 最近一次有自评分的句子
- 尚未练习句子

不建议第一版做太复杂的颜色系统，避免和现有视觉风格冲突。先保证高亮、已完成、普通三态即可。

---

## 9. 内容与管理流程

### 9.1 内容来源

第一版继续使用 lesson 作为唯一内容来源。

导入层建议扩展 `lessons_*.json`，支持新增字段：

```json
{
  "title": "第3课：コンビニで買い物",
  "jlpt_level": "N5",
  "audio_url": "/audio/lessons/convini.wav",
  "video_url": "",
  "shadowing_enabled": true,
  "shadowing_version": 1,
  "shadowing_config": {
    "media_type": "audio",
    "media_duration_ms": 86300
  }
}
```

如果媒体只有远程 URL，建议同时在 manifest 里提供 `media_duration_ms`。否则“最后一句 `end_ms` 是否超过媒体时长”这个校验很容易退化成半手工检查。

其中 `video_url` 只是为后续视频阶段预留。Phase 0/1 走 audio-first，不依赖 `video_url`；若运行时是 PostgreSQL，Phase 0/1 也先忽略该字段，等统一媒体引用层再接入视频。

### 9.2 材料获取流程

当前方案此前只覆盖了“材料进入 lesson 后如何承载”，还需要把“材料如何获取”作为 Phase 0 的正式前置输入。第一版建议采用人工精选试点材料，不做开放式抓取或自动采集。

允许的材料来源：

- 已有 lesson 文本
  使用当前 `data/seed/lessons_*.json` 中已有的课文、句子、中文释义和时间戳，补齐音频资源后开启 shadowing。若原始来源是视频，Phase 0 只人工抽取/校准音频，并保留视频来源说明，不把视频播放作为 MVP 运行时要求。
- 自制或授权素材
  项目自己录制、购买授权、获得明确许可的音频/视频和字幕。
- 开放授权素材
  公有领域或明确开放许可证素材，必须记录来源 URL、许可证和使用说明。
- 生成式/TTS 素材
  用项目已有或后续扩展的 TTS 能力生成课文音频，适合 Phase 0 的 audio-first 试点；生成后仍需人工校验句子切分和时间轴。

不允许的材料来源：

- 未授权搬运课程、影视、综艺、YouTube/Bilibili 视频或字幕。
- 只记录播放页 URL，但没有保存授权说明、媒体文件或可验证字幕来源的材料。
- 无法确认来源、许可证或可使用范围的第三方素材。

推荐材料生产流水线：

1. 建立候选材料清单。
2. 记录 `source_type/source_name/source_url/license/permission_note`。
3. 准备文本、中文释义和振假名 tokens。
4. 准备音频文件，并记录 `media_duration_ms`；若来源是视频，保留原始来源记录或附件，但运行时先使用抽取后的音频。
5. 生成或人工校准句子级 `start_ms/end_ms`。
6. 运行 `scripts/validate_lessons_shadowing.py`。
7. 通过 lesson import upsert 写入试点课文。
8. 人工打开 shadowing 预览，确认当前句高亮和媒体同步。

Phase 0 的人工 material pack 必须先定义 `shadowing_config_json.source` 的最小 JSON schema，不需要引入管理端 jobs。建议把材料来源信息放入 `shadowing_config_json.source`：

```json
{
  "source": {
    "source_type": "owned",
    "source_name": "internal-recording-001",
    "source_url": "",
    "license": "owned",
    "permission_note": "Recorded for this project",
    "subtitle_source": "manual"
  }
}
```

最小字段约束：

- `source_type` 必填，建议枚举：`owned | licensed | open_license | tts_generated`。
- `source_name` 必填，用于人工识别素材来源。
- `source_url` 可空；第三方开放授权素材必须填写。
- `license` 必填，记录 `owned`、许可证名称或授权类型。
- `permission_note` 必填，说明为什么本项目可使用该素材。
- `subtitle_source` 必填，建议枚举：`manual | provided_srt | asr_draft | none`；Phase 0 不使用 `asr_draft` 作为自动发布依据。

Phase 0 不需要建设完整素材后台，但必须产出 1 到 3 个“可验收 material pack”。每个 pack 至少包含：

- lesson JSON
- 音频文件路径；视频原始文件仅作为来源附件或后续 Phase 2 输入，不是 Phase 0 运行时必需项
- 来源/授权记录
- 媒体时长
- 句子级时间轴
- 校验脚本结果

### 9.3 后续：AI 跟读配置生成管理台

可以做一个 AI 辅助工具，根据指定视频自动生成 shadowing material pack。这个能力最终应该做到管理平台里，作为内容生产和审核入口；但它不进入 Phase 0/1 的 MVP 主路径。Phase 0 只准备人工 material pack、导入 upsert 和校验脚本，Phase 1 先跑通学习端影子跟读闭环。

AI 生成管理台建议作为 Phase 2 或独立 Epic 实现。它的定位不是“自动发布课程”，而是“生成可审核草稿”，再由管理端预览、校验脚本和人工确认决定是否导入。

后续 Epic 推荐新增独立 admin 页面：

```text
front/admin/src/pages/ShadowingMaterials/
```

管理端导航建议新增：

```text
Shadowing Materials
```

这不是完整 Lesson 管理后台，只负责影子跟读材料的生成、审核和导入。CLI 可以保留为底层调试入口，但不是第一入口。

该 Epic 的 admin 前端需要新增：

- `front/admin/src/pages/ShadowingMaterials/ShadowingMaterials.tsx`
- `front/admin/src/App.tsx` 增加 `shadowing-materials` 路由
- `front/admin/src/components/Layout/Layout.tsx` 增加导航项

该 Epic 的 admin 后端需要在 `internal/module/admin/handler.go` 注册 shadowing material routes。

后续推荐后端接口：

```http
POST /api/admin/shadowing/materials/jobs
GET  /api/admin/shadowing/materials/jobs
GET  /api/admin/shadowing/materials/jobs/{id}
POST /api/admin/shadowing/materials/jobs/{id}/import
DELETE /api/admin/shadowing/materials/jobs/{id}
```

输入要求：

- 管理端上传本地视频文件，或填写已授权且可下载的媒体 URL。
- `title`、`jlpt_level`、`source_type`、`license`、`permission_note`。
- 可选：已有字幕文件 `.srt/.vtt`，用于提高时间轴质量。
- 可选：目标句子长度、是否生成中文释义、是否生成振假名 tokens。

管理端页面流程：

1. 上传视频或填写媒体 URL。
2. 填写标题、JLPT 等级、来源、授权说明和可选字幕。
3. 点击“Generate Draft”创建生成任务。
4. 页面展示任务状态：`queued | processing | needs_review | failed | imported`。
5. 任务完成后进入审核视图，展示视频、字幕时间轴、日文、中文、振假名和校验报告。
6. 管理员可编辑标题、句子文本、中文释义、时间轴和来源说明。
7. 校验通过后点击“Import as Lesson”写入 lesson，并按需开启 `shadowing_enabled`。

生成流程：

1. 用 `ffmpeg` 抽取音频并读取媒体时长。
2. 如果提供字幕，先解析字幕作为主时间轴。
3. 如果没有字幕，调用 ASR 生成日文转写和片段时间戳。
4. 对 ASR 结果做句子切分，合并过短片段，拆分过长片段。
5. 生成中文释义。
6. 生成振假名 tokens。
7. 产出 `lesson JSON + shadowing_config_json.source + media_duration_ms + start_ms/end_ms`。
8. 运行 `scripts/validate_lessons_shadowing.py`。
9. 输出草稿报告，标记低置信度句子、过短/过长句子、重叠时间轴和缺失翻译。

输出目录建议：

```text
data/generated/shadowing/<slug>/
```

目录内容：

- `lesson.json`
- `source.json`
- `media.mp4` 或原始媒体引用
- `audio.wav`
- `transcript.srt`
- `validation-report.json`
- `review-notes.md`

AI 生成结果必须有人工确认门槛：

- 默认只写入 `data/generated/shadowing/...` 并在管理端显示为 `needs_review`，不直接导入数据库。
- 只有管理员在审核页显式点击导入且校验通过时，才允许调用 lesson upsert。
- 如果来源/授权字段不完整，工具必须拒绝导入。
- 如果 ASR 或对齐置信度低于阈值，工具只生成草稿，不允许自动开启 `shadowing_enabled`。

模型与依赖建议：

- ASR：优先复用后续口语评分里规划的 vLLM Whisper 服务；本地开发可用离线 Whisper 工具作为替代。
- 翻译和句子润色：复用项目已有 AI 配置，不在工具内硬编码供应商。
- 振假名：优先复用前端已有 kuromojin 思路，或在后端/脚本侧引入等价日语分词工具；输出必须符合当前 `FuriganaToken` 结构。
- 媒体处理：依赖 `ffmpeg/ffprobe`，缺失时给出明确错误。

该 Epic 的第一版保守范围：

- 支持单个视频生成一个 lesson。
- 支持管理端上传本地文件优先，不做站点爬取。
- 支持“字幕优先，ASR 补全”的路径。
- 不做批量抓取，不做自动版权判断，不做无人审核发布。
- 不要求同时实现完整 lesson 列表管理，只在生成任务完成后通过 lesson upsert 导入。

### 9.4 为什么 MVP 不先做管理端 UI

当前项目 admin 还没有 lesson CRUD。若 Phase 0/1 同时补 lesson 管理台、Shadowing Materials Admin、媒体上传、任务系统、时间轴编辑器和预览播放器，范围会明显超过“试点 + 学习闭环”。

因此建议按下面顺序推进：

1. 先通过 JSON + CLI 导入跑通 1 到 3 个试点 lesson。
2. 跟读页稳定后，再做 Shadowing Materials Admin 这样的内容生产自动化。
3. 再补 lesson 管理台的媒体配置、开关和预览。
4. 最后考虑可视化切句编辑器。

### 9.5 数据校验

由于项目已有严格的数据导入规范，影子跟读也应该补充校验脚本，至少检查：

- Phase 0/1 中 `shadowing_enabled=true` 的 lesson 必须有可播放 `audio_url`
- Phase 2 若启用视频，`video_url` 必须同时满足媒体时长可校验、音轨可用、来源授权完整
- `shadowing_enabled=true` 的 lesson 必须有来源/授权记录
- 所有句子的 `start_ms/end_ms` 非负且单调递增
- 句子列表不能为空
- 每句必须有日文 token 和中文释义
- 最后一条句子的 `end_ms` 不能明显超过媒体总时长

媒体时长校验策略建议明确化：

- 若导入清单能访问本地媒体源，校验脚本直接 probe 实际时长。
- 若只提供远程 URL，则要求 manifest 提供 `media_duration_ms`。
- 若两者都没有，校验脚本应报 warning 或直接失败，而不是假装完成了时长校验。

建议新增：

```text
scripts/validate_lessons_shadowing.py
```

在导入或发布试点课文前先跑校验。

### 9.6 SQLite 迁移约束

当前项目的 SQLite 迁移系统没有真正的迁移追踪，因此 shadowing 相关迁移必须显式按“重复执行安全”来设计。Phase 0 的文档和实现都应遵守：

- `ALTER TABLE ADD COLUMN` 只能使用常量默认值
- 不使用函数型默认值
- `CREATE TABLE IF NOT EXISTS`
- `CREATE INDEX IF NOT EXISTS`
- 不依赖“一次性 seed”来写 shadowing 元数据
- 不要把数据清理 DML 和建表/加列迁移混在同一个自动迁移文件里
- 重复启动服务不会重复插入 progress/attempts，也不会破坏已有数据

换句话说，shadowing 的迁移设计要默认“server 和 admin 每次启动都会再次跑一遍”。

---

## 10. 分阶段落地方案

### 阶段 0：试点内容准备

目标：

- 将 lesson 详情从内部 state 改为路由化 `/lesson/:id`
- 对齐 Lesson API / TypeScript 契约
- 扩展全局 APIError 协议，支持可选 `details`
- 选 1 到 3 篇 lesson 做 pilot
- 产出 1 到 3 个可验收 shadowing material pack
- 补齐 `audio_url`
- 校验句子时间戳
- 给 lesson 增加 `shadowing_enabled`
- 实现重复 lesson 报表，并提供单独 cleanup CLI
- 在确认目标数据库无重复后，再启用唯一索引迁移和 lesson upsert
- 保证 shadowing 迁移在 SQLite 重复执行下仍然安全

交付：

- 数据字段和迁移：SQLite 运行时迁移；PostgreSQL 仅 schema-only 更新，不实现 shadowing runtime store/service/router
- `/lesson`、`/lesson/:id` 路由拆分
- Lesson API / TS 类型修正
- `httputil.APIError` / 前端 API client 支持可选 `details`
- 试点 material pack：lesson JSON、音频文件、来源/授权记录、媒体时长、句子级时间轴；视频原始文件只作为来源附件或 Phase 2 输入
- JSON 导入支持
- lesson upsert 能力；唯一索引必须在重复清理完成后单独启用，不作为自动迁移的静默副作用
- 重复 lesson 报表与手动 cleanup CLI
- 校验脚本

### 阶段 1：音频版影子跟读页

目标：

- 上线 `/lesson/:id/shadowing`
- 支持字幕同步、当前句卡、慢速、循环、录音、本地回放、自评、进度恢复

这一阶段不要求视频、不要求录音持久化、不要求自动评分。若运行时是 PostgreSQL，则本阶段只要求 migration 通过、代码可编译、接口签名兼容；不注册 shadowing 路由、不返回 shadowing 入口，直到 `audio_object_id -> audio_url` 映射和媒体契约补齐。

### 阶段 2：视频支持与管理端内容生产增强

目标：

- `video_url` 生效
- 管理平台 `Shadowing Materials` 页面支持上传视频、填写来源、生成草稿、预览审核、导入 lesson
- 管理端 shadowing material jobs API
- 管理台支持配置 shadowing 开关和媒体
- 增加跟读预览能力

这一阶段可以作为独立 Epic 拆出，不阻塞 Phase 0/1 的学习端闭环。

### 阶段 3：自动评分与进阶复盘

目标：

- 接入 ASR / 自动评分
- 增加句子维度表现趋势
- 根据 attempt 生成“难句复习”

---

## 11. 测试策略

### 11.1 后端

优先补表格驱动测试，覆盖：

- 句子索引和版本号校验
- 进度 upsert 行为
- attempt 写入与聚合摘要
- `GET /shadowing` 的完成度只按当前 `shadowing_version` 聚合，旧版本 attempt 不参与 `completed_sentence_count` / `attempt_summary`
- `ERR_SHADOWING_VERSION_STALE` 的 `details.current_shadowing_version` 返回
- nullable `self_score` 下的 `best_score/last_score` 聚合
- Phase 1 禁止 `normal` / `slow` attempt 持久化
- 未开启 shadowing 的 lesson 拒绝进入跟读接口
- 缺少媒体资源的 lesson 返回明确错误

### 11.2 前端

至少验证：

- `APIError.details` 能被正确解析
- 当前时间切换时当前句高亮是否正确
- 点击字幕是否能正确 seek
- `0.75x`、循环、录音状态是否互斥合理
- 页面刷新后是否能恢复上次句子和播放位置
- 移动端底部操作栏在录音状态下是否仍可用

### 11.3 后续 AI 生成管理台测试

Phase 2 / 独立 Epic 再补：

- 管理端 `Shadowing Materials` 页面能上传视频、创建任务、显示任务状态和进入审核视图
- 审核页在校验失败或缺少授权信息时禁用导入
- 管理端 AI material pack 生成任务拒绝缺少来源/授权字段的输入
- 管理端 AI material pack 生成任务对低置信度 ASR/时间轴只输出草稿，不自动导入

### 11.4 手动验收

建议做一套最小验收清单：

1. 打开试点 lesson，能进入影子跟读页。
2. 播放时字幕自动高亮。
3. 点击任一句能跳转到对应时间。
4. 当前句可慢速播放和循环。
5. 可录音并立即回放。
6. 自评分保存后，下次打开还能看到句子进度痕迹。

---

## 12. 风险与回滚

### 12.1 主要风险

- 现有 lesson 虽有时间戳，但未必都存在真实可用的音频资源。
- 时间戳质量如果不稳定，页面体验会直接崩坏。
- 后续 AI 生成管理台里的 ASR、翻译、振假名和时间轴可能存在错漏。
- 移动端浏览器录音权限和自动播放策略存在兼容性波动。
- 如果未来切句重做但没有版本号，旧进度会错位。

### 12.2 缓解策略

- 只对试点 lesson 开启 `shadowing_enabled`
- 先走 audio-first，减少视频链路复杂度
- 增加 lesson shadowing 数据校验
- 后续 AI 生成工具默认只产出草稿包，必须人工审核后导入
- 用 `shadowing_version` 处理切句变更
- 录音失败时允许用户退化为“只听 + 只跟读 + 只自评”

### 12.3 回滚方案

本方案的回滚成本较低：

- 关闭 `shadowing_enabled` 即可隐藏入口
- `/lesson` 原有阅读路径不受影响
- 新增的 progress/attempts 表是旁路数据，不会破坏既有学习记录

---

## 13. 最终建议

基于当前项目实际情况，最稳的做法不是“把 speaking 改造成视频页”，而是：

1. 让 `lesson` 成为影子跟读唯一内容来源。
2. 为 lesson 增加 shadowing 元数据和用户练习记录。
3. 先用音频版试点验证整条训练链路。
4. 等课文媒体和管理流程稳定后，再补视频、自动评分和难句复盘。

这条路径最符合当前仓库已有的数据结构，也最能避免后续维护两套字幕和练习系统。
