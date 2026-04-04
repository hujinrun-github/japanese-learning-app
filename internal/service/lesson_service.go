package service

import (
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/lesson"
)

// LessonStoreInterface defines the data access methods required by LessonService.
type LessonStoreInterface interface {
	ListSummaries(level lesson.JLPTLevel, tag string) ([]lesson.LessonSummary, error)
	GetDetail(id int64) (*lesson.Lesson, error)
	GetSentences(id int64) ([]lesson.Sentence, error)
}

// LessonService handles business logic for lesson browsing and reading.
type LessonService struct {
	store LessonStoreInterface
}

// NewLessonService creates a LessonService instance.
func NewLessonService(store LessonStoreInterface) *LessonService {
	return &LessonService{store: store}
}

// ListSummaries returns lesson summaries for the given JLPT level, optionally filtered by tag.
func (s *LessonService) ListSummaries(level lesson.JLPTLevel, tag string) ([]lesson.LessonSummary, error) {
	slog.Debug("LessonService.ListSummaries called", "level", level, "tag", tag)

	summaries, err := s.store.ListSummaries(level, tag)
	if err != nil {
		slog.Error("LessonService.ListSummaries: failed", "err", err)
		return nil, fmt.Errorf("service.LessonService.ListSummaries: %w", err)
	}

	slog.Debug("LessonService.ListSummaries done", "level", level, "count", len(summaries))
	return summaries, nil
}

// GetDetail returns full lesson detail including sentences and word IDs.
func (s *LessonService) GetDetail(id int64) (*lesson.Lesson, error) {
	slog.Debug("LessonService.GetDetail called", "lesson_id", id)

	l, err := s.store.GetDetail(id)
	if err != nil {
		slog.Error("LessonService.GetDetail: failed", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("service.LessonService.GetDetail: %w", err)
	}

	slog.Debug("LessonService.GetDetail done", "lesson_id", id, "title", l.Title)
	return l, nil
}

// GetSentences returns the sentences of a lesson for speaking practice.
func (s *LessonService) GetSentences(id int64) ([]lesson.Sentence, error) {
	slog.Debug("LessonService.GetSentences called", "lesson_id", id)

	sentences, err := s.store.GetSentences(id)
	if err != nil {
		slog.Error("LessonService.GetSentences: failed", "err", err, "lesson_id", id)
		return nil, fmt.Errorf("service.LessonService.GetSentences: %w", err)
	}

	slog.Debug("LessonService.GetSentences done", "lesson_id", id, "count", len(sentences))
	return sentences, nil
}
