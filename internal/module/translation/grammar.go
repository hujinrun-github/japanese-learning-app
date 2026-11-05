package translation

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	grammarSourceBank  = "bank"
	grammarSourceAI    = "ai"
	grammarSourceLocal = "local"
)

func grammarText(direction, source, reference string) string {
	if direction == string(DirectionCN2JP) {
		return reference
	}
	return source
}

func applyGrammarFeedback(feedback *TranslationFeedback, sentence TranslationSentence, reference string) {
	var bankItems []GrammarExplanation
	seen := make(map[string]bool)
	for _, raw := range sentence.GrammarPoints {
		point := strings.TrimSpace(raw)
		if point != "" && !seen[point] {
			seen[point] = true
			bankItems = append(bankItems, GrammarExplanation{GrammarPoint: point})
		}
	}
	if len(bankItems) > 0 {
		feedback.GrammarExplanations = bankItems
		feedback.GrammarSource = grammarSourceBank
		return
	}
	if feedback.GrammarSource == grammarSourceLocal {
		return
	}

	japanese := grammarText(sentence.Direction, sentence.SourceText, reference)
	var verified []GrammarExplanation
	for _, item := range feedback.GrammarExplanations {
		point := strings.TrimSpace(item.GrammarPoint)
		evidence := strings.TrimSpace(item.Evidence)
		explanation := strings.TrimSpace(item.Explanation)
		if point == "" || explanation == "" || seen[point] ||
			utf8.RuneCountInString(evidence) < 2 || !strings.Contains(japanese, evidence) {
			continue
		}
		seen[point] = true
		verified = append(verified, GrammarExplanation{GrammarPoint: point, Evidence: evidence, Explanation: explanation})
	}
	if len(verified) > 0 {
		feedback.GrammarExplanations = verified
		feedback.GrammarSource = grammarSourceAI
		return
	}
	feedback.GrammarExplanations = detectGrammarExplanations(japanese)
	feedback.GrammarSource = ""
	if len(feedback.GrammarExplanations) > 0 {
		feedback.GrammarSource = grammarSourceLocal
	}
}

// Local recognition is intentionally limited to distinctive clause endings.
// Standalone particles and ambiguous strings need grammatical context.
func detectGrammarExplanations(japanese string) []GrammarExplanation {
	patterns := []struct {
		name, explanation string
		endings           []string
	}{
		{"〜ている", "表示动作正在进行或状态持续", []string{"ています", "ている", "んでいます", "んでいる", "いでいます", "いでいる"}},
		{"〜ていた", "表示过去正在进行的动作或持续状态", []string{"ていました", "ていた", "んでいました", "んでいた", "いでいました", "いでいた"}},
		{"〜ことができる", "表示有能力或条件做某事", []string{"ことができます", "ことができる"}},
		{"〜たことがある", "表示曾经有过某种经历", []string{"たことがあります", "たことがある", "だことがあります", "だことがある"}},
	}
	clauses := strings.FieldsFunc(japanese, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	var result []GrammarExplanation
	for _, pattern := range patterns {
		for _, clause := range clauses {
			if !hasGrammarEnding(clause, pattern.endings) {
				continue
			}
			result = append(result, GrammarExplanation{
				GrammarPoint: pattern.name, Explanation: pattern.explanation, Evidence: clause,
			})
			break
		}
	}
	return result
}

func hasGrammarEnding(clause string, endings []string) bool {
	for _, ending := range endings {
		if strings.HasSuffix(clause, ending) {
			return true
		}
	}
	return false
}
