package cli

import (
	"log/slog"
	"strings"
	"unicode"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// AutoFillWords uses kagome morphological analysis to fill in missing fields
// (reading, part_of_speech, reading_type) for each word where those fields are empty.
func AutoFillWords(words []wordImport) []wordImport {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		slog.Warn("AutoFillWords: failed to init tokenizer, skipping auto-fill", "err", err)
		return words
	}

	result := make([]wordImport, len(words))
	for i, w := range words {
		result[i] = autoFillOne(t, w)
	}
	return result
}

func autoFillOne(t *tokenizer.Tokenizer, w wordImport) wordImport {
	tokens := t.Tokenize(w.KanjiForm)

	// Collect readings and part-of-speech from kagome tokens
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

	// Fill reading (katakana → hiragana)
	if w.Reading == "" && len(readingParts) > 0 {
		w.Reading = katakanaToHiragana(strings.Join(readingParts, ""))
	}

	// Fill part_of_speech
	if w.PartOfSpeech == "" && pos != "" {
		w.PartOfSpeech = mapPoS(pos)
	}

	// Fill reading_type
	if w.ReadingType == "" {
		w.ReadingType = inferReadingType(w.KanjiForm, w.Reading)
	}

	return w
}

// katakanaToHiragana converts katakana characters to hiragana.
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

// mapPoS maps kagome's Japanese part-of-speech labels to Chinese abbreviations.
func mapPoS(pos string) string {
	m := map[string]string{
		"名詞":   "名詞",
		"動詞":   "動詞",
		"形容詞":  "形容詞",
		"形容動詞": "形容動詞",
		"副詞":   "副詞",
		"助詞":   "助詞",
		"助動詞":  "助動詞",
		"接続詞":  "接続詞",
		"感動詞":  "感動詞",
		"連体詞":  "連体詞",
		"接頭詞":  "接頭詞",
		"代名詞":  "代名詞",
		// fallback: return the raw kagome label
	}
	if mapped, ok := m[pos]; ok {
		return mapped
	}
	return pos
}

// inferReadingType guesses reading type based on kanji count and reading characters.
//   "1" = 音読み (on'yomi)  — compounds of mostly kanji, typically on'yomi
//   "2" = 訓読み (kun'yomi) — single kanji or with okurigana
//   "6" = その他 (other)    — can't determine
func inferReadingType(kanjiForm, reading string) string {
	kanjiCount := 0
	for _, r := range kanjiForm {
		if unicode.Is(unicode.Han, r) {
			kanjiCount++
		}
	}
	if kanjiCount == 0 {
		return "6" // no kanji, can't determine
	}
	if kanjiCount >= 2 {
		return "1" // compound → likely on'yomi
	}
	// Single kanji: check if there are okurigana (kana after kanji in the form)
	if len([]rune(kanjiForm)) > kanjiCount {
		return "2" // has okurigana → kun'yomi
	}
	return "6" // single kanji, unsure — default other
}
