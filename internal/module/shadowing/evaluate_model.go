package shadowing

// EvaluationMode separates synchronous shadowing from pause-and-repeat practice.
type EvaluationMode string

const (
	EvaluationModeShadow EvaluationMode = "shadow"
	EvaluationModeRepeat EvaluationMode = "repeat"
)

const (
	ERR_SHADOWING_EVAL_INVALID      = "ERR_SHADOWING_EVAL_INVALID"
	ERR_SHADOWING_REFERENCE_MISSING = "ERR_SHADOWING_REFERENCE_MISSING"
	ERR_SHADOWING_UNSCORABLE        = "ERR_SHADOWING_UNSCORABLE"
	ERR_SHADOWING_AUDIO_UNSUPPORTED = "ERR_SHADOWING_AUDIO_UNSUPPORTED"
)

const (
	ErrShadowingEvalInvalid      = ERR_SHADOWING_EVAL_INVALID
	ErrShadowingReferenceMissing = ERR_SHADOWING_REFERENCE_MISSING
	ErrShadowingUnscorable       = ERR_SHADOWING_UNSCORABLE
	ErrShadowingAudioUnsupported = ERR_SHADOWING_AUDIO_UNSUPPORTED
)

const (
	// S7 recalibrated confidence from voiced-frame ratios to absolute voiced
	// frame counts and made the rubric mode-aware; both version components are
	// bumped so historical scores remain comparable.
	ScorerVersionLocal    = "local-prosody-1.1"
	WeightsVersionShadow  = "w-shadow-1.1"
	WeightsVersionRepeat  = "w-repeat-1.1"
	ModelVersionLocalOnly = "none"
	ScoreSourceLocal      = "local_prosody_v1"
)

// DimensionScore is one scored dimension with its evidence strength.
type DimensionScore struct {
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
	Weight     float64 `json:"weight"`
	Available  bool    `json:"available"`
}

// Issue is one concrete, actionable improvement step. Dimension ties it back
// to the score row, EvidenceZH quotes the measured numbers behind the advice,
// and ActionZH is the drill the learner should actually perform.
type Issue struct {
	Type       string  `json:"type"`
	Dimension  string  `json:"dimension,omitempty"`
	Confidence float64 `json:"confidence"`
	EvidenceZH string  `json:"evidence_zh,omitempty"`
	MessageZH  string  `json:"message_zh"`
	ActionZH   string  `json:"action_zh,omitempty"`
}

// AudioFeatures is the compact input to the unscorable classifier.
type AudioFeatures struct {
	VoicedRatio float64
	SpeechMS    int64
	ReferenceMS int64
	LagMS       int64
	Peak        float64
}

// AggregateResult is the outcome of the confidence-gated aggregation.
// Weights are the effective (renormalized) weights actually used, and
// AchievableWeight is the share of the ideal rubric this attempt could reach.
type AggregateResult struct {
	Overall          int
	Partial          bool
	Valid            bool
	Code             string
	Weights          map[string]float64
	AchievableWeight float64
}

// EvaluationRequest is the internal request passed from the HTTP handler.
type EvaluationRequest struct {
	UserID              int64
	LessonID            int64
	ShadowingVersion    int
	SentenceIndex       int
	Mode                EvaluationMode
	ReferencePositionMS int64
	ClientLagMS         int64
	UserAudio           []byte
}

// EvaluationResult is the API response payload for one evaluation.
type EvaluationResult struct {
	Status           string                    `json:"status"`
	UnscorableCode   string                    `json:"unscorable_code,omitempty"`
	Mode             EvaluationMode            `json:"mode"`
	OverallScore     *int                      `json:"overall_score,omitempty"`
	Partial          bool                      `json:"partial"`
	Scores           map[string]DimensionScore `json:"scores"`
	RecognizedText   string                    `json:"recognized_text"`
	LagMS            int64                     `json:"lag_ms"`
	Issues           []Issue                   `json:"issues"`
	ScoreSource      string                    `json:"score_source"`
	ScorerVersion    string                    `json:"scorer_version"`
	WeightsVersion   string                    `json:"weights_version"`
	ModelVersion     string                    `json:"model_version"`
	ShadowingVersion int                       `json:"shadowing_version"`
	LatencyMS        int64                     `json:"latency_ms"`
}

func weightsForMode(mode EvaluationMode) map[string]float64 {
	return ScoringConfigForMode(mode).Weights
}

func weightsVersionForMode(mode EvaluationMode) string {
	return ScoringConfigForMode(mode).WeightsVersion
}
