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
	ListUnpracticedSentences(userID, sourceID int64, limit int) ([]TranslationSentence, error)
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
	return s.store.ListUnpracticedSentences(userID, 0, count)
}

// GetFreeSentences returns a source's sentences for practice or review.
func (s *TranslationService) GetFreeSentences(userID int64, direction string, sourceID int64, unpracticed bool) ([]TranslationSentence, error) {
	if sourceID > 0 {
		if unpracticed {
			return s.store.ListUnpracticedSentences(userID, sourceID, 0)
		}
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
		UserID:               userID,
		SentenceID:           sentenceID,
		UserTranslation:      userTranslation,
		ReferenceTranslation: strings.TrimSpace(sent.ReferenceTranslation),
		RuleScore:            ruleScore,
		Score:                ruleScore, // initial score = rule score, updated after AI review
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
		generated := strings.TrimSpace(fb.ReferenceTranslation)
		if rec.ReferenceTranslation == "" {
			rec.ReferenceTranslation = generated
		}
		applyGrammarFeedback(&fb, *sent, rec.ReferenceTranslation)
		slog.Debug("translation grammar selected", "sentence_id", sentenceID, "source", fb.GrammarSource, "count", len(fb.GrammarExplanations))
		rec.AIFeedback = &fb
		rec.Score = fb.AIScore
		// Update record with AI feedback or the local fallback.
		if raw, marshalErr := json.Marshal(fb); marshalErr != nil {
			slog.Error("TranslationService.SubmitTranslation: marshal feedback failed", "err", marshalErr, "record_id", recID)
		} else if updateErr := s.store.UpdateRecordFeedback(recID, fb.AIScore, string(raw)); updateErr != nil {
			slog.Error("TranslationService.SubmitTranslation: UpdateRecordFeedback failed", "err", updateErr, "record_id", recID)
		}
		if generated != "" && strings.TrimSpace(sent.ReferenceTranslation) == "" {
			if err := s.store.UpdateSentenceReference(sentenceID, generated); err != nil {
				slog.Error("TranslationService.SubmitTranslation: UpdateSentenceReference failed", "err", err, "sentence_id", sentenceID)
			}
		}
	}

	rec.ID = recID
	slog.Debug("TranslationService.SubmitTranslation done", "record_id", recID, "rule_score", ruleScore)
	return rec, nil
}

// GetResult returns a single record by ID.
func (s *TranslationService) GetResult(recordID int64) (*TranslationRecord, error) {
	rec, err := s.store.GetRecord(recordID)
	if err != nil {
		return nil, fmt.Errorf("translation.TranslationService.GetResult: %w", err)
	}
	if rec != nil {
		resolveRecordReference(rec)
	}
	return rec, nil
}

// ListRecords returns all translation records for a user.
func (s *TranslationService) ListRecords(userID int64) ([]TranslationRecord, error) {
	records, err := s.store.ListRecords(userID)
	if err != nil {
		return nil, fmt.Errorf("translation.TranslationService.ListRecords: %w", err)
	}
	for i := range records {
		resolveRecordReference(&records[i])
	}
	return records, nil
}

func resolveRecordReference(rec *TranslationRecord) {
	rec.ReferenceTranslation = strings.TrimSpace(rec.ReferenceTranslation)
	if rec.ReferenceTranslation == "" && rec.AIFeedback != nil {
		rec.ReferenceTranslation = strings.TrimSpace(rec.AIFeedback.ReferenceTranslation)
	}
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
