package translation

import (
	"strings"
	"unicode"
)

// SplitSentences splits text into sentences by Japanese/Chinese punctuation marks.
// Min sentence length: 3 runes. Max: 200 runes (longer segments are further split).
func SplitSentences(text string) []string {
	type boundary struct {
		idx int // byte position after the punctuation rune
	}
	var boundaries []boundary
	runes := []rune(text)
	for i, r := range runes {
		switch r {
		case '。', '！', '？', '…', '.', '!', '?':
			bytePos := 0
			for j := 0; j <= i; j++ {
				bytePos += len(string(runes[j]))
			}
			boundaries = append(boundaries, boundary{idx: bytePos})
		}
	}

	var parts []string
	prev := 0
	for _, b := range boundaries {
		part := strings.TrimSpace(text[prev:b.idx])
		if len([]rune(part)) >= 3 {
			parts = append(parts, part)
		}
		prev = b.idx
	}

	remaining := strings.TrimSpace(text[prev:])
	if len([]rune(remaining)) >= 3 {
		parts = append(parts, remaining)
	}

	if len(boundaries) == 0 && len(runes) >= 3 {
		parts = append(parts, strings.TrimSpace(text))
	}

	var result []string
	for _, p := range parts {
		if len([]rune(p)) > 200 {
			result = append(result, splitLongChunk(p)...)
		} else {
			result = append(result, p)
		}
	}
	return result
}

func splitLongChunk(s string) []string {
	var chunks []string
	runes := []rune(s)
	for i := 0; i < len(runes); i += 200 {
		end := i + 200
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// DetectDirection detects translation direction by counting CJK characters
// vs hiragana/katakana in the text. More kana = jp2cn, more CJK-only = cn2jp.
func DetectDirection(text string) string {
	var kana int
	var cjk int
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			kana++
		} else if unicode.Is(unicode.Han, r) {
			cjk++
		}
	}
	if kana > 0 && kana >= cjk/3 {
		return "jp2cn"
	}
	if cjk > 0 && kana < cjk/3 {
		return "cn2jp"
	}
	if kana >= cjk {
		return "jp2cn"
	}
	return "cn2jp"
}
