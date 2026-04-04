package lesson_test

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/module/lesson"
)

// --- fakes ---

type fakeLessonStore struct {
	summaries map[int64]*lesson.LessonSummary
	lessons   map[int64]*lesson.Lesson
	sentences map[int64][]lesson.Sentence
}

func newFakeLessonStore() *fakeLessonStore {
	return &fakeLessonStore{
		summaries: make(map[int64]*lesson.LessonSummary),
		lessons:   make(map[int64]*lesson.Lesson),
		sentences: make(map[int64][]lesson.Sentence),
	}
}

func (f *fakeLessonStore) ListSummaries(level lesson.JLPTLevel) ([]lesson.LessonSummary, error) {
	var result []lesson.LessonSummary
	for _, s := range f.summaries {
		if s.JLPTLevel == level {
			result = append(result, *s)
		}
	}
	return result, nil
}

func (f *fakeLessonStore) GetDetail(id int64) (*lesson.Lesson, error) {
	l, ok := f.lessons[id]
	if !ok {
		return nil, errors.New("lesson not found")
	}
	return l, nil
}

func (f *fakeLessonStore) GetSentences(lessonID int64) ([]lesson.Sentence, error) {
	s, ok := f.sentences[lessonID]
	if !ok {
		return nil, nil
	}
	return s, nil
}

// --- tests ---

func TestLessonService_ListSummaries(t *testing.T) {
	store := newFakeLessonStore()
	store.summaries[1] = &lesson.LessonSummary{ID: 1, Title: "自己紹介", JLPTLevel: lesson.LevelN5}
	store.summaries[2] = &lesson.LessonSummary{ID: 2, Title: "会社生活", JLPTLevel: lesson.LevelN4}

	svc := lesson.NewLessonService(store)
	summaries, err := svc.ListSummaries(lesson.LevelN5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary for N5, got %d", len(summaries))
	}
	if summaries[0].Title != "自己紹介" {
		t.Errorf("expected title 自己紹介, got %s", summaries[0].Title)
	}
}

func TestLessonService_GetDetail(t *testing.T) {
	store := newFakeLessonStore()
	store.lessons[1] = &lesson.Lesson{
		LessonSummary: lesson.LessonSummary{ID: 1, Title: "自己紹介", JLPTLevel: lesson.LevelN5},
		Sentences: []lesson.Sentence{
			{Index: 0, Chinese: "我的名字是田中", Tokens: []lesson.FuriganaToken{{Surface: "私", Reading: "わたし"}}},
		},
	}

	svc := lesson.NewLessonService(store)
	l, err := svc.GetDetail(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.Title != "自己紹介" {
		t.Errorf("expected title 自己紹介, got %s", l.Title)
	}
	if len(l.Sentences) != 1 {
		t.Errorf("expected 1 sentence, got %d", len(l.Sentences))
	}
}

func TestLessonService_GetDetail_NotFound(t *testing.T) {
	store := newFakeLessonStore()
	svc := lesson.NewLessonService(store)
	_, err := svc.GetDetail(999)
	if err == nil {
		t.Error("expected error for nonexistent lesson")
	}
}

func TestLessonService_GetSentences(t *testing.T) {
	store := newFakeLessonStore()
	store.lessons[1] = &lesson.Lesson{
		LessonSummary: lesson.LessonSummary{ID: 1, Title: "自己紹介", JLPTLevel: lesson.LevelN5},
	}
	store.sentences[1] = []lesson.Sentence{
		{Index: 0, Chinese: "今日はいい天気です"},
		{Index: 1, Chinese: "よろしくお願いします"},
	}

	svc := lesson.NewLessonService(store)
	sentences, err := svc.GetSentences(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentences) != 2 {
		t.Errorf("expected 2 sentences, got %d", len(sentences))
	}
}
