# 影子跟读评分体系重设计方案

- 版本：v1.0（设计稿，未实现）
- 日期：2026-11-16
- 适用：`internal/module/shadowing`、`front/react/src/pages/shadowing`、PostgreSQL + MinIO 链路
- 上位约束：`constitution.md`（简单性、TDD、标准库优先、显式错误处理与日志）、`AGENTS.md`
- 相关文档：`docs/spec/japanese-pronunciation-mvp-design.md`（朗读场景，本方案保留其 mora 规则与反馈结构，但**不沿用其权重与场景假设**）、`docs/shadowing.md`（UI 设计稿）

---

## 0. 决策摘要

**不要直接买一个发音评测 API，把它的总分当成影子跟读分数。** 这是本次重新设计的核心结论。

推荐方案：**双练习模式 + 五维分项 + 置信度门控 + 可插拔评分器 + 版本化 + 掌握度模型**。

| 决策 | 结论 | 理由 |
| --- | --- | --- |
| 模式 | 拆成「同步影子 shadow」与「逐句复读 repeat」两套评分 | 同步跟读的原音串音、吞词、延迟会让音素级评测失真；研究也表明影子跟读训练的是超音段而非单音素 |
| 主分 | 影子模式把「同步/节奏 + 韵律」提到 60%，音段准确度只占 10% | Whitworth & Rose (2025) 对 44 项研究的系统综述 |
| 底座 | P0 复用项目已有的 vLLM Whisper（`VLLMScorer`）+ 纯 Go 韵律层，零新增付费依赖 | 符合宪法「少依赖 / 标准库优先」，代码已存在且测试完整，只是未接线 |
| 增强 | P2 才把 Azure `ja-JP` 作为可插拔 Provider，**只在 repeat 模式**启用 | Azure 日语音段评测可靠且便宜，但 Prosody 仅 en-US、不打分调型 |
| 安全 | 先判「能不能评」，再打分；不可评不显示分数 | 借鉴 TonePerfect 的不同文本拒评与母语者误报测试，避免错误低分打击学习者 |
| 掌握度 | 用 EMA + 有效评测次数替代现在的 `DISTINCT sentence_index` | 当前只要有一次 attempt 就算「完成」，与分数完全无关 |

---

## 1. 现状与问题

### 1.1 代码事实（已核对）

| 位置 | 现状 | 问题 |
| --- | --- | --- |
| `ShadowingPage.tsx` | 录音走 `useAudioRecorder` 生成浏览器本地 Blob，只本地回放 | 录音**不上传**，服务端拿不到音频 |
| `api/shadowing.ts` | `SaveAttemptRequest` 只提交 `self_score` 等 | 手填 0–100 就是「评分」的全部 |
| `shadowing/model.go` | `Attempt` 已预留 `recognition_text`、`audio_ref` | 字段存在但前端从不传，后端也无评测逻辑 |
| `002_shadowing.sql` | attempts 表有 `self_score`、`recognition_text`、`audio_ref` | 无评测结果、无评分器版本字段 |
| `shadowing_store.go:115` | `MAX(a.self_score) AS best_score` | 最优分 = 手填最高分 |
| `shadowing_store.go:159` | `SELECT DISTINCT sentence_index` 即「已完成句」 | **只要录过一次就算完成**，与质量无关 |
| `speaking/scorer.go` | `WaveformScorer`：RMS + 过零率两个全局标量做相似度 | 无判别力：任意两段响度/过零率接近的音频都得高分，不能当发音分 |
| `speaking/vllm_scorer.go` | vLLM Whisper ASR + `ComputeCER` → 0–100 + 转写文本，**已实现且有完整测试** | **未接线**（`main.go` / `postgres_app.go` 注入的是 `WaveformScorer`）；且 CER 只测「ASR 听成了什么」，不测发音质量 |
| `docs/spec/japanese-pronunciation-mvp-design.md` | 四维（完整度 30 / 准确度 35 / 节奏 25 / 流利度 10）+ mora 规则 + issues 结构 | 面向**固定文本朗读**，未处理同步跟读的串音、lag、破损语音；未实现；无置信度、不可评、版本化、掌握度 |

### 1.2 由此得出的设计约束

