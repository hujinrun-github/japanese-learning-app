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
