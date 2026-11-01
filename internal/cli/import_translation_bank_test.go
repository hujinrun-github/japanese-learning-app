package cli

import (
	"encoding/json"
	"testing"
)

func TestGroupSentencesByLevel(t *testing.T) {
	sentences := []TranslationBankSentence{
		{ID: "N4-0001", Level: "N4", ZH: "我每天早上七点起床。", JA: "私は毎日朝七時に起きます。"},
		{ID: "N3-0001", Level: "n3", ZH: "请把窗户关上。", JA: "窓を閉めてください。"},
		{ID: "N2-0001", Level: "N2", ZH: "空也可以。", JA: "空でもいいです。"},
		{ID: "BAD-1", Level: "N9", ZH: "x", JA: "y"},
		{ID: "EMPTY-1", Level: "N4", ZH: "", JA: "y"},
	}

	if _, err := groupSentencesByLevel(sentences); err == nil {
		t.Fatal("groupSentencesByLevel expected error for invalid level N9")
	}

	levels, err := groupSentencesByLevel(sentences[:3])
	if err != nil {
		t.Fatalf("groupSentencesByLevel: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("levels = %d, want 3", len(levels))
	}
	if len(levels["N3"]) != 1 {
		t.Fatalf("N3 bucket size = %d, want 1 (level should be upper-cased)", len(levels["N3"]))
	}
}

func TestGroupSentencesByLevelSkipsIncompletePairs(t *testing.T) {
	levels, err := groupSentencesByLevel([]TranslationBankSentence{
		{ID: "a", Level: "N4", ZH: "", JA: "x"},
		{ID: "b", Level: "N4", ZH: "x", JA: ""},
	})
	if err != nil {
		t.Fatalf("groupSentencesByLevel: %v", err)
	}
	if got := len(levels["N4"]); got != 0 {
		t.Fatalf("incomplete pairs kept = %d, want 0", got)
	}
}

func TestBuildPointLevelIndexAndResolve(t *testing.T) {
	sentences := []TranslationBankSentence{
		{ID: "a", Level: "N4", Points: []string{"ます形"}},
		{ID: "b", Level: "N4", Points: []string{"ます形"}},
		{ID: "c", Level: "N3", Points: []string{"ます形"}},
		{ID: "d", Level: "N2", Points: []string{"ばかり"}},
	}

	index := buildPointLevelIndex(sentences)

	if level, ok := pointLevel(index, "ます形"); !ok || level != "N4" {
		t.Fatalf("pointLevel(ます形) = %q,%v want N4,true", level, ok)
	}
	if level, ok := pointLevel(index, "unknown", "ばかり"); !ok || level != "N2" {
		t.Fatalf("pointLevel fallback to name = %q,%v want N2,true", level, ok)
	}
	if _, ok := pointLevel(index, "不存在"); ok {
		t.Fatal("pointLevel(missing) = true, want false")
	}
}

func TestTranslationBankFileUnmarshal(t *testing.T) {
	raw := []byte(`{
      "sentences": [
        {"id":"N4-01","level":"N4","topic":"日常","zh":"我每天早上七点起床。","ja":"私は毎日朝七時に起きます。","origin":"builtin","tags":["ます形"],"points":["ます形","動詞"],"words":[{"w":"朝","r":"あさ","m":"早晨"}]}
      ],
      "grammar": {"ます形":{"name":"ます形","desc":"动词礼貌形"}}
    }`)

	var bank TranslationBankFile
	if err := json.Unmarshal(raw, &bank); err != nil {
		t.Fatalf("unmarshal bank: %v", err)
	}
	if len(bank.Sentences) != 1 || len(bank.Grammar) != 1 {
		t.Fatalf("sentences=%d grammar=%d, want 1/1", len(bank.Sentences), len(bank.Grammar))
	}
	if bank.Sentences[0].JA != "私は毎日朝七時に起きます。" {
		t.Fatalf("unexpected ja: %q", bank.Sentences[0].JA)
	}
	if bank.Grammar["ます形"].Desc != "动词礼貌形" {
		t.Fatalf("unexpected desc: %q", bank.Grammar["ます形"].Desc)
	}
}

func TestNormalizePointName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"動詞基本形＋時", "動詞基本形+時"},
		{"〜ように（目的）", "ように(目的)"},
		{"  ます形  ", "ます形"},
		{"んです/〜んですか", "んです/んですか"},
	}
	for _, tc := range tests {
		if got := normalizePointName(tc.in); got != tc.want {
			t.Fatalf("normalizePointName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildExampleIndexAndLookup(t *testing.T) {
	sentences := []TranslationBankSentence{
		{ID: "a", ZH: "为了不迟到，我提前出门了。", JA: "遅れないように、早めに家を出ました。", Points: []string{"〜ように（目的）"}},
		{ID: "b", ZH: "我坚持每天读日语新闻。", JA: "毎日日本語のニュースを読むようにしています。", Points: []string{"〜ようにする"}},
		{ID: "c", ZH: "请关窗。", JA: "窓を閉めてください。", Tags: []string{"て形"}},
	}

	index := buildExampleIndex(sentences)

	if got := exampleSentences(index, "ように(目的)"); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("exampleSentences(ように(目的)) = %+v, want sentence a", got)
	}
	// Full-width plus / tilde should still match the stored point label.
	if got := exampleSentences(index, "〜ようにする"); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("exampleSentences(〜ようにする) = %+v, want sentence b", got)
	}
	// Tags are indexed too.
	if got := exampleSentences(index, "て形"); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("exampleSentences(て形) = %+v, want sentence c", got)
	}
	if got := exampleSentences(index, "不存在"); len(got) != 0 {
		t.Fatalf("exampleSentences(missing) = %+v, want empty", got)
	}
}

