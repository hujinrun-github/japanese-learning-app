# 管理后台使用指南

## 1. 概述

管理后台是一个独立部署的 Web 应用，用于管理日语学习内容（单词、语法、口语、写作、翻译）和查看用户数据/学习记录。

- **后端**: Go 二进制，监听 `:8082`，复用主应用的 SQLite 数据库
- **前端**: React SPA，开发服务器 `:5174`
- **认证**: 通过环境变量 `ADMIN_TOKEN` 设置 token，前端登录页输入相同 token

### 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `ADMIN_TOKEN` | ✅ | - | 管理后台登录 token |
| `DB_PATH` | | `./data/app.db` | SQLite 数据库路径 |
| `LISTEN_ADDR` | | `:8082` | 监听地址 |
| `AI_API_KEY` | | `sk-a235671815e6469c8c15e69f18494500` | LLM API 密钥（默认 DeepSeek） |
| `AI_API_ENDPOINT` | | `https://api.deepseek.com/v1/chat/completions` | LLM API 地址 |
| `AI_MODEL` | | `deepseek-chat` | 模型名称 |

### 启动方式

```bash
# 最简启动（无 AI 功能）
ADMIN_TOKEN=your-password go run ./backend/cmd/admin/

# Anthropic Claude
AI_API_KEY=xxx ADMIN_TOKEN=your-password go run ./backend/cmd/admin/

# DeepSeek
AI_API_KEY=sk-xxx \
AI_API_ENDPOINT=https://api.deepseek.com/v1/chat/completions \
AI_MODEL=deepseek-chat \
ADMIN_TOKEN=your-password \
go run ./backend/cmd/admin/

# OpenAI
AI_API_KEY=sk-xxx \
AI_API_ENDPOINT=https://api.openai.com/v1/chat/completions \
AI_MODEL=gpt-4o-mini \
ADMIN_TOKEN=your-password \
go run ./backend/cmd/admin/

# 或使用 Makefile
ADMIN_TOKEN=your-password make admin-run

# 前端开发服务器
cd front/admin && npm run dev
# 或
make admin-front-dev

# 前端生产构建
cd front/admin && npm run build
# 或
make admin-front-build
```

启动后访问 `http://localhost:5174`，输入你设定的 token 登录。

### AI 提供商说明

代码根据 `AI_API_ENDPOINT` 自动切换 API 格式：

| 提供商 | endpoint | 默认 model | 认证头 |
|--------|----------|-----------|--------|
| Anthropic | `https://api.anthropic.com/v1/messages` | `claude-3-haiku-20240307` | `x-api-key` |
| DeepSeek | `https://api.deepseek.com/v1/chat/completions` | `deepseek-chat` | `Authorization: Bearer` |
| OpenAI | `https://api.openai.com/v1/chat/completions` | `gpt-4o-mini` | `Authorization: Bearer` |

判断逻辑：endpoint 中包含 `anthropic` → 用 Anthropic 格式，否则用 OpenAI 兼容格式。其他兼容 OpenAI 格式的 API（如 local LLM）也只需设 OpenAI 风格的 endpoint 即可。

---

## 2. 手动添加内容（表单方式）

每个模块页面顶部有工具栏：筛选下拉框、搜索框、**"+ Add New"** 按钮、**"Import"** 批量导入按钮。

点击 **"+ Add New"** 弹出表单，填写后保存。点击表格行的 **Edit** / **Delete** 按钮可修改或删除。

### 2.1 单词（Words）

| 字段 | 说明 | 必填 |
|------|------|------|
| Kanji Form | 汉字写法，如 `食べる` | ✅ |
| Reading | 假名读音，如 `たべる` | |
| Meaning | 中文释义，如 `吃` | ✅ |
| Part of Speech | 词性，如 `動詞` | |
| JLPT Level | N5 / N4 / N3 / N2 / N1 | ✅ |
| Reading Type | `1`=音読み, `2`=訓読み, `3`=音訓, `4`=訓音, `5`=熟字訓, `6`=その他 | |
| Examples | JSON 数组，格式见下方 | |