1. 必须先把「录音 → 服务端 → 可复现」这条链路打通，否则任何评分算法都落不了地。
2. 现有 `WaveformScorer` 应从发音评分中移除（可保留为静音/有效性检测的辅助）。
3. `VLLMScorer` 可复用，但只能作为**内容覆盖度 / 可懂度代理**，不能单独冒充发音分。
4. 数据库与 API 必须向后兼容现有 `self_score` 数据，不回填、不重写历史。

---

## 2. 业界案例调研（核对日期 2026-11-16）

> 价格与能力均为厂商公开页面口径，需以实际合同/实测为准；竞品厂商发布的 benchmark 需自行复跑。

### 2.1 商用发音评测 API

| 产品 | 日语 | 输出粒度与维度 | 日语韵律 | 价格口径 | 对本项目的意义 |
| --- | --- | --- | --- | --- | --- |
| **Azure AI Speech · Pronunciation Assessment** | `ja-JP` GA（33 locales 之一） | 音素/词/全文 `AccuracyScore`、`FluencyScore`、`CompletenessScore`、`PronScore`；`ErrorType` 含 Mispronunciation / Omission / Insertion / UnexpectedBreak 等；带 Offset/Duration 时间戳 | **Prosody 仅 en-US**；不判アクセント型 | 按 STT 音频时长，标准实时约 **$1/音频小时**（短句极便宜）；完整功能在 Speech SDK 内 | 性价比最高的**音段层**底座；但韵律缺口必须自建 |
| **SpeechSuper** | 支持（8 语种之一） | overall / phoneme / word / sentence / fluency / completeness；句级 2–200 字 | 未公开日语音调明细 | 按请求：句 $0.006、$20/月最低 | 句级计费简单，REST/WS 接入比 SDK 轻；可作 Azure 备选 |
| **TonePerfect** | `ja-JP` 2026-10 上线，官方标实验态 | sound / word / sentence，expected vs heard 音素，**逐词 correction decision** | 官方明确 **"Pitch accent is not scored"**；日语无词级时间戳 | $10/月起 2,500 credits（每 15s 一 credit） | 日语尚不成熟，但两个**产品化思想**值得抄：① 逐单元「该不该纠错」的决策位 ② 母语者误报率作为发布门槛 ③ 不同文本拒评且不计费 |
| **DolphinSOE** | 日本本土首个日语发音评测 API | 词/句/篇章；音素准确度、漏读/错读/多读、流利度、停顿 | 公开材料未承诺调型打分 | 未公开 | 专门为「ら行・つ・ん」设计，若允许外采可索取日语样例 JSON 后再评估 |
| **Speechace / ELSA API** | **不支持日语** | 英语为主（IELTS/PTE/CEFR 映射） | — | — | 直接排除 |
| **腾讯云 SOE / 有道 / 阿里云** | 日语「口语评测」是否在售未获公开确认 | 多为中英文准确度/完整度/流利度 | — | — | 不假定可用；若走国内云需先 POC 验证日语评测（非仅 ASR） |

**独立/半独立验证**：Lingolix 2026 年 10 月发布 6 语种 1,700 条录音的开放 benchmark（由其自身参与，需谨慎看待）：日语「植入错误 vs 正确读音」检测 ROC AUC，Azure 约 0.93；在 80% 召回下把正确词误标红的比例，日语 Azure 约 3%。这支持「Azure 日语音段层可用」，但**不能推出它适合影子跟读**。

### 2.2 日语专用参考与学术依据

- **OJAD / Suzuki-kun**（东京大学）：输入日文文本给出东京式调型、アクセント核位置、元音无声化拍、F0 参考曲线，并可生成模型音。**是参考真值，不是测评引擎**——正好用于自建韵律层的目标轮廓。
- **Whitworth & Rose (2025)**，*Research Synthesis in Applied Linguistics* 1(2), 239–269，44 项研究系统综述：影子跟读对**流利度、韵律/节奏、可理解度**有一致正效应；对**单音素准确度（segmental）证据不一致**，对口音减轻无显著变化。→ **权重设计的直接依据**。
- **Luo / Minematsu 等（东大，2008 Interspeech / ISCSLP）**：影子语音因追赶语速常「破损、含混」，传统朗读评分会失效；提出 GOP、连续音素识别（CPR）、时间约束自底向上聚类三种评分，与 TOEIC 相关性分别约 0.82 / — / 0.72，NPPW 达 0.84。→ **必须允许对齐「跳格」、必须处理吞词**。
- **Yue 等（Interspeech 2017）**：DNN 后验 + DTW 的影子语音自动评分，与教师评分相关性较 HMM 大幅提升（跨语言后验 DTW 仍达约 0.74）。→ **F0/后验 + DTW 是可行且轻量的技术路线**。
- **IJETI 2026**（Wav2Vec2 分词 + DTW 比对 pitch-accent 模式，LSTM 判别 F1≈0.92）、**PASQA 2026**（LY Corp，mora-conditioned 的日语声调质量评估，开源）：证明日语韵律可自动评估，但需要模型/标注数据，属于 P2 范畴。

