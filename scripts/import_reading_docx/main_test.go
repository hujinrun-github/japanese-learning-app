package main

import (
	"strings"
	"testing"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

func TestAlignParagraphs(t *testing.T) {
	cases := []struct {
		name string
		jp   []string
		cn   []string
		want []alignedSentence
	}{
		{
			name: "equal counts",
			jp:   []string{"日文一。", "日文二。"},
			cn:   []string{"中文一。", "中文二。"},
			want: []alignedSentence{
				{Japanese: "日文一。", Chinese: "中文一。"},
				{Japanese: "日文二。", Chinese: "中文二。"},
			},
		},
		{
			name: "more chinese paragraphs are appended to the last japanese one",
			jp:   []string{"日文一。", "日文二。"},
			cn:   []string{"中文一。", "中文二。", "多出来的中文。"},
			want: []alignedSentence{
				{Japanese: "日文一。", Chinese: "中文一。"},
				{Japanese: "日文二。", Chinese: "中文二。\n多出来的中文。"},
			},
		},
		{
			name: "missing chinese paragraphs leave the translation empty",
			jp:   []string{"日文一。", "日文二。", "日文三。"},
			cn:   []string{"中文一。"},
			want: []alignedSentence{
				{Japanese: "日文一。", Chinese: "中文一。"},
				{Japanese: "日文二。", Chinese: ""},
				{Japanese: "日文三。", Chinese: ""},
			},
		},
		{
			name: "no chinese at all",
			jp:   []string{"日文一。"},
			cn:   nil,
			want: []alignedSentence{{Japanese: "日文一。", Chinese: ""}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := alignParagraphs(tc.jp, tc.cn)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseChapterHeading(t *testing.T) {
	cases := []struct {
		name      string
		heading   string
		wantDate  string
		wantTitle string
	}{
		{"full heading", "第 1 篇  2026-01-26  故郷で過ごす時間", "2026-01-26", "故郷で過ごす時間"},
		{"no subtitle", "第 543 篇  2024-08-02", "2024-08-02", ""},
		{"double spaces", "第  12  篇   2025-12-31   初詣", "2025-12-31", "初詣"},
		{"not a chapter", "001  26.1.26日语阅读素材｜添える", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			date, title := parseChapterHeading(tc.heading)
			if date != tc.wantDate || title != tc.wantTitle {
				t.Errorf("parseChapterHeading(%q) = (%q, %q), want (%q, %q)",
					tc.heading, date, title, tc.wantDate, tc.wantTitle)
			}
		})
	}
}

func TestBuildTitle(t *testing.T) {
	cases := []struct {
		name    string
		number  int
		date    string
		heading string
		want    string
	}{
		{"chapter title wins", 1, "2026-01-26", "故郷で過ごす時間", "日语阅读素材 001｜故郷で過ごす時間"},
		{"fall back to numbered heading", 2, "2026-01-25", "添える", "日语阅读素材 002｜添える"},
		{"no subtitle uses date", 543, "2024-08-02", "", "日语阅读素材 543｜2024-08-02"},
		{"no subtitle and no date", 544, "", "", "日语阅读素材 544"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildTitle(tc.number, tc.date, tc.heading); got != tc.want {
				t.Errorf("buildTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestKatakanaToHiragana(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"コンニチハ", "こんにちは"},
		{"カタカナ", "かたかな"},
		{"ヒラガナ", "ひらがな"},
		{"あいうえお", "あいうえお"},
		{"漢字", "漢字"},
		{"コーヒー", "こーひー"},
	}
	for _, tc := range cases {
		if got := katakanaToHiragana(tc.in); got != tc.want {
			t.Errorf("katakanaToHiragana(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTokenizeAddsReadingsForKanjiOnly(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatalf("tokenizer.New: %v", err)
	}
	tokens := tokenizeText(tok, "今日はいい天気です。")
	if len(tokens) == 0 {
		t.Fatal("tokenizeText returned no tokens")
	}
	var surface strings.Builder
	withReading := 0
	for _, item := range tokens {
		surface.WriteString(item.Surface)
		if item.Reading != "" {
			withReading++
			if !containsKanji(item.Surface) {
				t.Errorf("token %q has reading %q but no kanji", item.Surface, item.Reading)
			}
		}
	}
	if surface.String() != "今日はいい天気です。" {
		t.Errorf("reconstructed text = %q, want original", surface.String())
	}
	if withReading == 0 {
		t.Error("expected at least one kanji token with a reading")
	}
}

func TestValidateLessons(t *testing.T) {
	good := func() []lesson {
		return []lesson{{
			Title:     "日语阅读素材 001｜テスト",
			JLPTLevel: "N2",
			Sentences: []sentenceOut{{
				Index: 0, Paragraph: 0,
				Tokens:  []furiganaToken{{Surface: "テスト。", Reading: ""}},
				Chinese: "测试。",
			}},
		}}
	}
	cases := []struct {
		name    string
		mutate  func([]lesson)
		wantErr string
	}{
		{"valid", func([]lesson) {}, ""},
		{"empty title", func(l []lesson) { l[0].Title = "" }, "title"},
		{"invalid level", func(l []lesson) { l[0].JLPTLevel = "N7" }, "jlpt_level"},
		{"no sentences", func(l []lesson) { l[0].Sentences = nil }, "sentences"},
		{"no tokens", func(l []lesson) { l[0].Sentences[0].Tokens = nil }, "tokens"},
		{"index gap", func(l []lesson) { l[0].Sentences[0].Index = 3 }, "index"},
		{"end before start", func(l []lesson) { l[0].Sentences[0].EndMS = -1 }, "end_ms"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lessons := good()
			tc.mutate(lessons)
			err := validateLessons(lessons)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateLessons() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateLessons() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildLessonsKeepsChineseOnlyArticles(t *testing.T) {
	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		t.Fatalf("tokenizer.New: %v", err)
	}
	articles := []article{
		{number: 1, jp: []string{"日本語です。"}, cn: []string{"这是日语。"}},
		{number: 2, jp: nil, cn: []string{"只有中文一。", "只有中文二。"}, date: "2025-05-12"},
	}
	lessons, chineseOnly, stats := buildLessons(articles, "N2", tok)
	if len(lessons) != 1 {
		t.Fatalf("lessons = %d, want 1", len(lessons))
	}
	if len(chineseOnly) != 1 {
		t.Fatalf("chineseOnly = %d, want 1", len(chineseOnly))
	}
	if got := chineseOnly[0].ChineseParagraphs; len(got) != 2 || got[0] != "只有中文一。" {
		t.Errorf("ChineseParagraphs = %v, want both paragraphs kept", got)
	}
	if chineseOnly[0].Title == "" || chineseOnly[0].Number != 2 {
		t.Errorf("chineseOnly metadata = %+v", chineseOnly[0])
	}
	if len(stats.skippedNoText) != 1 || stats.skippedNoText[0] != 2 {
		t.Errorf("skippedNoText = %v, want [2]", stats.skippedNoText)
	}
}

func TestValidateLessonsRejectsDuplicateTitle(t *testing.T) {
	one := lesson{
		Title:     "日语阅读素材 001｜テスト",
		JLPTLevel: "N2",
		Sentences: []sentenceOut{{Tokens: []furiganaToken{{Surface: "テスト。"}}}},
	}
	err := validateLessons([]lesson{one, one})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("validateLessons() error = %v, want duplicate title error", err)
	}
}
