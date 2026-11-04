package translation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
)

// StoreInterface defines data access methods required by TranslationService.
type StoreInterface interface {
	SaveSource(s TranslationSource) (int64, error)
	SaveSentence(s TranslationSentence) (int64, error)
	ListSources() ([]TranslationSource, error)
	ListSentencesBySource(sourceID int64) ([]TranslationSentence, error)
	GetSentenceByID(id int64) (*TranslationSentence, error)
	GetDailyQueue(userID int64, limit int) ([]TranslationSentence, error)
	SaveRecord(r TranslationRecord) (int64, error)
	GetRecord(id int64) (*TranslationRecord, error)
	ListRecords(userID int64) ([]TranslationRecord, error)
	UpdateRecordFeedback(recordID int64, score int, feedbackJSON string) error
	UpdateSentenceReference(sentenceID int64, reference string) error
	DeleteSource(id int64) error
}

// TranslationService handles business logic for translation practice.
type TranslationService struct {
	store    StoreInterface
	reviewer TranslationReviewer
}

// NewTranslationService creates a TranslationService.
func NewTranslationService(store StoreInterface, reviewer TranslationReviewer) *TranslationService {
	slog.Debug("TranslationService created")
	return &TranslationService{store: store, reviewer: reviewer}
}

// ImportSource saves a source material, splits it into sentences, detects direction,
// and saves all sentences. Returns the saved source with ID and Direction populated.
func (s *TranslationService) ImportSource(src TranslationSource) (*TranslationSource, error) {
	slog.Debug("TranslationService.ImportSource called", "title", src.Title)

	id, err := s.store.SaveSource(src)
	if err != nil {
		return nil, fmt.Errorf("translation.TranslationService.ImportSource SaveSource: %w", err)
	}

	sentences := SplitSentences(src.RawContent)
	direction := DetectDirection(src.RawContent)

	for i, text := range sentences {
		sent := TranslationSentence{
			SourceID:   id,
			Direction:  string(direction),
			SourceText: text,
			Position:   i,
		}
		if _, err := s.store.SaveSentence(sent); err != nil {
			slog.Error("TranslationService.ImportSource SaveSentence failed", "err", err, "text", text)
			return nil, fmt.Errorf("translation.TranslationService.ImportSource SaveSentence: %w", err)
		}
	}

	src.ID = id
	src.Direction = string(direction)
	slog.Debug("TranslationService.ImportSource done", "source_id", id, "sentences", len(sentences))
	return &src, nil
}

// GetDailyQueue returns today's practice sentences for the user.
func (s *TranslationService) GetDailyQueue(userID int64, count int) ([]TranslationSentence, error) {
	slog.Debug("TranslationService.GetDailyQueue called", "user_id", userID)
	return s.store.GetDailyQueue(userID, count)
}

// GetFreeSentences returns sentences filtered by direction and/or source.
func (s *TranslationService) GetFreeSentences(userID int64, direction string, sourceID int64) ([]TranslationSentence, error) {
	_ = userID // reserved for future filtering
	if sourceID > 0 {
		return s.store.ListSentencesBySource(sourceID)
	}
	return nil, nil
}

// DeleteSource deletes a source and all its sentences (cascaded by DB).
func (s *TranslationService) DeleteSource(id int64) error {
	slog.Debug("TranslationService.DeleteSource called", "source_id", id)
	if err := s.store.DeleteSource(id); err != nil {
		return fmt.Errorf("translation.TranslationService.DeleteSource: %w", err)
	}
	slog.Debug("TranslationService.DeleteSource done", "source_id", id)
	return nil
}

// ListSources returns all translation sources.
func (s *TranslationService) ListSources() ([]TranslationSource, error) {
	return s.store.ListSources()
}

