package service_test

import (
	"errors"
	"fmt"
	"testing"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeLessonStore struct {
	summaries []lesson.LessonSummary
	lessons   map[int64]*lesson.Lesson
}

func (f *fakeLessonStore) ListSummaries(level lesson.JLPTLevel, tag string) ([]lesson.LessonSummary, error) {
	var result []lesson.LessonSummary
	for _, s := range f.summaries {
		if s.JLPTLevel != level {
			continue
		}
		if tag != "" {
			found := false
			for _, t := range s.Tags {
				if t == tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		result = append(result, s)
	}
	return result, nil
}

func (f *fakeLessonStore) GetDetail(id int64) (*lesson.Lesson, error) {
	l, ok := f.lessons[id]
	if !ok {
		return nil, fmt.Errorf("lesson %d: %w", id, errors.New("not found"))
	}
	return l, nil
}

func (f *fakeLessonStore) GetSentences(id int64) ([]lesson.Sentence, error) {
	l, ok := f.lessons[id]
	if !ok {
		return nil, fmt.Errorf("lesson %d: %w", id, errors.New("not found"))
	}
	return l.Sentences, nil
}

// --- tests ---

func TestLessonService_ListSummaries(t *testing.T) {
	store := &fakeLessonStore{
		summaries: []lesson.LessonSummary{
			{ID: 1, Title: "挨拶", JLPTLevel: lesson.LevelN5, Tags: []string{"greetings", "daily"}},
			{ID: 2, Title: "自己紹介", JLPTLevel: lesson.LevelN5, Tags: []string{"self-intro"}},
			{ID: 3, Title: "仕事の会話", JLPTLevel: lesson.LevelN4, Tags: []string{"business"}},
		},
		lessons: map[int64]*lesson.Lesson{},
	}

	svc := service.NewLessonService(store)

	tests := []struct {
		name      string
		level     lesson.JLPTLevel
		tag       string
		wantCount int
	}{
		{"all N5", lesson.LevelN5, "", 2},
		{"N5 greetings", lesson.LevelN5, "greetings", 1},
		{"N4 all", lesson.LevelN4, "", 1},
		{"N5 nonexistent tag", lesson.LevelN5, "nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summaries, err := svc.ListSummaries(tt.level, tt.tag)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(summaries) != tt.wantCount {
				t.Errorf("expected %d summaries, got %d", tt.wantCount, len(summaries))
			}
		})
	}
}

func TestLessonService_GetDetail(t *testing.T) {
	store := &fakeLessonStore{
		summaries: nil,
		lessons: map[int64]*lesson.Lesson{
			1: {
				LessonSummary: lesson.LessonSummary{ID: 1, Title: "挨拶", JLPTLevel: lesson.LevelN5},
				Sentences: []lesson.Sentence{
					{Index: 0, Chinese: "你好", Tokens: []lesson.FuriganaToken{{Surface: "こんにちは"}}},
				},
				WordIDs: []int64{10, 11},
			},
		},
	}

	svc := service.NewLessonService(store)
	detail, err := svc.GetDetail(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.ID != 1 {
		t.Errorf("expected id 1, got %d", detail.ID)
	}
	if len(detail.Sentences) != 1 {
		t.Errorf("expected 1 sentence, got %d", len(detail.Sentences))
	}
	if len(detail.WordIDs) != 2 {
		t.Errorf("expected 2 word ids, got %d", len(detail.WordIDs))
	}
}

func TestLessonService_GetDetail_NotFound(t *testing.T) {
	store := &fakeLessonStore{
		summaries: nil,
		lessons:   map[int64]*lesson.Lesson{},
	}

	svc := service.NewLessonService(store)
	_, err := svc.GetDetail(999)
	if err == nil {
		t.Fatal("expected error for nonexistent lesson")
	}
}

func TestLessonService_GetSentences(t *testing.T) {
	store := &fakeLessonStore{
		summaries: nil,
		lessons: map[int64]*lesson.Lesson{
			1: {
				LessonSummary: lesson.LessonSummary{ID: 1, Title: "挨拶", JLPTLevel: lesson.LevelN5},
				Sentences: []lesson.Sentence{
					{Index: 0, Chinese: "你好", Tokens: []lesson.FuriganaToken{{Surface: "こんにちは"}}},
					{Index: 1, Chinese: "再见", Tokens: []lesson.FuriganaToken{{Surface: "さようなら"}}},
				},
			},
		},
	}

	svc := service.NewLessonService(store)
	sentences, err := svc.GetSentences(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentences) != 2 {
		t.Errorf("expected 2 sentences, got %d", len(sentences))
	}
}
