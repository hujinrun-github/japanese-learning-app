# 影子跟读评分 · 纯本地方案实施计划

> 本文是 `docs/spec/shadowing-scoring-redesign.md` 的实施细化，聚焦 **P0/P1 不依赖任何外部付费发音评测 API** 的落地路径。任务用 `- [ ]` 跟踪，按 TDD 逐条完成。

> **进度（2026-11-16）**：S1 音频底座、S2 最小闭环、S3 前端（`useWavRecorder` + `ScoreCard` + `ShadowingPage` 接线）均已完成并验收通过；**真实浏览器麦克风录音联调也已在 headless Chrome 全流程跑通**（录音 → 16 kHz WAV → multipart → 本地评分 → ScoreCard 显示 72 分），并据此修掉一个真实生产缺陷。后端 `go test ./...` / `go vet ./...` / `go build ./...` 全绿且真实种子音频 curl 端到端已跑通；前端 `vitest` 19 文件 168 测试、`tsc --noEmit`、`vite build` 全绿。另按用户要求补了「评分后给出具体修改建议」：新增证据驱动的 `BuildAdvice` 建议层（§16）。S4–S8 待实施。详见 §13（S1/S2）、§14（S3）、§15（浏览器联调）、§16（建议层）。

- 日期：2026-11-16
- 上位约束：`constitution.md`（简单性、TDD/表格驱动、标准库优先、错误包装 + 日志）、`AGENTS.md`
- 技术栈：Go 1.24 标准库、`net/http`、`database/sql`、PostgreSQL + MinIO（已有）、React 18 / TypeScript（已有）
- 明确不做：Azure/任何云评测 API、LLM 生成反馈、wav2vec2/MFA 模型服务、外购声调词典

---

## 0. 先说清「本地方案」的能力边界

**能做到的**（用课程自带的标准音频作参照）：
- 用户的**节奏、同步延迟、句调轮廓、语速、停顿、完整度**与标准音的相对相似度
- 静音 / 原音串音 / 读错句 / 置信度不足的**不可评判定**
- 可解释的分项分 + 定位到时间的中文反馈 + 掌握度

**做不到的**（不要对外承诺）：
- **绝对意义的「这个词的アクセント型读错了」**：没有调型真值。本地算的是「F0 形状像不像参考音」，不是「第 2 拍该下降」。
- **音素级发音判错**（如「が」读成「か」）：vLLM Whisper 只给转写文本，不给音素后验。CER 只能作为「ASR 有没有听懂」的代理。
- 因此 repeat 模式的 `Segmental` 在 P0 只能给粗粒度分；要精确到音素，才需要 P2 的 Azure/SpeechSuper 或自建 GOP，那是另一条路。

**一句话**：本地方案评的是「像不像」，不是「错在哪一个音」。这恰好覆盖了影子跟读真正该奖的超音段维度（见设计文档 §2.2 的 44 项研究综述）。

### 两条起步路径（取决于 vLLM Whisper ASR 是否已有实例）

| | 有 ASR 实例 | 无 ASR 实例 |
| --- | --- | --- |
| P0 可评维度 | Sync / Prosody / Fluency / Coverage / Segmental（粗） | Sync / Prosody / Fluency 三维 |
| 输出 | 完整分 | `partial=true`，`score_source=local_prosody_v1`，Coverage/Segmental 标 unavailable 并做权重重分配 |
| 是否可上线验证 | 是 | 是（先验证韵律层与不可评判定，ASR 后补） |

**建议先按「无 ASR」跑通最小闭环**，因为韵律层是本地方案的核心价值，也是唯一不依赖外部模型的部分；ASR 只是补 Coverage。

---

## 1. 已核实的可复用资产

| 资产 | 位置 | 怎么用 |
| --- | --- | --- |
| WAV 解析 + 30ms/10ms RMS VAD + hangover | `internal/cli/wav_trim.go` | 抽到 `internal/audio`，作为 PCM/VAD 底座 |
| vLLM Whisper ASR + CER | `internal/module/speaking/vllm_scorer.go`、`cer.go` | 复用 `ComputeCER` / `ScoreFromCER`；`VLLMScorer` 已实现且有测试，只是没接线 |
| 音频对象存储 | `internal/data/postgres/audio_object_store.go`（Insert / GetByID / SoftDelete） | 用户录音存 `audio_objects`，`kind='shadowing_attempt'`、`visibility='private'`、`OwnerUserID` |
| MinIO 取对象 | `backend/cmd/server/audio_stream.go` 的 `newAudioMinIOClient` / `GetObject` | 同一客户端读取课程标准音频 |
| 本地音频路径 | `audio_stream.go` 的 `serveLegacy`（`os.OpenInRoot(AudioStorePath, key)`） | 种子教材 `audio_url` 是 `/audio/lessons/xxx.wav`，走本地文件 |
| 课程句子时间轴 | `lesson.Sentence{StartMS, EndMS, Tokens[].Reading}`、`data/seed/lessons_shadowing_pilot.json` | **按 `start_ms/end_ms` 从整段 WAV 切出参考句音频**，这是韵律层的基础 |
| 版本校验 | `shadowing.Service.loadValidLesson` / `ERR_SHADOWING_VERSION_STALE` | 评测请求沿用 |
| 配置 | `internal/config/config.go` | 新增 4 个环境变量 |
| 服务注入点 | `backend/cmd/server/postgres_app.go:85`、`main.go` | `shadowing.NewService(...)` 加参数 |

**重要细节**：种子教材音频是 legacy 本地路径，迁移后的课程是 `/api/v1/audio/{id}/stream`。参考音频读取必须同时支持这两种来源，否则 PG 环境与本地种子环境会有一边跑不通。

---

## 2. 文件清单

### 新建

| 文件 | 职责 |
| --- | --- |
| `internal/audio/wav.go` | `ParseWAV` / `SliceWAV` / `EncodeWAV`，16-bit PCM，mono/stereo |
| `internal/audio/envelope.go` | 帧 RMS 包络、归一化、VAD、静音段 |
| `internal/audio/pitch.go` | 简化 YIN/CMND 基频提取 + 半音/z-score 轮廓 |
| `internal/audio/dtw.go` | 带 Sakoe-Chiba 带的 DTW + 路径回溯 + 相似度 |
| `internal/audio/correlation.go` | 包络互相关求 lag / 峰值（含串音判定） |
| `internal/audio/text.go` | 日语文本规范化（片假名→平假名、过滤非假名、长音符保留） |
| `internal/audio/*_test.go` | 表格驱动单测 |
| `internal/module/shadowing/evaluator.go` | `Evaluator` 接口、`EvaluationRequest/Result`、`LocalEvaluator` |
| `internal/module/shadowing/dimensions.go` | 五维计算、置信度、聚合、分档 |
| `internal/module/shadowing/classify.go` | unscorable 判定 |
| `internal/module/shadowing/issues.go` | 规则反馈（模板，无 LLM） |
| `internal/module/shadowing/mastery.go` | EMA 掌握度与完成判定 |
| `internal/module/shadowing/refaudio.go` | `ReferenceAudioReader`：解析课程音频来源 → 切出句子 WAV |
| `internal/module/shadowing/handler_eval.go` | multipart 评测端点 |
| `internal/data/postgres/shadowing_eval_store.go` | `shadowing_evaluations` 读写、幂等 |
| `internal/data/migrations/postgres/schema/011_shadowing_scoring.sql` | 新表 + attempts 加列 |
| `front/react/src/hooks/useWavRecorder.ts` | 16k mono WAV 录音（替代 webm 直传） |
| `front/react/src/pages/shadowing/ScoreCard.tsx` | 分项分 / issues / 录音回放展示 |

### 修改

| 文件 | 改动 |
| --- | --- |
| `internal/module/shadowing/model.go` | 加评测 DTO、错误码、模式枚举 |
| `internal/module/shadowing/service.go` | 注入 `Evaluator` + `EvalStore` + `ReferenceAudioReader`；新增 `Evaluate` |
| `internal/module/shadowing/handler.go` | 注册两条新路由 |
| `internal/config/config.go` | `SHADOWING_ASR_URL` / `_MODEL` / `_TIMEOUT_MS` / `SHADOWING_SCORING_ENABLED` |
| `backend/cmd/server/postgres_app.go` | 构造 LocalEvaluator 并注入；注册私有录音回放路由 |
| `backend/cmd/server/audio_stream.go` | 私有对象本人鉴权（或新增独立 handler） |
| `front/react/src/api/shadowing.ts` | `evaluateShadowing()` |
| `front/react/src/pages/shadowing/ShadowingPage.tsx` | Record 按钮改为提交评测，展示 ScoreCard |
| `Makefile` | `seed-shadowing` / 本地评测自测命令（可选） |

---

## 3. 核心算法（`internal/audio`，全部标准库）

统一约定：**16-bit PCM WAV（mono 或 stereo，stereo 取均值）**，单句 3–15s。非 16-bit PCM 的 WAV 返回明确错误，不猜测格式。

**采样率不要求一致（实测更正）**：种子教材 `data/audio/lessons/shadowing-pilot-n5-day.wav` 实际是 **8 kHz / mono / 19s**，而浏览器录音通常是 16 kHz。原计划假定统一 16 kHz 是错的。所有特征都落在 10 ms 帧网格上（`RMSFrames` / `DetectVAD` / `BestLag` / `DTW`），F0 先换算成 Hz 再做半音 z-score，因此两条信号可以各按自己的采样率提特征、无需重采样。唯一要求是帧窗大小分别跟随各自的采样率。

`RawF0` 内部对高于 16 kHz 的输入做整数抽取（boxcar 抗混叠）再算 CMND：F0 上限 500 Hz，16 kHz 远超所需奈奎斯特频率，而 CMND 是 `O(tau_max × frame) ≈ O(rate²)`，不抽取会让 48 kHz 录音慢约 9 倍。

### 3.1 `ParseWAV` / `SliceWAV`

```
ParseWAV(data []byte) (samples []float64, rate int, err error)
  - 校验 RIFF/WAVE、fmt chunk、bitsPerSample==16
  - stereo 取两声道均值 → mono；归一化 /32768
SliceWAV(data []byte, startMS, endMS int64) ([]byte, error)
  - 按 sampleRate * ms / 1000 定位样本区间，重建 44 字节头（复用 TrimWAVSilence 的头部写法）
  - 边界 clamp，越界返回 fmt.Errorf("audio.SliceWAV: %w", err)
```

参考句音频 = `SliceWAV(整段课程WAV, sentence.StartMS, sentence.EndMS)`。

### 3.2 包络与 VAD（`envelope.go`）

