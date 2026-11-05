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
		"Japanese reference translation",
		"grammar_explanations",
		"evidence",
		"return an empty array",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not contain %q", want)
		}
	}
}
