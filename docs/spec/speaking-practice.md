# 口语练习功能设计文档（v1）

## 概述

口语模块 v1：素材浏览 → TTS 参考朗读 + 高亮跟读 → 录音 → 浏览器语音识别评分 → 历史记录。

**v1 核心原则**：纯前端完成核心交互（TTS / 录音 / 评分），后端负责数据 CRUD。后续 v2 接入 vLLM + Whisper 做服务端精准评分（详见[第 3 节](#3-v2-自动评分方案vllm-部署)）。

---

## 1. API 设计

### 1.1 GET /api/v1/speaking/materials —— 素材列表

**参数**：`type`（可选 `shadow|free`）、`level`（可选 `N5|N4|N3`），均为可选，不传则返回全部。

**响应**：
```json
{
  "data": [{
    "id": 1,
    "type": "shadow",
    "title": "挨拶の基本",
    "text": "おはようございます。今日もよろしくお願いします。",
    "audio_url": "",
    "jlpt_level": "N5"
  }]
}
```

### 1.2 GET /api/v1/speaking/materials/{id} —— 素材详情

同上，单个对象。

### 1.3 POST /api/v1/speaking/practice —— 修改评估

**当前问题**：接口要求上传 `reference_audio` + `user_audio` 两个文件，走服务端波形评分。但素材无预录音频，浏览器 TTS 生成的音频无法直接捕获为 PCM。

**v1 方案**：修改为接受 JSON body，score 由前端计算好传入，音频用 `audio_ref` 字符串（blob URL 或空字符串）标记即可，不上传音频文件。

```json
// Request
{
  "type": "shadow",
  "material_id": 1,
  "score": 85,
  "recognition_text": "おはようございます。きょうもよろしくおねがいします。"
}

// Response
{
  "data": {
    "id": 1,
    "user_id": 7,
    "type": "shadow",
    "material_id": 1,
    "score": 85,
    "practiced_at": "2026-05-14T..."
  }
}
```

> 注意：当前 `SpeakingRecord.Score` 是 `int`，`AudioRef` 是 `string`。v1 不上传音频文件，`audio_ref` 存空字符串。后续 v2 若有需要再扩展。

### 1.4 GET /api/v1/speaking/records —— 不变

已有接口，无需改动。

### 1.5 路由冲突检查

新增路由 `GET /api/v1/speaking/materials` 需要放在 `GET /api/v1/speaking/materials/{id}` 之前注册，避免 `materials` 被解析为 `{id}`。Go 1.22+ ServeMux 按字面量优先匹配，`/api/v1/speaking/materials` 不会冲突。

---

## 2. 前端评分方案（v1 浏览器 SpeechRecognition）

### 2.1 原理

```
用户录音 → webkitSpeechRecognition 转文字 → 与素材原文逐字对比 → 计算匹配率
```

### 2.2 具体步骤

1. 用户录音结束，拿到 Blob
2. 创建 `webkitSpeechRecognition` 实例，设置 `lang = 'ja-JP'`
3. 将录音 Blob 转为 audio 元素播放，同时开启 recognition 识别
   - **注意**：`SpeechRecognition` 只能监听麦克风实时输入，无法对已有 Blob 做识别
   - **替代方案**：录音时同时开启 recognition，实时获取识别文本，录完后汇总
4. 拿到识别文本（假名混合汉字），用 MeCab/kuromoji 转为纯假名
5. 将素材原文也转为纯假名
6. 计算编辑距离 / 匹配率 → 得出 0-100 分

### 2.3 风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| `webkitSpeechRecognition` 仅 Chrome 支持 | Firefox/Safari 不可用 | 显示降级提示"请使用 Chrome 浏览器" |
| 识别准确率受环境噪音影响 | 评分偏低 | 安静环境提示 + 这不是考试，只是辅助 |
| 实时识别与录音同步 | 时序复杂 | 可以简化：录完后再播放录音，用 recognition 监听扬声器输出... 这也不靠谱 |
| ℃ 转为假名后的对比不精确 | 得分不完全反映发音质量 | v1 可接受，v2 用 vLLM + Whisper 替换 |

### 2.4 简化方案（推荐）

> **放弃 SpeechRecognition，v1 不做自动评分。用户自评即可。**

理由：
- `SpeechRecognition` 只能识别麦克风实时输入，无法对录音文件做离线识别
- 要在录音同时获取识别结果，时序复杂且不可靠
- 识别文本和原文对比需要额外的假名转换和编辑距离计算

**v1 替代方案**：用户录音 → 回放对比 → 自行判断发音是否接近 → 手动打分（1-5 星 或 简单/良好/优秀）→ 保存记录。

这样：
- 不需要改 POST practice 接口的评分逻辑
- 保留 `score` 字段，范围 1-100，前端把星级转为分数（1星=20分, 5星=100分）
- 后续 v2 接入服务端自动评分时（见[第 3 节](#3-v2-自动评分方案vllm-部署)），前端无需改动

---

## 3. v2 自动评分方案（vLLM 部署）

### 3.1 为什么用 vLLM

- vLLM v0.10+ 原生支持 Whisper ASR 模型，暴露 OpenAI 兼容的 `/v1/audio/transcriptions` API
- HF 官方实测 vLLM 部署 Whisper Large V3 获 **8x 推理加速**（CUDA Graphs + torch.compile + float8 KV Cache）
- 支持 INT4/INT8 量化，降低 GPU 显存需求
- 与 Go 后端解耦：Go 通过 HTTP 调用 vLLM API，无需 Python sidecar
- 后续可扩展部署 LLM（如评分反馈生成）在同一 vLLM 实例上

### 3.2 整体架构

```
用户录音 → Go 后端保存音频 → 调用 vLLM API → ASR 识别文本
                                              → 与原文计算 CER/WER
                                              → 映射 0-100 分 → 存入 DB
                                              → 返回评分结果
```

Go 后端只做文本对比和评分计算，ASR 推理完全交给 vLLM。

### 3.3 ASR 模型选型

所有候选模型均为 Whisper 架构（vLLM 原生支持）：

| 模型 | vLLM 兼容 | 参数量 | 日语性能 | 推荐场景 |
|------|----------|--------|----------|----------|
| `openai/whisper-large-v3-turbo` | 官方已验证 | 809M | 通用多语言 | 首选（最稳） |
| `kotoba-tech/kotoba-whisper-v2.0` | distil-whisper 架构，理论兼容 | 756M | ReazonSpeech 720万条训练，日语专项优化 | 日语精度优先 |
| `nm-testing/whisper-large-v3.w4a16` | INT4 量化版 | 809M | 同 large-v3 | 显存受限 |

> **推荐**：优先用 `whisper-large-v3-turbo`（vLLM 官方验证过，最稳定），日语精度不够再换 `kotoba-whisper-v2.0`。

### 3.4 vLLM 部署

```bash
# 安装（带音频支持）
pip install -U vllm[audio]

# 启动 vLLM ASR 服务
vllm serve openai/whisper-large-v3-turbo \
    --served-model-name whisper \
    --task transcription \
    --gpu-memory-utilization 0.9 \
    --max-model-len 448 \
    --host 0.0.0.0 \
    --port 8000

# 验证
curl -X POST "http://localhost:8000/v1/audio/transcriptions" \
  -F file="@sample.wav" \
  -F model="whisper" \
  -F language="ja" \
  -F response_format="text"
```

若换用 kotoba-whisper-v2.0：
```bash
vllm serve kotoba-tech/kotoba-whisper-v2.0 \
    --served-model-name whisper \
    --task transcription \
    --trust-remote-code \
    --gpu-memory-utilization 0.9 \
    --host 0.0.0.0 --port 8000
```

### 3.5 Go 后端改动（v2.0）

在 `internal/module/speaking/` 下新增 `scorer.go`：

```go
// Scorer 调用 vLLM 做 ASR + 文本对比评分
type Scorer struct {
    vllmURL string // http://localhost:8000/v1/audio/transcriptions
}

// Score 上传音频到 vLLM 识别，与参考文本对比返回 0-100 分
func (s *Scorer) Score(ctx context.Context, audio []byte, referenceText string) (int, string, error) {
    // 1. POST multipart/form-data 到 vLLM
    // 2. 拿到识别文本 recognitionText
    // 3. 转为假名后计算 CER
    // 4. CER → 分数映射：score = max(0, 100 - CER*100)
}
```

Go 调用 vLLM 的代码示例（标准库 `mime/multipart`，零外部依赖）：

```go
func (s *Scorer) transcribe(ctx context.Context, audio []byte) (string, error) {
    var buf bytes.Buffer
    w := multipart.NewWriter(&buf)
    fw, _ := w.CreateFormFile("file", "audio.webm")
    fw.Write(audio)
    w.WriteField("model", "whisper")
    w.WriteField("language", "ja")
    w.WriteField("response_format", "text")
    w.Close()

    req, _ := http.NewRequestWithContext(ctx, "POST", s.vllmURL, &buf)
    req.Header.Set("Content-Type", w.FormDataContentType())
    resp, err := http.DefaultClient.Do(req)
    // ... 读取识别文本
}
```

### 3.6 发音评分升级路径

v2 自动评分分两阶段：

| 阶段 | 方案 | 模型 | 评分维度 |
|------|------|------|----------|
| **v2.0** | ASR + CER 文本对比 | vLLM + Whisper | 识别文本与原文的字符匹配率 |
| **v2.1** | CTC-based GOP 音素评分 | Japanese HuBERT (独立部署) | 逐音素发音质量 |

v2.1 的 GOP 模型（HuBERT/CTC）无法走 vLLM（vLLM 目前不支持 encoder-only 音素识别模型），需要独立的 Python 推理服务。v2.1 的部署方式后续再定。

### 3.7 部署拓扑（v2.0）

```
┌─────────────────────────────────────────────────┐
│  服务器                                          │
│  ┌──────────┐    HTTP     ┌───────────────────┐ │
│  │ Go 后端  │ ←─────────→ │ vLLM (port 8000)  │ │
│  │ :8081    │  multipart  │ Whisper ASR       │ │
│  │          │  /v1/audio  │ GPU: 4GB+ VRAM    │ │
│  └──────────┘             └───────────────────┘ │
└─────────────────────────────────────────────────┘
```

- Go 和 vLLM 部署在同一台机器，内网 HTTP 通信，延迟 < 100ms
- vLLM 单 GPU 可支持并发多请求（PagedAttention），满足单用户学习场景
- 若 GPU 资源紧张，Whisper Large V3 Turbo INT4 量化版仅需 ~1.5GB VRAM

---

## 4. 前端 UI

### 4.1 页面结构

```
SpeakingPage.tsx
├── 筛选栏
│   ├── [影子跟读] [自由朗读]
│   └── [N5] [N4] [N3]
│
├── 素材列表（默认视图）
│   └── 卡片: 标题 | 文本预览 | 类型标签 | 难度标签
│
├── 练习面板（选中素材后展开）
│   ├── 课文展示区
│   │   └── 逐句显示，带假名注音（furigana）
│   ├── 工具栏
│   │   ├── 🔊 播放参考音频（TTS + 逐字高亮）
│   │   ├── 🎙️ 录音 / ⏹ 停止
│   │   ├── ▶️ 回放录音
│   │   └── 🔄 重新录制
│   ├── 自评区
│   │   └── ⭐⭐⭐⭐⭐ 星级评分
│   └── 📤 提交 → 显示提交成功 + 分数
│
└── 练习历史（可折叠）
    └── 每次: 日期 | 素材标题 | 分数 | 类型标签
```

### 4.2 影子跟读 vs 自由朗读

| | 影子跟读（shadow） | 自由朗读（free） |
|------|------|------|
| **参考音频** | TTS 逐句播放 + 高亮 | 无（仅展示文本） |
| **录音方式** | 跟读，逐句或整段录 | 自由朗读整段录 |
| **评分** | 自评 | 自评 |
| **UI 差异** | 高亮条跟随 TTS | 静态文本展示 |

### 4.3 高亮跟读实现

**方案**：JSX 逐 token 渲染 + `onboundary` 事件

```ts
// 1. 将素材 text 按 token 解析，每个 token 包含 surface + reading
//    复用内部 kagome 分词逻辑（前端用 kuromojin）
const tokens = await toFuriganaTokens(text)
// tokens: [{surface: "今日", reading: "きょう"}, {surface: "も", reading: ""}, ...]

// 2. JSX 渲染: kanji 用 <ruby>，kana 用 <span>，当前朗读位置加 .highlight class
tokens.map((t, i) => (
  <span key={i} className={i === highlightIdx ? styles.highlight : ''}>
    {t.reading ? <ruby>{t.surface}<rt>{t.reading}</rt></ruby> : t.surface}
  </span>
))

// 3. TTS onboundary 计算当前 token 索引
const u = new SpeechSynthesisUtterance(text)
u.onboundary = (e) => {
  // charIndex 对应原文 character 位置，映射到 token 索引
  const idx = charIndexToTokenIndex(e.charIndex, tokens)
  setHighlightIdx(idx)
}
```

**关键依赖**：`front/react/src/util/furigana.ts` 已有 `toFuriganaHTML` 函数（基于 kuromojin），需要新增一个 `toFuriganaTokens` 函数返回结构化 token 数组，供 JSX 渲染。

**`charIndex → token index` 映射**：token 数组已知每个 token 的 surface 长度，累加即可定位。

### 4.4 录音功能

复用 `hooks/useAudioRecorder.ts`，需要小改：

- `start()` / `stop()` 返回 Blob
- 新增 `audioURL` state 用于 `<audio>` 回放
- 新增"重新录制"：释放旧 URL，重置 state

---

## 5. 后端改动

| 文件 | 改动 |
|------|------|
| `internal/module/speaking/model.go` | 新增 `SpeakingMaterial` 结构体 |
| `internal/data/speaking_store.go` | 新增 `ListMaterials` + `GetMaterialByID` |
| `internal/module/speaking/service.go` | 接口 + 实现新增方法 |
| `internal/module/speaking/handler.go` | 新增 2 个 GET 路由 + 修改 POST practice 接受 JSON body |

**POST practice 改动说明**：

当前 `handlePractice` 接受 multipart/form-data，需要改为接受 JSON。处理方式：
- 检查 `Content-Type`：若是 `multipart/form-data`，走旧逻辑（兼容）；若是 `application/json`，走新逻辑
- 或者直接改为 JSON-only（v1 不需要音频上传，旧接口也没人用）

**推荐直接改为 JSON-only**，更简洁。旧逻辑保留在代码历史中。

### 5.1 SpeakingMaterial 结构体

```go
type SpeakingMaterial struct {
    ID        int64  `json:"id"`
    Type      string `json:"type"`
    Title     string `json:"title"`
    Text      string `json:"text"`
    AudioURL  string `json:"audio_url"`
    JLPTLevel string `json:"jlpt_level"`
}
```

### 5.2 Store 新增方法

```go
func (s *SpeakingStore) ListMaterials(practiceType, level string) ([]speaking.SpeakingMaterial, error)
func (s *SpeakingStore) GetMaterialByID(id int64) (*speaking.SpeakingMaterial, error)
```

SQL 参数化，可选过滤用条件拼接。

---

## 6. 前端改动

| 文件 | 改动 |
|------|------|
| `pages/speaking/SpeakingPage.tsx` | 重写：素材列表 + 练习面板 + 自评 + 历史 |
| `pages/speaking/SpeakingPage.module.css` | 新增样式 |
| `util/furigana.ts` | 新增 `toFuriganaTokens(text) → Token[]` 函数 |
| `types/api.ts` | 新增 `SpeakingMaterial` 类型 |
| `i18n/locales/*.ts` | 新增翻译键 |

### 6.1 i18n 新增键

```ts
speaking: {
  // ...existing
  materials: {
    title: '选择练习素材',
    empty: '该等级暂无素材',
  },
  practice: {
    playRef: '播放示范',
    startRecord: '开始录音',
    stopRecord: '停止录音',
    playback: '回放',
    reRecord: '重新录制',
    submit: '提交',
    selfRate: '自我评分',
    submitted: '已保存',
    chromeOnly: '请使用 Chrome 浏览器以获得完整功能',
  },
}
```

---

## 7. 潜在问题检查

### 7.1 `toFuriganaTokens` 的 kuromojin 依赖
- `front/react/src/util/furigana.ts` 已经 import 了 kuromojin（`toFuriganaHTML` 在用）
- 新增 `toFuriganaTokens` 复用同一个 tokenizer，无新依赖
- **确认**：语法例句目前用 `dangerouslySetInnerHTML` + `toFuriganaHTML`，后续可以统一改用 token 渲染

### 7.2 浏览器兼容性
- TTS（`SpeechSynthesis`）：Chrome / Firefox / Safari / Edge 均支持 ✓
- 录音（`MediaRecorder`）：Chrome / Firefox / Edge 支持，Safari 14.5+ ✓
- 高亮（`onboundary`）：Chrome / Safari 支持，Firefox 不支持 `charIndex`（已知 bug）
  - Firefox 降级：播放 TTS 但不高亮，只显示静态文本

### 7.3 WebM 录音格式
- `MediaRecorder` 默认输出 `audio/webm`，`<audio>` 标签可直接播放 ✓
- v1 不上传音频到后端，格式不影响

### 7.4 移动端体验
- `npx vite --host 0.0.0.0` 已在运行，手机可通过局域网 IP 访问
- 移动端 Chrome 支持 TTS + MediaRecorder
- iOS Safari 需要用户手势才能启动 AudioContext（按钮点击触发录音满足此条件）

### 7.5 评分精度问题的诚实态度
- v1 自评模式：用户自行判断，分数是主观的
- 这不是考试系统，是学习辅助工具，自评完全可以接受
- v2 接入服务端模型自动评分，详见 [第 3 节](#3-v2-自动评分方案vllm-部署)

---

## 8. 实施顺序

| # | 步骤 | 说明 |
|---|------|------|
| 1 | 后端 Materials API | model + store + service + handler |
| 2 | `toFuriganaTokens` 工具函数 | furigana.ts 新增 |
| 3 | 前端类型 + i18n | types/api.ts + locales |
| 4 | SpeakingPage 素材列表 | 筛选 + 卡片列表 + 选择 |
| 5 | SpeakingPage 练习面板 | 课文展示 + TTS + 高亮 + 录音 + 回放 |
| 6 | 自评 + 提交 + 历史 | 星级评分 → POST → 刷新记录 |
| 7 | 联调测试 | Chrome + 移动端 |
