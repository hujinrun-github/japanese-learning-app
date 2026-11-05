package translation

import (
	"slices"
	"testing"
)

func TestLocalGrammarAvoidsSubstringMatches(t *testing.T) {
	for _, tc := range []struct {
		name, direction, source, reference string
		want                               []string
	}{
		{"ordinary words and place of origin", "jp2cn", "こんにちは。たいへんでした。東京から来ました。", "", nil},
		{"lexical ながら and unrelated ことが", "jp2cn", "残念ながら、そういうことが多いです。", "", nil},
		{"separate location and いる", "jp2cn", "庭にいました。", "", nil},
		{"ability", "jp2cn", "日本語を話すことができます。", "", []string{"〜ことができる"}},
		{"past experience", "jp2cn", "京都へ行ったことがあります。", "", []string{"〜たことがある"}},
		{"dedup and voiced forms", "jp2cn", "雨が降っています。猫が寝ています。泳いでいました。", "", []string{"〜ている", "〜ていた"}},
		{"Chinese prompt uses Japanese reference", "cn2jp", "我会说日语。", "日本語を話すことができます。", []string{"〜ことができる"}},
		{"Chinese prompt with no reference", "cn2jp", "今天没下雨。", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := detectGrammarExplanations(grammarText(tc.direction, tc.source, tc.reference))
			var got []string
			for _, item := range items {
				got = append(got, item.GrammarPoint)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGrammarFeedbackSources(t *testing.T) {
	aiItem := GrammarExplanation{GrammarPoint: "〜ている", Explanation: "动作正在进行", Evidence: "降っています"}
	for _, tc := range []struct {
		name      string
		points    []string
		reference string
		aiItems   []GrammarExplanation
		want      []string
		source    string
	}{
		{"bank annotations win", []string{" て形 ", "", "て形"}, "雨が降っています。", []GrammarExplanation{aiItem}, []string{"て形"}, "bank"},
		{"AI evidence must be in the reference", nil, "日本語を話すことができます。", []GrammarExplanation{aiItem}, []string{"〜ことができる"}, "local"},
		{"AI evidence is present", nil, "雨が降っています。", []GrammarExplanation{aiItem, aiItem}, []string{"〜ている"}, "ai"},
		{"no reliable grammar", nil, "こんにちは。", []GrammarExplanation{{GrammarPoint: "は", Explanation: "提示主题", Evidence: "は"}}, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := TranslationFeedback{GrammarExplanations: tc.aiItems}
			applyGrammarFeedback(&fb, TranslationSentence{
				Direction: "cn2jp", SourceText: "这是中文题目。", GrammarPoints: tc.points,
			}, tc.reference)
			var got []string
			for _, item := range fb.GrammarExplanations {
				got = append(got, item.GrammarPoint)
			}
			if !slices.Equal(got, tc.want) || fb.GrammarSource != tc.source {
				t.Errorf("got %v from %q, want %v from %q", got, fb.GrammarSource, tc.want, tc.source)
			}
		})
	}
}
