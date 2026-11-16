package shadowing

import (
	"fmt"
	"log/slog"
	"math"
	"sort"

	"japanese-learning-app/internal/audio"
)

// Classify decides whether an utterance carries enough evidence to be scored.
// An empty string means scoreable; any other value is an unscorable code.
func Classify(f AudioFeatures, mode EvaluationMode) string {
	cfg := ScoringConfigForMode(mode)
	if f.VoicedRatio < cfg.SilenceVoicedRatio {
		return "silence"
	}
	if f.ReferenceMS > 0 && f.SpeechMS < int64(float64(f.ReferenceMS)*cfg.TooShortRatio) {
		return "too_short"
	}
	if mode == EvaluationModeShadow && f.Peak > cfg.BleedPeak && math.Abs(float64(f.LagMS)) < float64(cfg.BleedLagMS) {
		return "bleed_through"
	}
	return ""
}

// Aggregate applies the lowest-dimension-weighted formula. Dimensions that
// cannot report a score contribute no weight; the remaining rubric is
// renormalized to 1.0 before the confidence gate, so a mode whose ASR
// dimensions are unavailable is judged against the evidence it can actually
// produce. A minimum number of trusted dimensions and a minimum share of the
// ideal rubric still prevent one surviving dimension from producing a score.
func Aggregate(scores map[string]DimensionScore, mode EvaluationMode) AggregateResult {
	cfg := ScoringConfigForMode(mode)
	weights := effectiveWeights(cfg, scores)
	achievable := achievableWeight(cfg, scores)
	result := AggregateResult{Weights: weights, AchievableWeight: achievable}
	if achievable < cfg.MinAchievableWeight {
		result.Code = "low_confidence"
		return result
	}

	type effective struct {
		score  float64
		weight float64
	}
	names := make([]string, 0, len(weights))
	for name := range weights {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic float accumulation across runs

	active := make(map[string]effective)
	totalWeight := 0.0
	for _, name := range names {
		d, ok := scores[name]
		if !ok || !d.Available || d.Confidence < cfg.MinDimensionConfidence {
			continue
		}
		active[name] = effective{score: d.Score, weight: weights[name]}
		totalWeight += weights[name]
	}
	if len(active) < cfg.MinActiveDimensions || totalWeight < cfg.MinEffectiveWeight {
		result.Code = "low_confidence"
		slog.Debug("shadowing.Aggregate gated", "mode", mode, "version", cfg.WeightsVersion,
			"trusted_dimensions", len(active), "trusted_weight", totalWeight, "achievable_weight", achievable)
		return result
	}

	minName := ""
	minScore := math.Inf(1)
	for _, name := range names {
		e, ok := active[name]
		if ok && e.score < minScore {
			minScore = e.score
			minName = name
		}
	}

	othersSum, othersWeight := 0.0, 0.0
	for _, name := range names {
		if name == minName {
			continue
		}
		e, ok := active[name]
		if !ok {
			continue
		}
		othersSum += e.score * e.weight
		othersWeight += e.weight
	}
	othersAvg := minScore
	if othersWeight > 0 {
		othersAvg = othersSum / othersWeight
	}
	overall := int(math.Round(0.4*minScore + 0.6*othersAvg))
	overall = int(clampScoreValue(float64(overall)))

	result.Overall = overall
	result.Partial = len(active) < len(cfg.Weights) || achievable < 1.0-1e-9
	result.Valid = true
	return result
}

// EvaluateWAV scores a user recording against the reference sentence WAV using
// only local signal processing (no ASR, no external services). Coverage and
// segmental dimensions stay unavailable until an ASR provider is wired in.
func EvaluateWAV(referenceWAV, userWAV []byte, mode EvaluationMode) (EvaluationResult, error) {
	cfg := ScoringConfigForMode(mode)
	slog.Debug("shadowing.EvaluateWAV called", "ref_bytes", len(referenceWAV),
		"user_bytes", len(userWAV), "mode", mode, "weights_version", cfg.WeightsVersion)
	result := newEvaluationResult(mode)

	refParsed, err := audio.ParseWAV(referenceWAV)
	if err != nil {
		return result, fmt.Errorf("shadowing.EvaluateWAV parse reference: %w", err)
	}
	userParsed, err := audio.ParseWAV(userWAV)
	if err != nil {
		return result, fmt.Errorf("shadowing.EvaluateWAV parse user audio: %w", err)
	}
	// Reference WAVs come from the lesson material (8 kHz locally, 24 kHz in
	// the PostgreSQL/MinIO path) while browser recorders usually capture at
	// 16 kHz. Every feature below is computed on a 10 ms frame grid, and F0 is
	// converted to Hz then z-scored semitones, so the two signals can be
	// compared without resampling. Only the frame window sizes follow each
	// signal's own sample rate.
	refRate := refParsed.SampleRate
	userRate := userParsed.SampleRate
	referenceMS := int64(len(refParsed.Samples)) * 1000 / int64(refRate)

	refFrames, err := audio.RMSFrames(refParsed.Samples, refRate, audio.DefaultFrameMS, audio.DefaultHopMS)
	if err != nil {
		return result, fmt.Errorf("shadowing.EvaluateWAV reference frames: %w", err)
	}
	userFrames, err := audio.RMSFrames(userParsed.Samples, userRate, audio.DefaultFrameMS, audio.DefaultHopMS)
	if err != nil {
		return result, fmt.Errorf("shadowing.EvaluateWAV user frames: %w", err)
	}
	refEnv := audio.NormalizeFrames(refFrames)
	userEnv := audio.NormalizeFrames(userFrames)

	lag, err := audio.BestLag(refEnv, userEnv, audio.DefaultHopMS, int(cfg.MaxLagSearchMS))
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: lag search unavailable", "err", err)
		lag = audio.LagResult{}
	}
	vad, err := audio.DetectVAD(userFrames, audio.DefaultHopMS)
	if err != nil {
		return result, fmt.Errorf("shadowing.EvaluateWAV vad: %w", err)
	}

	features := AudioFeatures{
		VoicedRatio: vad.VoicedRatio,
		SpeechMS:    vad.SpeechMS,
		ReferenceMS: referenceMS,
		LagMS:       lag.LagMS,
		Peak:        lag.Peak,
	}
	result.LagMS = lag.LagMS
	if code := Classify(features, mode); code != "" {
		slog.Debug("shadowing.EvaluateWAV: unscorable", "code", code, "features", features)
		result.Status = "unscorable"
		result.UnscorableCode = code
		result.Scores = dimensionResults(scoresForMode(mode), cfg.Weights)
		result.Issues = BuildAdvice(adviceInput(mode, 0, result.Scores, lag, vad, 0, referenceMS, code))
		return result, nil
	}

	scores := scoresForMode(mode)
	envSim := scoreEnvelope(refEnv, userEnv, scores, mode, lag, vad, len(userFrames), cfg, referenceMS)
	scoreProsody(refParsed, userParsed, cfg, scores)
	scoreFluency(vad, referenceMS, len(userFrames), cfg, scores)
	scoreCoverageAndSegmental(scores)

	agg := Aggregate(scores, mode)
	result.Scores = dimensionResults(scores, agg.Weights)
	slog.Debug("shadowing.EvaluateWAV aggregation", "mode", mode, "version", cfg.WeightsVersion,
		"valid", agg.Valid, "code", agg.Code, "trusted_weight", agg.AchievableWeight,
		"effective_weights", agg.Weights, "scores", scores)
	if !agg.Valid {
		result.Status = "unscorable"
		result.UnscorableCode = agg.Code
		result.Issues = BuildAdvice(adviceInput(mode, 0, result.Scores, lag, vad, envSim, referenceMS, agg.Code))
		return result, nil
	}
	overall := agg.Overall
	result.Status = "scoreable"
	result.OverallScore = &overall
	result.Partial = agg.Partial
	result.Issues = BuildAdvice(adviceInput(mode, overall, result.Scores, lag, vad, envSim, referenceMS, ""))

	slog.Debug("shadowing.EvaluateWAV done", "overall", overall, "partial", agg.Partial, "lag_ms", lag.LagMS)
	return result, nil
}

