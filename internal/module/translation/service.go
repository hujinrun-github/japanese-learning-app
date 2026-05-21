package translation

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

	ruleScore := computeRuleScore(string(Direction(sent.Direction)), sent.SourceText, userTranslation)

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
		} else {
			rec.AIFeedback = &fb
			rec.Score = fb.AIScore
			// Update record with AI feedback
			if raw, err := json.Marshal(fb); err == nil {
				s.store.UpdateRecordFeedback(recID, fb.AIScore, string(raw))
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

// computeRuleScore is the local rule-based scoring algorithm.
func computeRuleScore(direction, source, translation string) int {
	score := 50

	runes := []rune(translation)
	srcRunes := []rune(source)

	// Length ratio check
	if len(srcRunes) > 0 {
		ratio := float64(len(runes)) / float64(len(srcRunes))
		if ratio >= 0.5 && ratio <= 2.0 {
			score += 25
		}
	}

	// Grammar marker check for JP→CN
	if direction == "jp2cn" {
		markers := []string{"は", "が", "を", "に", "で", "へ", "と", "も", "か", "から", "まで", "より", "ので", "のに"}
		present := 0
		for _, m := range markers {
			if contains(source, m) {
				present++
			}
		}
		if present > 0 {
			score += min(present*2, 25)
		}
	}

	// Keyword check for CN→JP
	if direction == "cn2jp" {
		if len(runes) >= 2 {
			score += 25
		}
	}

	if score > 100 {
		score = 100
	}
	return score
}

func contains(s, substr string) bool {
	runes := []rune(s)
	subRunes := []rune(substr)
	for i := 0; i <= len(runes)-len(subRunes); i++ {
		match := true
		for j, r := range subRunes {
			if runes[i+j] != r {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