**功能开关（仅新增时）：**

- **Auto-fill reading/POS (kagome)**: 默认勾选。使用日语形态素分析引擎（kagome）自动补全读音、词性和读音类型。只需填写 `kanji_form` 和 `meaning` 即可。
- **Generate examples via AI**: 调用 LLM 大模型为单词生成 2-3 个例句，每个例句附带中文翻译和汉字振假名（furigana）。需要设置 `AI_API_KEY` 环境变量。

**Examples 格式：**
```json
[
  {"japanese": "朝ごはんを食べる", "chinese": "吃早饭"},
  {"japanese": "パンを食べた", "chinese": "吃了面包"}
]
```

### 2.2 语法（Grammar）

| 字段 | 说明 | 必填 |
|------|------|------|
| Name | 语法名称，如 `〜は〜です` | ✅ |
| Meaning | 中文释义，如 `……是……（表示判断）` | ✅ |
| Conjunction Rule | 接续规则，如 `名詞 + は + 名詞 + です` | |
| Usage Note | 使用说明 | |
| JLPT Level | N5 / N4 / N3 / N2 / N1 | ✅ |
| Examples | JSON 数组，格式见下方 | |
| Quiz Questions | JSON 数组，格式见下方 | |

**Examples 格式：**
```json
[
  {
    "japanese": "私は学生です",
    "chinese": "我是学生",
    "linked_word_ids": []
  }
]
```

**Quiz Questions 格式：**
```json
[
  {
    "id": 1,
    "type": "fill_blank",
    "prompt": "私___学生です。",
    "options": null,
    "answer": "は",
    "explanation": "「は」是主题助词"
  }
]
```

`type` 可选值：`fill_blank`（填空）或 `multi_choice`（选择）。选择题需提供 `options` 数组，如 `["は", "が", "を", "に"]`。

### 2.3 口语（Speaking）

| 字段 | 说明 | 必填 |
|------|------|------|
| Type | `read_aloud` / `picture_description` / `free_talk` / `question_answer` | ✅ |
| Title | 标题，如 `挨拶の基本` | ✅ |
| Text | 口语练习文本 | |
| Audio URL | 音频文件路径 | |
| JLPT Level | N5 / N4 / N3 / N2 / N1 | ✅ |

### 2.4 写作（Writing）

| 字段 | 说明 | 必填 |
|------|------|------|
| Type | `input`（输入题）或 `sentence`（造句题） | ✅ |
| Prompt | 题目描述，如 `请输入「おはよう」的汉字形式` | ✅ |
| Expected Answer | 期望答案，用于自动评分 | |
| JLPT Level | N5 / N4 / N3 / N2 / N1 | |
| Grammar Point ID | 关联的语法点 ID，无关联填 `0` | |

### 2.5 翻译（Translation）

| 字段 | 说明 | 必填 |
|------|------|------|
| Source ID | 翻译来源 ID | |
| Direction | `cn2jp`（中→日）或 `jp2cn`（日→中） | ✅ |
| Source Text | 原文 | ✅ |
| Reference Translation | 参考译文 | |
| Position | 排序位置 | |

### 2.6 用户（Users）

只读页面，显示所有注册用户。点击某行展开该用户各模块的学习统计（待复习数、已掌握数、总进度、今日完成数/每日目标）。

### 2.7 记录（Records）

通过下拉框选择模块（word/grammar/speaking/writing），可选按 user_id 筛选，查看学习记录。

---

## 3. 批量导入（JSON 文件）

所有导入接口为 `POST /api/admin/import/{module}`，上传 `.json` 文件。JSON 文件内容是一个**数组**，每个元素对应一条记录。

