package translation

import (
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	grammarSourceBank  = "bank"
	grammarSourceLocal = "local"
)

func grammarText(direction, source, reference string) string {
	if direction == string(DirectionCN2JP) {
		return reference
	}
	return source
}

func referenceGrammar(sentence TranslationSentence, reference string) (string, []GrammarExplanation) {
	japanese := grammarText(sentence.Direction, sentence.SourceText, reference)
	var bankItems []GrammarExplanation
	var rejected []string
	seen := make(map[string]bool)
	for _, raw := range sentence.GrammarPoints {
		point := strings.TrimSpace(raw)
		if point == "" || seen[point] {
			continue
		}
		evidence := bankGrammarEvidence(japanese, point)
		if evidence == "" {
			rejected = append(rejected, point)
			continue
		}
		seen[point] = true
		bankItems = append(bankItems, GrammarExplanation{GrammarPoint: point, Evidence: evidence})
	}
	if len(rejected) > 0 {
		slog.Debug("translation bank grammar points rejected without evidence",
			"rejected", rejected, "japanese", japanese)
	}
	if len(bankItems) > 0 {
		return grammarSourceBank, bankItems
	}
	if localItems := detectGrammarExplanations(japanese); len(localItems) > 0 {
		return grammarSourceLocal, localItems
	}
	return "", nil
}

// bankGrammarEvidence returns the literal Japanese substring that proves a
// question-bank annotation occurs in this sentence. Category labels such as
// 動詞 / 可能形 and one-character particles have no literal evidence, so they
// are rejected and local clause detection can take over.
func bankGrammarEvidence(japanese, point string) string {
	evidence := strings.TrimLeft(strings.TrimSpace(point), "〜～~")
	if utf8.RuneCountInString(evidence) < 2 || !strings.Contains(japanese, evidence) {
		return ""
	}
	return evidence
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
