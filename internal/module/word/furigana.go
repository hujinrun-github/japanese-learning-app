package word

import (
	"strings"
	"unicode"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var sharedTokenizer *tokenizer.Tokenizer

func init() {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		sharedTokenizer = nil
		return
	}
	sharedTokenizer = t
}

// FuriganaHTML wraps kanji in <ruby> tags with hiragana readings.
// Returns the original text unchanged if the tokenizer is unavailable.
func FuriganaHTML(text string) string {
	if sharedTokenizer == nil {
		return text
	}
	tokens := sharedTokenizer.Tokenize(text)
	var sb strings.Builder
	for _, tok := range tokens {
		surface := tok.Surface
		if containsKanji(surface) {
			reading, ok := tok.Reading()
			if !ok {
				sb.WriteString(surface)
				continue
			}
			sb.WriteString("<ruby>")
			sb.WriteString(surface)
			sb.WriteString("<rt>")
			sb.WriteString(katakanaToHiraganaStr(reading))
			sb.WriteString("</rt></ruby>")
		} else {
			sb.WriteString(surface)
		}
	}
	return sb.String()
}

func katakanaToHiraganaStr(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= 0x30A1 && r <= 0x30F6 {
			sb.WriteRune(r - 0x60)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func containsKanji(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
