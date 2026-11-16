package shadowing

import (
	"fmt"
	"sort"

	"japanese-learning-app/internal/audio"
)

const (
	// maxAdviceCount keeps the feedback panel actionable rather than a wall of text.
	maxAdviceCount = 3
	// A shadowing start within this window around the target lag is on time.
	lagToleranceMS = 300
	lagBoost       = 50
	prosodyBoost   = 10
	pauseBoost     = 20
	paceBoost      = 15
	rhythmBoost    = 5
	weakProsody    = 70
	weakEnvelope   = 60
	paceDeviation  = 0.25
)

var dimensionLabels = map[string]string{
	"sync":      "同步",
	"prosody":   "句调",
	"coverage":  "完整度",
	"fluency":   "流利度",
	"segmental": "音段",
}

// AdviceInput is the evidence EvaluateWAV has already computed. BuildAdvice is
// a pure function so advice rules can be tested without audio or HTTP.
type AdviceInput struct {
	Mode           EvaluationMode
	Overall        int
	Scores         map[string]DimensionScore
	LagMS          int64
	SpeechMS       int64
	ReferenceMS    int64
	VoicedRatio    float64
	Pauses         []audio.PauseSpan
	EnvelopeSim    float64
	UnscorableCode string
}

type adviceCandidate struct {
	issue     Issue
	rank      float64
	dimension string
}

// adviceInput collects the features EvaluateWAV already computed; no audio is
// reprocessed just to produce feedback.
func adviceInput(mode EvaluationMode, overall int, scores map[string]DimensionScore,
	lag audio.LagResult, vad audio.VADResult, envSim float64, referenceMS int64, code string) AdviceInput {
	return AdviceInput{
		Mode:           mode,
		Overall:        overall,
		Scores:         scores,
		LagMS:          lag.LagMS,
		SpeechMS:       vad.SpeechMS,
		ReferenceMS:    referenceMS,
		VoicedRatio:    vad.VoicedRatio,
		Pauses:         vad.Pauses,
		EnvelopeSim:    envSim,
		UnscorableCode: code,
	}
}

// BuildAdvice turns scoring evidence into concrete next steps. It never returns
// an empty slice: a strong attempt gets a "polish the weakest dimension" drill,
// and an unscorable attempt gets the specific fix for its failure reason.
// Results are already ordered by expected benefit, so clients should not
// re-sort them.
func BuildAdvice(in AdviceInput) []Issue {
	if in.UnscorableCode != "" {
		return []Issue{unscorableAdvice(in)}
	}

	candidates := scoreableCandidates(in)
	if len(candidates) == 0 {
		return []Issue{keepPracticeAdvice(in)}
	}

	// Rank by dimension shortfall (score x weight) plus a boost for rules with
	// directly measurable, immediately fixable evidence such as a late start.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].rank > candidates[j].rank
	})

	// One advice per dimension: two tips about the same score row is noise.
	seen := make(map[string]bool)
	out := make([]Issue, 0, maxAdviceCount)
	for _, candidate := range candidates {
		if candidate.dimension != "" {
			if seen[candidate.dimension] {
				continue
			}
			seen[candidate.dimension] = true
		}
		out = append(out, candidate.issue)
		if len(out) == maxAdviceCount {
			break
		}
	}
	return out
}

