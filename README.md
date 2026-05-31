# Japanese Learning App

面向「边上班边学日语」人群的 Web 应用，覆盖 JLPT N5～N1 全级别。支持单词记忆（SM-2 间隔重复）、语法学习、课文阅读（振り仮名）、口语练习和 AI 写作批改。

## 技术栈

| 维度 | 选型 |
|---|---|
| 后端 | Go 1.22 + 标准库 `net/http` |
| 数据库 | SQLite（`modernc.org/sqlite`，纯 Go 实现） |
| 前端 | React 18 + TypeScript + Vite |
| 认证 | JWT（HMAC-SHA256） |
| AI | Anthropic Claude API（写作批改，可选） |
| TTS | Qwen3-TTS (vLLM) / style-bert-vits2 (FastAPI) — 单词/例句日语语音合成 |
| i18n | i18next + react-i18next（zh / ja / en） |

## 快速开始

### 环境要求

- Go ≥ 1.22
- Node.js ≥ 18（前端开发）

### 启动后端

```bash
# 最简启动（默认监听 :8080）
make run

# 或指定配置
JWT_SECRET=your-secret DB_PATH=./data/app.db go run ./backend/cmd/server/
```

### 导入种子数据

```bash
make seed-all    # 导入全部（词汇 + 语法 + 课文 + 口语 + 写作）
make seed        # 仅导入词汇（N5/N4）
```

### CLI 数据导入

服务器二进制提供了多个 `import-*` 子命令，用于批量或单条导入数据。

```bash
# 构建服务器
go build -o ./server ./backend/cmd/server/

# 导入单词
./server import-words --file ./data/seed/words_n5.json

# 导入单词 + 自动生成语音
./server import-words --file ./data/seed/words_n5.json --generate-audio=sbv
./server import-words --file ./data/seed/words_n5.json --generate-audio=vllm --tts-url=http://localhost:8091/v1/audio/speech

# 导入语法 / 课文 / 口语 / 写作
./server import-grammar --file ./data/seed/grammar_n5.json
./server import-lessons --file ./data/seed/lessons_n5.json
./server import-speaking --file ./data/seed/speaking_materials.json --generate-audio=sbv
./server import-writing --file ./data/seed/writing_questions.json
```

**`import-words` 选项:**

| 选项 | 默认值 | 说明 |
|---|---|---|
| `--file` | (必填) | JSON 文件路径，文件内容为单词对象数组 |
| `--db` | `./data/app.db` | SQLite 数据库路径 |
| `--auto-fill` | `false` | 使用 kagome 形态分析自动填充缺失字段 |
| `--generate-audio` | `""` | 导入后自动生成单词语音：`vllm` 或 `sbv` |
| `--provider` | `vllm` | TTS provider（与 `--generate-audio` 配合使用） |
| `--tts-url` | `http://...` | vLLM TTS 端点 |
| `--tts-model` | `Qwen/...` | TTS 模型名 |
| `--voice` | `ono_anna` | 语音名称 |
| `--sbv-url` | `http://127.0.0.1:7862` | style-bert-vits2 服务地址 |
| `--sbv-model` | `amitaro` | style-bert-vits2 模型名 |
| `--sbv-speaker` | `あみたろ` | style-bert-vits2 说话人 |
| `--sbv-style` | `Neutral` | style-bert-vits2 风格 |
| `--out` | `./data/audio/words` | 音频输出目录 |

**单词 JSON 格式:**

```json
[
  {
    "kanji_form": "経験",
    "reading": "",
    "part_of_speech": "",
    "meaning": "experience",
    "jlpt_level": "N4",
    "reading_type": ""
  }
]
```

必需字段：`kanji_form`、`meaning`、`jlpt_level`。其余字段为空时可启用 `--auto-fill` 自动补全。

**仅有关键字段时自动补全（`--auto-fill`）:**

当你拿到的单词数据只有 `kanji_form` 和 `meaning` 时，`--auto-fill` 会调用 kagome 日语形态分析器自动补全以下字段：

