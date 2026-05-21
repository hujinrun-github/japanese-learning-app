package translation

import (
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
	reviewer interface{} // TranslationReviewer (defined in ai_client.go)
}

// NewTranslationService creates a TranslationService.
func NewTranslationService(store StoreInterface, reviewer interface{}) *TranslationService {
	slog.Debug("TranslationService created")
	return &TranslationService{store: store, reviewer: reviewer}
}