```
Envelope(samples, frameMS=20, hopMS=10) []float64
  rms[i] = sqrt(mean(x²)) → log1p(rms*K) → 除以 95 分位数归一化
VAD(env, onThr=0.12, offThr=0.08, hangover=3) (SpeechSpan, []PauseSpan, voicedRatio)
  - 复用 TrimWAVSilence 的连续窗 + hangover 逻辑
```

输出：`voicedRatio`（有效帧占比）、`speechMS`、长度 > 200ms 的停顿列表 → 供 `classify` / `Fluency` 使用。

### 3.3 基频轮廓（`pitch.go`，简化 YIN/CMND）

```
F0Contour(samples, rate, frameMS=40, hopMS=10) ([]float64, voicedRatio)
  对每帧 x（W=640 样本，去均值、加 Hann 窗）:
    d[τ]   = Σ_{j=0}^{W-1} (x[j]-x[j+τ])²        τ ∈ [rate/400, rate/80] = [40, 200]
    d'[τ]  = d[τ] / ((1/τ)·Σ_{k<=τ} d[k])        累积均值归一化
    τ*     = 第一个 d'[τ] < 0.15 的局部极小，抛物线插值细化
    无谷值或帧能量低于阈值 → 记为无声帧（0）
  后处理:
    semitone = 12·log2(f0 / median(f0_voiced))    消除男女声绝对音高差
    z = (semitone - mean) / (std + 1e-9)          只比形状
    无声帧线性插值（两端用最近有声值）
```

**关键**：必须先做半音 + z-score 归一化，否则男声跟 TTS 女声比会系统性低分。这是本地韵律层能不能用的分水岭。

### 3.4 对齐与 lag（`dtw.go` / `correlation.go`）

```
DTW(a, b []float64, bandRatio=0.2) (dist float64, pathLen int)
  累积距离矩阵 + Sakoe-Chiba 带；回溯取路径长度；返回 dist/pathLen
Similarity(dist) = 100 · exp(-dist / k)   k 初值 1.0，由 §8 校准样本定

BestLag(refEnv, userEnv, maxLagSec=2.0) (lagMS int64, peak float64)
  归一化互相关（包络已降到 100 fps，3s ≈ 300 点 → O(n²) ≈ 9e4，可忽略）
  lagMS = argmax_offset × hopMS
```

- **串音判定**：`peak > 0.85 && |lagMS| < 150` → `bleed_through`。正常同步跟读 lag 应在 300–1500ms；用户声音与参考音在 lag≈0 处高度重合，只可能是外放/回放。阈值需用真实样本校准。
- **lag 目标**：shadow 模式 `targetLag=600ms, σ=500ms`；repeat 模式不评 lag（没有同时播放的原音），Sync 改为「时长比 + 内部节奏稳定度」。

### 3.5 文本规范化（`text.go`）

```
NormalizeJA(s) []rune
  片假名 U+30A1..U+30F6 → 平假名（r - 0x60）；长音符「ー」保留
  过滤掉所有非平假名/非「ー」字符（汉字、标点、空白）
  → 参考文本 = concat(tokens[].Reading 或 Surface) 后 NormalizeJA
  → ASR 文本同样 NormalizeJA
  → ComputeCER(ref, rec) 复用 speaking.ComputeCER
```

**为什么过滤汉字**：Whisper 常把「七時」写成「七時」或「しちじ」或「7時」，直接按字符 CER 会误判。只比假名序列能消掉汉字/数字差异，是本地方案最稳的内容比对口径。

---

## 4. 评分器实现（`internal/module/shadowing`）

### 4.1 接口与注入（保持简单，不做工厂/注册表）

```go
type Evaluator interface {
    Evaluate(ctx context.Context, req EvaluationRequest) (EvaluationResult, error)
}

type Service struct {
    lessons   LessonReader
    store     Store
    evalStore EvalStore          // 新增
    refAudio  ReferenceAudioReader // 新增
    evaluator Evaluator          // 新增，可为 nil（关闭自动评分）
}

func NewService(lessons LessonReader, store Store, evalStore EvalStore,
                refAudio ReferenceAudioReader, evaluator Evaluator) *Service
```

`LocalEvaluator` 持有 `ASRClient`（可为 nil）与 `audio` 包的纯函数；所有依赖通过构造参数注入，无全局变量。错误一律 `fmt.Errorf("shadowing.LocalEvaluator.Evaluate ...: %w", err)`，错误点 `slog.Error`，入口/出口 `slog.Debug`（宪法第三、四条）。

### 4.2 执行顺序（先判可评，再打分）

```
1. ParseWAV(userAudio) / SliceWAV(refAudio) → refSamples, userSamples
2. Envelope + VAD → voicedRatio, speechMS, pauses
3. BestLag → lagMS, corrPeak
4. Classify（见 4.3）→ 若 unscorable，直接落库返回，不写 overall_score
5. 并行（同一 goroutine 顺序执行即可，总耗时 < 300ms）：
     envDTW / F0Contour / 停顿统计
6. ASRClient（若非 nil、超时 3s）→ recognizedText → Coverage / Segmental
7. 各维打分 + 置信度 → Aggregate → issues → 落库 → 返回
```

### 4.3 `classify.go` 判定表（表格驱动测试直接照此写）

| code | 条件（初值） | confidence 处理 |
| --- | --- | --- |
| `silence` | `voicedRatio < 0.05` | 不打分 |
| `too_short` | `speechMS < 0.30 × 参考句时长` | 不打分 |
| `bleed_through` | `corrPeak > 0.85 && abs(lagMS) < 150`（仅 shadow） | 不打分 |
| `content_mismatch` | ASR 可用且 `CER > 0.7` | 不打分 |
| `low_confidence` | 有效维度权重和 `< 0.5` | 不打分 |
| `provider_unavailable` | ASR 失败且韵律层也失败 | 保留自评 |

### 4.4 各维计算与置信度

| 维度 | 计算 | 置信度 |
| --- | --- | --- |
| Sync | `0.5·lagScore + 0.5·envSimilarity`（shadow）；repeat 用 `时长比 + 包络 CV` | `0.5·voicedRatio + 0.5·对齐路径质量` |
| Prosody | `F0 轮廓 DTW 相似度`（双方有声帧 ≥50% 才计算） | `min(1, voicedRatio/0.6)`，有声帧不足时直接剔除该维 |
| Coverage | `100 − CER×100`（ASR 可用时） | ASR 空/超时 → 0（剔除） |
| Fluency | `100 − f(>200ms 停顿次数, 最长停顿, speechMS/参考时长)` | `voicedRatio` |
| Segmental | `0.7·CER 分 + 0.3·专项规则`（长音/促音/拨音在转写中缺失则扣分） | 固定 0.55（P0 粗粒度，低置信度会被门控降权） |

聚合按设计文档 §4.3：`overall = 0.4·s_min + 0.6·weighted_avg(其余 effective)`；某维 `confidence < 0.60` 剔除后重分配权重并标 `partial=true`；有效权重和 < 0.5 → `low_confidence`。

`issues.go` 用规则表生成，最多 2 条 + 1 个动作；置信度 < 0.60 不展示，0.60–0.85 用「可能/建议」。P0 规则只有：`shadow_lag_too_large`、`speech_too_slow`、`long_pause`、`missing_sokuon/long_vowel`（来自文本比对）、`unstable_rhythm`。

`mastery.go`：`mastery = 0.5·last + 0.3·prev + 0.2·prev2`（只取 `scoreable` 且主版本一致），`completed = 有效次数 ≥2 && mastery ≥80 && last ≥75`。**同时替换** `shadowing_store.go:159` 的 `SELECT DISTINCT sentence_index` 与 `:115` 的 `MAX(self_score)`。

---

## 5. 参考音频读取（`refaudio.go`）— 最容易踩坑的一环

```go
type ReferenceAudioReader interface {
    ReadSentence(ctx context.Context, l *lesson.Lesson, s lesson.Sentence) ([]byte, error)
}
```

实现 `minioReferenceAudioReader` 按 `l.AudioURL` 分派：

| AudioURL 形态 | 读取方式 |
| --- | --- |
| `/api/v1/audio/{id}/stream`（PG 迁移后的课程） | 解析 id → 查 `audio_objects` 取 bucket/object_key → `GetObject` |
| `/audio/lessons/xxx.wav`（种子教材） | `os.OpenInRoot(cfg.AudioStorePath, "lessons/xxx.wav")`，复用 `serveLegacy` 的安全路径校验 |
| 其他 / 视频课程只有 `video_url` | 返回明确错误 → `status=unscorable/reference_missing`，**不猜测** |

拿到整段 WAV 后 `audio.SliceWAV(data, s.StartMS, s.EndMS)`。整段音频建议进程内 LRU 缓存（按 audio_object_id，容量 32，纯 `container/list` + map 即可），避免每句评测都回源 MinIO。

---

## 6. 后端接线

### 6.1 配置（`internal/config/config.go`）

```
SHADOWING_SCORING_ENABLED  默认 false（灰度开关）
SHADOWING_ASR_URL          默认 ""（空 = 无 ASR，走三维 partial）
SHADOWING_ASR_MODEL        默认 "whisper-large-v3"
SHADOWING_ASR_TIMEOUT_MS   默认 3000
```

ASR 端点协议沿用 `speaking.VLLMScorer`：`POST /v1/audio/transcriptions`，multipart `file` + `model` + `language=ja` + `response_format=text`（OpenAI 兼容，vLLM 自带）。**不新写 HTTP 客户端**，把 `VLLMScorer` 的 `transcribe` 抽成 `internal/asr` 或直接在 `LocalEvaluator` 里复用 `speaking.NewVLLMScorer(url, model, timeout)` 并调用 `ScoreAudio`。

### 6.2 路由

```
POST /api/v1/lessons/{id}/shadowing/evaluations   （protectedMux，已有的 /api/v1/lessons/ 鉴权覆盖）
GET  /api/v1/shadowing/evaluations/{eid}          （需在 mux 增加 /api/v1/shadowing/ 鉴权前缀）
GET  /api/v1/audio/{id}/stream                    （放开 private 且 owner==当前用户）
```

multipart 限制：`r.Body = http.MaxBytesReader(w, r.Body, 2<<20)`，音频 ≤ 15s；字段 `audio / sentence_index / mode / reference_position_ms / client_lag_ms / shadowing_version / self_score`；`Idempotency-Key` 头必填（可用 `crypto/rand` 生成前端 UUID），服务端按 `(user_id, key)` 唯一约束返回结果，避免重复计费/重复记录。

### 6.3 迁移（`011_shadowing_scoring.sql`）

