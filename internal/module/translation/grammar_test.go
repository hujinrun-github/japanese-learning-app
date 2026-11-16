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
	reasonItem := GrammarExplanation{GrammarPoint: "から", Explanation: "表示原因或理由", Evidence: "雨がやんだから"}
	for _, tc := range []struct {
		name         string
		direction    string // empty defaults to cn2jp
		sourceText   string // empty defaults to a Chinese prompt
		points       []string
		reference    string
		aiItems      []GrammarExplanation
		want         []string
		wantSource   string
		wantEvidence []string // nil skips the evidence assertion
	}{
		{"bank annotations win", "", "", []string{" おかげで ", "", "おかげで"}, "天気予報が当たったおかげで、雨に降られずに済んだ。", []GrammarExplanation{aiItem}, []string{"おかげで"}, "bank", []string{"おかげで"}},
		{"bank annotation with tilde prefix", "", "", []string{"〜から"}, "もう雨がやんだから、私たちは家に帰れます。", nil, []string{"〜から"}, "bank", []string{"から"}},
		{"mixed bank annotations keep only evidenced", "", "", []string{"から", "ので", "動詞"}, "もう雨がやんだから、私たちは家に帰れます。", nil, []string{"から"}, "bank", []string{"から"}},
		// Inflected surfaces such as 降っています have no literal ている; the local
		// recognizer knows the voiced/polite endings and gives a fuller explanation.
		{"bank annotation with inflected surface falls back to local", "", "", []string{"ている"}, "雨が降っています。", nil, []string{"〜ている"}, "local", nil},
		{"bank annotations without evidence fall back to AI", "", "", []string{"可能形", "ので", "動詞"}, "もう雨がやんだから、私たちは家に帰れます。", []GrammarExplanation{reasonItem}, []string{"から"}, "ai", nil},
		{"bank annotations without evidence fall back to local", "", "", []string{"名詞", "動詞"}, "日本語を話すことができます。", []GrammarExplanation{aiItem}, []string{"〜ことができる"}, "local", nil},
		{"bank annotations without evidence and no fallback", "", "", []string{"ので"}, "もう雨がやんだから、私たちは家に帰れます。", nil, nil, "", nil},
		{"single character bank annotation rejected", "", "", []string{"と"}, "友達と映画を見ました。", nil, nil, "", nil},
		{"jp2cn bank evidence uses Japanese source", "jp2cn", "もう雨がやんだから、私たちは家に帰れます。", []string{"から"}, "既然雨停了，我们可以回家了。", nil, []string{"から"}, "bank", []string{"から"}},
		{"AI evidence must be in the reference", "", "", nil, "日本語を話すことができます。", []GrammarExplanation{aiItem}, []string{"〜ことができる"}, "local", nil},
		{"AI evidence is present", "", "", nil, "雨が降っています。", []GrammarExplanation{aiItem, aiItem}, []string{"〜ている"}, "ai", nil},
		{"no reliable grammar", "", "", nil, "こんにちは。", []GrammarExplanation{{GrammarPoint: "は", Explanation: "提示主题", Evidence: "は"}}, nil, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			direction := tc.direction
			if direction == "" {
				direction = string(DirectionCN2JP)
			}
			sourceText := tc.sourceText
			if sourceText == "" {
				sourceText = "这是中文题目。"
			}
			fb := TranslationFeedback{GrammarExplanations: tc.aiItems}
			applyGrammarFeedback(&fb, TranslationSentence{
				Direction: direction, SourceText: sourceText, GrammarPoints: tc.points,
			}, tc.reference)
			var got, gotEvidence []string
			for _, item := range fb.GrammarExplanations {
				got = append(got, item.GrammarPoint)
				gotEvidence = append(gotEvidence, item.Evidence)
			}
			if !slices.Equal(got, tc.want) || fb.GrammarSource != tc.wantSource {
				t.Errorf("got %v from %q, want %v from %q", got, fb.GrammarSource, tc.want, tc.wantSource)
			}
			if tc.wantEvidence != nil && !slices.Equal(gotEvidence, tc.wantEvidence) {
				t.Errorf("got evidence %v, want %v", gotEvidence, tc.wantEvidence)
			}
		})
	}
}
