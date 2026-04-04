package grammar

import (
	"fmt"
	"log/slog"
	"time"
)

// GrammarStoreInterface defines data access methods required by GrammarService.
type GrammarStoreInterface interface {
	GetByID(id int64) (*GrammarPoint, error)
	ListByLevel(level JLPTLevel) ([]GrammarPoint, error)
	GetRecord(userID, grammarPointID int64) (*GrammarRecord, error)
	UpsertRecord(r GrammarRecord) error
	ListDueRecords(userID int64) ([]GrammarRecord, error)
}

// GrammarService handles business logic for grammar learning.
type GrammarService struct {
	store GrammarStoreInterface
}

// NewGrammarService creates a GrammarService instance.
func NewGrammarService(store GrammarStoreInterface) *GrammarService {
	return &GrammarService{store: store}
}

// GetPoint returns a grammar point by ID with quiz answers stripped.
func (s *GrammarService) GetPoint(id int64) (*GrammarPoint, error) {
	slog.Debug("GrammarService.GetPoint called", "grammar_point_id", id)

	p, err := s.store.GetByID(id)
	if err != nil {
		slog.Error("GrammarService.GetPoint: failed", "err", err, "grammar_point_id", id)
		return nil, fmt.Errorf("grammar.GrammarService.GetPoint: %w", err)
	}

	// Strip answers before returning to client
	sanitized := *p
	questions := make([]QuizQuestion, len(p.QuizQuestions))
	for i, q := range p.QuizQuestions {
		q.Answer = ""
		questions[i] = q
	}
	sanitized.QuizQuestions = questions

	slog.Debug("GrammarService.GetPoint done", "grammar_point_id", id)
	return &sanitized, nil
}

// ListByLevel returns all grammar points at the given JLPT level (answers stripped).
func (s *GrammarService) ListByLevel(level JLPTLevel) ([]GrammarPoint, error) {
	slog.Debug("GrammarService.ListByLevel called", "level", level)

	points, err := s.store.ListByLevel(level)
	if err != nil {
		slog.Error("GrammarService.ListByLevel: failed", "err", err, "level", level)
		return nil, fmt.Errorf("grammar.GrammarService.ListByLevel: %w", err)
	}

	slog.Debug("GrammarService.ListByLevel done", "level", level, "count", len(points))
	return points, nil
}

// ScoreQuiz grades the user's quiz submissions for a grammar point,
// persists a QuizAttempt record, and returns the detailed result.
func (s *GrammarService) ScoreQuiz(userID, grammarPointID int64, submissions []QuizSubmission) (*QuizResult, error) {
	slog.Debug("GrammarService.ScoreQuiz called", "user_id", userID, "grammar_point_id", grammarPointID)

	p, err := s.store.GetByID(grammarPointID)
	if err != nil {
		slog.Error("GrammarService.ScoreQuiz: grammar point not found", "err", err, "grammar_point_id", grammarPointID)
		return nil, fmt.Errorf("grammar.GrammarService.ScoreQuiz GetByID: %w", err)
	}

	// Build answer map
	answerMap := make(map[int64]QuizQuestion, len(p.QuizQuestions))
	for _, q := range p.QuizQuestions {
		answerMap[q.ID] = q
	}

	var items []QuizItemResult
	correctCount := 0
	for _, sub := range submissions {
		q, ok := answerMap[sub.QuestionID]
		if !ok {
			continue
		}
		correct := sub.Answer == q.Answer
		if correct {
			correctCount++
		}
		item := QuizItemResult{
			QuestionID: sub.QuestionID,
			Correct:    correct,
			UserAnswer: sub.Answer,
			Expected:   q.Answer,
		}
		if !correct {
			item.Explanation = q.Explanation
		}
		items = append(items, item)
	}

	score := 0
	if len(submissions) > 0 {
		score = correctCount * 100 / len(submissions)
	}

	result := &QuizResult{Score: score, Results: items}

	// Persist quiz attempt
	rec, err := s.store.GetRecord(userID, grammarPointID)
	if err != nil {
		slog.Error("GrammarService.ScoreQuiz: GetRecord failed", "err", err)
		return nil, fmt.Errorf("grammar.GrammarService.ScoreQuiz GetRecord: %w", err)
	}

	var base GrammarRecord
	if rec != nil {
		base = *rec
	} else {
		base = GrammarRecord{UserID: userID, GrammarPointID: grammarPointID, Status: StatusLearning}
	}

	base.QuizHistory = append(base.QuizHistory, QuizAttempt{
		Score:       score,
		AttemptedAt: time.Now(),
	})
	if score >= 70 {
		base.Status = StatusMastered
	}
	base.NextReviewAt = time.Now().Add(24 * time.Hour)

	if err := s.store.UpsertRecord(base); err != nil {
		slog.Error("GrammarService.ScoreQuiz: UpsertRecord failed", "err", err)
		return nil, fmt.Errorf("grammar.GrammarService.ScoreQuiz UpsertRecord: %w", err)
	}

	slog.Debug("GrammarService.ScoreQuiz done", "user_id", userID, "grammar_point_id", grammarPointID, "score", score)
	return result, nil
}