### 2.3 影子跟读特有的工程前提（业界共识）

1. **原音串音（bleed-through）**：外放时麦克风会收进模型音频，评分直接失真。业界做法是开始前检测/提示佩戴耳机，并做 AEC 或回采消除。
2. **lag 估计**：用户相对原音的起始偏移（典型 0.3–1.5s）本身是有效指标，也是对齐的前提。
3. **对齐容错**：吞词时先做词级粗对齐再局部细化，允许跳过，否则整句对齐崩塌。

---

## 3. 设计原则

1. **影子跟读 ≠ 朗读**：不同模式不同权重，同步模式不做音素级判错。
2. **先判可评，再打分**：证据不足返回 `unscorable`，绝不给一个看起来像机器分的低分。
3. **可诊断 > 可比较**：分项 + 定位到 mora/词 + 置信度 + 一条可执行建议；分数只用于用户与自己的上一次对比，不做跨用户、跨句排名（句长/难度天然不可比）。
4. **小接口、显式注入**：一个 `Evaluator` 接口 + 两个实现，不做插件注册表/工厂链（YAGNI）；依赖通过 `NewService(...)` 参数注入，禁止全局变量。
5. **版本化与可降级**：`scorer_version` / `weights_version` / `model_version` / `shadowing_version` 四元组入库；失败逐级降级并标记 `score_source`，绝不伪造机器分。
6. **隐私最小化**：录音默认私有、限期删除、不落日志、不用于训练。

---

## 4. 核心设计

### 4.1 双练习模式

| | `shadow` 同步影子 | `repeat` 逐句复读 |
| --- | --- | --- |
| 交互 | 原音播放中跟随复述 | 暂停原音，听完一句再录 |
| 串音风险 | 高（必须耳机检测 + 后端串音判定） | 低 |
| 训练目标 | 节奏、句调、可懂度、听-说同步 | 音素准确度、拍结构、内容完整 |
| 是否做音素级判错 | **否**（仅高置信度参考项） | **是** |
| 现有 UI 落点 | 现有「Record」按钮改造为提交评测 | 新增「逐句精练」入口（可与 `docs/spec/japanese-pronunciation-mvp-design.md` 的朗读闭环合并） |

### 4.2 评分维度（各维 0–100 + `confidence`）

| 维度 | 定义与计算 | shadow 权重 | repeat 权重 | 置信度来源 |
| --- | ---: | ---: | --- | --- |
| **Sync 同步/节奏** | 与参考音频的能量包络 DTW 距离 → lag 偏移、拍间时长相关、拍等时性 CV | **30%** | 10% | 对齐路径质量、VAD 有效帧比 |
| **Prosody 韵律** | F0 轮廓（半音、去 declination）与参考的 DTW 相似度 + 句末边界调 + 词调型 H/L 比对（参考 OJAD） | **30%** | 25% | 有声帧占比、F0 提取有效帧比 |
| **Coverage 完整度** | ASR 转写与参考文本的 mora/词覆盖率（漏读、多读、替换） | 15% | 15% | ASR 置信度、文本规范化成功率 |
| **Fluency 流利度** | 非预期静默比、句中破断次数、语速比 | 15% | 15% | VAD 稳定性 |
| **Segmental 音段** | 音素/mora 级准确度（本地：CER + 专项规则；P2：Azure 音素分 + ErrorType） | 10% | **35%** | Provider 返回置信度 |
| 报告项（不计入主分） | 可懂度代理（ASR WRR）、lag(ms)、自评与机器分偏差 | — | — | — |