按设计文档 §5.4 建 `shadowing_evaluations` 并给 `lesson_shadowing_attempts` 加 `evaluation_id / machine_score / score_source`。全部 `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS`；**不用函数表达式 DEFAULT**（`AGENTS.md` §7.1）；只在 PG schema 目录加，不进 SQLite 迁移。

### 6.4 私有录音回放

`audio_stream.go:58` 现在是 `visibility != "public" → 404`。改为：

```
public                      → 直接放
private && owner_user_id == 当前用户 → 放（需 AuthMiddleware 能拿到 userID，或单独包一层鉴权）
private 但无身份 / 非本人  → 404（不要泄露对象存在性）
```

若不想改动现有公共端点，可新挂 `GET /api/v1/shadowing/recordings/{id}` 独立 handler，更简单、影响面更小（**推荐**）。

---

## 7. 前端改造

### 7.1 `useWavRecorder.ts`（替代 webm 直传）

```ts
const ctx = new AudioContext({ sampleRate: 16000 })   // 关键：decodeAudioData 会重采样到 ctx 的采样率
const stream = await navigator.mediaDevices.getUserMedia({
  audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
})
const mime = MediaRecorder.isTypeSupported('audio/webm') ? 'audio/webm' : 'audio/mp4'
const rec = new MediaRecorder(stream, { mimeType: mime })
// 停止后：
const buf = await new Blob(chunks, { type: mime }).arrayBuffer()
const audio = await ctx.decodeAudioData(buf)     // 得到 16k AudioBuffer
const pcm = audio.getChannelData(0)               // 已是 mono/16k
const wav = encodeWAV(pcm, 16000)                 // 44 字节头 + int16 little-endian
```

- 兼容兜底：Safari 对 webm 解码不稳 → `audio/mp4`；`decodeAudioData` 失败时提示重录，**不要静默回退到 webm 直传**（后端只收 WAV）。
- 保留原 `useAudioRecorder` 的本地 Blob URL 回放能力，避免改造期间丢功能。
- 录音前提示「请戴耳机」，并做一次 `getUserMedia` 后的设备确认（外放串音由后端 `bleed_through` 兜底）。
- `echoCancellation: true` 能减轻但不能替代后端串音检测。

### 7.2 提交与展示

```ts
const fd = new FormData()
fd.append('audio', new Blob([wav], { type: 'audio/wav' }), 'attempt.wav')
fd.append('sentence_index', String(idx))
fd.append('mode', 'shadow')                  // 或 repeat
fd.append('shadowing_version', String(v))
fd.append('reference_position_ms', ...)
await apiFetch('POST', `/api/v1/lessons/${id}/shadowing/evaluations`, fd, {
  'Idempotency-Key': crypto.randomUUID()
})
```

`ScoreCard` 展示：总分（或「本次不评分 + 原因」）、五维条（带置信度，低置信度置灰标「证据不足」）、最多 2 条 issue + 1 个动作、我的录音 vs 标准音并排播放、再练一次。**不可评时不显示任何数字分数**。

---

## 8. 校准（本地方案能否成立的关键）

在写死阈值之前，先用 20–30 条真实录音定表：

| 样本 | 用途 | 期望 |
| --- | --- | --- |
| 母语者/老师正常跟读 10 条 | 假阳性 | 不出现红色 issue，`overall >= 80` |
| 故意读另一句 / 只读半句 5 条 | 内容判定 | `content_mismatch` 或 `too_short` 命中 |
| 外放原音录一遍 5 条 | 串音判定 | `bleed_through` 100% 命中 |
| 慢速、抢拍、长停顿各 5 条 | 权重/单调性 | Sync / Fluency 分数单调下降 |
| 男声 vs 女声读同一句 | 音高归一化 | 分数差 < 8 分（验证半音 z-score 有效） |

要定的常数：`VAD on/off`、`bleed_through` 的 `corr/|lag|`、`F0 谷值阈值 0.15`、DTW `k`、各维置信度下限 0.60、`targetLag/σ`。全部放进 `weights_version` 对应的配置结构，**不散落在代码魔数里**。

---

## 9. TDD 测试清单（表格驱动优先，拒绝 mock 对象）

| 测试 | 用例形态 |
| --- | --- |
| `TestParseWAV` | 合法 16-bit mono/stereo、坏头、8-bit、空 data chunk、越界切片 |
| `TestEnvelopeVAD` | 合成正弦 + 静音段 → 期望 span/pauses/voicedRatio |
| `TestF0Contour` | 合成 120Hz / 200Hz / 升调 / 纯噪声 → F0 估计误差 < 5Hz、噪声判无声 |
| `TestDTW` | 相同序列、时间拉伸、局部插入 → 距离单调、路径长度合理 |
| `TestNormalizeJA` | 片假名、汉字、标点、长音符、促音 → 期望假名序列 |
| `TestClassify` | §4.3 六类 + 正常样本，逐行断言 code |
| `TestAggregate` | 维度组合 → 期望 overall / partial / low_confidence，含置信度剔除后的权重重分配 |
| `TestMastery` | 历史分序列（混入 unscorable、跨版本）→ mastery / completed |
| `TestIssues` | 各规则 → 条数 ≤ 2、置信度措辞、排序 |
| `TestEvaluateHandler` | 集成：真实 PG + `httptest` 起 vLLM 风格 HTTP 服务 + 真实 `audio_objects` 写入，走完整 Handler→Service→Store |
| `TestIdempotency` | 同 key 重复提交 → 返回同一结果、只写一行 |
| `TestVersionStale` | 复用现有 `ERR_SHADOWING_VERSION_STALE` |

先写失败测试 → 最小实现 → 跑通 → 重构。验收命令：`make test`、`go vet ./...`、`make build`。

---

## 10. 分步实施顺序（每步独立可验收）

- [x] **S1 · 音频底座**（2026-11-16 完成）：`internal/audio`（wav/envelope/pitch/dtw/correlation/text）+ 全部表格驱动单测。验收：`go test ./internal/audio/...` 通过；新增高采样率 F0、近常数包络 lag、真实种子音频自比较三组回归。
- [x] **S2 · 最小闭环（无 ASR、无 MinIO 写）**（2026-11-16 完成）：`refaudio.go` 双来源读取 → `classify` → Sync/Prosody/Fluency 三维 → 聚合 → 直接返回结果 JSON（**先不落库**）。验收：用种子教材第一句通过 curl 上传 16 kHz WAV 拿到 `overall=93`、`partial=true` 与分项分；原音回灌返回 `bleed_through`，静音返回 `silence`，版本过期返回 409。
- [x] **S3 · 前端**（2026-11-16 完成）：`useWavRecorder` + `evaluateShadowing` + `ScoreCard` + `ShadowingPage` 接线。验收：`vitest` 18 文件 153 测试全过、`tsc --noEmit` 通过、`vite build` 通过且新组件与 CSS 已进入产物；`apiFetch` 契约测试覆盖 multipart 字段与 data 解包。代码层验收达成「页面录音 → 提交 → 展示分项与不可评原因」，真实浏览器麦克风联调亦已完成（§15）。详见 §14、§15。
- [ ] **S4 · 持久化**：`011` 迁移 + `EvalStore` + `audio_objects` 私有写入 + 私有回放路由 + 幂等键。验收：刷新页面能回看历史结果，重复提交不产生第二条记录。
- [ ] **S5 · 掌握度**：替换 `DISTINCT sentence_index` / `MAX(self_score)`，进度条由 mastery 驱动。验收：录一次不再自动标「完成」。
- [ ] **S6 · ASR 接入**：配置 `SHADOWING_ASR_URL`，复用 `VLLMScorer`，补 Coverage/Segmental 与 `content_mismatch`。验收：故意读另一句不打分。
- [ ] **S7 · 校准**（2026-11-16 **部分完成**，见 §20）：阈值与权重已收进 `ScoringConfig` 并按 `weights_version` 版本化，置信度口径改为绝对有声帧数，repeat 本地权重按模式重分配。**未达成**：§8 要求的 20–30 条真人录音样本集、母语者误报率、串音 100% 命中、男女声分差 < 8 均未验收。
- [ ] **S8 · P1 增强**：mora 拍级节奏（用 `Tokens[].Reading` 生成参考拍序列）、issues 规则扩充、repeat 模式 UI 与独立权重。

**P0 的定义 = S1–S5**，全程零外部 API、零新增 Go 依赖。S6 需要可用的 vLLM Whisper 实例；没有也可以停在 S5 并明确 `partial=true`。

---

## 11. 性能与风险

**计算量**（3s 单句 @16k）：包络 300×320 ≈ 1e5、DTW 300×300 ≈ 9e4、F0 300 帧 × 640×200 ≈ 3.8e7 乘加。纯 Go 预计 **< 300ms**；ASR 1–2s 时端到端 P95 可控制在 3s 内。F0 可做 τ 递推与帧降采样（hop 20ms）进一步优化。

| 风险 | 缓解 |
| --- | --- |
| F0 在噪声/串音下不稳 | 用 `voicedRatio` 做置信度门控，不够就剔除 Prosody 维并标 `partial`；绝不硬给分 |
| 串音误判正常跟读 | lag 阈值 150ms 远小于正常 300–1500ms；用 §8 样本校准 |
| 汉字/数字导致 CER 虚高 | 只比规范化后的假名序列 |
| 课程音频是视频而非 WAV | `reference_missing` 明确不可评，P0 不支持视频教材的韵律评分 |
| TTS 参考音的 F0 不自然 | 半音 z-score 只比形状；若 TTS 有明显机械感，可在教材入库时预生成并缓存参考轮廓 |
| 私有录音外泄 | 独立 handler + owner 校验 + 90 天 `expires_at` + 不写音频进日志 |
| 迁移系统已知缺陷 | 只用幂等 DDL，不动 `002_seed.sql`；`busy_timeout` 已由 `OpenDB` 处理（`AGENTS.md` §7.1） |

---

## 12. 什么时候该升级到云 API

出现以下任一需求时，本地方案才需要 P2 的可插拔 Provider（设计文档 §5.2）：
1. 用户/教研要求「指出具体哪个音素读错」；
2. repeat 模式要做精确音段评分并对外宣称发音准确度；
3. 需要 CEFR/JLPT 口语等级映射或考试型评分；
4. 校准后发现母语者误报率无法压到 5% 以下。

届时 `LocalEvaluator` 与 `AzureEvaluator` 实现同一个 `Evaluator` 接口，只替换 `Segmental` 维，其余本地韵律层继续复用，改造面很小。

---

## 13. S1/S2 实施记录（2026-11-16）

### 实际落地文件（与 §2 计划命名的差异）

计划中的 `evaluator.go` / `dimensions.go` / `classify.go` / `issues.go` 在 S2 阶段合并为以下文件，保持「先跑通最小闭环、不做过度抽象」：