func scoresForMode(mode EvaluationMode) map[string]DimensionScore {
	weights := weightsForMode(mode)
	scores := make(map[string]DimensionScore, len(weights))
	for name, weight := range weights {
		scores[name] = DimensionScore{Weight: weight, Available: false}
	}
	return scores
}

func dimensionResults(scores map[string]DimensionScore, weights map[string]float64) map[string]DimensionScore {
	out := make(map[string]DimensionScore, len(scores))
	for name, d := range scores {
		if weight, ok := weights[name]; ok {
			d.Weight = weight
		}
		out[name] = d
	}
	return out
}

func scoreEnvelope(refEnv, userEnv []float64, scores map[string]DimensionScore, mode EvaluationMode,
	lag audio.LagResult, vad audio.VADResult, frameCount int, cfg ScoringConfig, referenceMS int64) float64 {

	alignment, err := audio.DTW(refEnv, userEnv, cfg.DTWBandRatio)
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: envelope DTW failed", "err", err)
		return 0
	}
	envSim := audio.Similarity(alignment.NormalizedDistance, cfg.EnvDistanceK)
	alignConfidence := math.Max(0, 1-alignment.NormalizedDistance)
	voicedConfidence := clamp01(vad.VoicedRatio * float64(frameCount) / float64(cfg.FluencyFullConfidenceFrames))
	syncConfidence := clamp01(cfg.SyncVoicedConfidenceWeight*voicedConfidence +
		(1-cfg.SyncVoicedConfidenceWeight)*alignConfidence)

	if mode == EvaluationModeRepeat {
		durationScore := 100.0
		if referenceMS > 0 {
			ratio := float64(vad.SpeechMS) / float64(referenceMS)
			durationScore = clampScoreValue(100 - math.Abs(1-ratio)*100)
		}
		scores["sync"] = DimensionScore{
			Score:      clampScoreValue(0.5*durationScore + 0.5*envSim),
			Confidence: syncConfidence,
			Available:  true,
		}
		return envSim
	}

	lagScore := 100 * math.Exp(-math.Abs(float64(lag.LagMS)-float64(cfg.TargetLagMS))/float64(cfg.LagSigmaMS))
	scores["sync"] = DimensionScore{
		Score:      clampScoreValue(0.5*lagScore + 0.5*envSim),
		Confidence: syncConfidence,
		Available:  true,
	}
	return envSim
}

