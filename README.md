# Japanese Learning App

面向「边上班边学日语」人群的 Web 应用，覆盖 JLPT N5～N1 全级别。支持单词记忆（SM-2 间隔重复）、语法学习、课文阅读（振り仮名）、口语练习和 AI 写作批改。

## 技术栈

| 维度 | 选型 |
|---|---|
| 后端 | Go 1.25 + 标准库 `net/http` |
| 数据库 | PostgreSQL 18.4（pgx） |
| 前端 | React 18 + TypeScript + Vite |
| 认证 | JWT（HMAC-SHA256） |
| AI | Anthropic Claude API（写作批改，可选） |
| TTS | Qwen3-TTS (vLLM) / style-bert-vits2 (FastAPI) — 单词/例句日语语音合成 |
| i18n | i18next + react-i18next（zh / ja / en） |

## 快速开始

### 端口对照

| 服务 | 端口 | 说明 |
|---|---:|---|
| 主后端 / 正式服务 | `30081` | `backend/cmd/server` 默认 `LISTEN_ADDR=:30081` |
| 学习端前端 | `35173` | React/Vite dev server，`/api` 和 `/audio` 代理到 `:30081` |
| 管理后台 API | `30082` | `backend/cmd/admin` 默认 `LISTEN_ADDR=:30082` |
| 管理后台前端 | `35174` | Admin React/Vite dev server |

`8080` 不是本项目当前默认端口；如果本机有 `8080` 监听，先确认进程来源再使用。

### Tailscale 访问入口

当前 Tailnet 后缀为 `king-shiner.ts.net`。Tailscale 域名约定如下：

| 环境 | Tailscale 域名 | 学习端前端 | 主后端/API | 管理后台前端 | 管理后台 API |
|---|---|---|---|---|---|
| 正式 | `tylerhu-1.king-shiner.ts.net` | `http://tylerhu-1.king-shiner.ts.net:35173` | `http://tylerhu-1.king-shiner.ts.net:30081` | `http://tylerhu-1.king-shiner.ts.net:35174` | `http://tylerhu-1.king-shiner.ts.net:30082` |
| 测试 | `tylerhu.king-shiner.ts.net` | `http://tylerhu.king-shiner.ts.net:35173` | `http://tylerhu.king-shiner.ts.net:30081` | `http://tylerhu.king-shiner.ts.net:35174` | `http://tylerhu.king-shiner.ts.net:30082` |

注意：这些是本应用的 Tailscale/MagicDNS 域名约定；实际外网访问需确保对应服务监听在可被 Tailscale 转发的地址，或额外配置 `tailscale serve` / `tailscale funnel`。当前 `tailscale funnel status` 中的 `/all-note`、`/all-note-test` 和 `:10000` 映射属于其他项目，不是本应用入口。

### 环境要求

- Go ≥ 1.25
- Node.js ≥ 18（前端开发）

### 启动后端

复制 `.env.example` 为 `.env.local`，配置应用库 `DATABASE_URL` 和独立测试库 `DATABASE_URL_TEST`。两者可以连接同一 PostgreSQL 实例，但数据库名必须不同。`.env.local` 和 `.pgpass.local` 已加入忽略列表。


```bash
# 最简启动（默认监听 :30081）
make run

# 或指定配置
JWT_SECRET=your-secret DATABASE_URL="$DATABASE_URL" go run ./backend/cmd/server/
```

### 导入种子数据

```bash
make seed-all    # 导入全部（词汇 + 语法 + 课文 + 口语 + 写作）
make seed        # 仅导入词汇（N5/N4）
```

### CLI 数据导入

`appctl` 提供多个 `import-*` 子命令，用于批量或单条导入数据。

```bash
# 构建导入工具
go build -o ./appctl ./backend/cmd/appctl/

# 导入单词
./appctl import-words --file ./data/seed/words_n5.json

# 导入单词 + 自动生成语音
./appctl import-words --file ./data/seed/words_n5.json --generate-audio=sbv
./appctl import-words --file ./data/seed/words_n5.json --generate-audio=vllm --tts-url=http://localhost:8091/v1/audio/speech

# 导入语法 / 课文 / 口语 / 写作
./appctl import-grammar --file ./data/seed/grammar_n5.json
./appctl import-lessons --file ./data/seed/lessons_n5.json
./appctl import-speaking --file ./data/seed/speaking_materials.json --generate-audio=sbv
./appctl import-writing --file ./data/seed/writing_questions.json
```

