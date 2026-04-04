package service

import (
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/grammar"
)

// GrammarStoreInterface defines the data access methods required by GrammarService.
type GrammarStoreInterface interface {
	GetByID(id int64) (*grammar.GrammarPoint, error)
	ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error)
	GetRecord(userID, grammarPointID int64) (*grammar.GrammarRecord, error)
	UpsertRecord(r grammar.GrammarRecord) error
	ListDueRecords(userID int64) ([]grammar.GrammarRecord, error)
}

// GrammarService handles business logic for grammar learning and quiz grading.
type GrammarService struct {
	store GrammarStoreInterface
}

// NewGrammarService creates a GrammarService instance.
func NewGrammarService(store GrammarStoreInterface) *GrammarService {
	return &GrammarService{store: store}
}

// ListByLevel returns all grammar points for the given JLPT level.
func (s *GrammarService) ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error) {
	slog.Debug("GrammarService.ListByLevel called", "level", level)

	points, err := s.store.ListByLevel(level)
	if err != nil {
		slog.Error("GrammarService.ListByLevel: failed", "err", err)
		return nil, fmt.Errorf("service.GrammarService.ListByLevel: %w", err)
	}

	slog.Debug("GrammarService.ListByLevel done", "level", level, "count", len(points))
	return points, nil
}

// GetByID returns a single grammar point by ID.
func (s *GrammarService) GetByID(id int64) (*grammar.GrammarPoint, error) {
	slog.Debug("GrammarService.GetByID called", "grammar_point_id", id)

	gp, err := s.store.GetByID(id)
	if err != nil {
		slog.Error("GrammarService.GetByID: failed", "err", err, "grammar_point_id", id)
		return nil, fmt.Errorf("service.GrammarService.GetByID: %w", err)
	}

	slog.Debug("GrammarService.GetByID done", "grammar_point_id", id, "name", gp.Name)
	return gp, nil
}

// SubmitQuiz grades the user's quiz answers for a grammar point and updates the learning record.
func (s *GrammarService) SubmitQuiz(userID, grammarPointID int64, submissions []grammar.QuizSubmission) (*grammar.QuizResult, error) {
	slog.Debug("GrammarService.SubmitQuiz called", "user_id", userID, "grammar_point_id", grammarPointID)

	gp, err := s.store.GetByID(grammarPointID)
	if err != nil {
		slog.Error("GrammarService.SubmitQuiz: failed to get grammar point", "err", err)
		return nil, fmt.Errorf("service.GrammarService.SubmitQuiz get grammar point: %w", err)
	}

	// Build answer map for O(1) lookup
	answerMap := make(map[int64]grammar.QuizQuestion, len(gp.QuizQuestions))
	for _, q := range gp.QuizQuestions {
		answerMap[q.ID] = q
	}

	result := &grammar.QuizResult{
		Results: make([]grammar.QuizItemResult, 0, len(submissions)),
	}

	correct := 0
	for _, sub := range submissions {
		q, ok := answerMap[sub.QuestionID]
		if !ok {
			slog.Error("GrammarService.SubmitQuiz: unknown question id", "question_id", sub.QuestionID)
			return nil, fmt.Errorf("service.GrammarService.SubmitQuiz: unknown question id %d", sub.QuestionID)
		}

		isCorrect := sub.Answer == q.Answer
		if isCorrect {
			correct++
		}

		item := grammar.QuizItemResult{
			QuestionID: sub.QuestionID,
			Correct:    isCorrect,
			UserAnswer: sub.Answer,
			Expected:   q.Answer,
		}
		if !isCorrect {
			item.Explanation = q.Explanation
		}
		result.Results = append(result.Results, item)
	}

	if len(submissions) > 0 {
		result.Score = correct * 100 / len(submissions)
	}

	// Update grammar record
	if err := s.updateGrammarRecord(userID, grammarPointID, result.Score); err != nil {
		return nil, fmt.Errorf("service.GrammarService.SubmitQuiz update record: %w", err)
	}

	slog.Debug("GrammarService.SubmitQuiz done", "user_id", userID, "grammar_point_id", grammarPointID, "score", result.Score)
	return result, nil
}

// GetDueQueue returns grammar points with due review records for the user.
func (s *GrammarService) GetDueQueue(userID int64) ([]grammar.GrammarPoint, error) {
	slog.Debug("GrammarService.GetDueQueue called", "user_id", userID)

	records, err := s.store.ListDueRecords(userID)
	if err != nil {
		slog.Error("GrammarService.GetDueQueue: failed to list due records", "err", err)
		return nil, fmt.Errorf("service.GrammarService.GetDueQueue: %w", err)
	}

	points := make([]grammar.GrammarPoint, 0, len(records))
	for _, r := range records {
		gp, err := s.store.GetByID(r.GrammarPointID)
		if err != nil {
			slog.Error("GrammarService.GetDueQueue: failed to get grammar point", "err", err, "grammar_point_id", r.GrammarPointID)
			return nil, fmt.Errorf("service.GrammarService.GetDueQueue get point %d: %w", r.GrammarPointID, err)
		}
		points = append(points, *gp)
	}

	slog.Debug("GrammarService.GetDueQueue done", "user_id", userID, "count", len(points))
	return points, nil
}

// updateGrammarRecord creates or updates the grammar learning record after a quiz.
// Status advances to "learning" if first attempt, "mastered" if score >= 80.
func (s *GrammarService) updateGrammarRecord(userID, grammarPointID int64, score int) error {
	r, err := s.store.GetRecord(userID, grammarPointID)
	if err != nil {
		if isNotFound(err) {
			r = &grammar.GrammarRecord{
				UserID:         userID,
				GrammarPointID: grammarPointID,
				Status:         grammar.StatusUnlearned,
			}
		} else {
			return fmt.Errorf("get grammar record: %w", err)
		}
	}

	attempt := grammar.QuizAttempt{
		Score:       score,
		AttemptedAt: time.Now(),
	}
	r.QuizHistory = append(r.QuizHistory, attempt)

	if score >= 80 {
		r.Status = grammar.StatusMastered
		r.NextReviewAt = time.Now().Add(7 * 24 * time.Hour)
	} else {
		r.Status = grammar.StatusLearning
		r.NextReviewAt = time.Now().Add(24 * time.Hour)
	}

	return s.store.UpsertRecord(*r)
}
