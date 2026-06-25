# SQLite → PostgreSQL 迁移指南

## 环境配置

### PostgreSQL

| 项目 | 值 |
|------|-----|
| Host | `192.168.1.20` |
| Port | `19588` |
| User | `postgres` |
| Password | `12345` |

连接串格式（需替换 `<dbname>` 为目标数据库名）：

```
postgres://postgres:12345@192.168.1.20:19588/<dbname>?sslmode=disable
```

### MinIO

| 项目 | 值 |
|------|-----|
| Endpoint | `http://192.168.1.20:19000` |
| User（Access Key） | `tylerhu` |
| Password（Secret Key） | `123456hjr` |

---

## 前置准备

### 1. 创建目标数据库

```bash
psql.exe -h 192.168.1.20 -p 19588 -U postgres -c "CREATE DATABASE japanese;"
```

### 2. 设置环境变量

**Windows PowerShell：**

```powershell
$env:DATABASE_URL = "postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable"
$env:MINIO_ENDPOINT = "192.168.1.20:19000"
$env:MINIO_ACCESS_KEY = "tylerhu"
$env:MINIO_SECRET_KEY = "123456hjr"
$env:MINIO_BUCKET_AUDIO = "audio"
```

---

## 快速开始

### 第零步：确认 PostgreSQL 目标库已存在

`DATABASE_URL` 中的数据库名必须先存在；迁移命令会自动初始化表结构，但不会自动创建数据库。

```powershell
$env:PGPASSWORD = "12345"
createdb.exe -h 192.168.1.20 -p 19588 -U postgres japanese
```

### 第一步：试运行（不写入数据）

```bash
make migrate-to-pg-dry-run DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable"
```

试运行会连接 SQLite 和 PostgreSQL，扫描所有业务数据、验证格式、预估行数；它**不会写入业务数据**，但会按需执行 SQLite 本地迁移和 PostgreSQL schema 初始化。

### 第二步：迁移数据（跳过音频）

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" SKIP_AUDIO=1
```

### 第三步：迁移音频到 MinIO

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" PHASE=audio
```

### 一步到位（数据 + 音频）

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable"
```

> **注意**：完整迁移需要在环境变量中配置 `MINIO_ENDPOINT`、`MINIO_ACCESS_KEY`、`MINIO_SECRET_KEY`，或通过 Make 变量传入。

---

## CLI 参数

| 参数 | 环境变量 | 默认值 | 说明 |
|------|---------|--------|------|
| `--database-url` | `DATABASE_URL` | — | **必填**。PostgreSQL 连接串 |
| `--sqlite-db` | — | `./data/app.db` | SQLite 数据库文件路径 |
| `--audio-dir` | — | `./data/audio` | 本地音频文件目录 |
| `--minio-endpoint` | `MINIO_ENDPOINT` | — | MinIO 服务端点（`192.168.1.20:19000`） |
| `--minio-bucket` | `MINIO_BUCKET_AUDIO` | `audio` | MinIO 存储桶名称 |
| `--minio-access-key` | `MINIO_ACCESS_KEY` | — | MinIO 访问密钥（`tylerhu`） |
| `--minio-secret-key` | `MINIO_SECRET_KEY` | — | MinIO 秘密密钥（`123456hjr`） |
| `--minio-use-ssl` | `MINIO_USE_SSL` | `false` | MinIO 是否启用 SSL |
| `--dry-run` | — | `false` | 仅验证不写入 |
| `--skip-audio` | — | `false` | 跳过音频迁移 |
| `--phase` | — | 全部 | 只执行指定阶段 |
| `--batch-size` | — | `100` | 每批 INSERT 行数 |
| `--skip-tables` | — | — | 跳过指定表（逗号分隔） |
| `--reset-pg` | — | `false` | 迁移前清空 PG 表（需 `--force`） |
| `--force` | — | `false` | 确认危险操作 |

### `--phase` 可选值

| 值 | 迁移内容 |
|----|---------|
| `users` | 用户表 |
| `content` | 内容根表（words、grammar_points、lessons、speaking_materials、writing_questions、translation_sources） |
| `content-children` | JSON 拆分子表（word_examples、grammar_examples、grammar_quiz_questions、lesson_sentences、lesson_words、translation_sentences） |
| `userdata` | 用户数据表（word_records、grammar_records、speaking_records、writing_records、translation_records、word_bookmarks、notes、note_links） |
| `sessions` | 会话 & 影子跟读（study_sessions、session_summaries、lesson_shadowing_progress、lesson_shadowing_attempts） |
| `bookkeeping` | 工具表 & 序列重置（password_reset_tokens + 所有序列 `setval`） |
| `audio` | 音频文件上传到 MinIO + FK 更新 |

---

## 常用命令（实际配置）

> 以下命令使用实际环境配置，可直接复制执行。

### 数据库连接测试

```bash
psql.exe -h 192.168.1.20 -p 19588 -U postgres -d japanese -c "SELECT version();"
```

### 试运行

```bash
make migrate-to-pg-dry-run DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable"
```

### 仅迁移数据（跳过音频）

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" SKIP_AUDIO=1
```

