package admin

import (
	"strings"
	"unicode"

	"japanese-learning-app/internal/module/word"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// autoFillWord uses kagome morphological analysis to fill in missing reading,
// part_of_speech, and reading_type fields on a Word.
func autoFillWord(w *word.Word) {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return
	}

	tokens := t.Tokenize(w.KanjiForm)
	var readingParts []string
	var pos string
	for _, tok := range tokens {
		if r, ok := tok.Reading(); ok {
			readingParts = append(readingParts, r)
		}
		if pos == "" {
			feat := tok.Features()
			if len(feat) > 0 {
				pos = feat[0]
			}
		}
	}

	if w.Reading == "" && len(readingParts) > 0 {
		w.Reading = katakanaToHiragana(strings.Join(readingParts, ""))
	}
	if w.PartOfSpeech == "" && pos != "" {
		w.PartOfSpeech = mapPoS(pos)
	}
	if w.ReadingType == "" {
		w.ReadingType = inferReadingType(w.KanjiForm, w.Reading)
	}
}

func katakanaToHiragana(s string) string {
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

var posMap = map[string]string{
	"名詞": "名詞", "動詞": "動詞", "形容詞": "形容詞",
	"形容動詞": "形容動詞", "副詞": "副詞", "助詞": "助詞",
	"助動詞": "助動詞", "接続詞": "接続詞", "感動詞": "感動詞",
	"連体詞": "連体詞", "接頭詞": "接頭詞", "代名詞": "代名詞",
}

func mapPoS(pos string) string {
	if m, ok := posMap[pos]; ok {
		return m
	}
	return pos
}

func inferReadingType(kanjiForm, reading string) string {
	kanjiCount := 0
	for _, r := range kanjiForm {
		if unicode.Is(unicode.Han, r) {
			kanjiCount++
		}
	}
	if kanjiCount == 0 {
		return "6"
	}
	if kanjiCount >= 2 {
		return "1"
	}
	if len([]rune(kanjiForm)) > kanjiCount {
		return "2"
	}
	return "6"
}
