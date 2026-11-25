package word

import "github.com/ikawaha/kagome/v2/tokenizer"

type FuriganaToken struct {
	Surface string `json:"surface"`
	Reading string `json:"reading"`
}

// FuriganaTokens returns the original text in parts, with readings for known kanji.
func FuriganaTokens(text string) []FuriganaToken {
	if sharedTokenizer == nil || !containsKanji(text) {
		return nil
	}
	tokens := sharedTokenizer.Tokenize(text)
	parts := make([]FuriganaToken, 0, len(tokens))
	annotated := false
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		part := FuriganaToken{Surface: token.Surface}
		if end := counterEnd(tokens, i); end > i {
			counter := tokens[end]
			part.Surface = text[token.Position : counter.Position+len(counter.Surface)]
			// Separate dictionary entries can give the wrong reading for a
			// numeral plus a counter, including dates and irregular counters.
			switch part.Surface {
			case "一人", "1人", "１人":
				part.Reading = "ひとり"
			case "二人", "2人", "２人":
				part.Reading = "ふたり"
			}
			i = end
		} else if reading, ok := token.Reading(); ok && reading != "*" && containsKanji(token.Surface) {
			part.Reading = katakanaToHiraganaStr(reading)
		}
		annotated = annotated || part.Reading != ""
		parts = append(parts, part)
	}
	if !annotated {
		return nil
	}
	return parts
}

func counterEnd(tokens []tokenizer.Token, start int) int {
	i := start
	for ; i < len(tokens); i++ {
		pos := tokens[i].POS()
		if len(pos) < 2 || pos[1] != "数" {
			if i > start && len(pos) > 2 && pos[2] == "助数詞" {
				return i
			}
			break
		}
	}
	return start
}