func scoreableCandidates(in AdviceInput) []adviceCandidate {
	cfg := ScoringConfigForMode(in.Mode)
	nominal := weightsForMode(in.Mode)
	var out []adviceCandidate

	// add only emits advice for dimensions that actually have reliable scores.
	// Rank with the effective weight returned by the aggregator so advice
	// follows the same mode-aware rubric as the score.
	add := func(dimension string, boost float64, issue Issue) {
		d, ok := in.Scores[dimension]
		if !ok || !d.Available || d.Confidence < cfg.MinDimensionConfidence {
			return
		}
		weight := d.Weight
		if weight <= 0 {
			weight = nominal[dimension]
		}
		rank := (100-d.Score)*weight + boost
		issue.Dimension = dimension
		out = append(out, adviceCandidate{issue: issue, rank: rank, dimension: dimension})
	}

	if in.Mode == EvaluationModeShadow {
		delta := in.LagMS - cfg.TargetLagMS
		switch {
		case delta > lagToleranceMS:
			add("sync", lagBoost, Issue{
				Type:       "sync_lag_late",
				Confidence: 0.90,
				EvidenceZH: fmt.Sprintf("起步延迟 %d ms，目标约 %d ms，晚了 %d ms",
					in.LagMS, cfg.TargetLagMS, delta),
				MessageZH: "开口比原音晚了不少，跟读变成了「听完再复述」。",
				ActionZH:  "听到第一个音就立刻开口，别等整句听完；先用 0.75 倍速把起音点对齐，再回原速。",
			})
		case delta < -lagToleranceMS:
			add("sync", lagBoost, Issue{
				Type:       "sync_lag_early",
				Confidence: 0.85,
				EvidenceZH: fmt.Sprintf("起步延迟 %d ms，目标约 %d ms，早了 %d ms",
					in.LagMS, cfg.TargetLagMS, -delta),
				MessageZH: "抢在原音前面开口了，容易听不清自己的发音。",
				ActionZH:  "等原音第一个音出来再开口；先跟着原音打一遍拍子，找到起音点再录。",
			})
		}
	}

	if d, ok := in.Scores["prosody"]; ok && d.Available && d.Score < weakProsody {
		add("prosody", prosodyBoost, Issue{
			Type:       "prosody_contour",
			Confidence: 0.80,
			EvidenceZH: fmt.Sprintf("句调轮廓与标准音相似度 %.0f 分", d.Score),
			MessageZH:  "高低起伏和原音对不上，句尾的升降尤其明显。",
			ActionZH:   "先听原音 2 遍并用手比划高低，再用 0.75 倍速跟读，重点还原句尾的升降，最后回原速。",
		})
	}

	if longest := longestPause(in.Pauses); longest != nil {
		start := longest.StartMS
		length := longest.EndMS - longest.StartMS
		add("fluency", pauseBoost, Issue{
			Type:       "fluency_long_pause",
			Confidence: 0.85,
			EvidenceZH: fmt.Sprintf("第 %.1f s 处停了 %d ms", float64(start)/1000, length),
			MessageZH:  "句子中间断开了，节奏被这一下停顿打断。",
			ActionZH:   fmt.Sprintf("把第 %.1f s 附近连起来读：先用 0.75 倍速整句读顺，停顿处先轻轻带过、不换气，再回原速。", float64(start)/1000),
		})
	}

	if in.ReferenceMS > 0 {
		ratio := float64(in.SpeechMS) / float64(in.ReferenceMS)
		if deviation := ratio - 1; deviation > paceDeviation || deviation < -paceDeviation {
			add("fluency", paceBoost, Issue{
				Type:       "fluency_pace",
				Confidence: 0.75,
				EvidenceZH: fmt.Sprintf("有效读音 %d ms，标准句长 %d ms（%.0f%%）",
					in.SpeechMS, in.ReferenceMS, ratio*100),
				MessageZH: "整句时长和标准音差得比较多。",
				ActionZH:  "跟着原音打拍子读，长音、促音也要占满一拍；先慢速把每一拍走匀，再逐步提速。",
			})
		}
	}

	if in.EnvelopeSim > 0 && in.EnvelopeSim < weakEnvelope {
		add("sync", rhythmBoost, Issue{
			Type:       "rhythm_unstable",
			Confidence: 0.70,
			EvidenceZH: fmt.Sprintf("节奏包络相似度 %.0f 分", in.EnvelopeSim),
			MessageZH:  "轻重快慢的分布和标准音差别较大。",
			ActionZH:   "先按拍跟读，把每个假名读得一样长，再对照原音把该轻的地方放轻。",
		})
	}

	return out
}

func longestPause(pauses []audio.PauseSpan) *audio.PauseSpan {
	var best *audio.PauseSpan
	for i := range pauses {
		p := &pauses[i]
		if best == nil || p.EndMS-p.StartMS > best.EndMS-best.StartMS {
			best = p
		}
	}
	return best
}