**`import-words` 选项:**

| 选项 | 默认值 | 说明 |
|---|---|---|
| `--file` | (必填) | JSON 文件路径，文件内容为单词对象数组 |
| `--database-url` | `DATABASE_URL` | PostgreSQL 连接地址 |
| `--auto-fill` | `false` | 使用 kagome 形态分析自动填充缺失字段 |
| `--generate-audio` | `""` | 导入后自动生成单词语音：`vllm`、`sbv` 或 `gradio` |
| `--provider` | `vllm` | TTS provider（与 `--generate-audio` 配合使用） |
| `--tts-url` | `http://...` | vLLM TTS 端点；`gradio` 时填写 Gradio 根地址 |
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
./appctl import-words --file ./data/n4_words.json --auto-fill
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
npm run dev      # http://localhost:35173，/api 和 /audio 代理至 :30081
```

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LISTEN_ADDR` | `:30081` | HTTP 监听地址 |
| `DATABASE_URL` | 必填 | 应用 PostgreSQL 连接地址 |
| `DATABASE_URL_TEST` | 必填（测试） | 独立 PostgreSQL 测试库，名称以 `_test` 结尾 |
| `JWT_SECRET` | `change-me-in-production` | 生产环境必须修改 |
| `LOG_LEVEL` | `INFO` | DEBUG / INFO / WARN / ERROR |
| `AI_API_KEY` | `""` | Claude API 密钥（留空时，主题写作会保存为未批改） |
| `RESEND_API_KEY` | `""` | Resend API Key；设置后密码重置邮件优先通过 Resend SMTP 发送 |
| `RESEND_FROM` | `""` | Resend 发件地址，如 `noreply@example.com`；为空时使用 `SMTP_FROM` |
| `SMTP_HOST` | `""` | SMTP 服务器（密码重置邮件，空则使用 Stub） |
| `SMTP_PORT` | `587` | SMTP 端口 |
| `SMTP_USER` | `""` | SMTP 用户名 |
| `SMTP_PASS` | `""` | SMTP 密码 |
| `SMTP_FROM` | `noreply@japanese-learning.app` | SMTP 发件地址 |
| `APP_BASE_URL` | `http://localhost:35173` | 密码重置链接前缀 |

### 二进制生产部署

生产环境使用 GitHub Actions 构建 Linux amd64 Go 二进制和前端静态文件，再通过 SSH/SCP 直接发布到服务器。服务器只需解压发布包并重启进程，不需要从镜像仓库下载镜像。配置要求和切换步骤见 [二进制部署文档](docs/binary-deployment.md)。

生产公网入口：

- 学习端：<https://japanese.jinrunlab.site>
- 管理端：<https://japanese-admin.jinrunlab.site>

部署流水线未配置 `APP_BASE_URL` 时默认使用学习端公网地址，确保密码重置邮件中的链接可以从外网访问。

#### Resend 密码重置邮件

本项目支持用 Resend 的免费额度发送真实密码重置邮件。先在 Resend 验证你的域名，并把 `RESEND_FROM` 设置为已验证域名下的发件地址。

PowerShell 示例：

```powershell
$env:RESEND_API_KEY="re_xxx"
$env:RESEND_FROM="noreply@your-domain.com"
$env:APP_BASE_URL="http://localhost:35173"
make run
```

设置 `RESEND_API_KEY` 后会优先使用 Resend SMTP；如果没有设置 Resend，但设置了 `SMTP_HOST`，则使用通用 SMTP；两者都没有设置时，后端会使用 `StubMailer`，只把重置链接打印到日志。

### TTS 语音合成

支持三种 TTS 后端，通过 `--provider` 参数切换。

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

#### 方案 C：Gradio `/run_instruct`

适合接入暴露了 `http://127.0.0.1:8000/gradio_api/info` 的 Gradio TTS 模型。当前适配 `/run_instruct`，字段为 `text`、`lang_disp`、`spk_disp`、`instruct`。

**生成单词语音：**

```bash
go run ./backend/cmd/server/ generate-word-audio \
  --provider=gradio \
  --tts-url=http://127.0.0.1:8000 \
  --voice=Vivian \
  --instructions="標準語で、自然にはっきり発音してください。"
```

**通用选项（三个 provider 都支持）：**

