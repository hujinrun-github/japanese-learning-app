package shadowing_test

import (
	"strings"
	"testing"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/module/shadowing"
)

func adviceScores(sync, prosody, fluency float64) map[string]shadowing.DimensionScore {
	return map[string]shadowing.DimensionScore{
		"sync":      {Score: sync, Confidence: 0.9, Available: true},
		"prosody":   {Score: prosody, Confidence: 0.9, Available: true},
		"fluency":   {Score: fluency, Confidence: 0.9, Available: true},
		"coverage":  {Available: false},
		"segmental": {Available: false},
	}
}

func baseAdvice() shadowing.AdviceInput {
	return shadowing.AdviceInput{
		Mode:        shadowing.EvaluationModeShadow,
		Overall:     72,
		Scores:      adviceScores(80, 50, 100),
		LagMS:       630,
		SpeechMS:    2980,
		ReferenceMS: 3000,
		VoicedRatio: 0.83,
		EnvelopeSim: 82,
	}
}

func adviceTypes(issues []shadowing.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Type)
	}
	return out
}

// A scored attempt must never leave the learner with an empty feedback panel.
func TestBuildAdviceAlwaysNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   shadowing.AdviceInput
	}{
		{"typical", baseAdvice()},
		{"all high", func() shadowing.AdviceInput {
			in := baseAdvice()
			in.Overall = 94
			in.Scores = adviceScores(96, 94, 92)
			return in
		}()},
		{"no dimensions", func() shadowing.AdviceInput {
			in := baseAdvice()
			in.Scores = nil
			return in
		}()},
		{"unavailable dimensions", func() shadowing.AdviceInput {
			in := baseAdvice()
			in.Scores = map[string]shadowing.DimensionScore{
				"sync": {Score: 80, Confidence: 0.3, Available: true},
			}
			return in
		}()},
		{"repeat mode", func() shadowing.AdviceInput {
			in := baseAdvice()
			in.Mode = shadowing.EvaluationModeRepeat
			return in
		}()},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := shadowing.BuildAdvice(tt.in)
			if len(got) == 0 {
				t.Fatal("BuildAdvice() returned no advice")
			}
			for i, issue := range got {
				if issue.Type == "" {
					t.Errorf("advice[%d] has no type", i)
				}
				if strings.TrimSpace(issue.MessageZH) == "" {
					t.Errorf("advice[%d] (%s) has no message", i, issue.Type)
				}
				if strings.TrimSpace(issue.ActionZH) == "" {
					t.Errorf("advice[%d] (%s) has no concrete action", i, issue.Type)
				}
			}
		})
	}
}

// Advice must quote the measured numbers, not just say "needs work".
func TestBuildAdviceEvidenceIsConcrete(t *testing.T) {
	t.Run("late start quotes measured and target lag", func(t *testing.T) {
		in := baseAdvice()
		in.LagMS = 1500
		got := shadowing.BuildAdvice(in)
		found := false
		for _, issue := range got {
			if issue.Dimension == "sync" {
				found = true
				for _, want := range []string{"1500", "600", "900"} {
					if !strings.Contains(issue.EvidenceZH, want) {
						t.Errorf("sync evidence %q missing %q", issue.EvidenceZH, want)
					}
				}
			}
		}
		if !found {
			t.Fatalf("no sync advice for late start: %v", adviceTypes(got))
		}
	})

	t.Run("early start quotes measured lag", func(t *testing.T) {
		in := baseAdvice()
		in.LagMS = 100
		got := shadowing.BuildAdvice(in)
		found := false
		for _, issue := range got {
			if issue.Type == "sync_lag_early" {
				found = true
				if !strings.Contains(issue.EvidenceZH, "100") || !strings.Contains(issue.EvidenceZH, "500") {
					t.Errorf("early evidence lacks numbers: %q", issue.EvidenceZH)
				}
			}
		}
		if !found {
			t.Fatalf("no early-start advice: %v", adviceTypes(got))
		}
	})

	t.Run("pause quotes its position and length", func(t *testing.T) {
		in := baseAdvice()
		in.Pauses = []audio.PauseSpan{{StartMS: 1200, EndMS: 1560}}
		got := shadowing.BuildAdvice(in)
		found := false
		for _, issue := range got {
			if issue.Dimension == "fluency" {
				found = true
				if !strings.Contains(issue.EvidenceZH, "1.2") || !strings.Contains(issue.EvidenceZH, "360") {
					t.Errorf("pause evidence lacks position/length: %q", issue.EvidenceZH)
				}
			}
		}
		if !found {
			t.Fatalf("no fluency advice for a long pause: %v", adviceTypes(got))
		}
	})

	t.Run("prosody quotes its score", func(t *testing.T) {
		got := shadowing.BuildAdvice(baseAdvice())
		found := false
		for _, issue := range got {
			if issue.Dimension == "prosody" {
				found = true
				if !strings.Contains(issue.EvidenceZH, "50") {
					t.Errorf("prosody evidence lacks the score: %q", issue.EvidenceZH)
				}
			}
		}
		if !found {
			t.Fatalf("no prosody advice for a weak contour: %v", adviceTypes(got))
		}
	})

	t.Run("pace quotes both durations", func(t *testing.T) {
		in := baseAdvice()
		in.SpeechMS = 1600
		in.ReferenceMS = 3000
		got := shadowing.BuildAdvice(in)
		for _, issue := range got {
			if issue.Type == "fluency_pace" {
				if !strings.Contains(issue.EvidenceZH, "1600") || !strings.Contains(issue.EvidenceZH, "3000") {
					t.Errorf("pace evidence lacks durations: %q", issue.EvidenceZH)
				}
				return
			}
		}
		t.Fatalf("no pace advice: %v", adviceTypes(got))
	})
}