在页面中点击 **"Import"** 按钮选择 JSON 文件即可。导入结果弹窗显示 `inserted`（成功导入数）。

### 3.1 导入单词

**接口**: `POST /api/admin/import/words`

```json
[
  {
    "kanji_form": "食べる",
    "reading": "たべる",
    "part_of_speech": "動詞",
    "meaning": "吃",
    "jlpt_level": "N5",
    "examples": [
      {"japanese": "朝ごはんを食べる", "chinese": "吃早饭"}
    ],
    "reading_type": "2"
  },
  {
    "kanji_form": "飲む",
    "reading": "のむ",
    "part_of_speech": "動詞",
    "meaning": "喝",
    "jlpt_level": "N5",
    "examples": [
      {"japanese": "水を飲む", "chinese": "喝水"}
    ],
    "reading_type": "1"
  }
]
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `kanji_form` | string | ✅ | 汉字写法 |
| `reading` | string | | 假名读音 |
| `part_of_speech` | string | | 词性 |
| `meaning` | string | ✅ | 中文释义 |
| `jlpt_level` | string | ✅ | N5~N1 |
| `examples` | array | | 例句数组 `[{japanese, chinese}]` |
| `reading_type` | string | | `1`音読 `2`訓読 `3`音訓 `4`訓音 `5`熟字訓 `6`その他 |

唯一约束：`(kanji_form, reading)` 重复则跳过。

### 3.2 导入语法

**接口**: `POST /api/admin/import/grammar`

```json
[
  {
    "name": "〜は〜です",
    "meaning": "……是……（表示判断和说明）",
    "conjunction_rule": "名詞 + は + 名詞 + です",
    "usage_note": "最基本的日语句型，用于说明事物的性质、状态或身份。",
    "jlpt_level": "N5",
    "examples": [
      {
        "japanese": "私は学生です。",
        "chinese": "我是学生。",
        "linked_word_ids": []
      }
    ],
    "quiz_questions": [
      {
        "id": 1,
        "type": "fill_blank",
        "prompt": "私___学生です。",
        "options": null,
        "answer": "は",
        "explanation": "「は」是主题助词"
      }
    ]
  }
]
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✅ | 语法名称 |
| `meaning` | string | ✅ | 中文释义 |
| `conjunction_rule` | string | | 接续规则 |
| `usage_note` | string | | 使用说明 |
| `jlpt_level` | string | ✅ | N5~N1 |
| `examples` | array | | 例句 `[{japanese, chinese, linked_word_ids}]` |
| `quiz_questions` | array | | 测验题 `[{id, type, prompt, options, answer, explanation}]` |

唯一约束：`(name, jlpt_level)` 重复则跳过。

### 3.3 导入口语

**接口**: `POST /api/admin/import/speaking`

```json
[
  {
    "type": "shadow",
    "title": "挨拶の基本",
    "text": "おはようございます。今日もよろしくお願いします。",
    "audio_url": "",
    "jlpt_level": "N5"
  }
]
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | ✅ | `shadow`（跟读）或 `free`（自由会话） |
| `title` | string | ✅ | 标题 |
| `text` | string | | 练习文本 |
| `audio_url` | string | | 音频路径 |
| `jlpt_level` | string | ✅ | N5~N1 |

唯一约束：`(type, title, jlpt_level)` 重复则跳过。

### 3.4 导入写作

**接口**: `POST /api/admin/import/writing`

```json
[
  {
    "type": "input",
    "prompt": "请输入「おはよう」的汉字形式（提示：早上好）",
    "expected_answer": "お早う",
    "grammar_point_id": 0,
    "jlpt_level": "N5"
  },
  {
    "type": "sentence",
    "prompt": "用「〜たい」造句，表达你想做的事",
    "expected_answer": "",
    "grammar_point_id": 5,
    "jlpt_level": "N5"
  }
]
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | ✅ | `input`（输入题）或 `sentence`（造句题） |
| `prompt` | string | ✅ | 题目描述 |
| `expected_answer` | string | | 期望答案 |
| `grammar_point_id` | int | | 关联语法点 ID，无关联填 `0` |
| `jlpt_level` | string | ✅ | N5~N1 |

