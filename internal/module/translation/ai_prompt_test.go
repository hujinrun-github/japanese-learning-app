package translation

import (
	"strings"
	"testing"
)

func TestBuildTranslationReviewPromptRequestsJapaneseGrammar(t *testing.T) {
	prompt := buildTranslationReviewPrompt("jp2cn", "雨が降っています。", "正在下雨。", "正在下雨。")

	for _, want := range []string{
		"Japanese grammar",
		"jp2cn",
		"Japanese source text",
		"Japanese student translation",
		"grammar_explanations",
		"Do not leave this array empty",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not contain %q", want)
		}
	}
}
