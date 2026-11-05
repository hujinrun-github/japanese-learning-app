package writing

import (
	"fmt"
	"log/slog"
	"time"
)

// WritingStoreInterface defines data access methods required by WritingService.
type WritingStoreInterface interface {
	GetDailyQueue(userID int64) ([]WritingQuestion, error)
	GetQuestionByID(id int64) (*WritingQuestion, error)
	SaveRecord(r WritingRecord) error
	ListRecords(userID int64) ([]WritingRecord, error)
}

// WritingService handles business logic for writing practice.
type WritingService struct {
	store    WritingStoreInterface
	reviewer AIReviewer
}

// NewWritingService creates a WritingService instance.
func NewWritingService(store WritingStoreInterface, reviewer AIReviewer) *WritingService {
	return &WritingService{store: store, reviewer: reviewer}
}

// GetDailyQueue returns today's writing questions for the user.
// ExpectedAnswer is stripped before returning so it is never sent to the client.
func (s *WritingService) GetDailyQueue(userID int64) ([]WritingQuestion, error) {
	slog.Debug("WritingService.GetDailyQueue called", "user_id", userID)

	questions, err := s.store.GetDailyQueue(userID)
	if err != nil {
		slog.Error("WritingService.GetDailyQueue: store failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("writing.WritingService.GetDailyQueue: %w", err)
	}

	// Strip expected answers before returning to client.
	stripped := make([]WritingQuestion, len(questions))
	for i, q := range questions {
		q.ExpectedAnswer = ""
		stripped[i] = q
	}

	slog.Debug("WritingService.GetDailyQueue done", "user_id", userID, "count", len(stripped))
	return stripped, nil
}

// SubmitInput grades a keyboard-input practice answer (exact-match scoring)
// and saves the record.
func (s *WritingService) SubmitInput(userID int64, questionID int64, question, userAnswer string) (WritingRecord, error) {
	slog.Debug("WritingService.SubmitInput called", "user_id", userID, "question_id", questionID)

	q, err := s.store.GetQuestionByID(questionID)
	if err != nil {
		slog.Error("WritingService.SubmitInput: GetQuestionByID failed", "err", err, "question_id", questionID)
		return WritingRecord{}, fmt.Errorf("writing.WritingService.SubmitInput GetQuestionByID: %w", err)
	}

	score := 0
	if userAnswer == q.ExpectedAnswer {
		score = 100
	}

	var fb *AIFeedback
	if score < 100 {
		fb = &AIFeedback{
			ReferenceAnswer: q.ExpectedAnswer,
		}
	}

	rec := WritingRecord{
		UserID:      userID,
		Type:        WritingTypeInput,
		Question:    question,
		UserAnswer:  userAnswer,
		AIFeedback:  fb,
		Score:       score,
		PracticedAt: time.Now(),
	}

	if err := s.store.SaveRecord(rec); err != nil {
		slog.Error("WritingService.SubmitInput: SaveRecord failed", "err", err, "user_id", userID)
		return WritingRecord{}, fmt.Errorf("writing.WritingService.SubmitInput SaveRecord: %w", err)
	}

	slog.Debug("WritingService.SubmitInput done", "user_id", userID, "score", score)
	return rec, nil
}

// SubmitSentence sends the user's sentence to the AI reviewer, saves the record,
// and returns it with AI feedback attached.
func (s *WritingService) SubmitSentence(userID int64, question, userAnswer string) (WritingRecord, error) {
	reviewer := s.reviewer
	if reviewer == nil {
		// The legacy sentence endpoint has always returned a feedback object.
		reviewer = &StubReviewer{}
	}
	return s.saveOpenWriting(userID, WritingTypeSentence, question, userAnswer, question, reviewer)
}

func (s *WritingService) submitTopic(userID int64, topic WritingTopic, mode WritingType, userAnswer string) (WritingRecord, error) {
	prompt := topic.SentencePrompt
	criteria := "Evaluate one original Japanese sentence for use of the requested grammar, fit to the situation, and natural wording."
	if mode == WritingTypeEssay {
		prompt = topic.EssayPrompt
		criteria = "Evaluate the entire short essay for completion of the requested points, coherence between sentences, grammar, and natural wording. Correct the full essay while preserving the student's meaning and paragraph breaks."
	}
	question := topic.Title + "\n" + prompt
	reviewTask := fmt.Sprintf("Level: %s\nTask: %s\nReview criteria: %s", topic.JLPTLevel, question, criteria)
	return s.saveOpenWriting(userID, mode, question, userAnswer, reviewTask, s.reviewer)
}

func (s *WritingService) saveOpenWriting(userID int64, mode WritingType, question, userAnswer, reviewTask string, reviewer AIReviewer) (WritingRecord, error) {
	slog.Debug("WritingService.saveOpenWriting called", "user_id", userID, "type", mode)
	rec := WritingRecord{
		UserID:      userID,
		Type:        mode,
		Question:    question,
		UserAnswer:  userAnswer,
		PracticedAt: time.Now(),
	}
	if reviewer != nil {
		feedback, err := reviewer.Review(reviewTask, userAnswer)
		if err != nil {
			slog.Error("WritingService.saveOpenWriting: reviewer failed", "err", err, "user_id", userID, "type", mode)
			return WritingRecord{}, fmt.Errorf("writing.WritingService.saveOpenWriting Review: %w", err)
		}
		rec.AIFeedback = &feedback
		rec.Score = feedback.Score
	}

	if err := s.store.SaveRecord(rec); err != nil {
		slog.Error("WritingService.saveOpenWriting: SaveRecord failed", "err", err, "user_id", userID, "type", mode)
		return WritingRecord{}, fmt.Errorf("writing.WritingService.saveOpenWriting SaveRecord: %w", err)
	}

	slog.Debug("WritingService.saveOpenWriting done", "user_id", userID, "type", mode, "reviewed", rec.AIFeedback != nil, "score", rec.Score)
	return rec, nil
}

// ListRecords returns all writing records for the user.
func (s *WritingService) ListRecords(userID int64) ([]WritingRecord, error) {
	slog.Debug("WritingService.ListRecords called", "user_id", userID)

	records, err := s.store.ListRecords(userID)
	if err != nil {
		slog.Error("WritingService.ListRecords: failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("writing.WritingService.ListRecords: %w", err)
	}

	slog.Debug("WritingService.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}