唯一约束：`(type, prompt)` 重复则跳过。

---

## 4. 批量重新生成语音

管理后台支持对已有单词、语法、口语材料批量生成或重新生成 TTS 音频。入口在 `Words`、`Grammar`、`Speaking` 三个页面的表格工具栏。

### 4.1 页面操作

1. 启动管理后台接口和前端：

```bash
ADMIN_TOKEN=your-password make admin-run
make admin-front-dev
```

如果需要在页面中预听 `/audio/...` 文件，还需要启动主后端：

```bash
make start-backend
```

2. 打开 `http://localhost:5174`，输入 `ADMIN_TOKEN` 登录。
3. 进入 `Words`、`Grammar` 或 `Speaking` 页面。
4. 使用筛选、搜索、分页定位数据后，勾选表格左侧的行。表头 checkbox 只会全选当前页可见行。
5. 勾选工具栏里的 **Audio**，选择 TTS Provider。通常使用 `vLLM (Qwen3-TTS)`。
6. 检查 TTS 配置：
   - `TTS Endpoint URL`: vLLM TTS 地址，通常是 `http://<host>:8091/v1/audio/speech`
   - `Model`: 默认 `Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice`
   - `Voice`: 默认 `ono_anna`
   - `Instructions`: 发音风格指令
7. 如果要覆盖已有音频，勾选 **Force regenerate (overwrite existing audio files)**。不勾选时，已有文件会跳过并计入 `existing`。
8. 点击 **Regenerate Selected Audio (n)**，确认弹窗后开始生成。
9. 完成后弹窗显示 `generated`、`already existed`、`failed`、`total`。

### 4.2 当前生成规则

| 模块 | 批量输入 | 输出目录 | 数据库更新 | 说明 |
|------|----------|----------|------------|------|
| Words | 选中单词的 `reading` | `data/audio/words/` | 更新 `words.audio_url` | 单词音频会裁剪头尾静音 |
| Grammar | 选中语法点的第一个例句 `japanese`；没有例句时用 `name` | `data/audio/examples/` | 不更新 DB | 页面播放时按文本 hash 推导音频路径 |
| Speaking | 选中口语材料的 `text` | `data/audio/examples/` | 不更新 DB | 页面播放时按文本 hash 推导音频路径 |

音频文件名固定为：

```text
sha256(text)[:16].wav
```

示例：文本相同则生成同一个文件名。批量生成默认跳过已存在文件；只有开启 `force` 才会删除并重写。

### 4.3 单条重新生成

表格每行的音频列有两个按钮：

- 播放按钮：优先播放预生成音频；失败时回退到浏览器 TTS。
- 重新生成按钮：打开单条 `Regenerate Audio` 弹窗，默认选择 `vLLM`，点击 `Generate` 后会立即生成并自动试听。

单条重新生成接口会直接写入目标文件；如果是 `word` 模块且传入了 `word_id`，会同步更新 `words.audio_url`。

### 4.4 用命令调用批量 API

管理后台页面目前只提交“当前页被勾选的可见行”。如果需要按条件跑更大范围，推荐直接用命令调用后端 API。此方式不需要启动管理前端，只需要：

- 管理后台后端 `:8082` 已启动
- TTS 服务可访问，例如 `http://127.0.0.1:8091/v1/audio/speech`
- 请求头带 `Authorization: Bearer <ADMIN_TOKEN>`

PowerShell 启动管理后台后端：

```powershell
$env:ADMIN_TOKEN = "your-password"
go run ./backend/cmd/admin/
```

另开一个 PowerShell 窗口执行批量生成命令。

#### 4.4.1 PowerShell 通用变量