// The weakest, highest-weight dimension should come first, and duplicates must
// never repeat the same advice type.
func TestBuildAdvicePriorityAndLimit(t *testing.T) {
	in := baseAdvice()
	in.Scores = adviceScores(45, 92, 96) // sync is clearly the weakest
	in.LagMS = 1500
	in.Pauses = []audio.PauseSpan{{StartMS: 1200, EndMS: 1560}}
	in.EnvelopeSim = 30

	got := shadowing.BuildAdvice(in)
	if len(got) == 0 {
		t.Fatal("no advice")
	}
	if got[0].Dimension != "sync" && got[0].Dimension != "prosody" && got[0].Dimension != "fluency" {
		t.Errorf("first advice should target a scored dimension, got %q", got[0].Dimension)
	}
	if len(got) > 3 {
		t.Errorf("advice count = %d, want <= 3: %v", len(got), adviceTypes(got))
	}
	seen := map[string]bool{}
	for _, issue := range got {
		if seen[issue.Type] {
			t.Errorf("duplicate advice type %q", issue.Type)
		}
		seen[issue.Type] = true
		if issue.Confidence < 0.6 {
			t.Errorf("advice %q has confidence %.2f below display threshold", issue.Type, issue.Confidence)
		}
	}
}

// A strong attempt still gets a concrete "what to polish next" suggestion.
func TestBuildAdviceFallbackForHighScores(t *testing.T) {
	in := baseAdvice()
	in.Overall = 94
	in.Scores = adviceScores(96, 94, 92)
	got := shadowing.BuildAdvice(in)
	if got[0].Type != "keep_practice" {
		t.Fatalf("expected keep_practice fallback, got %v", adviceTypes(got))
	}
	if !strings.Contains(got[0].EvidenceZH, "92") {
		t.Errorf("fallback should name the weakest dimension score: %q", got[0].EvidenceZH)
	}
	if !strings.Contains(got[0].ActionZH, "再") && !strings.Contains(got[0].ActionZH, "遍") {
		t.Errorf("fallback action is not a concrete drill: %q", got[0].ActionZH)
	}
}

// Repeat mode must not nag about shadowing lag.
func TestBuildAdviceRepeatModeIgnoresShadowLag(t *testing.T) {
	in := baseAdvice()
	in.Mode = shadowing.EvaluationModeRepeat
	in.LagMS = 1500
	for _, issue := range shadowing.BuildAdvice(in) {
		if issue.Type == "sync_lag_late" || issue.Type == "sync_lag_early" {
			t.Errorf("repeat mode got shadowing lag advice %q", issue.Type)
		}
	}
}

// Every unscorable reason needs its own measured evidence and a specific fix.
func TestBuildAdviceUnscorableActions(t *testing.T) {
	tests := []struct {
		code        string
		wantType    string
		wantNumbers []string
		wantAction  string
	}{
		{"silence", "unscorable_silence", nil, "麦克风"},
		{"too_short", "unscorable_too_short", []string{"760", "3000"}, "整句"},
		{"bleed_through", "unscorable_bleed_through", []string{"0"}, "耳机"},
		{"low_confidence", "unscorable_low_confidence", nil, "安静"},
		{"content_mismatch", "unscorable_content_mismatch", nil, "句子"},
		{"provider_unavailable", "unscorable_provider_unavailable", nil, "重试"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			in := baseAdvice()
			in.UnscorableCode = tt.code
			in.SpeechMS = 760
			in.LagMS = 0
			got := shadowing.BuildAdvice(in)
			if len(got) != 1 {
				t.Fatalf("want exactly one unscorable action, got %v", adviceTypes(got))
			}
			if got[0].Type != tt.wantType {
				t.Errorf("type = %q, want %q", got[0].Type, tt.wantType)
			}
			for _, n := range tt.wantNumbers {
				if !strings.Contains(got[0].EvidenceZH, n) {
					t.Errorf("evidence %q missing %q", got[0].EvidenceZH, n)
				}
			}
			if !strings.Contains(got[0].ActionZH, tt.wantAction) {
				t.Errorf("action %q missing %q", got[0].ActionZH, tt.wantAction)
			}
			if strings.TrimSpace(got[0].EvidenceZH) == "" {
				t.Error("unscorable advice has no evidence")
			}
		})
	}
}

// The real pipeline must populate advice on both branches.
func TestEvaluateWAVPopulatesAdvice(t *testing.T) {
	ref := speechWAV(16000, 3)
	user := shiftedWAV(ref, 600)

	scoreable, err := shadowing.EvaluateWAV(ref, user, shadowing.EvaluationModeShadow)
	if err != nil {
		t.Fatalf("EvaluateWAV scoreable: %v", err)
	}
	if scoreable.Status == "scoreable" && len(scoreable.Issues) == 0 {
		t.Error("scoreable result has no advice")
	}

	unscorable, err := shadowing.EvaluateWAV(ref, silentWAV(16000, 3), shadowing.EvaluationModeShadow)
	if err != nil {
		t.Fatalf("EvaluateWAV unscorable: %v", err)
	}
	if unscorable.Status != "unscorable" {
		t.Fatalf("expected unscorable, got %s", unscorable.Status)
	}
	if len(unscorable.Issues) == 0 {
		t.Error("unscorable result has no actionable advice")
	}
	if !strings.Contains(unscorable.Issues[0].ActionZH, "麦克风") {
		t.Errorf("silence advice is not specific: %+v", unscorable.Issues[0])
	}
}