func TestPlanBankGrammarFallbackAndExamples(t *testing.T) {
	bank := TranslationBankFile{
		Sentences: []TranslationBankSentence{
			{ID: "a", Level: "N4", ZH: "我每天早上七点起床。", JA: "私は毎日朝七時に起きます。", Points: []string{"ます形"}},
			{ID: "b", Level: "N3", ZH: "请把窗户关上。", JA: "窓を閉めてください。"},
		},
		Grammar: map[string]TranslationBankGrammar{
			"ます形":     {Name: "ます形", Desc: "动词礼貌形"},
			" orphan": {Name: "孤立语法", Desc: "没有句子引用"},
		},
	}

	plans, fallback := planBankGrammar(bank, "N3")
	if fallback != 1 {
		t.Fatalf("fallback = %d, want 1", fallback)
	}
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(plans))
	}
	for _, p := range plans {
		switch p.Name {
		case "ます形":
			if p.Level != "N4" {
				t.Fatalf("ます形 level = %q, want N4", p.Level)
			}
			if len(p.Examples) != 1 {
				t.Fatalf("ます形 examples = %d, want 1", len(p.Examples))
			}
		case "孤立语法":
			if p.Level != "N3" {
				t.Fatalf("orphan level = %q, want N3 default", p.Level)
			}
			if len(p.Examples) != 0 {
				t.Fatalf("orphan examples = %d, want 0", len(p.Examples))
			}
		default:
			t.Fatalf("unexpected plan %q", p.Name)
		}
	}
}

func TestIsJLPTLevel(t *testing.T) {
	for _, level := range []string{"N1", "N2", "N3", "N4", "N5", "n3"} {
		if !isJLPTLevel(level) {
			t.Fatalf("isJLPTLevel(%q) = false, want true", level)
		}
	}
	for _, level := range []string{"", "N6", "N", "abc"} {
		if isJLPTLevel(level) {
			t.Fatalf("isJLPTLevel(%q) = true, want false", level)
		}
	}
}
