package translation

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

type AssessmentResult string

const (
	AssessmentReferenceMatch AssessmentResult = "reference_match"
	AssessmentEquivalent     AssessmentResult = "equivalent"
	AssessmentMeaningChanged AssessmentResult = "meaning_changed"
	AssessmentUnrecognized   AssessmentResult = "unrecognized"
	AssessmentNoReference    AssessmentResult = "no_reference"
)

// The rules replace known equivalent phrases in the target language. All
// remaining text must still match, including negation, quantities and tense.
func assessTranslation(direction, reference, answer string) AssessmentResult {
	expected := normalizeTranslation(reference)
	if expected == "" {
		return AssessmentNoReference
	}
	submitted := normalizeTranslation(answer)
	if submitted == expected {
		return AssessmentReferenceMatch
	}

	expectedMeaning := canonicalTranslation(direction, expected)
	submittedMeaning := canonicalTranslation(direction, submitted)
	if submittedMeaning == expectedMeaning {
		return AssessmentEquivalent
	}
	if oppositePolarity(direction, expectedMeaning, submittedMeaning) {
		return AssessmentMeaningChanged
	}
	return AssessmentUnrecognized
}

func canonicalTranslation(direction, value string) string {
	switch Direction(direction) {
	case DirectionCN2JP:
		return japaneseEquivalents.Replace(value)
	case DirectionJP2CN:
		value = chineseEquivalents.Replace(value)
		for _, suffix := range []string{"票价是多少钱", "票价是多少", "票价多少錢", "票价多少钱"} {
			if prefix, ok := strings.CutSuffix(value, suffix); ok {
				return prefix + "票价多少"
			}
		}
	}
	return value
}

// Require the rest of the sentence to agree before flagging a known pair of
// opposing endings. Unmatched rewrites stay available for manual comparison.
func oppositePolarity(direction, reference, answer string) bool {
	var endings [][2]string
	switch Direction(direction) {
	case DirectionCN2JP:
		endings = [][2]string{{"ません", "ます"}, {"ませんでした", "ました"}, {"ではありません", "です"}}
	case DirectionJP2CN:
		endings = [][2]string{{"不去", "要去"}, {"不可以", "可以"}, {"不同意", "同意"}}
	}
	for _, pair := range endings {
		for i, ending := range pair {
			if prefix, ok := strings.CutSuffix(reference, ending); ok && answer == prefix+pair[1-i] {
				return true
			}
		}
	}
	return false
}

// Prefer phrases that keep the referent and role explicit. Avoid standalone
// substitutions for ambiguous words and for particles, tense or negation.
var japaneseEquivalents = strings.NewReplacer(
	"バスの乗車料金", "バスの運賃",
	"バスの料金", "バスの運賃",
	"バス運賃", "バスの運賃",
	"バス賃", "バスの運賃",
	"バス代", "バスの運賃",
	"いくらでしょうか", "いくらですか",
	"何時から始まりますか", "何時に始まりますか",
	"冷蔵庫の中に", "冷蔵庫に",
)

var chineseEquivalents = strings.NewReplacer(
	"公共汽车的票价", "公交车票价",
	"公交车的票价", "公交车票价",
	"公共汽车票价", "公交车票价",
	"公车票价", "公交车票价",
	"巴士票价", "公交车票价",
	"要是没有下雨", "如果没下雨",
	"如果没有下雨", "如果没下雨",
	"要是没下雨", "如果没下雨",
	"什么时候", "何时",
	"自行车", "脚踏车",
)

func normalizeTranslation(value string) string {
	value = norm.NFC.String(width.Fold.String(value))
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	var normalized strings.Builder
	for i, r := range runes {
		if r == ' ' && i > 0 && i+1 < len(runes) && isCJK(runes[i-1]) && isCJK(runes[i+1]) {
			continue
		}
		normalized.WriteRune(r)
	}
	value = strings.TrimRight(normalized.String(), "。.!！")
	// A question mark can change the meaning of an unmarked sentence. Only
	// relax it when the end already explicitly marks the sentence as a question.
	stem := strings.TrimSuffix(value, "?")
	for _, ending := range []string{"ですか", "ますか", "でしょうか", "吗", "嗎", "票价多少", "票价是多少", "多少钱", "多少錢"} {
		if strings.HasSuffix(stem, ending) {
			return stem
		}
	}
	return value
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}