| 实际文件 | 内容 |
| --- | --- |
| `internal/audio/{wav,envelope,pitch,dtw,correlation,text}.go` | 音频底座，纯标准库 |
| `internal/audio/*_test.go` | 表格驱动单测 + 真实种子音频回归 |
| `internal/module/shadowing/evaluate_model.go` | 模式、DTO、错误码、版本四元组、权重 |
| `internal/module/shadowing/evaluate_scoring.go` | `Classify` / `Aggregate` / `EvaluateWAV` / 三维打分 / issues |
| `internal/module/shadowing/refaudio.go` | 双来源参考音频读取 + 按时间轴切句 |
| `internal/module/shadowing/evaluate_service.go` | 独立于原 progress/attempt `Service` 的编排层 |
| `internal/module/shadowing/evaluate_handler.go` | `POST /api/v1/lessons/{id}/shadowing/evaluations` |
| `internal/module/shadowing/evaluate_test.go`、`evaluate_e2e_test.go` | 单测 + 真实种子音频端到端回归 |
| `backend/cmd/server/shadowing_refaudio.go` | PG runtime 的 MinIO `ObjectLoader` |
| `backend/cmd/server/main.go`、`postgres_app.go` | SQLite / PG 两个 runtime 的接线 |

`EvaluationService` 刻意独立于原 `Service`：S2 不写 DB、不改 schema、不影响现有 `self_score` 与进度逻辑。

### 全量验收结果

| 命令 | 结果 |
| --- | --- |
| `go test ./internal/audio/... ./internal/module/shadowing/... -count=1` | 通过 |
| `make test`（`go test ./... -v -count=1`） | 通过，FAIL 数 0 |
| `go vet ./...` | 通过 |
| `make build` | 通过，产出 `bin/server` |
| 真实 TCP 往返（`httptest.NewServer` + `http.Client`） | `overall=93`、`latency_ms=51` |
| 独立进程 + curl multipart（真实种子 WAV） | 4 类请求全部符合预期 |

curl 端到端（参考句 0–3000ms，8 kHz 参考 / 16 kHz 上采样录音）：

| 场景 | 结果 |
| --- | --- |
| repeat / 16 kHz 录音 | `scoreable`，`overall=93`，`partial=true`，`scorer=local-prosody-1.0`，`weights=w-repeat-1.0`，`latency_ms=37`；Sync 99.4 / Prosody 83.9 / Fluency 99.7，Coverage + Segmental `available=false` |
| shadow / 原音回灌 | `unscorable`，`bleed_through`，无机器分 |
| 静音录音 | `unscorable`，`silence`，无机器分 |
| `shadowing_version=99` | HTTP 409，`ERR_SHADOWING_VERSION_STALE` |

### 验证中发现并修复的三个实测缺陷

1. **采样率强校验导致真实教材跑不通**：`EvaluateWAV` 原先要求 ref/user 采样率相等，而种子教材是 8 kHz、浏览器录音是 16 kHz，真实链路直接 422。修复为两条信号各按自己的采样率提特征（帧网格与 z-score 半音轮廓天然与采样率无关），并新增 8k/16k、16k/8k、44.1k/16k 三组回归。
2. **`BestLag` 在近常数包络下选错 lag**：真实 TTS 参考音前 3 秒的归一化包络几乎恒定（约 0.99–1.0），各 lag 的 Pearson 相关性都逼近 1；原先浮点严格大于比较让搜索窗口的**第一个**候选（-2000ms）胜出，导致 `bleed_through` 漏判、shadow 同步分失真。修复为相关性并列（`1e-6` 容差）时优先重叠更长、其次 `|lag|` 更小的对齐；真实音频自比较现在稳定得到 `lag_ms=0, peak=1`。
3. **高采样率下 F0 计算量随 rate² 增长**：44.1 kHz 单句评测耗时约 0.23s，48 kHz 更慢，对同步 HTTP 端点是隐患。`RawF0` 内部增加整数抽取（boxcar 抗混叠）把分析率压到 ≤16 kHz；32k/44.1k/48k 三档用例总耗时从 0.22s 降到 0.04s，F0 估计误差仍 < 5 Hz，帧网格不变。

### 已知限制（留给 S3–S8）

- 前端仍走 `useAudioRecorder` 的 webm 本地回放，尚未接入评测端点；`ScoreCard` 未实现。
- 评测结果不落库、无幂等键、无私有录音存储与回放，`shadowing_evaluations` 表未建。
- 完成判定仍是 `DISTINCT sentence_index`、最佳分仍是 `MAX(self_score)`，mastery EMA 未替换。
- Coverage / Segmental 恒为 `available=false`，`content_mismatch` 判定要等 S6 的 ASR。
- 阈值（`bleedPeak`、`shadowTargetLagMS`、DTW `k`、置信度下限）仍是初值，未经 §8 的真实样本校准；`internal/config` 的 4 个开关也未加入。
- curl 自测用的是只挂载评测端点的临时探针进程（已清理），绕过了认证中间件；认证与真实 DB 装载需在 S3/S4 接线后再做联调。

---

## 14. S3 实施记录（2026-11-16）

分两步交付：先做 `useWavRecorder` 和 `ScoreCard`，再经用户确认后接线到 `ShadowingPage`。

### 文件清单

| 文件 | 内容 |
| --- | --- |
| `front/react/src/util/wav.ts` | `floatToPCM16` / `downmixToMono` / `encodeWAVBytes` / `encodeWAVBlob` / `pickRecorderMimeType` / `wavDurationMS`，纯标准库 |
| `front/react/src/util/__tests__/wav.test.ts` | 27 个表格驱动用例（WAV 头字段、小端样本、量化与 clamp、声道混音、mime 兜底、时长） |
| `front/react/src/hooks/useWavRecorder.ts` | `getUserMedia` + `MediaRecorder`（webm→mp4→ogg）+ `AudioContext({sampleRate:16000})` 的 `decodeAudioData` 重采样 → 16k mono 16-bit WAV；本地回放 URL、错误与资源清理 |
| `front/react/src/types/shadowing.ts` | 追加 `EvaluationMode` / `EvaluationStatus` / `DimensionScore` / `EvaluationIssue` / `EvaluationResult`，与后端 JSON 对齐 |
| `front/react/src/util/shadowing/scoreCard.ts` | 五维固定顺序与中文标签、置信度门控（<0.60 标「证据不足」）、不可评文案、分数分档、issue 排序截断（最多 2 条） |
| `front/react/src/pages/shadowing/ScoreCard.tsx` / `.module.css` | 总分 + 五维条 + partial 标记 + issues + 录音回放 + 再录按钮；用主题变量与 emoji |
| `front/react/src/pages/shadowing/ScoreCard.test.tsx` | 37 个用例：纯逻辑表格驱动 + `react-dom/server` 真实组件渲染断言 |

### 设计要点

- **后端只收 WAV**：`decodeAudioData` 失败时给出明确错误，**不静默回退到 webm 直传**。`encodeWAVBytes` 返回 `Uint8Array<ArrayBuffer>`（TS 5.9 的 `BlobPart` 需要精确的 ArrayBuffer 泛型）。
- **不可评时不显示任何数字分数**：组件 `unscorable` 分支完全不渲染总分、维度条和版本信息；测试用 `data-testid` 结构断言而非数字正则（避免 CSS module hash 误判）。
- **五维恒按顺序展示**：缺失/不可用维度仍占位并标「证据不足」，不静默丢弃；`available=false` 与 `confidence<0.60` 走同一门控。
- **采样率与 S2 对齐**：录音固定 16k，参考音频是 8k；S2 已验证两者可直接比较，前端无需知道课程音频采样率。
- UI 规范：图标用 emoji，按钮/播放器都有可见文字标签，无冷门 Unicode 符号。

### 验收结果

| 检查 | 结果 |
| --- | --- |
| TDD Red | 两个测试文件因模块不存在失败（`Cannot find module` / `Failed to load url ../wav`） |
| 新增测试 Green | 64/64 通过（wav 27 + ScoreCard 37） |
| `npx vitest run`（前端全量） | 16 个文件、135 个测试全通过 |
| `npx tsc --noEmit` | 通过 |
| `npx vite build --outDir /tmp/...` | 通过（仅有项目既有 kuromoji 的 `path` externalized 警告） |
| `vitest` 开启 `css: true` 真实编译 CSS module | 37 个组件测试仍全通过 |

TDD 过程中修正了一个测试期望错误：`visibleIssues` 按设计最多返回 2 条，原排序用例却用 3 条输入断言 3 条输出；实现正确，改测试为 2 条输入。

### 接线补记（同日稍后）

用户确认「继续接线，把 ShadowingPage 接上录音提交评测」后完成：

- `api/shadowing.ts` 新增 `buildEvaluationFormData` / `evaluateShadowing`。字段与 S2 handler 严格对齐：`audio` / `mode` / `sentence_index` / `shadowing_version`；FormData 交给 `apiFetch`，不手工设 `Content-Type`（浏览器负责 multipart boundary）。
- `util/shadowing/evaluationFlow.ts` 新增 `classifyEvaluationError`（`ERR_SHADOWING_VERSION_STALE` → 刷新会话并提示，其余保留原始错误）与 `recordButtonLabel`（录音中 / 评分中 / 空闲三态）。
- `ShadowingPage` 换用 `useWavRecorder`；停止录音后**先评测、再保存 attempt**，评测失败不影响练习记录（S2 不落库，现有 `best_score = MAX(self_score)` 与进度统计仍依赖它）；stale 时刷新会话；展示 `ScoreCard`（含录音回放与「再录一次」）；加入戴耳机提示与「正在评分」状态。原有 progress / loop / self_score 逻辑全部保留。
- 旧 `useAudioRecorder` 仍被 `SpeakingPage` 使用，**未删除**；只清掉了 ShadowingPage 里已无用的本地回放卡片 CSS。
- 修 UI 规范违规：`ScoreCard` 同步维度图标 `⏱️`（U+23F1，AGENTS.md 明令禁止的 U+23xx）改为 `🔄`，并在两处加回归断言禁止 U+23xx 符号。
- TDD：先写 3 组测试看 Red（8 个失败，含 U+23xx 违规），再实现；新增 17 个用例（api 7 + flow 10）。修了一处 `vi.fn` 参数推断为无参导致的 `tsc` 类型断言错误（测试侧）。

| 验收项 | 结果 |
| --- | --- |
| `npx vitest run` 全量 | 18 文件 / 153 测试全通过 |
| `npx tsc --noEmit` | 通过 |
| `npx vite build --outDir /tmp/...` | 通过，219 个模块（接线前 213），`部分维度暂无数据` / `正在评分` 与 `partialBadge` / `dimensionMuted` / `hint` 均进入产物 |

