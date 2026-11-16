package shadowing

// ScoringConfig is one versioned calibration of the local evaluator. Every
// threshold and every weight lives here so a score can be reproduced from its
// scorer_version / weights_version tuple instead of from scattered magic
// numbers. Callers receive a fresh copy and may freely inspect or mutate it.
type ScoringConfig struct {
	WeightsVersion string
	// Weights is the ideal rubric when every provider (including ASR) is present.
	Weights map[string]float64
	// LocalWeights is the mode-specific rubric used while the ASR-backed
	// coverage and segmental dimensions cannot produce a score. It is the
	// nominal rubric renormalized over Sync/Prosody/Fluency so no weight is
	// stranded on a dimension that cannot report evidence.
	LocalWeights map[string]float64

	// Confidence gate. Evidence is measured in absolute frame counts, not in
	// ratios against an assumed "fully voiced" utterance: Japanese TTS and
	// natural speech both contain many unvoiced consonants and pauses.
	MinDimensionConfidence float64
	MinEffectiveWeight     float64
	MinActiveDimensions    int
	MinAchievableWeight    float64

	ProsodyFullConfidenceFrames int
	FluencyFullConfidenceFrames int
	SyncVoicedConfidenceWeight  float64

	// Signal thresholds carried over from the S2 initial values. They are
	// grouped here so §8 calibration can change them in one place.
	SilenceVoicedRatio float64
	TooShortRatio      float64
	BleedPeak          float64
	BleedLagMS         int64
	TargetLagMS        int64
	LagSigmaMS         int64
	MaxLagSearchMS     int64
	EnvDistanceK       float64
	PitchDistanceK     float64
	DTWBandRatio       float64
}

// ScoringConfigForMode returns the calibrated rubric for one evaluation mode.
// The two modes intentionally differ: shadowing emphasises synchronisation,
// while repeat emphasises segmental accuracy once ASR is available. In the
// local-only stage repeat therefore redistributes its 0.50 ASR share onto the
// three measurable dimensions instead of leaving it unusable.
func ScoringConfigForMode(mode EvaluationMode) ScoringConfig {
	cfg := ScoringConfig{
		MinDimensionConfidence: 0.60,
		MinEffectiveWeight:     0.50,
		MinActiveDimensions:    2,
		SilenceVoicedRatio:     0.05,
		TooShortRatio:          0.30,
		BleedPeak:              0.85,
		BleedLagMS:             150,
		TargetLagMS:            600,
		LagSigmaMS:             500,
		MaxLagSearchMS:         2000,
		EnvDistanceK:           0.25,
		PitchDistanceK:         1.00,
		DTWBandRatio:           0.35,

		ProsodyFullConfidenceFrames: 60,
		FluencyFullConfidenceFrames: 90,
		SyncVoicedConfidenceWeight:  0.50,
	}

	if mode == EvaluationModeRepeat {
		cfg.WeightsVersion = WeightsVersionRepeat
		cfg.Weights = map[string]float64{
			"sync": 0.10, "prosody": 0.25, "coverage": 0.15,
			"fluency": 0.15, "segmental": 0.35,
		}
		cfg.LocalWeights = map[string]float64{
			"sync": 0.20, "prosody": 0.50, "coverage": 0,
			"fluency": 0.30, "segmental": 0,
		}
		cfg.MinAchievableWeight = 0.30
		return cfg
	}

	cfg.WeightsVersion = WeightsVersionShadow
	cfg.Weights = map[string]float64{
		"sync": 0.30, "prosody": 0.30, "coverage": 0.15,
		"fluency": 0.15, "segmental": 0.10,
	}
	cfg.LocalWeights = map[string]float64{
		"sync": 0.40, "prosody": 0.40, "coverage": 0,
		"fluency": 0.20, "segmental": 0,
	}
	cfg.MinAchievableWeight = 0.35
	return cfg
}

// effectiveWeights resolves the rubric that actually applies to this attempt.
// Dimensions that could not produce a score get weight 0; the remaining
// weights are renormalized to 1.0. Renormalizing inside the gate (rather than
// only inside the weighted average) is what lets repeat mode work before ASR.
func effectiveWeights(cfg ScoringConfig, scores map[string]DimensionScore) map[string]float64 {
	asrPresent := false
	for _, name := range []string{"coverage", "segmental"} {
		if d, ok := scores[name]; ok && d.Available {
			asrPresent = true
		}
	}
	base := cfg.LocalWeights
	if asrPresent {
		base = cfg.Weights
	}

	total := 0.0
	for name, weight := range base {
		if d, ok := scores[name]; ok && d.Available {
			total += weight
		}
	}
	out := make(map[string]float64, len(base))
	for name, weight := range base {
		if d, ok := scores[name]; ok && d.Available && total > 0 {
			out[name] = weight / total
		} else {
			out[name] = 0
		}
	}
	return out
}

// achievableWeight is the share of the ideal (full-provider) rubric that this
// attempt could possibly produce. It stops a single surviving dimension from
// turning into a confident-looking overall score.
func achievableWeight(cfg ScoringConfig, scores map[string]DimensionScore) float64 {
	total := 0.0
	for name, weight := range cfg.Weights {
		if d, ok := scores[name]; ok && d.Available {
			total += weight
		}
	}
	return total
}