### 完整迁移（数据 + 音频）

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable"
```

### 分阶段迁移

```bash
# 先迁移用户
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" PHASE=users

# 再迁移内容
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" PHASE=content

# 再迁移内容子表
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" PHASE=content-children

# 再迁移用户数据
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" PHASE=userdata

# 最后迁移音频
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" PHASE=audio
```

### 指定 SQLite 路径

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" SQLITE_DB="./backup/app.db"
```

### 清空 PG 后重新迁移（危险操作）

```bash
make migrate-to-pg DATABASE_URL="postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" RESET_PG=1 FORCE=1
```

### 直接使用 Go CLI

```bash
go run ./backend/cmd/appctl migrate-sqlite-to-pg \
  --database-url "postgres://postgres:12345@192.168.1.20:19588/japanese?sslmode=disable" \
  --sqlite-db ./data/app.db \
  --minio-endpoint "192.168.1.20:19000" \
  --minio-access-key "tylerhu" \
  --minio-secret-key "123456hjr" \
  --dry-run
```

---

## 迁移过程详解

### Phase 1: users（零依赖表）
- `jlpt_levels`：JSON 字符串数组 → PostgreSQL `TEXT[]`
- `daily_goals_json`：TEXT → JSONB
- `created_at`：SQLite DATETIME → TIMESTAMPTZ

### Phase 2: 内容根表
- **words**：`examples_json` 不在此插入（留给 Phase 3 拆分）
- **grammar_points**：同理，JSON 列延后处理
- **lessons**：`tags_json` → TEXT[]、`shadowing_enabled` INTEGER→BOOLEAN
- **speaking_materials**：`lines` JSON → TEXT[]、`audio_url` → `audio_url_legacy`
- **writing_questions**：`grammar_point_id = 0` → `NULL`
- **translation_sources**：直接映射

### Phase 3: 从 JSON 拆分的子表
- `word_examples` ← `words.examples_json`（每元素一行）
- `grammar_examples` ← `grammar_points.examples_json`
- `grammar_quiz_questions` ← `grammar_points.quiz_questions_json`
- `lesson_sentences` ← `lessons.content_furigana_json`
- `lesson_words` ← `lessons.word_ids_json`
- `translation_sentences`：直接映射

### Phase 4: 用户数据表
- `word_records`：`interval` → `interval_days`、`review_history_json` → JSONB
- `grammar_records`：`quiz_history_json` → JSONB
- `speaking_records`：`audio_ref` → `audio_ref_legacy`
- `writing_records`：`ai_feedback_json` 中 `"null"` 字符串 → SQL NULL
- `translation_records`：**JOIN translation_sentences** 填充 4 个 snapshot 列
- `word_bookmarks`：直接映射
- `notes`：`tags_json` → TEXT[]、`interval` → `interval_days`
- `note_links`：直接映射

### Phase 5: 会话 & 影子跟读
- `study_sessions`、`session_summaries`、`lesson_shadowing_progress`、`lesson_shadowing_attempts`

### Phase 6: 工具表 & 序列重置
- `password_reset_tokens`：`used` INTEGER→BOOLEAN
- 所有表 `SELECT setval(...)` 重置序列