### 仍待后续

- ~~**未在真实浏览器里用麦克风录音联调**~~ → 已于 2026-11-16 完成，见 §15。
- 评测固定 `mode='shadow'`；repeat 模式 UI 与模式切换属 S8。
- 幂等键（`Idempotency-Key`）、评测结果持久化、私有录音回放属 S4；完成判定与 mastery EMA 属 S5；Coverage/Segmental 与 `content_mismatch` 属 S6；阈值校准属 S7。
- 组件测试用 `renderToStaticMarkup`（项目无 jsdom / testing-library），覆盖渲染输出与文案，不覆盖点击 / 录音等交互。
- 认证 + 真实 DB 装载的端到端联调待 S4 接线后再做。

---

## 15. 真实浏览器麦克风联调记录（2026-11-16）

用户要求「先做浏览器麦克风录音的联调」。目标不是再补单测，而是在真实浏览器里验证 `录音 → 停止 → WAV 生成 → multipart 提交 → 后端评分 → ScoreCard 展示` 这条完整链路。

### 联调方式（全临时，不碰生产数据）

| 组成 | 做法 |
| --- | --- |
| 浏览器 | 本机 Chrome headless + CDP（`--headless --no-sandbox --disable-gpu --use-fake-device-for-media-stream --use-fake-ui-for-media-stream --use-file-for-fake-audio-capture=<wav>`） |
| 假麦克风 | 由真实种子音频前 3 s 上采样到 48 kHz，前置 600 ms 静音，得到 3.6 s 恒定幅度 WAV |
| 后端 | 临时 `backend/cmd/e2eprobe`：静态 session + **真实的 `EvaluationHandler`**，监听 `127.0.0.1:30081`（项目默认后端端口），额外保存浏览器上传的 WAV 并打印响应 |
| 前端 | 临时 `front/react/e2e.html` + `src/e2e/main.tsx`，用 `MemoryRouter` 渲染**真实的 `ShadowingPage`**；Vite dev server 35173 的既有 `/api`、`/audio` 代理指向探针 |
| 驱动 | CDP WebSocket 脚本，点击 Record/Stop、轮询 ScoreCard、抓网络请求、console error、异常并截图 |

**没有**启动真实后端、没有登录、没有改 `data/app.db`、没有写任何生产数据。所有临时文件与进程在收尾时清理。

### 首轮结果：链路全通，但判为 `too_short`

页面加载、按钮三态切换、浏览器生成录音 blob、`POST .../evaluations` 200、`POST .../attempts`、ScoreCard 渲染不可评分支、录音回放 3.18 s、无 JS 异常 —— 链路本身完全正常。但后端返回 `too_short`，只显示「录音太短，可能只跟读了半句」。探针确认上传的是合法 `16 kHz / 1 ch / 16 bit / 3.18 s` WAV，字段 `mode=shadow`、`version=1`、`sentence_index=0` 均正确，`lag_ms=580`。

### 定位：WebRTC 音频处理破坏了能量包络

离线对上传 WAV 做 20 ms 窗 / 10 ms hop 的 RMS 分析，并与假麦克风源 WAV 对比：

| | 源 WAV (48 kHz) | 浏览器上传 WAV (16 kHz) |
| --- | --- | --- |
| RMS 包络 | 600 ms 后稳定在 ≈0.022，直到结尾 | 650 ms 冲到 0.110，随后指数衰减到 0.003 |
| 自适应阈值 | `on = peak*0.12 = 0.00268` | `on = peak*0.12 = 0.01324` |
| VAD | `speech_ms=2980`、`voiced_ratio=0.83` | `speech_ms=760`、`voiced_ratio=0.24` |

3 s 参考句的 30% 门槛是 900 ms，760 ms 因此被正确判为 `too_short`。**后端判定没错，错的是录音信号**：恒定幅度的一句话被浏览器录成了衰减包络。

用 CDP 在页面加载前拦截 `getUserMedia`、把三项音频处理全关掉后重跑，上传 WAV 立刻恢复平坦包络（`speech_ms=2980`），后端返回 `scoreable`。由此确认根因是 `useWavRecorder` 原来把 `echoCancellation` / `noiseSuppression` / `autoGainControl` 全设为 `true`。

### 生产修复（TDD）

- 新增 `front/react/src/util/audioCapture.ts` 的 `scoringAudioConstraints()`，三项全部 `false`；注释写明原因：AGC / 降噪会随时间改变幅度，而 Sync / Fluency / VAD / 串音判定全靠能量包络；回声消除会把原音从麦克风里抹掉，反而掩盖设计上要显式报出的 `bleed_through`（UI 已提示戴耳机）。
- `useWavRecorder` 改用该函数。新增 `src/util/__tests__/audioCapture.test.ts` 3 个用例（三项均关、新增字段也不得默认开启、每次返回新对象）。先跑 Red（`Failed to load url ../audioCapture`）再实现跑绿。

### 最终验收（不加任何 CDP 改写，纯生产代码）

| 检查 | 结果 |
| --- | --- |
| 浏览器实际请求的约束 | `{echoCancellation:false, noiseSuppression:false, autoGainControl:false}`，`apmPatched=false` |
| 音轨 `getSettings()` | `autoGainControl:false` / `noiseSuppression:false` / `echoCancellation:false`，`channelCount:2`、`sampleRate:44100`（假设备给双声道 44.1 kHz，验证 downmix + `AudioContext` 重采样有效） |
| 上传 WAV | `16000 Hz / 1 ch / 16 bit / 3.60 s`，RMS 平坦、`speech_ms=2980` |
| 后端响应 | `status=scoreable`、`overall_score=72`、`partial=true`、`lag_ms=630`、`latency_ms=36`；Sync 79.8 / Prosody 50.4 / Fluency 99.8 可用，Coverage / Segmental `available:false`；版本 `local-prosody-1.0` / `w-shadow-1.0` / `none` |
| ScoreCard | 显示 `72 分`、「部分维度暂无数据」、五维固定顺序（完整度/音段标「证据不足」）、录音回放、`再录一次` |
| 网络与异常 | `POST evaluations` 200、`POST attempts`、`GET shadowing` 刷新；`exceptions: []`，仅 React Router v7 future flag 警告与一个 favicon 404 |
| 不可评分支 | 首轮已验证 `too_short` 时完全不显示任何数字分数 |

全量回归：`npx vitest run` **19 文件 / 156 测试**全通过、`npx tsc --noEmit` 通过、`npx vite build` **220 模块**通过；Go `go test ./... -count=1`、`go vet ./...`、`go build ./...` 全通过。

### 收尾

临时探针 `backend/cmd/e2eprobe/`、`front/react/e2e.html`、`front/react/src/e2e/`、`/tmp/jla_*` 与 Chrome / Vite / 探针进程全部清理；`front/react/vite.config.ts` 未修改（30081 本就是项目默认后端端口，代理是仓库既有配置）。保留的只有生产修复、单测与本文档。**未做**：真实物理麦克风 / 真人发音、认证 + 真实数据库装载的端到端（待 S4），以及 S4–S8。

---

## 16. 评分后的具体修改建议（2026-11-16，用户追加需求）

用户要求「评分后要给出具体的修改建议」。核查后发现**前端早有 `issues` 展示区，问题出在后端**：旧的 `buildIssues()` 只有三条粗规则（`lag>1200`、有停顿、`envSim<60`），文案笼统、不带任何测量数值，而且触发条件苛刻 —— 联调那次 72 分的真实结果 `issues` 直接是 `[]`，等于评分完没有任何建议；不可评分支更是只有一句通用提示。

### 设计原则

| 原则 | 做法 |
| --- | --- |
| 建议要具体 | 每条带 `evidence_zh`，引用**实测数字**：起步延迟 ms 与目标 ms、停顿位置与时长、读音时长与标准句长的百分比、维度得分 |
| 建议要可执行 | 每条带 `action_zh`，写清练习动作（0.75 倍速、先听 2 遍、按拍读、戴耳机、检查麦克风等），不是「多练练」 |
| 建议要可追溯 | `dimension` 把建议绑回对应的分数行， learner 知道这条建议是冲着哪一维度去的 |
| **永不为空** | 高分无短板时兜底 `keep_practice`，指出最弱维度并给进阶练习；不可评时按 `unscorable_code` 给专属修法 |
| 不过载 | 后端已按「维度短板 × 权重 + 可操作性加成」排序、每维度最多一条、上限 3 条；前端只过滤低置信度，**不再重排**（重排会把「很确定但不重要」的提示顶到最前） |

### 落地文件

- `internal/module/shadowing/advice.go`（新）：`AdviceInput` / `BuildAdvice()` 纯函数、`scoreableCandidates`、`keepPracticeAdvice`、`unscorableAdvice`、`adviceInput()` 适配器。
- `internal/module/shadowing/evaluate_model.go`：`Issue` 增加 `dimension` / `evidence_zh`（omitempty，向后兼容）。
- `internal/module/shadowing/evaluate_scoring.go`：删掉旧 `buildIssues` 与相关常量；`EvaluateWAV` 的 **scoreable / unscorable / low_confidence 三条分支都填 `Issues`**。
- `front/react/src/types/shadowing.ts`：`EvaluationIssue` 补两个可选字段。
- `front/react/src/util/shadowing/scoreCard.ts`：`MAX_VISIBLE_ISSUES` 2→3、`visibleIssues` 改为保持后端顺序、新增 `dimensionLabel()`。
- `front/react/src/pages/shadowing/ScoreCard.tsx` + `.module.css`：抽出 `AdviceItem`，新增「📋 下一步这样改」区块（维度标签 + 证据 + 说明 + 动作），**可评与不可评两种状态都渲染**；不可评通用提示改为引导看建议。

### 规则表

| type | 触发 | 证据示例 | 动作要点 |
| --- | --- | --- | --- |
| `sync_lag_late` | shadow 且 `lag - 600 > 300ms` | `起步延迟 1500 ms，目标约 600 ms，晚了 900 ms` | 听到第一个音就开口，0.75x 对齐起音点 |
| `sync_lag_early` | shadow 且 `lag - 600 < -300ms` | `起步延迟 100 ms，目标约 600 ms，早了 500 ms` | 等原音出来再开口，先打拍子 |
| `prosody_contour` | 韵律 < 70 分 | `句调轮廓与标准音相似度 50 分` | 听 2 遍比划高低，0.75x 抠句尾升降 |
| `fluency_long_pause` | 有 ≥200ms 内部停顿 | `第 1.2 s 处停了 360 ms` | 把该处连起来读，停顿处不换气 |
| `fluency_pace` | 时长偏离 >25% | `有效读音 1600 ms，标准句长 3000 ms（53%）` | 跟原音打拍子，长音促音占满一拍 |
| `rhythm_unstable` | 包络相似度 < 60 | `节奏包络相似度 42 分` | 按拍读，每个假名一样长 |
| `keep_practice` | 无其它短板（兜底） | `本次 93 分，最弱的是句调（84 分）` | 针对最弱维度再跟读 2 遍 |
| `unscorable_<code>` | 不可评 | `只录到 760 ms 人声，标准句长 3000 ms` | 按具体原因修：麦克风/耳机/整句/安静环境 |

