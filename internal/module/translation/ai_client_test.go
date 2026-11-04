package translation_test

import (
	"fmt"
	"testing"

	"japanese-learning-app/internal/module/translation"
)

func TestStubReviewer_Review(t *testing.T) {
	stub := &translation.StubReviewer{
		Feedback: translation.TranslationFeedback{
			AIScore: 90,
			GrammarExplanations: []translation.GrammarExplanation{
				{GrammarPoint: "〜ている", Explanation: "ongoing state"},
			},
			CorrectedTranslation: "我在吃饭。",
			ReferenceTranslation: "我在吃饭。",
		},
	}

	fb, err := stub.Review("jp2cn", "ご飯を食べています。", "我在吃饭。", "正在吃饭。")
	if err != nil {
		t.Fatalf("Review error: %v", err)
	}
	if fb.AIScore != 90 {
		t.Errorf("AIScore = %d, want 90", fb.AIScore)
	}
	if len(fb.GrammarExplanations) != 1 {
		t.Errorf("GrammarExplanations len = %d, want 1", len(fb.GrammarExplanations))
	}
}

func TestStubReviewer_Error(t *testing.T) {
	stub := &translation.StubReviewer{
		Err: fmt.Errorf("simulated error"),
	}

	_, err := stub.Review("cn2jp", "你好", "こんにちは", "")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestStubReviewer_DefaultFeedbackIncludesGrammar(t *testing.T) {
	tests := []struct {
		name             string
		direction        string
		sourceText       string
		userTranslation  string
		reference        string
		wantGrammarCount int
		wantNonZeroScore bool
	}{
		{
			name:             "Japanese source for jp2cn",
			direction:        "jp2cn",
			sourceText:       "雨が降っています。",
			userTranslation:  "正在下雨。",
			reference:        "正在下雨。",
			wantGrammarCount: 1,
			wantNonZeroScore: true,
		},
		{
			name:             "Japanese answer for cn2jp",
			direction:        "cn2jp",
			sourceText:       "正在下雨。",
			userTranslation:  "雨が降っています。",
			reference:        "雨が降っています。",
			wantGrammarCount: 1,
			wantNonZeroScore: true,
		},
		{
			name:             "reference fallback for cn2jp",
			direction:        "cn2jp",
			sourceText:       "正在下雨。",
			userTranslation:  "下雨了。",
			reference:        "雨が降っています。",
			wantGrammarCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb, err := (&translation.StubReviewer{}).Review(tt.direction, tt.sourceText, tt.userTranslation, tt.reference)
			if err != nil {
				t.Fatalf("Review error: %v", err)
			}
			if tt.wantNonZeroScore && fb.AIScore == 0 {
				t.Error("default feedback score should use local scoring")
			}
			if len(fb.GrammarExplanations) < tt.wantGrammarCount {
				t.Errorf("GrammarExplanations len = %d, want at least %d", len(fb.GrammarExplanations), tt.wantGrammarCount)
			}
		})
	}
}