- **reading** — 假名注音（片假名自动转为平假名）
- **part_of_speech** — 词性（名詞 / 動詞 / 形容詞 等）
- **reading_type** — 读音类型（1=音読み / 2=訓読み / 6=其他）

已有值的字段不会被覆盖（仅填充空字段）。

```bash
# 示例：导入只含 kanji_form + meaning 的 N4 词汇
./server import-words --file ./data/n4_words.json --auto-fill
```

示例 JSON（`n4_words.json`）:

```json
[
  {"kanji_form": "経験", "meaning": "experience", "jlpt_level": "N4"},
  {"kanji_form": "文化", "meaning": "culture", "jlpt_level": "N4"},
  {"kanji_form": "病院", "meaning": "hospital", "jlpt_level": "N4"},
  {"kanji_form": "薬", "meaning": "medicine", "jlpt_level": "N4"},
  {"kanji_form": "集める", "meaning": "to collect", "jlpt_level": "N4"},
  {"kanji_form": "返事", "meaning": "reply, response", "jlpt_level": "N4"}
]
```

> **注意：** `reading_type` 对于单个汉字且无送假名的单词（如「薬」），无法区分音读/训读，会标记为「6（其他）」。导入后建议快速检查，手动校准少数词条。

### 启动前端

```bash
cd front/react
npm install
npm run dev      # http://localhost:5173，/api 代理至 :8080
```

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP 监听地址 |
| `DB_PATH` | `./data/app.db` | SQLite 数据库路径 |
| `JWT_SECRET` | `change-me-in-production` | 生产环境必须修改 |
| `LOG_LEVEL` | `INFO` | DEBUG / INFO / WARN / ERROR |
| `AI_API_KEY` | `""` | Claude API 密钥（空则使用 Stub 评分） |
| `SMTP_HOST` | `""` | SMTP 服务器（密码重置邮件，空则使用 Stub） |

### TTS 语音合成

支持两种 TTS 后端，通过 `--provider` 参数切换。

#### 方案 A：Qwen3-TTS（vLLM）

适合通用中文/日语语音合成，部署后 API 即用，无需加载模型。

**启动服务：**

```bash
docker run -d \
  --name qwen-tts \
  --gpus all \
  --network=host \
  --ipc=host \
  -e HF_ENDPOINT=https://hf-mirror.com \
  -e HF_HOME=/root/.cache/huggingface \
  -e HUGGINGFACE_HUB_CACHE=/root/.cache/huggingface \
  -e TRANSFORMERS_VERBOSITY=debug \
  -e HF_HUB_VERBOSITY=debug \
  -v /home/tylerhu/.cache/huggingface:/root/.cache/huggingface \
  vllm/vllm-omni:v0.20.0 \
  vllm serve /root/.cache/huggingface/models--Qwen--Qwen3-TTS-12Hz-1.7B-CustomVoice/snapshots/0c0e3051f131929182e2c023b9537f8b1c68adfe \
  --omni \
  --port 8091 \
  --trust-remote-code \
  --enforce-eager \
  --gpu-memory-utilization 0.85
```

**生成单词语音：**

```bash
go run ./backend/cmd/server/ generate-word-audio \
  --provider=vllm \
  --tts-url=http://localhost:8091/v1/audio/speech \
  --tts-model="Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice"

# 仅 N5，强制覆盖
go run ./backend/cmd/server/ generate-word-audio --provider=vllm --level N5 --force
```

#### 方案 B：style-bert-vits2（FastAPI）

日语专精，支持情感风格控制（Neutral/Angry/Happy/Sad 等），发音更自然。

**启动服务：**

```bash
cd /home/tylerhu/Style-Bert-VITS2
nohup venv/bin/python server_fastapi.py --cpu > /tmp/tts_server.log 2>&1 &
```