repeat 模式不生成 shadow 起步延迟类建议。

### 验收

- TDD：先写 `advice_test.go`（20 例：永不为空、数字证据、优先级/去重/上限、高分兜底、repeat 不检查 lag、6 种不可评码、真实 `EvaluateWAV` 两条分支）与前端 Red 测试（13 个失败），再实现跑绿。
- 真实种子音频回归 `TestPilotAdviceIsConcrete`：8 kHz 教材 + 16 kHz 录音跑 HTTP，断言建议非空且每条证据含数字、每条有动作；`TestPilotEndToEndEvaluation` 四个场景额外断言 `Issues` 非空。`advice: type=keep_practice dimension=prosody evidence="本次 93 分，最弱的是句调（84 分）"`。
- 全量：`go test ./... -count=1` / `go vet ./...` / `go build ./...` 通过；前端 `vitest` **19 文件 168 测试**、`tsc --noEmit`、`vite build` 220 模块通过；生产产物确认含「下一步这样改」与 `advice-evidence` / `advice-dimension` / `adviceTitle`。
- API 契约新增一条断言：`evidence_zh` / `dimension` 从响应透传不丢失。

**未做**：建议仍是纯本地规则，没有用 LLM 生成（符合零外部 API 约束）；Coverage / Segmental 未接入 ASR 前不参与建议；建议内容尚未做母语者样本的 A/B 校准（属 S7）。

---

## 17. 真实本地服务端到端验证（2026-11-16）

用户要求「开启本地服务测试」。此前 §13–§16 的所有验证都是单测 / curl / 临时探针，**从未启动过真实后端**。本节记录真实服务 + 真实 SQLite 库 + 真实前端的一次完整验证。

### 启动与地址

| 服务 | 地址 | 说明 |
| --- | --- | --- |
| 主后端 | `http://localhost:30081` | `go run ./backend/cmd/server/`，`JWT_SECRET=local-dev-secret`，SQLite `data/app.db` |
| 学习端前端 | `http://localhost:35173` | `npm run dev`，既有 `/api`、`/audio` 代理指向 30081 |
| 影子跟读页 | `http://localhost:35173/lesson/1/shadowing` | 需登录 |
| 体验账号 | `local-demo@jla.test` / `demo12345` | 本次新建（用户 id 16） |

未启动 admin API(:30082) 与 admin 前端(:35174)；本任务只用学习端链路。

### 真实库的两个数据问题

1. `data/app.db` 里 30 篇课文的 `shadowing_enabled` 全为 0、`audio_url` 为空，但 `content_furigana_json` 与 `sentence_timestamps_json` **已经包含完整 6 句与 0–19000 ms 时间轴**。
2. `make ... import-lessons --file data/seed/lessons_shadowing_pilot.json` 直接失败：`duplicate lessons for title "私の一日" and jlpt_level "N5" (ids [1 16])`，CLI 要求先跑 `cleanup-lesson-duplicates`。**没有擅自删除任何重复记录**。

因为只需三个开关字段，采用最小侵入做法：全库先备份到 `/tmp/jla_app_db_backup_20261116.db`，然后只对 `id=1` 执行一条参数化 `UPDATE`，设置 `shadowing_enabled=1`、`audio_url='/audio/lessons/shadowing-pilot-n5-day.wav'`、`shadowing_config_json`（取自种子文件）、`shadowing_version=1`。`id=16` 与其它课文一字未动。种子 CLI 的重复课文清理留给用户决定。

### 验证结果

| 检查 | 结果 |
| --- | --- |
| `GET /api/v1/lessons/1/shadowing`（签名 JWT，复用现有用户 id 8） | 200，6 句、正确 `audio_url`、`shadowing_enabled=true` |
| `GET /audio/lessons/shadowing-pilot-n5-day.wav` | 200，`audio/x-wav`，304044 字节 |
| `POST .../evaluations`（原音回灌） | 200，`unscorable/bleed_through`，返回 `unscorable_bleed_through` 建议：「录音与原音几乎重合（延迟 0 ms）」→ 戴耳机/关外放 |
| `POST .../evaluations`（600 ms 起步延迟录音） | 200，`scoreable`，66 分、`partial=true`、`lag_ms=600`、`latency_ms=35`；2 条建议：句调 39 分、节奏包络 52 分，均带数值证据与动作 |
| 真实浏览器（CDP + fake mic）录音 → 停止 → 评分 | `POST evaluations` / `POST attempts` / `GET shadowing` 全部 200；ScoreCard 显示 **70 分**、五维、「📋 下一步这样改」、`韵律` 标签、`句调轮廓与标准音相似度 44 分` 与具体练习动作；`exceptions: []`（仅 React Router v7 future flag 警告） |

**遗留**：`私の一日` 的重复课文（id 1/16）会让标准种子导入 CLI 失败；如需用 `import-lessons` 完整导入，需先决定保留哪条。本次测试产生的 demo 账号与一条 attempt 记录留在本地开发库，可从备份恢复。服务以后台进程运行，需手动停止。

---

## 18. 本地测试切换到线上 PostgreSQL（2026-11-16）

用户要求「本地测试期间改为使用线上的 PostgreSQL」，主要动机是绕开 §17 里 SQLite 那批重复课文。

### 目标确认

| 候选 | 结论 |
| --- | --- |
| `192.168.1.20:19588`（`docs/migration.md` 示例） | TCP 通，但 psql 报 `server closed the connection unexpectedly`，不是可用 PG 协议端 |
| 本机 `127.0.0.1:5432` 的 `yanye_rike` / `yanye_rike_test` | 属于别的项目（表是 `sentences` / `attempts` / `grammar_points`），无 `lessons` 表 |
| **`119.91.114.203:19588/japanese`** | 本项目线上**生产库**，可达；`lessons` 613 篇、`613` 个不同 `(title,jlpt_level)`，**零重复** |

该地址与 MinIO / JWT 等配置来自 `.workbuddy/memory/2026-11-02.md` 的「环境备忘」（此前本地就按 PG 模式跑过）。**线上没有独立测试库**（库列表只有 `japanese` / `flowspace_*` / `portfolio` / `postgres` / `tylerhu`），而 `DATABASE_URL_TEST` 绝不能指向生产库（测试会 `DROP SCHEMA public CASCADE`）。

### 启动配置

`RELATIONAL_STORE=postgres` + 历史备忘里的 `DATABASE_URL` / MinIO / `JWT_SECRET` / `APP_TIMEZONE` 等；按历史踩坑记录用 `go build -o /tmp/jl-server-pg ./backend/cmd/server/` 后再运行（避免 `go run` 长任务被 SIGKILL）。启动日志确认 `relational_store=postgres`，同时 `postgres_app.go` 会执行 `adapter.RunMigrations`。前端仍是 :35173 的 Vite dev server。

### 只读验证

| 检查 | 结果 |
| --- | --- |
| `GET /api/v1/lessons/1` | 200，`私の一日`，6 句，`audio_url=/api/v1/audio/6049/stream`（PG 侧音频走 MinIO 对象） |
| `GET /api/v1/audio/6049/stream` | 200，943820 字节，`24 kHz / mono / 16-bit` WAV（SQLite 侧是 8 kHz，两者不同） |
| `GET /api/v1/lessons/1/shadowing` | 403 `ERR_SHADOWING_DISABLED` —— 线上 613 篇课文 `shadowing_enabled` 全为 0 |

### 临时启用 → 测试 → 还原

评测端点强制要求 `ShadowingEnabled`，用户选择「临时启用→测试→还原」。脚本只改 `id=1` 的 `shadowing_enabled` 一列，评测端点本身不写库（S2 设计），用完立即还原：

- 临时启用后 session 200：6 句、`audio=/api/v1/audio/6049/stream`。
- `POST /api/v1/lessons/1/shadowing/evaluations`（24 kHz 参考音切句 + 16 kHz 合成跟读）→ 200，`unscorable/low_confidence`、`lag_ms=600`、`latency_ms=2201`；仍返回 1 条具体建议「可用评分维度不足一半，证据不够可靠」→「换到安静环境、靠近麦克风，完整匀速地读一遍整句再录」。
- **踩坑**：psql `-At` 把布尔输出为 `f`/`t`，脚本把原值 `f` 拼进还原 SQL 导致语法错误，自动还原失败；已立刻手工 `update ... set shadowing_enabled=false` 还原并复核 `enabled=0 / lessons=613 / distinct=613`，**生产库零残留**。

### 值得注意的两个差异

1. **置信度门控在 PG 真实教材上生效**：24 kHz TTS 参考音 + 合成录音使 `sync` / `prosody` 置信度不足被剔除，有效权重 < 0.5 → `low_confidence`，不给分。设计上正确（不硬编分数），但说明线上教材需要 S7 阈值校准，否则评分会经常不可用。
2. **延迟**：MinIO 下载 + 24 kHz F0 使单次评测 `latency_ms=2201`，而 SQLite 本地文件是 35 ms。PG/MinIO 模式需关注 P95。

### 当前状态与遗留

- 服务保持运行：后端 :30081（PG 模式）、前端 :35173。因为线上库影子跟读仍是 disabled，页面 `/lesson/1/shadowing` 会 403；要在浏览器完整体验需再次临时启用或选「启用并保留」。
- SQLite `data/app.db` 一字未动（30 行、id=1 仍 enabled、demo 账号仍在）；§17 的 15 组重复课文仍未清理，但现在服务走 PG，不影响测试。
- 两份 SQLite 备份与两张截图保留；线上库无写入残留。

---

## 19. 线上 PG 的 shadow / repeat 双模式真实评分结果（2026-11-16）

用户要求「临时启用吧，先给我看评分结果」。`/tmp/jla_pg_show.py` 的执行步骤：

