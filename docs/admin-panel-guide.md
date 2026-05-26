# 管理后台使用指南

## 1. 概述

管理后台是一个独立部署的 Web 应用，用于管理日语学习内容（单词、语法、口语、写作、翻译）和查看用户数据/学习记录。

- **后端**: Go 二进制，监听 `:8082`，复用主应用的 SQLite 数据库
- **前端**: React SPA，开发服务器 `:5174`
- **认证**: 通过环境变量 `ADMIN_TOKEN` 设置 token，前端登录页输入相同 token

### 启动方式

```bash
# 后端（二选一）
ADMIN_TOKEN=your-password go run ./backend/cmd/admin/
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

## 4. API 路由参考

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

---

## 5. 已有种子数据位置

如果需要参考现有数据格式，可以查看：

```
data/seed/
├── words_n5.json              # N5 单词（60条）
├── grammar_n5.json            # N5 语法
├── speaking_materials.json    # 口语材料
├── writing_questions.json     # 写作题目
└── lessons_n5.json            # 课文（暂无管理界面）
```