> 如果 GPU 正常（PyTorch ≥ 2.5 + CUDA ≥ 12.5），去掉 `--cpu` 并将 `config.yml` 中 `server.device` 设为 `cuda` 可大幅提速。

服务默认监听 `http://localhost:7862`，API 文档：`http://localhost:7862/docs`

**生成单词语音：**

```bash
# 生成全部级别的单词
go run ./backend/cmd/server/ generate-word-audio \
  --provider=sbv \
  --sbv-url=http://127.0.0.1:7862 \
  --sbv-model=amitaro \
  --sbv-speaker=あみたろ

# 仅生成 N5，强制覆盖已有音频
go run ./backend/cmd/server/ generate-word-audio \
  --provider=sbv \
  --sbv-url=http://127.0.0.1:7862 \
  --sbv-model=amitaro \
  --sbv-speaker=あみたろ \
  --level=N5 --force

# 换其他模型/风格（更多风格查看 http://localhost:7862/models/info）
go run ./backend/cmd/server/ generate-word-audio \
  --provider=sbv \
  --sbv-url=http://127.0.0.1:7862 \
  --sbv-model=jvnv-F1-jp \
  --sbv-speaker=jvnv-F1-jp \
  --sbv-style=Happy
```

**通用选项（两个 provider 都支持）：**

| 选项 | 说明 |
|---|---|
| `--level` | 仅生成指定 JLPT 级别（N5/N4/N3/N2/N1） |
| `--dry-run` | 预览将要生成的单词，不实际调用 TTS |
| `--force` | 强制重新生成已存在的音频文件 |
| `--out` | 音频输出目录（默认 `./data/audio/words`） |
| `--db` | SQLite 数据库路径（默认 `./data/app.db`） |

**生成例句/口语语音：**

```bash
# 为所有例句和口语材料生成语音
go run ./scripts/generate_tts_examples/ \
  --db ./data/app.db \
  --tts-url http://localhost:8091/v1/audio/speech \
  --out ./data/audio/examples
```

语音文件存储在 `data/audio/words/` 和 `data/audio/examples/`，数据库 `words.audio_url` 记录文件路径。

## 功能模块

| 模块 | 说明 |
|---|---|
| **单词记忆** | SM-2 间隔重复算法，闪卡翻转复习，按 JLPT 等级筛选 |
| **语法学习** | N5-N1 语法点 + 即时测验（填空/选择），学习状态追踪 |
| **课文阅读** | 振り仮名（ruby）标注，中日翻译切换，已读标记 |
| **口语练习** | 影子跟读 + 自由朗读，波形评分 0-100 |
| **写作练习** | 输入练习 + AI 造句批改（语法/词汇/修正建议） |
| **学习总结** | 会话记录与聚合分析（强项/弱项/建议） |

## 项目结构

```
├── backend/cmd/server/    # 服务入口
├── internal/
│   ├── cli/               # CLI 子命令（导入种子数据等）
│   ├── config/            # 环境变量配置
│   ├── data/              # 数据访问层（Store + 迁移）
│   ├── httputil/          # HTTP 响应工具
│   └── module/            # 业务模块（word/grammar/lesson/speaking/writing/user/summary）
├── front/
│   └── react/             # React SPA（当前主前端）
├── data/seed/             # 种子数据 JSON
├── specs/                 # 需求文档
└── docs/architecture.md   # 架构文档
```

## Makefile 速查

```bash
make run           # 启动开发服务器
make build         # 编译 bin/server
make test          # 运行所有测试（含集成测试）
make lint          # 静态分析
make seed-all      # 导入全部种子数据
make front-build   # 编译前端 TypeScript
make clean         # 清理构建产物
```

## 运行测试

```bash
make test                        # 全部测试
go test ./internal/module/word/... -v   # 指定包
go test ./internal/data/... -run TestWordStore_GetByID -v  # 指定用例
```

测试策略：表格驱动 + 真实 SQLite（不使用 Mock），禁用缓存（`-count=1`）。