| 选项 | 说明 |
|---|---|
| `--level` | 仅生成指定 JLPT 级别（N5/N4/N3/N2/N1） |
| `--dry-run` | 预览将要生成的单词，不实际调用 TTS |
| `--force` | 强制重新生成已存在的音频文件 |
| `--out` | 音频输出目录（默认 `./data/audio/words`） |
| `--database-url` | PostgreSQL 连接地址，默认读取 `DATABASE_URL` |

**生成例句/口语语音：**

```bash
# 为所有例句和口语材料生成语音
go run ./scripts/generate_tts_examples/ \
  --database-url "$DATABASE_URL" \
  --tts-url http://localhost:8091/v1/audio/speech \
  --out ./data/audio/examples
```

语音文件存储在 `data/audio/words/` 和 `data/audio/examples/`，数据库 `words.audio_url` 记录文件路径。

**口语自动评分：** `POST /api/v1/speaking/evaluate` 复用课文影子跟读的本地评分引擎（无 ASR、无外部服务），需要参考音频：优先用材料的 `audio_url`，为空时按 `sha256(text)[:16].wav` 取 `data/audio/examples/` 下由上面命令生成的 TTS 文件。缺参考音频时返回 `ERR_SPEAKING_REFERENCE_MISSING`，页面回退到自评。

口语整段录音最多 **5 分钟 / 10 MiB**；前端使用 16 kHz 单声道 PCM16 WAV，5 分钟约占 9.2 MiB。参考音频和录音均支持最长 5 分钟的自动评分。

## 功能模块

| 模块 | 说明 |
|---|---|
| **单词记忆** | 按 JLPT 等级复习；「有印象」次日再练，「已完全掌握」至少 7 天后复习，连续掌握逐步延长间隔 |
| **语法学习** | N5-N1 语法点 + 即时测验（填空/选择），学习状态追踪 |
| **课文阅读** | 振り仮名（ruby）标注，中日翻译切换，已读标记 |
| **口语练习** | 影子跟读 + 自由朗读，录音后本地韵律评分（维度分 + 改进建议），无参考音频时自评兜底 |
| **写作练习** | N5～N1 共 88 个写作主题，每个主题可选「主题短文」或「情境造句」；短文由 AI 批改（语法/词汇/修正建议），支持草稿与练习进度 |
| **学习总结** | 会话记录与聚合分析（强项/弱项/建议） |
| **听力训练** | 教材音频盲听答题、逐句精听、关文本复测、难句收藏和间隔复习，完成后自动更新当天听力任务 |
| **学习计划** | 工作日最多三个学习时段，按星期展示当天任务并保存打卡；内置 N2 备考模板，支持 Apple 日历订阅，休息日不安排任务 |

### 写作主题库

主题由 `internal/data/migrations/postgres/schema/` 下的迁移维护：已发布迁移不可修改，扩充内容一律新增文件。当前 88 个主题覆盖 N5～N1，每个主题同时给出「主题短文」与「情境造句」两种任务及一个目标语法。题型参考公开资料补齐：

- **EJU 日本留学試験 記述問題**：二选一、400～500 字、「両方の立場に触れながら理由とともに述べる」，如本国是否举办国际大赛、大学是否需要长假、敬语、自动驾驶
- **高考日语 / 日语作文教材**：正反论证、图表说明、读后感、日记与贺年卡等应用文、5W1H 信息传达
- **意见文写法**：序論→本論→結論三段结构、事实与意见区分、文体统一（です・ます体 / だ・である体）、反论回应与具体例证

题目出处与资料链接见 `023_expand_writing_topics_sakubun.sql` 头部注释；主题完整性、级别分布与题型覆盖由 `internal/module/writing/topics_expansion_test.go` 锁定。

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
make test-db   # 在同一 PostgreSQL 实例上创建/检查独立测试库（不操作应用库）
make test      # 全部 Go 测试，数据库连接失败时直接失败
make web       # 构建主服务
make admin-build
cd front/react && npm test && npm run build
```

`DATABASE_URL_TEST` 必须与 `DATABASE_URL` 使用不同数据库，名称以 `_test` 结尾或包含 `_test_`。每个集成测试使用独立临时 schema，结束后只清理该 schema。CI 使用 PostgreSQL 18.4 服务执行相同测试。表格驱动测试优先，禁用测试缓存（`-count=1`）。
