package writing_test

import (
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/writing"
)

// --- fake store ---

type fakeWritingStore struct {
	questions []writing.WritingQuestion
	records   []writing.WritingRecord
	nextID    int64
}

func (f *fakeWritingStore) GetDailyQueue(userID int64) ([]writing.WritingQuestion, error) {
	return f.questions, nil
}

func (f *fakeWritingStore) SaveRecord(r writing.WritingRecord) error {
	f.nextID++
	r.ID = f.nextID
	f.records = append(f.records, r)
	return nil
}

func (f *fakeWritingStore) ListRecords(userID int64) ([]writing.WritingRecord, error) {
	var result []writing.WritingRecord
	for _, r := range f.records {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result, nil
}

// --- tests ---

func TestWritingService_GetDailyQueue_StripExpectedAnswer(t *testing.T) {
	store := &fakeWritingStore{
		questions: []writing.WritingQuestion{
			{ID: 1, Type: writing.WritingTypeInput, Prompt: "Write: apple", ExpectedAnswer: "りんご"},
			{ID: 2, Type: writing.WritingTypeSentence, Prompt: "Translate: I like cats", ExpectedAnswer: "猫が好きです"},
		},
	}
	svc := writing.NewWritingService(store, nil) // no reviewer needed for queue

	questions, err := svc.GetDailyQueue(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(questions))
	}
	for _, q := range questions {
		if q.ExpectedAnswer != "" {
			t.Errorf("ExpectedAnswer should be stripped, got %q for question %d", q.ExpectedAnswer, q.ID)
		}
	}
}

func TestWritingService_SubmitInput(t *testing.T) {
	store := &fakeWritingStore{}
	svc := writing.NewWritingService(store, nil)

	tests := []struct {
		name        string
		question    string
		userAnswer  string
		expected    string
		wantScore   int
		wantCorrect bool
	}{
		{"exact match", "Write apple", "りんご", "りんご", 100, true},
		{"wrong answer", "Write apple", "ねこ", "りんご", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := svc.SubmitInput(1, tt.question, tt.userAnswer, tt.expected)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Score != tt.wantScore {
				t.Errorf("Score = %d, want %d", rec.Score, tt.wantScore)
			}
		})
	}
}

func TestWritingService_SubmitSentence(t *testing.T) {
	store := &fakeWritingStore{}
	stub := &writing.StubReviewer{
		Feedback: writing.AIFeedback{
			Score:             80,
			GrammarCorrect:    true,
			VocabCorrect:      true,
			CorrectedSentence: "猫が好きです",
			ReferenceAnswer:   "猫が好きです",
		},
	}
	svc := writing.NewWritingService(store, stub)

	rec, err := svc.SubmitSentence(1, "Translate: I like cats", "猫が好きです")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Score != 80 {
		t.Errorf("expected score 80, got %d", rec.Score)
	}
	if rec.AIFeedback == nil {
		t.Fatal("expected AIFeedback to be set")
	}
	if rec.AIFeedback.GrammarCorrect != true {
		t.Errorf("expected GrammarCorrect=true")
	}
}

func TestWritingService_SubmitSentence_ReviewerError(t *testing.T) {
	store := &fakeWritingStore{}
	stub := &writing.StubReviewer{Err: errors.New("AI unavailable")}
	svc := writing.NewWritingService(store, stub)

	_, err := svc.SubmitSentence(1, "Translate: I like cats", "猫が好きです")
	if err == nil {
		t.Error("expected error when reviewer fails")
	}
}

func TestWritingService_ListRecords(t *testing.T) {
	store := &fakeWritingStore{}
	stub := &writing.StubReviewer{Feedback: writing.AIFeedback{Score: 90}}
	svc := writing.NewWritingService(store, stub)

	_, _ = svc.SubmitSentence(1, "q1", "a1")
	_, _ = svc.SubmitSentence(1, "q2", "a2")
	_, _ = svc.SubmitSentence(2, "q3", "a3")

	records, err := svc.ListRecords(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records for user 1, got %d", len(records))
	}
	_ = time.Now() // use import
}