// SubmitTranslation scores a user translation and triggers AI review.
func (s *TranslationService) SubmitTranslation(userID int64, sentenceID int64, userTranslation string) (TranslationRecord, error) {
	slog.Debug("TranslationService.SubmitTranslation called", "user_id", userID, "sentence_id", sentenceID)

	sent, err := s.store.GetSentenceByID(sentenceID)
	if err != nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation GetSentenceByID: %w", err)
	}
	if sent == nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation: sentence %d not found", sentenceID)
	}

	ruleScore := computeRuleScoreForReference(string(Direction(sent.Direction)), sent.SourceText, userTranslation, sent.ReferenceTranslation)
	slog.Debug("TranslationService.SubmitTranslation rule score calculated",
		"direction", sent.Direction,
		"source_len", len([]rune(sent.SourceText)),
		"translation_len", len([]rune(userTranslation)),
		"has_reference", normalizeTranslationForScore(sent.ReferenceTranslation) != "",
		"rule_score", ruleScore,
	)

	rec := TranslationRecord{
		UserID:          userID,
		SentenceID:      sentenceID,
		UserTranslation: userTranslation,
		RuleScore:       ruleScore,
		Score:           ruleScore, // initial score = rule score, updated after AI review
	}

	recID, err := s.store.SaveRecord(rec)
	if err != nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation SaveRecord: %w", err)
	}

	// Trigger AI review if reviewer is available
	if s.reviewer != nil {
		fb, aiErr := s.reviewer.Review(sent.Direction, sent.SourceText, userTranslation, sent.ReferenceTranslation)
		if aiErr != nil {
			slog.Error("TranslationService.SubmitTranslation: AI review failed", "err", aiErr)
			fb = TranslationFeedback{AIScore: ruleScore}
		}
		if len(fb.GrammarExplanations) == 0 {
			fb.GrammarExplanations = detectGrammarExplanations(sent.Direction, sent.SourceText, userTranslation, sent.ReferenceTranslation)
		}
		rec.AIFeedback = &fb
		rec.Score = fb.AIScore
		// Update record with AI feedback or the local fallback.
		if raw, marshalErr := json.Marshal(fb); marshalErr != nil {
			slog.Error("TranslationService.SubmitTranslation: marshal feedback failed", "err", marshalErr, "record_id", recID)
		} else if updateErr := s.store.UpdateRecordFeedback(recID, fb.AIScore, string(raw)); updateErr != nil {
			slog.Error("TranslationService.SubmitTranslation: UpdateRecordFeedback failed", "err", updateErr, "record_id", recID)
		}
		if aiErr == nil {
			// Write back AI-generated reference translation to the sentence
			// so subsequent users see it without needing another AI call.
			if fb.ReferenceTranslation != "" && sent.ReferenceTranslation == "" {
				if err := s.store.UpdateSentenceReference(sentenceID, fb.ReferenceTranslation); err != nil {
					slog.Error("TranslationService.SubmitTranslation: UpdateSentenceReference failed", "err", err)
				}
			}
		}
	}

	rec.ID = recID
	slog.Debug("TranslationService.SubmitTranslation done", "record_id", recID, "rule_score", ruleScore)
	return rec, nil
}

// GetResult returns a single record by ID.
func (s *TranslationService) GetResult(recordID int64) (*TranslationRecord, error) {
	return s.store.GetRecord(recordID)
}

// ListRecords returns all translation records for a user.
func (s *TranslationService) ListRecords(userID int64) ([]TranslationRecord, error) {
	return s.store.ListRecords(userID)
}

// computeRuleScore is kept for callers that do not have a reference translation.
// Without a reference, the result is only a structural estimate and is capped
// below a perfect score so it cannot claim semantic correctness.
func computeRuleScore(direction, source, translation string) int {
	return computeRuleScoreForReference(direction, source, translation, "")
}

// computeRuleScoreForReference scores a translation against the expected
// translation when one is available. This prevents the old length/grammar
// heuristic from returning 100 for almost any non-empty answer.
func computeRuleScoreForReference(direction, source, translation, reference string) int {
	answer := normalizeTranslationForScore(translation)
	if answer == "" {
		return 0
	}

	expected := normalizeTranslationForScore(reference)
	if expected != "" {
		return translationSimilarityScore([]rune(answer), []rune(expected))
	}

	return fallbackRuleScore(direction, source, answer)
}