```http
POST /api/admin/audio/batch
Authorization: Bearer <token>
Content-Type: application/json
```

```powershell
$token = "your-password"
$endpoint = "http://localhost:8082/api/admin/audio/batch"
$headers = @{ Authorization = "Bearer $token" }

$tts = @{
  provider = "vllm"
  tts_url = "http://127.0.0.1:8091/v1/audio/speech"
  tts_model = "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice"
  voice = "ono_anna"
  instructions = "Pronounce only the exact given word, in isolation. No extra sounds, no prefix, no suffix. Clean single-word pronunciation."
}
```

#### 4.4.2 按单词 ID 生成或重生成

```powershell
$body = @{
  module = "words"
  word_ids = @(1, 2, 3)
  force = $true
} + $tts

Invoke-RestMethod `
  -Method Post `
  -Uri $endpoint `
  -Headers $headers `
  -ContentType "application/json; charset=utf-8" `
  -Body ($body | ConvertTo-Json -Depth 5)
```

#### 4.4.3 按 JLPT 级别生成所有单词

```powershell
$body = @{
  module = "words"
  level = "N5"
  force = $false
} + $tts

Invoke-RestMethod `
  -Method Post `
  -Uri $endpoint `
  -Headers $headers `
  -ContentType "application/json; charset=utf-8" `
  -Body ($body | ConvertTo-Json -Depth 5)
```

不传 `level` 时会处理所有有 `reading` 的单词。

#### 4.4.4 生成语法例句音频

按级别从数据库收集语法点里的所有例句：

```powershell
$body = @{
  module = "grammar"
  level = "N5"
  force = $false
} + $tts

Invoke-RestMethod `
  -Method Post `
  -Uri $endpoint `
  -Headers $headers `
  -ContentType "application/json; charset=utf-8" `
  -Body ($body | ConvertTo-Json -Depth 5)
```

只生成指定文本：

```powershell
$body = @{
  module = "grammar"
  texts = @("私は学生です。", "これは本です。")
  force = $true
} + $tts

Invoke-RestMethod `
  -Method Post `
  -Uri $endpoint `
  -Headers $headers `
  -ContentType "application/json; charset=utf-8" `
  -Body ($body | ConvertTo-Json -Depth 5)
```

#### 4.4.5 生成口语材料音频

按级别和类型从数据库收集口语材料：

```powershell
$body = @{
  module = "speaking"
  level = "N5"
  type = "read_aloud"
  force = $false
} + $tts

Invoke-RestMethod `
  -Method Post `
  -Uri $endpoint `
  -Headers $headers `
  -ContentType "application/json; charset=utf-8" `
  -Body ($body | ConvertTo-Json -Depth 5)
```

只生成指定文本：

```powershell
$body = @{
  module = "speaking"
  texts = @("おはようございます。今日もよろしくお願いします。")
  force = $true
} + $tts

Invoke-RestMethod `
  -Method Post `
  -Uri $endpoint `
  -Headers $headers `
  -ContentType "application/json; charset=utf-8" `
  -Body ($body | ConvertTo-Json -Depth 5)
```

不传 `word_ids` / `texts` 时，后端会按模块从数据库收集文本：

- `module=words`: 可用 `level` 限定 JLPT 级别；不传则处理所有有 `reading` 的单词。
- `module=grammar`: 可用 `level` 限定 JLPT 级别；不传 `texts` 时会收集语法点里的所有例句。
- `module=speaking`: 可用 `level` 和 `type` 限定范围；不传 `texts` 时会收集所有非空 `text`。

返回格式：

```json
{
  "module": "words",
  "total": 3,
  "generated": 2,
  "existing": 1,
  "failed": 0
}
```

#### 4.4.6 curl.exe 示例

Windows PowerShell 中 `curl` 可能是别名，建议显式使用 `curl.exe`：

