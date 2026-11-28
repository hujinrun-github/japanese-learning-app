package translation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"japanese-learning-app/internal/module/word"
)

type StudyPlanTarget func(planJSON string, now time.Time) (goal int, location *time.Location, err error)

// StoreInterface defines data access methods required by TranslationService.
type StoreInterface interface {
	SaveSource(s TranslationSource) (int64, error)
	SaveSentence(s TranslationSentence) (int64, error)
	ListSources() ([]TranslationSource, error)
	ListSentencesBySource(sourceID int64) ([]TranslationSentence, error)
	GetSentenceByID(id int64) (*TranslationSentence, error)
	GetDailyProgress(ctx context.Context, userID int64, resolveTarget StudyPlanTarget) (DailyProgress, error)
	ListUnpracticedSentences(ctx context.Context, userID, sourceID int64, limit int, levels []string) ([]TranslationSentence, error)
	SaveRecord(r TranslationRecord) (int64, error)
	GetRecord(id int64) (*TranslationRecord, error)
	ListRecords(userID int64) ([]TranslationRecord, error)
	ListSentenceLinks(ctx context.Context, sentenceID int64) ([]SentenceLink, error)
	DeleteSource(id int64) error
}

// TranslationService handles business logic for translation practice.
type TranslationService struct {
	store         StoreInterface
	resolveTarget StudyPlanTarget
}

// NewTranslationService creates a TranslationService.
func NewTranslationService(store StoreInterface, resolveTarget StudyPlanTarget) *TranslationService {
	slog.Debug("TranslationService created")
	return &TranslationService{store: store, resolveTarget: resolveTarget}
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

func (s *TranslationService) GetDailyProgress(ctx context.Context, userID int64) (DailyProgress, error) {
	return s.store.GetDailyProgress(ctx, userID, s.resolveTarget)
}

// GetDailyQueue uses the remaining daily target when no count is requested.
func (s *TranslationService) GetDailyQueue(ctx context.Context, userID int64, count int) ([]TranslationSentence, error) {
	progress, err := s.GetDailyProgress(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve translation queue scope: %w", err)
	}
	if count == 0 {
		count = progress.Goal - progress.Completed
		if count <= 0 {
			return []TranslationSentence{}, nil
		}
	}
	slog.DebugContext(ctx, "loading translation queue", "user_id", userID, "count", count, "levels", progress.Levels)
	return s.store.ListUnpracticedSentences(ctx, userID, 0, count, progress.Levels)
}

// GetFreeSentences returns a source's sentences for practice or review.
func (s *TranslationService) GetFreeSentences(ctx context.Context, userID int64, direction string, sourceID int64, unpracticed bool) ([]TranslationSentence, error) {
	if sourceID > 0 {
		if unpracticed {
			return s.store.ListUnpracticedSentences(ctx, userID, sourceID, 0, nil)
		}
		return s.store.ListSentencesBySource(sourceID)
	}
	return nil, nil
}

// GetPracticeSentence loads the original question for a mistake-book retry.
func (s *TranslationService) GetPracticeSentence(id int64) (*TranslationSentence, error) {
	sentence, err := s.store.GetSentenceByID(id)
	if err != nil {
		return nil, fmt.Errorf("load translation practice sentence: %w", err)
	}
	return sentence, nil
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

// SubmitTranslation checks the answer locally and saves the result with the attempt.
func (s *TranslationService) SubmitTranslation(ctx context.Context, userID int64, sentenceID int64, userTranslation string) (TranslationRecord, error) {
	slog.Debug("TranslationService.SubmitTranslation called", "user_id", userID, "sentence_id", sentenceID)

	sent, err := s.store.GetSentenceByID(sentenceID)
	if err != nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation GetSentenceByID: %w", err)
	}
	if sent == nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation: sentence %d not found", sentenceID)
	}

	reference := strings.TrimSpace(sent.ReferenceTranslation)
	assessment := TranslationAssessment{Result: assessTranslation(sent.Direction, reference, userTranslation)}
	assessment.GrammarSource, assessment.GrammarExplanations = referenceGrammar(*sent, reference)
	rec := TranslationRecord{
		UserID:               userID,
		SentenceID:           sentenceID,
		Direction:            sent.Direction,
		UserTranslation:      userTranslation,
		ReferenceTranslation: reference,
		Assessment:           &assessment,
	}
	recID, err := s.store.SaveRecord(rec)
	if err != nil {
		return TranslationRecord{}, fmt.Errorf("translation.TranslationService.SubmitTranslation SaveRecord: %w", err)
	}

	rec.ID = recID
	annotateReference(&rec)
	links, err := s.store.ListSentenceLinks(ctx, sentenceID)
	if err != nil {
		slog.ErrorContext(ctx, "load knowledge links for saved translation", "record_id", recID, "sentence_id", sentenceID, "err", err)
	} else {
		rec.KnowledgeLinks = links
	}
	slog.Debug("TranslationService.SubmitTranslation done", "record_id", recID, "assessment", assessment.Result)
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
		annotateReference(rec)
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

func annotateReference(rec *TranslationRecord) {
	if rec.Direction == string(DirectionCN2JP) {
		rec.ReferenceTokens = word.FuriganaTokens(rec.ReferenceTranslation)
	}
}