**为什么 shadow 把超音段抬到 60%**：§2.2 的系统综述显示影子跟读的稳定收益在 fluency / prosody，segmental 证据不一致；若沿用朗读评测「准确度 35%」的权重，会把「追得上但个别音不完美」误判为低分，与训练目标相悖。**为什么 Coverage 只给 15%**：同步跟读天然允许滞后与局部吞词，Completeness 主导会惩罚初学者。

### 4.3 聚合算法

不用简单平均，采用「**最低分加权**」（借鉴 Azure `PronScore` 的加权聚合思想，非其公开公式）：

```
effective = 各维中 confidence >= 0.60 的维度
若 effective 权重和 < 0.50 → status = unscorable / low_confidence
otherwise:
  s_min  = effective 中得分最低者（按原始权重）
  others = 其余 effective 维度
  overall = 0.4 * s_min + 0.6 * weighted_avg(others, 原始权重归一化)
```

- 某维塌陷会被显著反映，符合「最短板决定这次练习」的教学直觉。
- 低置信度维度**不按 0 计**，而是剔除后重分配权重，结果标 `partial=true`。
- 展示分档：`>=85` 掌握 / `70–84` 合格 / `<70` 再练一次；档位仅在 `partial=false` 时生效。

示例（shadow）：Sync 70 / Prosody 85 / Coverage 90 / Fluency 80 / Segmental 88 → `s_min=70`，其余加权均值 ≈85.4 → `overall ≈ 79`。

### 4.4 不可评判定（关键，先于打分）

| code | 判定条件（初值，需真实样本校准） | 返回给用户 |
| --- | --- | --- |
| `silence` | VAD 有效语音帧 < 5% 或 RMS 低于阈值 | 「没检测到声音，请靠近麦克风再试」 |
| `too_short` | 有效时长 < 参考句时长的 30% | 「录音太短了，试着跟完整句」 |
| `bleed_through` | 录音与参考音频互相关峰值 > 0.8 且 lag ≈ 0；或能量包络几乎重合 | 「检测到原音被录入，请戴耳机或调低外放」 |
| `content_mismatch` | ASR 转写与参考文本 CER > 0.7（不计费/不计分） | 「这次读的好像不是当前句，已跳过评分」 |
| `low_confidence` | 有效维度权重和 < 0.5 | 「环境有点吵，这次先不打分」 |
| `provider_unavailable` | 评分器超时/失败且降级链耗尽 | 「自动评分暂不可用，已保留你的自评」 |

**不可评时不写 `overall_score`、不进入掌握度、不计入「已完成」**，但仍保留 attempt 与录音供用户自己回放。

### 4.5 反馈生成

沿用旧文档的 `issues[]` 结构并扩展：

```json
{
  "type": "sokuon",
  "mode": "repeat",
  "mora_index": 1,
  "audio_range_ms": [420, 510],
  "severity": "medium",
  "confidence": 0.82,
  "message_zh": "「がっこう」的促音「っ」停顿不够明显。",
  "action_zh": "先按「が・っ・こ・う」四拍慢读，再连起来。"
}
```

规则：
- 每句最多 **2 条 issue + 1 条动作**，按 `severity × confidence` 排序，避免刷屏。
- 措辞随置信度：`>=0.85` 断言；`0.60–0.85` 用「可能 / 建议」；`<0.60` 不展示。
- 日语专项规则复用旧文档 §6：长音、促音、拨音、清浊、拗音、拍节奏；P1 增加 `pitch_accent_type`、`intonation_contour`、`shadow_lag`。
- **不接 LLM 生成反馈**（YAGNI + 宪法），用模板 + 规则表，可控、可测、可解释。

---

## 5. 工程架构

### 5.1 数据流