```powershell
curl.exe -X POST "http://localhost:8082/api/admin/audio/batch" `
  -H "Authorization: Bearer your-password" `
  -H "Content-Type: application/json" `
  --data-raw '{ "module": "words", "level": "N5", "force": false, "provider": "vllm", "tts_url": "http://127.0.0.1:8091/v1/audio/speech", "tts_model": "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice", "voice": "ono_anna", "instructions": "Pronounce only the exact given word, in isolation. No extra sounds, no prefix, no suffix. Clean single-word pronunciation." }'
```

如果请求体中包含日文文本，优先使用上面的 `Invoke-RestMethod` + `ConvertTo-Json`，可以减少命令行编码问题。

### 4.5 独立 CLI（仅单词）

独立 CLI 当前只提供单词音频批量生成：

```bash
go run ./backend/cmd/server/ generate-word-audio \
  --db ./data/app.db \
  --level N5 \
  --provider vllm \
  --tts-url http://127.0.0.1:8091/v1/audio/speech \
  --voice ono_anna \
  --force
```

注意：该命令会执行数据库迁移。当前迁移系统没有追踪机制，重复执行迁移可能再次插入种子词；优先使用已运行的管理后台 UI/API，或在执行后按项目已知修复 SQL 清理重复词。

---

## 5. API 路由参考

所有接口需带 `Authorization: Bearer <token>` 请求头。

### 单词

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/words?level=&search=&page=&size=` | 列表，支持 JLPT 级别筛选、文本搜索、分页 |
| POST | `/api/admin/words` | 新增单词 |
| PUT | `/api/admin/words/{id}` | 更新单词 |
| DELETE | `/api/admin/words/{id}` | 删除单词 |

### 语法

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/grammar?level=&search=&page=&size=` | 列表，支持筛选/搜索/分页 |
| POST | `/api/admin/grammar` | 新增语法点 |
| PUT | `/api/admin/grammar/{id}` | 更新语法点 |
| DELETE | `/api/admin/grammar/{id}` | 删除语法点 |

### 口语

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/speaking?level=&type=&page=&size=` | 列表 |
| POST | `/api/admin/speaking` | 新增口语材料 |
| PUT | `/api/admin/speaking/{id}` | 更新 |
| DELETE | `/api/admin/speaking/{id}` | 删除 |

### 写作

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/writing?level=&type=&page=&size=` | 列表 |
| POST | `/api/admin/writing` | 新增写作题 |
| PUT | `/api/admin/writing/{id}` | 更新 |
| DELETE | `/api/admin/writing/{id}` | 删除 |

### 翻译

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/translation?source_id=&direction=&page=&size=` | 列表 |
| POST | `/api/admin/translation` | 新增句子 |
| PUT | `/api/admin/translation/{id}` | 更新 |
| DELETE | `/api/admin/translation/{id}` | 删除 |

### 用户

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/users?page=&size=` | 用户列表 |
| GET | `/api/admin/users/{id}/stats` | 用户学习统计 |

### 记录

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/admin/records/{module}?user_id=&page=&size=` | module = `word` / `grammar` / `speaking` / `writing` |

### 批量导入

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/admin/import/{module}` | multipart/form-data，字段名 `file`；module = `words` / `grammar` / `speaking` / `writing` |

返回：`{"inserted": N}`

### 批量语音

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/admin/audio/batch` | JSON body；module = `words` / `grammar` / `speaking`，支持 `force`、`level`、`type`、`word_ids`、`texts` |
| POST | `/api/admin/audio/regen` | JSON body；单条重新生成音频，module = `word` / `example` |

---

## 6. 已有种子数据位置

如果需要参考现有数据格式，可以查看：

```
data/seed/
├── words_n5.json              # N5 单词（60条）
├── grammar_n5.json            # N5 语法
├── speaking_materials.json    # 口语材料
├── writing_questions.json     # 写作题目
└── lessons_n5.json            # 课文（暂无管理界面）
```
