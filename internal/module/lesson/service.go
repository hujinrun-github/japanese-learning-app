package lesson

import (
	"fmt"
	"log/slog"
)

// LessonStoreInterface defines data access methods required by LessonService.
type LessonStoreInterface interface {
	ListSummaries(level JLPTLevel) ([]LessonSummary, error)
	GetDetail(id int64) (*Lesson, error)
	GetSentences(lessonID int64) ([]Sentence, error)
}

// LessonService handles business logic for lesson content.
type LessonService struct {
	store LessonStoreInterface
}

// NewLessonService creates a LessonService instance.
func NewLessonService(store LessonStoreInterface) *LessonService {
	return &LessonService{store: store}
}

// ListSummaries returns lesson summaries filtered by JLPT level.
func (s *LessonService) ListSummaries(level JLPTLevel) ([]LessonSummary, error) {
	slog.Debug("LessonService.ListSummaries called", "level", level)

	summaries, err := s.store.ListSummaries(level)
	if err != nil {
		slog.Error("LessonService.ListSummaries: failed", "err", err, "level", level)
		return nil, fmt.Errorf("lesson.LessonService.ListSummaries: %w", err)
	}

	slog.Debug("LessonService.ListSummaries done", "level", level, "count", len(summaries))
	return summaries, nil
}

// GetDetail returns a lesson's full content by ID.
func (s *LessonService) GetDetail(id int64) (*Lesson, error) {
	slog.Debug("LessonService.GetDetail called", "lesson_id", id)

	l, err := s.store.GetDetail(id)
	if err != nil {
		slog.Error("LessonService.GetDetail: failed", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("lesson.LessonService.GetDetail: %w", err)
	}

	slog.Debug("LessonService.GetDetail done", "lesson_id", id)
	return l, nil
}

// GetSentences returns the sentences for a lesson by ID.
func (s *LessonService) GetSentences(lessonID int64) ([]Sentence, error) {
	slog.Debug("LessonService.GetSentences called", "lesson_id", lessonID)

	sentences, err := s.store.GetSentences(lessonID)
	if err != nil {
		slog.Error("LessonService.GetSentences: failed", "err", err, "lesson_id", lessonID)
		return nil, fmt.Errorf("lesson.LessonService.GetSentences: %w", err)
	}

	slog.Debug("LessonService.GetSentences done", "lesson_id", lessonID, "count", len(sentences))
	return sentences, nil
}