func scoreProsody(refParsed, userParsed audio.WAV, cfg ScoringConfig, scores map[string]DimensionScore) {
	refHz, refVoiced, err := audio.RawF0(refParsed.Samples, refParsed.SampleRate, audio.PitchFrameMS, audio.PitchHopMS)
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: reference F0 failed", "err", err)
		return
	}
	userHz, userVoiced, err := audio.RawF0(userParsed.Samples, userParsed.SampleRate, audio.PitchFrameMS, audio.PitchHopMS)
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: user F0 failed", "err", err)
		return
	}
	refContour, err := audio.PitchContour(refHz, refVoiced)
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: reference pitch contour failed", "err", err)
		return
	}
	userContour, err := audio.PitchContour(userHz, userVoiced)
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: user pitch contour failed", "err", err)
		return
	}
	aligned, err := audio.DTW(refContour, userContour, cfg.DTWBandRatio)
	if err != nil {
		slog.Debug("shadowing.EvaluateWAV: pitch DTW failed", "err", err)
		return
	}

	voicedCount := 0
	for _, ok := range userVoiced {
		if ok {
			voicedCount++
		}
	}
	// A contour built from 60+ voiced frames (600 ms of voiced speech at a
	// 10 ms hop) is reliable even when the whole utterance is only ~30% voiced.
	// Dividing by an assumed "fully voiced" ratio used to reject normal TTS.
	scores["prosody"] = DimensionScore{
		Score:      audio.Similarity(aligned.NormalizedDistance, cfg.PitchDistanceK),
		Confidence: clamp01(float64(voicedCount) / float64(cfg.ProsodyFullConfidenceFrames)),
		Available:  true,
	}
}

func scoreFluency(vad audio.VADResult, referenceMS int64, frameCount int, cfg ScoringConfig,
	scores map[string]DimensionScore) {
	score := 100.0
	score -= float64(len(vad.Pauses)) * 8
	longest := int64(0)
	for _, p := range vad.Pauses {
		if d := p.EndMS - p.StartMS; d > longest {
			longest = d
		}
	}
	if longest > 300 {
		score -= float64(longest-300) / 20
	}
	if referenceMS > 0 {
		ratio := float64(vad.SpeechMS) / float64(referenceMS)
		score -= math.Abs(1-ratio) * 30
	}
	voicedFrames := vad.VoicedRatio * float64(frameCount)
	scores["fluency"] = DimensionScore{
		Score:      clampScoreValue(score),
		Confidence: clamp01(voicedFrames / float64(cfg.FluencyFullConfidenceFrames)),
		Available:  true,
	}
}

func scoreCoverageAndSegmental(scores map[string]DimensionScore) {
	// S2 has no ASR provider. Keep both dimensions explicitly unavailable so
	// the aggregation renormalizes weights instead of inventing a score.
	scores["coverage"] = DimensionScore{Available: false}
	scores["segmental"] = DimensionScore{Available: false}
}

func newEvaluationResult(mode EvaluationMode) EvaluationResult {
	return EvaluationResult{
		Status:         "unscorable",
		Mode:           mode,
		Scores:         map[string]DimensionScore{},
		Issues:         []Issue{},
		ScoreSource:    ScoreSourceLocal,
		ScorerVersion:  ScorerVersionLocal,
		WeightsVersion: weightsVersionForMode(mode),
		ModelVersion:   ModelVersionLocalOnly,
	}
}

func clampScoreValue(v float64) float64 {
	return math.Max(0, math.Min(100, v))
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