```
浏览器 WebAudio (16k/mono/PCM)
   │  录 WAV（避免引入 opus 解码依赖）
   ▼
POST /api/v1/lessons/{id}/shadowing/evaluations   (multipart, Idempotency-Key)
   ▼
shadowing.Handler
   ├─ 校验 shadowing_version / sentence_index / mode（沿用现有版本冲突机制）
   ├─ audio_objects(private, kind=shadowing_attempt) → MinIO
   ▼
shadowing.Service.Evaluate(ctx, req)
   ├─ 预处理：WAV 解析（复用 cli.TrimWAVSilence 的 PCM 读取）+ VAD + 有效性/串音判定
   ├─ LocalEvaluator
   │    ├─ vLLM Whisper ASR（复用 speaking.VLLMScorer）→ Coverage + recognized_text
   │    └─ 纯 Go 韵律层：能量包络 DTW + YIN/自相关 F0 + DTW → Sync / Prosody / Fluency
   ├─ [P2] AzureEvaluator（仅 repeat）→ 音素分 + ErrorType，替换 Segmental
   ├─ 置信度门控 → 聚合（§4.3）→ issues（§4.5）
   └─ 落库 shadowing_evaluations + 更新 attempt / mastery
   ▼
返回 EvaluationResult（同步；P95 目标 ≤ 3s，超时 8s）
```

### 5.2 Go 接口（贴合宪法）

```go
// internal/module/shadowing/evaluator.go
type EvaluationRequest struct {
    UserID, LessonID        int64
    ShadowingVersion        int
    SentenceIndex           int
    Mode                    EvaluationMode // shadow | repeat
    ReferenceText           string
    ReferenceAudio          []byte // 16k mono WAV，来自课程音频/MinIO
    UserAudio               []byte // 16k mono WAV
    ReferencePositionMS     int64
    ClientLagMS             int64
}

type Evaluator interface {
    Evaluate(ctx context.Context, req EvaluationRequest) (EvaluationResult, error)
}
```

- 实现只有两个：`LocalEvaluator`（默认）与 `AzureEvaluator`（P2 可选）；组合/降级逻辑写在 `Service` 内，不引入注册表。
- 注入方式：`shadowing.NewService(lessons, store, evaluator, audioStore, cfg)`，全部显式，无全局状态。
- 所有错误 `fmt.Errorf("shadowing.Service.Evaluate ...: %w", err)` 包装；错误点 `slog.Error`，关键输入输出 `slog.Debug`（宪法第四条）。
- 韵律层用纯 Go 标准库实现（自相关 F0 + DTW + 能量包络），**不引入 Python 服务、不引入音频第三方库**；项目已有 `internal/cli/wav_trim.go` 的 WAV/PCM 解析可直接抽取复用。

### 5.3 API

```
POST /api/v1/lessons/{id}/shadowing/evaluations
  Content-Type: multipart/form-data
  parts: audio(wav), sentence_index, mode, reference_position_ms, client_lag_ms,
         shadowing_version, self_score(optional)
  header: Idempotency-Key: <uuid>     // 防重复提交/重复计费

  200 → { "data": EvaluationResult }
  409 → ERR_SHADOWING_VERSION_STALE（沿用现有）
  422 → ERR_SHADOWING_UNSCORABLE + code（§4.4）

GET /api/v1/shadowing/evaluations/{eid}    // 历史结果回看

POST /api/v1/lessons/{id}/shadowing/attempts
  // 向后兼容：新增可选 evaluation_id / machine_score / score_source
```

`EvaluationResult`：

```json
{
  "evaluation_id": "ev_123",
  "status": "scoreable",
  "mode": "shadow",
  "overall_score": 79,
  "partial": false,
  "scores": {
    "sync":       {"score": 70, "confidence": 0.91, "weight": 0.30},
    "prosody":    {"score": 85, "confidence": 0.74, "weight": 0.30},
    "coverage":   {"score": 90, "confidence": 0.88, "weight": 0.15},
    "fluency":    {"score": 80, "confidence": 0.83, "weight": 0.15},
    "segmental":  {"score": 88, "confidence": 0.65, "weight": 0.10}
  },
  "recognized_text": "すみません、このお弁当はいくらですか",
  "lag_ms": 620,
  "issues": [],
  "self_score": 80,
  "score_source": "local_v1",
  "scorer_version": "scoring-1.0",
  "weights_version": "w-shadow-1.0",
  "model_version": "whisper-ja-v1",
  "shadowing_version": 3,
  "latency_ms": 1840
}
```

### 5.4 存储（新增 PG 迁移 `011_shadowing_scoring.sql`）

