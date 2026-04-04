package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeGrammarStore struct {
	points  map[int64]*grammar.GrammarPoint
	records map[string]*grammar.GrammarRecord // key: "userID:grammarPointID"
}

func (f *fakeGrammarStore) GetByID(id int64) (*grammar.GrammarPoint, error) {
	gp, ok := f.points[id]
	if !ok {
		return nil, fmt.Errorf("grammar_point %d: %w", id, errors.New("not found"))
	}
	return gp, nil
}

func (f *fakeGrammarStore) ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error) {
	var result []grammar.GrammarPoint
	for _, gp := range f.points {
		if gp.JLPTLevel == level {
			result = append(result, *gp)
		}
	}
	return result, nil
}

func (f *fakeGrammarStore) GetRecord(userID, grammarPointID int64) (*grammar.GrammarRecord, error) {
	key := fmt.Sprintf("%d:%d", userID, grammarPointID)
	r, ok := f.records[key]
	if !ok {
		return nil, fmt.Errorf("grammar_record not found: %w", errors.New("not found"))
	}
	return r, nil
}

func (f *fakeGrammarStore) UpsertRecord(r grammar.GrammarRecord) error {
	key := fmt.Sprintf("%d:%d", r.UserID, r.GrammarPointID)
	cp := r
	f.records[key] = &cp
	return nil
}

func (f *fakeGrammarStore) ListDueRecords(userID int64) ([]grammar.GrammarRecord, error) {
	var result []grammar.GrammarRecord
	for _, r := range f.records {
		if r.UserID == userID && !r.NextReviewAt.After(time.Now()) {
			result = append(result, *r)
		}
	}
	return result, nil
}

// --- tests ---

func TestGrammarService_ListByLevel(t *testing.T) {
	store := &fakeGrammarStore{
		points: map[int64]*grammar.GrammarPoint{
			1: {ID: 1, Name: "〜てもいい", JLPTLevel: grammar.LevelN5},
			2: {ID: 2, Name: "〜なければならない", JLPTLevel: grammar.LevelN4},
		},
		records: map[string]*grammar.GrammarRecord{},
	}

	svc := service.NewGrammarService(store)
	points, err := svc.ListByLevel(grammar.LevelN5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 N5 grammar point, got %d", len(points))
	}
	if points[0].Name != "〜てもいい" {
		t.Errorf("unexpected grammar point name: %s", points[0].Name)
	}
}

func TestGrammarService_SubmitQuiz(t *testing.T) {
	store := &fakeGrammarStore{
		points: map[int64]*grammar.GrammarPoint{
			1: {
				ID:        1,
				Name:      "〜てもいい",
				JLPTLevel: grammar.LevelN5,
				QuizQuestions: []grammar.QuizQuestion{
					{ID: 1, Type: grammar.QuizFillBlank, Prompt: "食べ___いい", Answer: "ても", Explanation: "〜てもいい表示许可"},
					{ID: 2, Type: grammar.QuizMultiChoice, Prompt: "行っ___いい", Answer: "ても", Options: []string{"ても", "でも", "から"}},
				},
			},
		},
		records: map[string]*grammar.GrammarRecord{},
	}

	svc := service.NewGrammarService(store)

	tests := []struct {
		name          string
		submissions   []grammar.QuizSubmission
		wantScore     int
		wantCorrect   int
	}{
		{
			name: "all correct",
			submissions: []grammar.QuizSubmission{
				{QuestionID: 1, Answer: "ても"},
				{QuestionID: 2, Answer: "ても"},
			},
			wantScore:   100,
			wantCorrect: 2,
		},
		{
			name: "one wrong",
			submissions: []grammar.QuizSubmission{
				{QuestionID: 1, Answer: "でも"},
				{QuestionID: 2, Answer: "ても"},
			},
			wantScore:   50,
			wantCorrect: 1,
		},
		{
			name: "all wrong",
			submissions: []grammar.QuizSubmission{
				{QuestionID: 1, Answer: "wrong"},
				{QuestionID: 2, Answer: "wrong"},
			},
			wantScore:   0,
			wantCorrect: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.SubmitQuiz(1, 1, tt.submissions)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Score != tt.wantScore {
				t.Errorf("expected score %d, got %d", tt.wantScore, result.Score)
			}
			correct := 0
			for _, r := range result.Results {
				if r.Correct {
					correct++
				}
			}
			if correct != tt.wantCorrect {
				t.Errorf("expected %d correct, got %d", tt.wantCorrect, correct)
			}
		})
	}
}

func TestGrammarService_SubmitQuiz_UpdatesRecord(t *testing.T) {
	store := &fakeGrammarStore{
		points: map[int64]*grammar.GrammarPoint{
			1: {
				ID:        1,
				Name:      "〜てもいい",
				JLPTLevel: grammar.LevelN5,
				QuizQuestions: []grammar.QuizQuestion{
					{ID: 1, Type: grammar.QuizFillBlank, Prompt: "食べ___いい", Answer: "ても"},
				},
			},
		},
		records: map[string]*grammar.GrammarRecord{},
	}

	svc := service.NewGrammarService(store)
	_, err := svc.SubmitQuiz(1, 1, []grammar.QuizSubmission{
		{QuestionID: 1, Answer: "ても"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := store.records["1:1"]
	if r == nil {
		t.Fatal("expected grammar record to be created after quiz")
	}
	if r.Status != grammar.StatusLearning && r.Status != grammar.StatusMastered {
		t.Errorf("expected record to be learning or mastered, got %s", r.Status)
	}
}

func TestGrammarService_GetDueQueue(t *testing.T) {
	now := time.Now()
	store := &fakeGrammarStore{
		points: map[int64]*grammar.GrammarPoint{
			1: {ID: 1, Name: "〜てもいい", JLPTLevel: grammar.LevelN5},
			2: {ID: 2, Name: "〜なければならない", JLPTLevel: grammar.LevelN4},
		},
		records: map[string]*grammar.GrammarRecord{
			"1:1": {ID: 1, UserID: 1, GrammarPointID: 1, Status: grammar.StatusLearning, NextReviewAt: now.Add(-time.Hour)},
			"1:2": {ID: 2, UserID: 1, GrammarPointID: 2, Status: grammar.StatusLearning, NextReviewAt: now.Add(time.Hour)},
		},
	}

	svc := service.NewGrammarService(store)
	points, err := svc.GetDueQueue(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 due grammar point, got %d", len(points))
	}
	if points[0].ID != 1 {
		t.Errorf("expected grammar point id 1, got %d", points[0].ID)
	}
}