- 前置状态 `BEFORE: shadowing_enabled=f version=1`；只改 `id=1` 的 `shadowing_enabled` 一列 → `true`。
- 从 `/api/v1/audio/6049/stream` 拉 MinIO 参考音（24 kHz / 19.66s），线性重采样到 16 kHz → `/tmp/jla_pg_ref.wav`。
- 生成两份 16 kHz mono 16-bit WAV：`/tmp/jla_pg_shadow.wav`（600 ms 前置静音 + 3 s 原音）、`/tmp/jla_pg_repeat.wav`（3 s 原音，无延迟）。
- 对 `POST /api/v1/lessons/1/shadowing/evaluations` 分别以 `mode=shadow` / `mode=repeat` 提交；结果写入 `/tmp/jla_pg_results.json`。

### 顶层结果

| 项 | shadow | repeat |
| --- | --- | --- |
| `status` | `unscorable` | `unscorable` |
| `unscorable_code` | `low_confidence` | `low_confidence` |
| `overall_score` | **无**（不下发机器分） | **无** |
| `partial` | false | false |
| `lag_ms` | 600（正好命中目标延迟） | 0 |
| `latency_ms` | 2815 | 3068 |
| `weights_version` | `w-shadow-1.0` | `w-repeat-1.0` |
| `score_source` / `scorer_version` / `model_version` | `local_prosody_v1` / `local-prosody-1.0` / `none` | 同左 |
| 具体建议 | 1 条 `unscorable_low_confidence` | 1 条同 |

### 五维明细（score / confidence / weight / available）

| 维度 | shadow | repeat |
| --- | --- | --- |
| sync | 99.18 / **0.704** / 0.30 / ✓ 过门控 | 80.60 / **0.745** / 0.10 / ✓ 过门控 |
| prosody | 89.30 / 0.462 / 0.30 / ✓ 淘汰 | 95.90 / 0.556 / 0.25 / ✓ 淘汰 |
| fluency | 77.92 / 0.412 / 0.15 / ✓ 淘汰 | 77.92 / 0.495 / 0.15 / ✓ 淘汰 |
| coverage | 0 / 0 / 0.15 / ✗ 无 ASR | 0 / 0 / 0.15 / ✗ 无 ASR |
| segmental | 0 / 0 / 0.10 / ✗ 无 ASR | 0 / 0 / 0.35 / ✗ 无 ASR |
| **有效权重** | **0.30** | **0.10** |

### 根因（核对 `evaluate_scoring.go`）

- `minDimensionConfidence = 0.60`、`minEffectiveWeight = 0.50`。
- `scoreFluency` 置信度 = `vad.VoicedRatio`（0.412 / 0.495）；`scoreProsody` 置信度 = `f0Ratio / 0.6`（反推有声帧比例仅约 0.277 / 0.333）。24 kHz TTS 参考音的 VAD/F0 有声帧比例偏低。
- `scoreEnvelope` 置信度 = `0.5*VoicedRatio + 0.5*(1 - DTW 归一化距离)`。本次合成录音是参考音的近拷贝，DTW 距离近 0，所以只有 sync 达到 0.704 / 0.745 过线。
- `scoreCoverageAndSegmental()` 在 P0 无 ASR 时恒 `Available:false`，永久缺 0.25(shadow) / 0.50(repeat) 权重。
- 有效权重 0.30 / 0.10 < 0.50 → `Aggregate` 返回 `Valid:false, Code:"low_confidence"`。**门控按设计正确工作，没有崩溃、没有硬凑分数。**

### 结论与待办

1. 线上 24 kHz TTS 教材需要 **S7 阈值校准**（`minDimensionConfidence` / `minEffectiveWeight` / F0 voiced ratio 归一化口径），否则 PG/MinIO 模式下评分会经常不可用。
2. **repeat 模式的结构性问题**：把 0.50 权重留给 ASR 两维，本地 P0 下三个本地维必须全部过 0.60 才「恰好」凑到 0.50，容错为零；S6 接 ASR 之前 repeat 基本不可评分。建议 S7 顺带按模式调整有效权重下限或给本地可评维度临时重分配。
3. 不可评建议文案「换到安静环境、靠近麦克风，完整匀速地读一遍整句再录」把**算法侧证据不足**归因成用户录音环境，属于误导；S7 应区分技术原因与录音质量原因。
4. 本次「录音」是把参考音重采样后回灌的合成音，**不是真人发音**，不能据此标定阈值；需真人样本集。

### 生产库收口

展示结果后执行 `update lessons set shadowing_enabled=false where id=1 and shadowing_enabled=true;` → `UPDATE 1`；复核 `id=1 → f / version=1 / config={}`、`lessons=613 / distinct=613 / enabled=0`，**生产库零残留**。服务仍以 PG 模式运行在 :30081 / 前端 :35173。

---

## 20. S7 阈值校准与 repeat 模式权重（2026-11-16，部分完成）

用户指令：「先开 S7 做阈值校准，repeat 权重按模式调」。触发点是 §19 里线上 PG 两种模式都返回 `unscorable/low_confidence`。

### 实测诊断（校准依据，不是拍脑袋改数字）

用 `/tmp/jla_pg_ref.wav`（线上 MinIO 24 kHz 参考音）、`/tmp/jla_pg_shadow.wav`、`/tmp/jla_pg_repeat.wav` 逐帧统计：

| 音频 | 采样率 | RMS 帧 | VAD 有声比 | VAD 有声帧 | F0 帧 | F0 有声帧 | F0 有声比 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 完整参考音 | 24 kHz | 1965 | 0.532 | 1046 | 1963 | 822 | 0.419 |
| shadow 合成录音 | 16 kHz | 359 | 0.412 | 148 | 357 | 99 | 0.277 |
| repeat 合成录音 | 16 kHz | 299 | 0.495 | 148 | 297 | 99 | 0.333 |

**关键观察**：F0 有声帧有 99 帧（≈990 ms 浊音），证据其实充足；只是「占全部帧的比例」只有 28–33%。旧公式把「比例」当成「可信度」，于是把可评的维度误杀。日语 TTS / 自然语音本来就含大量清辅音与停顿，60% 有声比不是合理基准。

### 两个根因

1. **置信度口径错误**：`prosody = f0Ratio / 0.6`、`fluency = vad.VoicedRatio`、`sync = 0.5*VoicedRatio + 0.5*alignConf`。三者都在用「相对比例」度量「绝对证据量」。
2. **repeat 权重结构性失效**：`coverage 0.15 + segmental 0.35 = 0.50` 在无 ASR 阶段永远 `Available:false`，但 `Aggregate` 拿未重分配的名义权重去和 `minEffectiveWeight=0.50` 比较，等于要求三个本地维全部过线才「恰好」达标，容错为零。注释写着 "renormalized"，实现却没做。

### 设计决策：不降低门槛，修的是测量

- `minDimensionConfidence` 仍是 **0.60**、`minEffectiveWeight` 仍是 **0.50**，门控没有放水。
- 置信度改为按**绝对有声帧数**归一化：
  - `prosody = voicedF0Frames / 60`（600 ms 浊音即满置信）
  - `fluency = vad.VoicedRatio * frameCount / 90`（900 ms 有声即满置信）
  - `sync = 0.5*voicedConfidence + 0.5*alignConfidence`
- `Classify` 仍先拦 `silence` / `too_short` / `bleed_through`，短录音不会因改口径而混进可评。

### 版本化配置（`internal/module/shadowing/evaluate_config.go`）

§8 要求「常数全部放进 `weights_version` 对应的配置结构，不散落在代码魔数里」，本次落地：

```go
type ScoringConfig struct {
    WeightsVersion string
    Weights        map[string]float64 // 全 provider 齐备时的理想 rubric
    LocalWeights   map[string]float64 // 无 ASR 时按模式重分配后的 rubric
    MinDimensionConfidence, MinEffectiveWeight, MinAchievableWeight float64
    MinActiveDimensions int
    ProsodyFullConfidenceFrames, FluencyFullConfidenceFrames int
    SyncVoicedConfidenceWeight float64
    // silence / too_short / bleed / targetLag / σ / DTW k / band / maxLag ...
}
func ScoringConfigForMode(mode EvaluationMode) ScoringConfig // 每次返回新副本
```

原来散在 `evaluate_scoring.go` 常量块的 14 个魔数全部移入。`Aggregate` 用 `sort.Strings` 固定遍历顺序，避免 map 随机顺序造成浮点累加差异。

### repeat 权重按模式调

| 模式 | 名义权重（全 provider） | 本地权重（无 ASR，和为 1） |
| --- | --- | --- |
| shadow | 0.30 / 0.30 / 0.15 / 0.15 / 0.10 | sync **0.40** / prosody **0.40** / fluency **0.20** |
| repeat | 0.10 / 0.25 / 0.15 / 0.15 / 0.35 | sync **0.20** / prosody **0.50** / fluency **0.30** |

- `effectiveWeights()` 先按「ASR 维度是否产出分数」选名义 rubric 或本地 rubric，再把不可用维度的权重清 0、其余归一化到 1.0。
- 门控现在是「**可获得的证据里有多少可信**」，而不是「理想 rubric 里有多少可信」。
- 新增两道防退化闸：`MinActiveDimensions = 2`（不允许只靠一个维度出分）、`MinAchievableWeight`（shadow 0.35 / repeat 0.30，防止只剩两个 ASR 维度时凑分）。
- `AggregateResult` 新增 `Weights` / `AchievableWeight`，`EvaluationResult.Scores[].Weight` 现在返回**实际生效权重**，JSON 自描述。
- `advice.go` 的建议排序改用 `DimensionScore.Weight`（即生效权重），并加名义权重兜底，保证与评分同一套 rubric。

### 版本四元组（bump，保证历史分可比较）

| 项 | 旧 | 新 |
| --- | --- | --- |
| `scorer_version` | `local-prosody-1.0` | `local-prosody-1.1`（置信度算法变了） |
| `weights_version` | `w-shadow-1.0` / `w-repeat-1.0` | `w-shadow-1.1` / `w-repeat-1.1` |
| `score_source` / `model_version` | `local_prosody_v1` / `none` | 不变 |

前端两份测试夹具同步改成 1.1；§13/§15/§19 是历史实测记录，保留旧版本号不改。

### TDD 与验证