```sql
CREATE TABLE IF NOT EXISTS shadowing_evaluations (
    id                  BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id           BIGINT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    shadowing_version   INTEGER NOT NULL,
    sentence_index      INTEGER NOT NULL,
    mode                TEXT NOT NULL CHECK (mode IN ('shadow','repeat')),
    status              TEXT NOT NULL CHECK (status IN ('scoreable','unscorable')),
    unscorable_code     TEXT NOT NULL DEFAULT '',
    audio_object_id     BIGINT,
    overall_score       INTEGER CHECK (overall_score IS NULL OR overall_score BETWEEN 0 AND 100),
    scores_jsonb        JSONB NOT NULL DEFAULT '{}'::jsonb,
    issues_jsonb        JSONB NOT NULL DEFAULT '[]'::jsonb,
    recognized_text     TEXT NOT NULL DEFAULT '',
    self_score          INTEGER,
    score_source        TEXT NOT NULL DEFAULT '',
    scorer_version      TEXT NOT NULL DEFAULT '',
    weights_version     TEXT NOT NULL DEFAULT '',
    model_version       TEXT NOT NULL DEFAULT '',
    latency_ms          INTEGER NOT NULL DEFAULT 0,
    idempotency_key     TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ,
    UNIQUE (user_id, idempotency_key)
);

ALTER TABLE lesson_shadowing_attempts
    ADD COLUMN IF NOT EXISTS evaluation_id BIGINT,
    ADD COLUMN IF NOT EXISTS machine_score INTEGER,
    ADD COLUMN IF NOT EXISTS score_source TEXT NOT NULL DEFAULT '';
```

- 录音复用 `audio_objects`（`kind='shadowing_attempt'`、`visibility='private'`、`owner_user_id`）+ MinIO，流播走已有 `/api/v1/audio/{id}/stream`（该端点当前只放 public 对象，需为私有录音加本人鉴权）。
- 保留期默认 90 天，`expires_at` 到期清理音频对象与行；用户可主动删除。
- **迁移纪律**（见 `AGENTS.md` §5/§7）：只用 `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS`，不写函数表达式 DEFAULT，不在 SQLite 迁移里加 PG 专属表；PG 路径幂等执行。

### 5.5 降级链与超时

```
LocalEvaluator(韵律 + vLLM ASR)   timeout 3s (ASR) / 8s (总)
   ├─ 失败 → 仅 CER 分，score_source=fallback_cer，status=partial（明确标注「简化评分」）
   ├─ 再失败 → 不返回机器分，status=unscorable/provider_unavailable，保留 self_score
   └─ 任何分支都不得把自评分伪装成机器分
```

### 5.6 隐私

录音属个人生物特征数据：首次录音前明示用途与保留期并取得同意；`private` + 本人鉴权；不写音频进日志（只记字节数、时长、耗时）；不用于模型训练；删除账号时级联清理 MinIO 对象。若 P2 启用 Azure，需单独评估录音出境合规，**默认关闭**。

---

## 6. 掌握度模型（替换现有「录过即完成」）

```
有效评测 = status=scoreable 且 scorer_version 主版本相同
mastery = 0.5*last + 0.3*prev + 0.2*prev2      （不足 3 次按实际次数归一化）
completed = 有效评测次数 >= 2 且 mastery >= 80 且 last >= 75
```

- 前端进度条 / `completed_sentence_indexes` 改由 mastery 驱动；`attempt_summary.best_score` 改为 `best_machine_score`，保留 `self_score` 作为对照。
- **单次分数只做反馈，不做排行榜**；掌握度才是长期指标。
- 自评分的新定位：主观校准信号。记录 `self_score - overall_score` 的长期偏差，用于「你对自己偏严格 / 偏宽松」的轻提示，不参与掌握度。
- 可选：同一句连续 5 次以上收益趋于平台（影子跟读文献中的重复次数结论），提示换句或降速，避免刷分。

---

## 7. 校准、验收与测试

### 7.1 上线前必做的三组验证（借鉴 TonePerfect / 业界 benchmark 做法）

用**真实手机麦克风**录同一批句子，覆盖：母语者 / 中高级学习者 / 已知错误的初学者。

1. **母语者假阳性**：正确读音不该被标红。目标 `@80% 召回` 下误标率 ≤ 5%（参考 benchmark 中 Azure 日语约 3%）。
2. **故意错读**：读另一句或故意吞掉长音/促音，得分中位数应 ≤ 30，而不是仍给 60+。
3. **外放串音**：同一句用扬声器外放录一遍，`bleed_through` 必须 100% 命中且不给分。