// keepPracticeAdvice is the fallback: even a near-perfect attempt gets one
// concrete drill aimed at its weakest reliable dimension.
func keepPracticeAdvice(in AdviceInput) Issue {
	key, label, score, ok := weakestDimension(in)
	if !ok {
		return Issue{
			Type:       "keep_practice",
			Confidence: 0.80,
			EvidenceZH: fmt.Sprintf("本次 %d 分", in.Overall),
			MessageZH:  "这次跟读可以评分，继续保持。",
			ActionZH:   "挑这一句再跟读 2 遍，先用 0.75 倍速把节奏走顺，再回原速录一次。",
		}
	}
	return Issue{
		Type:       "keep_practice",
		Dimension:  key,
		Confidence: 0.85,
		EvidenceZH: fmt.Sprintf("本次 %d 分，最弱的是%s（%.0f 分）", in.Overall, label, score),
		MessageZH:  "整体已经跟上原音，接着打磨最弱的一项就好。",
		ActionZH:   fmt.Sprintf("挑这一句再跟读 2 遍，重点保持%s；先用 0.75 倍速练一遍，再回原速。", label),
	}
}

func weakestDimension(in AdviceInput) (string, string, float64, bool) {
	cfg := ScoringConfigForMode(in.Mode)
	bestKey, bestLabel := "", ""
	bestScore := 101.0
	for _, key := range []string{"sync", "prosody", "coverage", "fluency", "segmental"} {
		d, ok := in.Scores[key]
		if !ok || !d.Available || d.Confidence < cfg.MinDimensionConfidence {
			continue
		}
		if d.Score < bestScore {
			bestKey, bestLabel, bestScore = key, dimensionLabels[key], d.Score
		}
	}
	return bestKey, bestLabel, bestScore, bestKey != ""
}

// unscorableAdvice gives the learner the exact fix for why scoring was refused.
func unscorableAdvice(in AdviceInput) Issue {
	issue := Issue{
		Type:       "unscorable_" + in.UnscorableCode,
		Confidence: 0.95,
	}
	switch in.UnscorableCode {
	case "silence":
		issue.EvidenceZH = fmt.Sprintf("没有检测到有效人声（有效音占比 %.0f%%）", in.VoicedRatio*100)
		issue.MessageZH = "这次录音几乎是静音，无法评分。"
		issue.ActionZH = "检查浏览器麦克风权限和系统输入音量；录完后先点「我的录音」确认能听到自己的声音，再录一次。"
	case "too_short":
		issue.EvidenceZH = fmt.Sprintf("只录到 %d ms 人声，标准句长 %d ms", in.SpeechMS, in.ReferenceMS)
		issue.MessageZH = "录音太短，很可能只跟读了半句。"
		issue.ActionZH = "等原音整句读完再按停止，把整句读满；先用 0.75 倍速完整跟一遍，再按原速录。"
	case "bleed_through":
		issue.EvidenceZH = fmt.Sprintf("录音与原音几乎重合（延迟 %d ms）", in.LagMS)
		issue.MessageZH = "麦克风把原音一起录进去了，这不是你的跟读声。"
		issue.ActionZH = "戴上耳机、关掉外放或把音量调低，再把麦克风靠近嘴边重录一次。"
	case "low_confidence":
		issue.EvidenceZH = "可用评分维度不足一半，证据不够可靠"
		issue.MessageZH = "这次录音的特征太弱，给分不可靠。"
		issue.ActionZH = "换到安静环境、靠近麦克风，完整匀速地读一遍整句再录。"
	case "content_mismatch":
		issue.EvidenceZH = "识别到的内容与当前句子不一致"
		issue.MessageZH = "读的好像不是屏幕上这一句。"
		issue.ActionZH = "照着屏幕上的句子读，先确认当前句再按录音。"
	case "provider_unavailable":
		issue.EvidenceZH = "评分服务本次没有返回结果"
		issue.MessageZH = "评分暂时没跑起来，和你的发音无关。"
		issue.ActionZH = "稍后重试一次；如果连续失败，先继续用 0.75 倍速练习这一句。"
	default:
		issue.EvidenceZH = "本次录音未达到可评条件"
		issue.MessageZH = "这次没有给出分数。"
		issue.ActionZH = "戴上耳机，完整跟读一遍整句后再录一次。"
	}
	return issue
}