- **Red**：新增 `internal/module/shadowing/evaluate_s7_test.go`，首次运行 4 处编译失败（`ScoringConfigForMode` / `AggregateResult.Weights` 不存在）。
- **Green**：5 组表格驱动测试 —— 配置完整性与不可变副本、模式化生效权重、证据不足拒绝出分、稀疏浊音 TTS 可评、真实种子音频回归。
- **稀疏浊音回归**：`sparseSpeechWAV(16000, 3, 0.20)` 复现线上 24 kHz TTS 的 F0 低有声比，旧实现会 `low_confidence`，新实现 `scoreable / overall=79`，prosody 置信度 0.95、fluency 0.72、sync 0.86。
- **线上 PG 真实音频离线复测**（`/tmp/jla_pg_*.wav`，`EvaluateWAV` 直调，**未连生产库、未写库**）：

  | 模式 | 修复前 | 修复后 |
  | --- | --- | --- |
  | shadow | `unscorable/low_confidence`，无总分，lag 600 ms | **`scoreable` 89 分**，`partial=true`，lag 600 ms；sync 99.1 / prosody 90.5 / fluency 79.9，权重 0.4/0.4/0.2；建议「第 1.4 s 处停了 360 ms」 |
  | repeat | `unscorable/low_confidence`，无总分，lag 0 ms | **`scoreable` 88 分**，`partial=true`，lag 0 ms；sync 83.8 / prosody 96.3 / fluency 79.9，权重 0.2/0.5/0.3；建议「第 0.8 s 处停了 360 ms」 |

- 既有回归未破：真实种子音频 repeat 8 kHz 100 分 / 16 kHz 93 分，`bleed_through`、`silence`、`too_short`、混合采样率、建议非空全部保持。
- 全量：Go `go test ./... -count=1`（27 包）、`go vet ./...`、`go build ./...` 全过；前端 `vitest` 19 文件 168 测试、`tsc --noEmit`、`vite build` 220 模块全过。
- 新增 `go build -o /tmp/jl-server-pg-s7 ./backend/cmd/server/` 备用二进制；**运行中的 :30081 仍是旧二进制**，未重启（PG 模式启动会 `RunMigrations`，避免对生产库产生额外副作用）。

### 仍未完成（S7 只算部分完成）

1. §8 要求的 **20–30 条真人录音样本集**：母语者不误报、串音 100% 命中、慢速/抢拍/长停顿单调性、男女声分差 < 8，均未验收。当前阈值来自合成样本 + 线上 TTS 教材，**不能据此宣布校准完成**。
2. `unscorable/low_confidence` 的建议文案仍写「换到安静环境、靠近麦克风」，把算法侧证据不足归因成用户录音环境（§19 第 3 条），本次按用户指定范围未改。
3. `sync` 置信度仍含 `alignConfidence = 1 - DTW 距离`，概念上把「节奏差异大」当成「测量不可信」，与评分目标相反；需真人样本才能重标。
4. `internal/config` 的 4 个评分开关仍未接入；真人样本集到位后应把校准常数做成可配置项而非编译期常量。
5. ~~运行中的 :30081 需重启才会带上 S7~~ → 已重启，见 §21；生产 PG 的 `shadowing_enabled` 保持 `false`。

---

## 21. S7 真实 HTTP 复测（重启 + 临时启用，2026-11-16）

用户指令：「先重启 :30081 并临时启用，做一次真实 HTTP 复测」。

### 重启

| 项 | 结果 |
| --- | --- |
| 旧进程 | PID 49963（`jl-server`，S7 之前的二进制）→ `kill`，端口 30081 释放 |
| 新进程 | PID 91638，`/tmp/jl-server-pg-s7`（`go build -o ... ./backend/cmd/server/`），按 §18 同一套 PG / MinIO / JWT 环境启动 |
| 启动日志 | `level=INFO msg="server starting" addr=:30081 relational_store=postgres` |
| 健康检查 | `GET /healthz` → 200 `ok` |
| 迁移副作用 | `schema_migrations` 仍 12 条，最新 `applied_at=2026-11-14`，**本次重启未新增迁移**；`lesson_shadowing_attempts=0` |

前端 :35173（PID 50236）保持运行，未改动。

### 临时启用 → 真实 HTTP → 还原

`/tmp/jla_pg_s7_http.py` 只改 `lessons.id=1` 的 `shadowing_enabled` 一列，`finally` 里用**固定 SQL 布尔字面量**还原（避开 §18 踩过的 psql `-At` 把布尔输出成 `f` 再拼进 SQL 的坑）。评测端点本身不写业务表。

- `BEFORE: f|1|{}` → `TEMP ENABLED` → `GET /api/v1/lessons/1/shadowing` 200，`enabled=True`、6 句、`audio=/api/v1/audio/6049/stream`
- 两份 16 kHz 录音复用 §19 生成的 `/tmp/jla_pg_shadow.wav`（600 ms 延迟）与 `/tmp/jla_pg_repeat.wav`，走 `POST /api/v1/lessons/1/shadowing/evaluations` 真实 multipart + MinIO 参考音

| 模式 | HTTP | status | 总分 | partial | lag | 耗时 | 版本 | 建议 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| shadow | 200 | **`scoreable`** | **88** | true | 600 ms | 2425 ms | `local-prosody-1.1` / `w-shadow-1.1` | 1 条：`fluency_long_pause`「第 1.4 s 处停了 360 ms」+ 具体练习动作 |
| repeat | 200 | **`scoreable`** | **86** | true | 0 ms | 3366 ms | `local-prosody-1.1` / `w-repeat-1.1` | 1 条：`fluency_long_pause`「第 0.8 s 处停了 360 ms」+ 具体练习动作 |

五维（真实 HTTP 响应）：

| 维度 | shadow 分数/置信/权重 | repeat 分数/置信/权重 |
| --- | --- | --- |
| sync | 99.2 / 1.00 / **0.40** | 80.6 / 1.00 / **0.20** |
| prosody | 89.3 / 1.00 / **0.40** | 95.9 / 1.00 / **0.50** |
| fluency | 77.9 / 1.00 / **0.20** | 77.9 / 1.00 / **0.30** |
| coverage / segmental | 0 / 0 / 0，unavailable | 同左 |

`weights_version=1.1` 与三维权重 0.4/0.4/0.2、0.2/0.5/0.3 同时证明**运行中的二进制确实是 S7 版本**、模式化权重在真实链路上生效。相比 §19 修复前的 `unscorable/low_confidence`（无总分），两种模式都已正常出分。

> 与 §20 离线直调的 89 / 88 分有 1–2 分差异：服务端按句子 timeline 从 MinIO 切参考音，离线复测用的是 `SliceWAV(ref, 0, 3000)`，切句边界略不同。属预期，不影响结论。

### 还原复核（独立执行，非脚本自报）

```
id=1            -> f|1|{}
totals          -> 613 lessons / 613 distinct / 0 enabled
migrations      -> 12（最新 2026-11-14，无新增）
attempts        -> lesson_shadowing_attempts = 0
GET /lessons/1/shadowing（flag off）-> 403 ERR_SHADOWING_DISABLED
```

**生产库零残留。** 服务保持运行：后端 :30081（S7 二进制、PG 模式）、前端 :35173。因为开关已还原，页面 `/lesson/1/shadowing` 会 403；若要在浏览器完整体验需再次临时启用。

完整响应保存在 `/tmp/jla_pg_s7_http_results.json`，脚本在 `/tmp/jla_pg_s7_http.py`。

### S7 仍未完成（与 §20 相同，本次未推进）

真人样本集（母语者误报 / 串音 100% / 男女声分差 < 8）、`low_confidence` 误导文案、`sync` 的 `alignConfidence` 概念倒挂、`internal/config` 的 4 个开关。本次只做「重启 + 真实 HTTP 复测」，未改任何业务代码。

---

## §22 合并课文段落线进影子跟读 PR（2026-11-17）

按「方案 1」把本地提交 `826a01d` 补进 `codex/shadowing-local-scoring`，解决两套阅读 UI 的取舍。

### 冲突处理

独立 worktree `/tmp/jlwt-pr-merge` 上 `git cherry-pick 826a01d`，2 处冲突：`LessonDetailPage.tsx`、`LessonPage.module.css`。两处都保留 PR 分支版本（`--ours`）——以远端 `LessonReading` 竖排/批注/点选组件为骨架；本地旧 `styles.article*` 和「显示翻译」开关由远端的「中日对照」模式覆盖，且这些类在远端已无任何引用。

### 段落能力整合（不回退阅读 UI）

- `lessonParagraphs.ts` 新增 `groupSentencesByParagraph()`，并把 `toArticleParagraphs()` 改成泛型。两者都接收 `indexLessonText()` 产出的带 `start` 偏移句子，**段落分组不重置全局 offset**，已保存的批注范围不会错位。
- 竖排模式：每个段落渲染一个 `<p class="verticalText" data-lesson-paragraph="N">`，自带 1em 缩进，竖排下自然另起一列；不再把全文拼成一条不断行的长段落。
- 对照模式：按段落包一层 `.paragraphGroup`，保留逐句翻译与逐句 `ErrataButton` 的 `itemKey`，段落之间用更重的分隔线标记。
- 旧课文没有 `paragraph` 字段 → 仍归为单段落，行为与远端原来一致。

### 迁移命名（易踩坑）

线上 PG 的 `schema_migrations` 已同时存在 `010_lesson_paragraph.sql`、`010_study_plans.sql`、`011_lesson_annotations.sql`。其中 `010_lesson_paragraph.sql` 的 checksum `54f21d1e...6885` 与本地文件完全一致，部署时是 no-op。**不要顺手把它改名成 `012_*`**：虽然 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 幂等，但改名会让部署机重复执行并多写一条 migration 记录。`010_` 前缀重复是历史事实，不是冲突。

### 提交与验证

- `ae84eba`：cherry-pick 原提交，28 文件 +2646/-131（冲突按上述解决）。
- `6e56136`：阅读 UI 整合，5 文件 +130/-28。
- `origin/codex/shadowing-local-scoring = 6e56136`，相对 `origin/master` 共 85 文件 +10534/-238。
- Go `test ./... -count=1` / `vet ./...` / `build ./...` 全过；前端 `vitest` **23 文件 196 测试** / `tsc --noEmit` / `vite build` 232 模块全过。新增 4 个段落分组测试与 2 个阅读渲染测试，其中一条断言跨段落 `data-lesson-offset` 仍连续为 `[0, 1]`。
- 完整 PR diff 扫描无生产凭据（文档里只有 `JWT_SECRET=local-dev-secret` 本地占位）。
- 合并链接不变：`https://github.com/hujinrun-github/japanese-learning-app/compare/master...codex/shadowing-local-scoring?expand=1`

---

## 参考

- `docs/spec/shadowing-scoring-redesign.md`（本计划的上位设计）
- `docs/spec/japanese-pronunciation-mvp-design.md`（mora 专项规则与 issues 结构）
- `internal/cli/wav_trim.go`、`internal/module/speaking/vllm_scorer.go`、`internal/module/speaking/cer.go`
- `backend/cmd/server/audio_stream.go`、`internal/data/postgres/audio_object_store.go`
- `constitution.md`、`AGENTS.md`
