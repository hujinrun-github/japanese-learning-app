package grammar_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"japanese-learning-app/internal/module/grammar"
)

// --- fakes ---

type fakeGrammarStore struct {
	points  map[int64]*grammar.GrammarPoint
	records map[string]*grammar.GrammarRecord // key: "userID:grammarPointID"
}

func newFakeGrammarStore() *fakeGrammarStore {
	return &fakeGrammarStore{
		points:  make(map[int64]*grammar.GrammarPoint),
		records: make(map[string]*grammar.GrammarRecord),
	}
}

func (f *fakeGrammarStore) GetByID(id int64) (*grammar.GrammarPoint, error) {
	p, ok := f.points[id]
	if !ok {
		return nil, errors.New("grammar point not found")
	}
	return p, nil
}

func (f *fakeGrammarStore) ListByLevel(level grammar.JLPTLevel) ([]grammar.GrammarPoint, error) {
	var result []grammar.GrammarPoint
	for _, p := range f.points {
		if p.JLPTLevel == level {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (f *fakeGrammarStore) GetRecord(userID, grammarPointID int64) (*grammar.GrammarRecord, error) {
	key := recordKey(userID, grammarPointID)
	r, ok := f.records[key]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (f *fakeGrammarStore) UpsertRecord(r grammar.GrammarRecord) error {
	key := recordKey(r.UserID, r.GrammarPointID)
	cp := r
	f.records[key] = &cp
	return nil
}

func (f *fakeGrammarStore) ListDueRecords(userID int64) ([]grammar.GrammarRecord, error) {
	var result []grammar.GrammarRecord
	now := time.Now()
	for _, r := range f.records {
		if r.UserID == userID && !r.NextReviewAt.After(now) {
			result = append(result, *r)
		}
	}
	return result, nil
}

func recordKey(userID, pointID int64) string {
	return fmt.Sprintf("%d:%d", userID, pointID)
}

// seedPoint adds a grammar point with quiz questions.
func (f *fakeGrammarStore) seedPoint(p grammar.GrammarPoint) {
	f.points[p.ID] = &p
}

// --- tests ---

func TestGrammarService_GetPoint(t *testing.T) {
	store := newFakeGrammarStore()
	store.seedPoint(grammar.GrammarPoint{
		ID:        1,
		Name:      "〜てもいい",
		JLPTLevel: grammar.LevelN5,
		QuizQuestions: []grammar.QuizQuestion{
			{ID: 1, Answer: "行ってもいい"},
		},
	})

	svc := grammar.NewGrammarService(store)
	p, err := svc.GetPoint(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "〜てもいい" {
		t.Errorf("expected name 〜てもいい, got %s", p.Name)
	}
	// Answer should be stripped from quiz questions
	for _, q := range p.QuizQuestions {
		if q.Answer != "" {
			t.Errorf("expected Answer to be empty, got %q", q.Answer)
		}
	}
}

func TestGrammarService_GetPoint_NotFound(t *testing.T) {
	store := newFakeGrammarStore()
	svc := grammar.NewGrammarService(store)
	_, err := svc.GetPoint(999)
	if err == nil {
		t.Error("expected error for nonexistent grammar point")
	}
}

func TestGrammarService_ScoreQuiz(t *testing.T) {
	store := newFakeGrammarStore()
	store.seedPoint(grammar.GrammarPoint{
		ID:        1,
		Name:      "〜てもいい",
		JLPTLevel: grammar.LevelN5,
		QuizQuestions: []grammar.QuizQuestion{
			{ID: 1, Answer: "行ってもいい", Explanation: "must use te-form"},
			{ID: 2, Answer: "食べてもいい", Explanation: "must use te-form"},
		},
	})

	svc := grammar.NewGrammarService(store)

	tests := []struct {
		name          string
		submissions   []grammar.QuizSubmission
		wantScore     int
		wantCorrect   []bool
	}{
		{
			name: "all correct",
			submissions: []grammar.QuizSubmission{
				{QuestionID: 1, Answer: "行ってもいい"},
				{QuestionID: 2, Answer: "食べてもいい"},
			},
			wantScore:   100,
			wantCorrect: []bool{true, true},
		},
		{
			name: "all wrong",
			submissions: []grammar.QuizSubmission{
				{QuestionID: 1, Answer: "wrong"},
				{QuestionID: 2, Answer: "wrong"},
			},
			wantScore:   0,
			wantCorrect: []bool{false, false},
		},
		{
			name: "half correct",
			submissions: []grammar.QuizSubmission{
				{QuestionID: 1, Answer: "行ってもいい"},
				{QuestionID: 2, Answer: "wrong"},
			},
			wantScore:   50,
			wantCorrect: []bool{true, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.ScoreQuiz(1, 1, tt.submissions)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Score != tt.wantScore {
				t.Errorf("Score = %d, want %d", result.Score, tt.wantScore)
			}
			for i, item := range result.Results {
				if item.Correct != tt.wantCorrect[i] {
					t.Errorf("Results[%d].Correct = %v, want %v", i, item.Correct, tt.wantCorrect[i])
				}
				if !item.Correct && item.Explanation == "" {
					t.Errorf("Results[%d]: expected non-empty explanation on wrong answer", i)
				}
			}
		})
	}
}

func TestGrammarService_ScoreQuiz_UpdatesRecord(t *testing.T) {
	store := newFakeGrammarStore()
	store.seedPoint(grammar.GrammarPoint{
		ID:        1,
		Name:      "〜てもいい",
		JLPTLevel: grammar.LevelN5,
		QuizQuestions: []grammar.QuizQuestion{
			{ID: 1, Answer: "行ってもいい"},
		},
	})

	svc := grammar.NewGrammarService(store)
	_, err := svc.ScoreQuiz(1, 1, []grammar.QuizSubmission{{QuestionID: 1, Answer: "行ってもいい"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := store.GetRecord(1, 1)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if rec == nil {
		t.Fatal("expected GrammarRecord to be created after quiz submission")
	}
	if len(rec.QuizHistory) != 1 {
		t.Errorf("expected 1 quiz attempt, got %d", len(rec.QuizHistory))
	}
	if rec.QuizHistory[0].Score != 100 {
		t.Errorf("expected score 100, got %d", rec.QuizHistory[0].Score)
	}
}

func TestGrammarService_ListByLevel(t *testing.T) {
	store := newFakeGrammarStore()
	store.seedPoint(grammar.GrammarPoint{ID: 1, Name: "〜てもいい", JLPTLevel: grammar.LevelN5})
	store.seedPoint(grammar.GrammarPoint{ID: 2, Name: "〜なければならない", JLPTLevel: grammar.LevelN4})

	svc := grammar.NewGrammarService(store)
	points, err := svc.ListByLevel(grammar.LevelN5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 grammar point for N5, got %d", len(points))
	}
}