// translationSimilarityScore returns a normalized Levenshtein similarity in
// the range [0, 100]. Punctuation and whitespace are removed before this
// function is called, so harmless formatting differences do not lower a score.
func translationSimilarityScore(answer, expected []rune) int {
	if len(answer) == 0 || len(expected) == 0 {
		return 0
	}
	if string(answer) == string(expected) {
		return 100
	}

	previous := make([]int, len(expected)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, answerRune := range answer {
		current := make([]int, len(expected)+1)
		current[0] = i + 1
		for j, expectedRune := range expected {
			cost := 0
			if answerRune != expectedRune {
				cost = 1
			}
			current[j+1] = min3(
				current[j]+1,
				previous[j+1]+1,
				previous[j]+cost,
			)
		}
		previous = current
	}

	distance := previous[len(expected)]
	maxLen := max(len(answer), len(expected))
	score := (maxLen - distance) * 100 / maxLen
	return clampScore(score)
}

// fallbackRuleScore estimates answer quality when no reference translation
// has been imported yet. It deliberately remains conservative because source
// and answer alone cannot establish semantic correctness.
func fallbackRuleScore(direction, source, answer string) int {
	score := 20

	srcRunes := []rune(source)
	runes := []rune(answer)

	if len(srcRunes) > 0 {
		ratio := float64(len(runes)) / float64(len(srcRunes))
		if ratio >= 0.5 && ratio <= 2.0 {
			score += 20
		} else if ratio >= 0.25 && ratio <= 4.0 {
			score += 10
		}
	}

	if targetScriptLooksPlausible(direction, runes) {
		score += 30
	}

	if len(runes) >= 2 {
		score += 5
	}
	return min(score, 75)
}

type grammarPattern struct {
	needle       string
	grammarPoint string
	explanation  string
}

// detectGrammarExplanations provides a small local fallback for environments
// where the AI reviewer is disabled or omits the grammar array. It intentionally
// reports only patterns that are visibly present in the Japanese text.
func detectGrammarExplanations(direction, sourceText, userTranslation, referenceTranslation string) []GrammarExplanation {
	japaneseText := sourceText
	if direction == string(DirectionCN2JP) {
		japaneseText = userTranslation
		if !containsJapaneseText(japaneseText) {
			japaneseText = referenceTranslation
		}
	}
	if !containsJapaneseText(japaneseText) {
		return nil
	}

	patterns := []grammarPattern{
		{needle: "ています", grammarPoint: "〜ている", explanation: "表示动作正在进行或状态持续"},
		{needle: "ていました", grammarPoint: "〜ていた", explanation: "表示过去正在进行的动作或持续状态"},
		{needle: "ている", grammarPoint: "〜ている", explanation: "表示动作正在进行或状态持续"},
		{needle: "ていた", grammarPoint: "〜ていた", explanation: "表示过去正在进行的动作或持续状态"},
		{needle: "ながら", grammarPoint: "〜ながら", explanation: "表示同时进行两个动作"},
		{needle: "ように", grammarPoint: "〜ように", explanation: "表示目标、方式或变化结果"},
		{needle: "ために", grammarPoint: "〜ために", explanation: "表示目的或原因"},
		{needle: "ことが", grammarPoint: "〜ことがある", explanation: "将动作或事件名词化后说明经验或可能性"},
		{needle: "のに", grammarPoint: "〜のに", explanation: "表示与预期相反或让步关系"},
		{needle: "ので", grammarPoint: "〜ので", explanation: "表示较为客观、委婉的原因"},
		{needle: "たら", grammarPoint: "〜たら", explanation: "表示条件或某动作完成之后"},
		{needle: "なら", grammarPoint: "〜なら", explanation: "提出条件或限定话题"},
		{needle: "ません", grammarPoint: "〜ません", explanation: "礼貌体的否定表达"},
		{needle: "ない", grammarPoint: "〜ない", explanation: "表示否定"},
		{needle: "たい", grammarPoint: "〜たい", explanation: "表示说话人的愿望"},
		{needle: "から", grammarPoint: "〜から", explanation: "表示原因或理由"},
		{needle: "まで", grammarPoint: "〜まで", explanation: "表示范围的终点"},
		{needle: "より", grammarPoint: "〜より", explanation: "用于比较或表示起点"},
		{needle: "は", grammarPoint: "は", explanation: "提示主题"},
		{needle: "が", grammarPoint: "が", explanation: "提示主语或强调对象"},
		{needle: "を", grammarPoint: "を", explanation: "标记动作的对象"},
		{needle: "に", grammarPoint: "に", explanation: "表示时间、地点或动作对象"},
		{needle: "で", grammarPoint: "で", explanation: "表示动作发生的地点或手段"},
		{needle: "へ", grammarPoint: "へ", explanation: "表示移动的方向"},
		{needle: "と", grammarPoint: "と", explanation: "表示共同动作对象或引用内容"},
		{needle: "も", grammarPoint: "も", explanation: "表示也、同样"},
	}

	result := make([]GrammarExplanation, 0, len(patterns))
	for _, pattern := range patterns {
		if strings.Contains(japaneseText, pattern.needle) {
			result = append(result, GrammarExplanation{
				GrammarPoint: pattern.grammarPoint,
				Explanation:  pattern.explanation,
			})
		}
	}
	return result
}

func containsJapaneseText(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

func normalizeTranslationForScore(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return -1
		}
		return r
	}, value)
}

func targetScriptLooksPlausible(direction string, answer []rune) bool {
	hasKana := false
	hasCJK := false
	for _, r := range answer {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			hasKana = true
		case unicode.In(r, unicode.Han):
			hasCJK = true
		}
	}

	switch direction {
	case "cn2jp":
		return hasKana
	case "jp2cn":
		return hasCJK && !hasKana
	default:
		return false
	}
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func min3(a, b, c int) int {
	return min(min(a, b), c)
}