### Phase 7: 音频迁移
1. 扫描 `./data/audio/words/`、`./data/audio/examples/`、`./data/audio/lessons/`，并兼容 `./data/audio/*.wav` 根目录 lesson 音频
2. 计算文件完整 SHA-256
3. 上传到 MinIO bucket `audio`（`words/<filename>`、`examples/<filename>`、`lessons/<filename>` 或根目录 `<filename>`）
4. INSERT `audio_objects` 记录（ON CONFLICT 更新）
5. 更新 `words.audio_object_id`、`speaking_materials.audio_object_id`、`lessons.audio_object_id`

---

## 类型转换速查

| SQLite 类型 | PostgreSQL 类型 | 转换逻辑 |
|------------|----------------|---------|
| TEXT (datetime) | TIMESTAMPTZ | 9 种格式自动解析 |
| TEXT (JSON) | JSONB | 直接传递，PG 侧 `::jsonb` 转换 |
| TEXT (JSON array) | TEXT[] | 解析 JSON → PG array literal `{elem1,elem2}` |
| INTEGER (0/1) | BOOLEAN | `!= 0` → true/false |
| REAL | DOUBLE PRECISION | 直接映射 |
| INTEGER AUTOINCREMENT | BIGINT GENERATED BY DEFAULT AS IDENTITY | 保留 ID，`ON CONFLICT DO UPDATE` |

---

## 迁移后验证

迁移完成后自动运行以下检查：

1. **行数对比**：每张表 SQLite COUNT vs PG COUNT
2. **FK 完整性**：检查 13 个外键关系是否存在孤立引用
3. **JSON 拆分验证**：子表行数 vs SQLite JSON 数组元素数
4. **关键转换**：
   - `writing_questions.grammar_point_id` 无 0 值
   - `word_records.interval_days` 无 NULL
   - `translation_records` snapshot 列非空
   - `grammar_quiz_questions.options` 非 NULL

### 手动验证 SQL（连接 PostgreSQL 执行）

```bash
psql.exe -h 192.168.1.20 -p 19588 -U postgres -d japanese
```

```sql
-- 数据量一致性
SELECT 'words' AS tbl, COUNT(*) FROM words
UNION ALL SELECT 'word_examples', COUNT(*) FROM word_examples
UNION ALL SELECT 'grammar_points', COUNT(*) FROM grammar_points
UNION ALL SELECT 'grammar_examples', COUNT(*) FROM grammar_examples
UNION ALL SELECT 'lessons', COUNT(*) FROM lessons
UNION ALL SELECT 'lesson_sentences', COUNT(*) FROM lesson_sentences
UNION ALL SELECT 'users', COUNT(*) FROM users;

-- 关键数据完整性
SELECT * FROM writing_questions WHERE grammar_point_id = 0;  -- 应为 0 行
SELECT * FROM word_records WHERE interval_days IS NULL;       -- 应为 0 行

-- 音频对象
SELECT COUNT(*) FROM audio_objects;
SELECT COUNT(*) FROM words WHERE audio_object_id IS NOT NULL;

-- 序列位置（确认在最大 ID 之上）
SELECT 'words' AS seq, currval('words_id_seq') AS cur, MAX(id) AS max FROM words;
```

---

## 错误处理

- **单行 JSON 解析失败**：跳过该行，记录 WARN 日志，继续其他行
- **单文件上传失败**：跳过该文件，记录 ERROR 日志，继续其他文件
- **阶段级事务**：阶段内任意表致命失败时回滚整个阶段
- **已提交阶段不回滚**：已完成阶段的数据保留
- **幂等运行**：使用 `ON CONFLICT DO UPDATE`，可安全重复执行

---

## 注意事项

1. **序列 ID 保留**：所有原始 ID 保留，确保外键关系正确
2. **音频文件命名**：文件名 = `sha256(text)[:16].wav`，相同文本产生相同文件名
3. **MinIO bucket 自动创建**：如果目标 bucket `audio` 不存在则自动创建
4. **`notes_fts` 不迁移**：SQLite FTS5 虚拟表无 PG 等价物，PG 模式已用 `pg_trgm` GIN 索引替代
5. **PG 专用表不迁移**：`audio_objects`、`video_objects`、`word_review_events`、`grammar_quiz_attempts`、`note_review_events` 在 SQLite 中无对应数据
6. **`--reset-pg` 是破坏性操作**：使用前确保已备份 PostgreSQL 数据
7. **MinIO 使用 HTTP**：当前 MinIO 端点使用 HTTP（非 HTTPS），`MINIO_USE_SSL` 应保持默认 `false`