阈值（VAD、串音相关、CER、置信度 0.60、DTW 归一化）必须先用 20–50 条真实样本按 **L1=中文母语者** 分组定，上线后按 `weights_version` 迭代，不写死在代码常量里。

### 7.2 其他验收指标

| 指标 | 目标 |
| --- | --- |
| 端到端 P95 延迟 | ≤ 3s（同步返回） |
| `unscorable` 率 | ≤ 10%（不含用户主动取消） |
| 同一音频重复提交 | 结果一致（幂等键 + 版本四元组） |
| 历史数据 | 旧 `self_score` 记录可正常展示，不回填、不误判为已完成 |

### 7.3 TDD 与表格驱动测试清单（宪法第二条）

- `TestAggregateOverall`：维度组合 → 期望分、最低分效应、置信度剔除与权重重分配、`partial`。
- `TestClassifyUnscorable`：能量/时长/互相关/CER/文本 → 期望 code，覆盖 6 类 + 正常样本。
- `TestMastery`：历史分序列（含 unscorable 混入、版本切换）→ mastery / completed。
- `TestF0ContourDTW`：合成两段不同 F0 曲线 → 距离单调、对齐路径合理。
- `TestScoreFromCER` / `TestComputeCER`：已有，补日语假名规范化用例。
- `TestShadowingVersionStale`：复用现有机制，确认评测请求也走版本校验。
- **集成测试**（拒绝 mock 对象）：真实 PG + `httptest` 启一个 vLLM 风格 HTTP 服务 + 真实 `audio_objects` 写入，走完整 Handler→Service→Store 链路；Azure Provider 若启用同样用 `httptest` 真实 HTTP 服务。
- 先写失败测试再实现，Red-Green-Refactor。

---

## 8. 分阶段落地

| 阶段 | 范围 | 交付 | 依赖 |
| --- | --- | --- | --- |
| **P0 · 链路打通** | 录音上传 WAV、`audio_objects` 私有存储、接上 `VLLMScorer`、CER + 有效性/串音判定、unscorable、`shadowing_evaluations` 表、结果 JSON、前端展示分项与回放、去掉 `WaveformScorer` 的发音分用途 | 用户能看到「不是自评」的分数与原因 | 已有 vLLM ASR 实例（**待确认**） |
| **P1 · 影子专属维度** | 纯 Go 韵律层（能量包络 DTW + F0 + DTW）、Sync/Prosody/Fluency、双模式权重、最低分聚合、issues 规则、mastery 替换「录过即完成」、校准三组验证 | 影子跟读评分真正贴合训练目标 | OJAD 调型真值来源（**待确认**） |
| **P2 · 增强与精练** | 可插拔 Azure `ja-JP`（仅 repeat 模式，音素分 + ErrorType）、pitch accent 型判定、个性化阈值、repeat 模式 UI | 音段级精练闭环 | 是否允许外部云 / 数据出境（**待决策**） |

**不做的事**：不在 P0/P1 引入 LLM 生成反馈、不做实时打断式纠音、不做跨用户排行榜、不追求「和真人老师分数完全一致」。

---

## 9. 备选方案对比与最终推荐

| 方案 | 优点 | 致命问题 | 结论 |
| --- | --- | --- | --- |
| A. 单接 Azure/SpeechSuper，直接用 `PronScore` | 最快、最便宜、音素级反馈成熟 | 影子跟读的串音与破损语音会污染分数；Prosody 仅 en-US；完全不覆盖日语アクセント；与影子跟读训练目标错位 | 否（可作 P2 组件，不可作整体方案） |
| B. 纯自建 wav2vec2/MFA 全栈 | 可控、无出境、可判调型 | 需要标注数据、模型服务与 MLOps，违反宪法「少依赖 / YAGNI」，个人项目难以维护 | 否（P2 之后再说） |
| **C. 双模式 + 分层评分 + 可插拔 Provider（本方案）** | 复用已有 vLLM / MinIO / PG / WAV 解析；先用规则和 DTW 覆盖影子跟读真正该奖的维度；云 API 只补它擅长的音段层；降级链与置信度门控保证不误伤 | 韵律层需要一次校准投入 | **推荐** |

