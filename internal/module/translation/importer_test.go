package translation_test

import (
	"reflect"
	"testing"

	"japanese-learning-app/internal/module/translation"
)

func TestSplitSentences_Japanese(t *testing.T) {
	text := "今日はいい天気です。明日も晴れるでしょう。"
	result := translation.SplitSentences(text)

	want := []string{"今日はいい天気です。", "明日も晴れるでしょう。"}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("SplitSentences = %q, want %q", result, want)
	}
}

func TestSplitSentences_Chinese(t *testing.T) {
	text := "今天天气很好。明天也会放晴吧。"
	result := translation.SplitSentences(text)

	want := []string{"今天天气很好。", "明天也会放晴吧。"}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("SplitSentences = %q, want %q", result, want)
	}
}

func TestSplitSentences_MinLength(t *testing.T) {
	text := "はい。今日はいい天気です。ええ。"
	result := translation.SplitSentences(text)

	if len(result) != 3 {
		t.Errorf("SplitSentences count = %d, want 3", len(result))
	}
}

func TestSplitSentences_MaxLength(t *testing.T) {
	long := ""
	for i := 0; i < 250; i++ {
		long += "あ"
	}
	result := translation.SplitSentences(long)
	if len(result) < 2 {
		t.Errorf("long text should be split into at least 2 chunks, got %d", len(result))
	}
	for _, r := range result {
		if len([]rune(r)) > 200 {
			t.Errorf("chunk length %d exceeds max 200", len([]rune(r)))
		}
	}
}

func TestDetectDirection_JP(t *testing.T) {
	direction := translation.DetectDirection("これは日本語のテキストです。")
	if direction != "jp2cn" {
		t.Errorf("DetectDirection = %q, want jp2cn", direction)
	}
}

func TestDetectDirection_CN(t *testing.T) {
	direction := translation.DetectDirection("这是中文文本。")
	if direction != "cn2jp" {
		t.Errorf("DetectDirection = %q, want cn2jp", direction)
	}
}

func TestDetectDirection_Mixed(t *testing.T) {
	direction := translation.DetectDirection("日本語のtextです。")
	if direction != "jp2cn" {
		t.Errorf("DetectDirection = %q, want jp2cn", direction)
	}
}
