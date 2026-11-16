package audio

import "strings"

// NormalizeJA keeps only the kana sequence of a Japanese string:
// katakana is folded to hiragana, the long-vowel mark is preserved, and kanji,
// punctuation, latin letters and digits are removed. Comparing kana-only
// sequences avoids false CER penalties when ASR emits different kanji or
// Arabic numerals than the reference text.
func NormalizeJA(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0x30A1 && r <= 0x30F6:
			b.WriteRune(r - 0x60)
		case r >= 0x3041 && r <= 0x3096:
			b.WriteRune(r)
		case r == 0x30FC:
			b.WriteRune(r)
		}
	}
	return b.String()
}