**一句话**：让云 API 回答「这个音发得准不准」，让自建层回答「跟得齐不齐、调子像不像」，让模式决定两者的权重，让置信度决定「这次到底该不该给分」。

---

## 10. 风险与待拍板事项

1. **是否允许录音上传到境外云（Azure）**：若否，P2 改评估 DolphinSOE 或纯本地方案；本方案 P0/P1 全程本地即可。
2. **vLLM Whisper ASR 实例是否已可用**：项目 vLLM TTS 已在用，`VLLMScorer` 已实现但未接线；若无 ASR 实例，P0 需先部署一个 Whisper 端点。
3. **OJAD 调型真值**：以离线词典还是按教材句子人工标注？后者更准但要维护；建议 P1 先只做 F0 轮廓相似度，调型判定推迟到有真值之后。
4. **前端录音改为 WAV 采集**：`MediaRecorder` 默认输出 webm/opus，改 WAV 需前端加约 60 行 PCM 编码；收益是后端零解码依赖，符合宪法。
5. **Azure 日语延迟**：社区有 ja-JP 并发下 6–7s 延迟与结果块缺失的报告，若启用需重试 + 结果块存在性校验 + 严格超时。
6. **F0 在噪声/串音下的稳定性**：这是 P1 最大技术风险，必须在 §7.1 的三组验证里单列。

---

## 附录 · 参考来源

- Microsoft, *Use pronunciation assessment*（Azure AI Speech 文档，参数与 Prosody 仅 en-US 说明）：https://docs.azure.cn/en-us/ai-services/speech-service/how-to-pronunciation-assessment
- Microsoft Tech Community, *Pronunciation Assessment is Generally Available*（ja-JP 属 GA 语言）：https://techcommunity.microsoft.com/t5/azure-ai-services-blog/speech-service-update-pronunciation-assessment-is-generally/ba-p/3740894
- Microsoft, *Transparency Note: Pronunciation Assessment*（维度与 ErrorType 定义）：https://go.microsoft.com/fwlink/?linkid=2277025
- SpeechSuper, *Pricing / API docs*（日语句级 $0.006、粒度矩阵）：https://www.speechsuper.com/pricing.html 、 http://docs.speechsuper.com/
- TonePerfect, *Languages / API reference / 2026 API 对比*（日语实验态、Pitch accent is not scored、correction decision、拒评与计费规则）：https://api.toneperfect.app/languages 、 https://api.toneperfect.app/docs 、 https://api.toneperfect.app/blog/best-pronunciation-assessment-apis-2026
- DolphinAI, *日语 AI 发音评测功能正式发布*：https://dolphinvoice.ai/zh/blog/dolphinsoe-japanese-pronunciation-evaluation-api-released
- Lingolix, *How well do pronunciation APIs actually judge learners?*（2026 开放 benchmark，竞品自述需复跑）：https://lingolix.com/en/blog/best-pronunciation-assessment-api-2026
- Whitworth, B. & Rose, H. (2025), *A Systematic Review of Research on the use of Shadowing for Second Language Pronunciation Teaching*, Research Synthesis in Applied Linguistics 1(2), 239–269. DOI: 10.1080/29984475.2025.2546827
- Luo, Minematsu, Yamauchi, Hirose (2008), *Automatic Assessment of Language Proficiency through Shadowing*（GOP / CPR / 聚类）：https://www.isca-archive.org/iscslp_2008/luo08_iscslp.pdf
- Yue et al. (2017), *Automatic Scoring of Shadowing Speech Based on DNN Posteriors and Their DTW*, Interspeech 2017. DOI: 10.21437/Interspeech.2017-728
- Bundasak et al. (2026), *Comparative Analysis of Japanese Speech: DTW + Wav2Vec2 for Pronunciation Assessment*, IJETI 16. DOI: 10.46604/ijeti.2026.16049
- PASQA (2026), *Pitch-Accent-Focused Speech Quality Assessment*（mora-conditioned，LY Corp 开源）：https://github.com/lycorp-jp/PASQA
- OJAD / Suzuki-kun（东京大学，调型与无声化参考真值）
- 项目内：`constitution.md`、`AGENTS.md`、`docs/spec/japanese-pronunciation-mvp-design.md`、`internal/module/speaking/vllm_scorer.go`、`internal/cli/wav_trim.go`
