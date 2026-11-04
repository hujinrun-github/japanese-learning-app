package translation_test

import (
	"fmt"
	"testing"

	"japanese-learning-app/internal/module/translation"
)

type fakeStore struct {
	sources   map[int64]translation.TranslationSource
	sentences map[int64]translation.TranslationSentence
	records   map[int64]translation.TranslationRecord
	nextID    int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sources:   make(map[int64]translation.TranslationSource),
		sentences: make(map[int64]translation.TranslationSentence),
		records:   make(map[int64]translation.TranslationRecord),
		nextID:    1,
	}
}

func (f *fakeStore) SaveSource(s translation.TranslationSource) (int64, error) {
	s.ID = f.nextID
	f.sources[f.nextID] = s
	f.nextID++
	return s.ID, nil
}

func (f *fakeStore) SaveSentence(s translation.TranslationSentence) (int64, error) {
	s.ID = f.nextID
	f.sentences[f.nextID] = s
	f.nextID++
	return s.ID, nil
}

func (f *fakeStore) ListSources() ([]translation.TranslationSource, error) {
	var out []translation.TranslationSource
	for _, s := range f.sources {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeStore) ListSentencesBySource(sourceID int64) ([]translation.TranslationSentence, error) {
	var out []translation.TranslationSentence
	for _, s := range f.sentences {
		if s.SourceID == sourceID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) GetSentenceByID(id int64) (*translation.TranslationSentence, error) {
	s, ok := f.sentences[id]
	if !ok {
		return nil, nil
	}
	return &s, nil
}

func (f *fakeStore) GetDailyQueue(userID int64, limit int) ([]translation.TranslationSentence, error) {
	var done map[int64]bool
	for _, r := range f.records {
		if r.UserID == userID && r.Score >= 80 {
			if done == nil {
				done = make(map[int64]bool)
			}
			done[r.SentenceID] = true
		}
	}
	var out []translation.TranslationSentence
	for _, s := range f.sentences {
		if !done[s.ID] && len(out) < limit {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) SaveRecord(r translation.TranslationRecord) (int64, error) {
	r.ID = f.nextID
	f.records[f.nextID] = r
	f.nextID++
	return r.ID, nil
}

func (f *fakeStore) UpdateRecordFeedback(recordID int64, score int, feedbackJSON string) error {
	r, ok := f.records[recordID]
	if !ok {
		return nil
	}
	r.Score = score
	f.records[recordID] = r
	return nil
}

func (f *fakeStore) UpdateSentenceReference(sentenceID int64, reference string) error {
	if s, ok := f.sentences[sentenceID]; ok {
		s.ReferenceTranslation = reference
		f.sentences[sentenceID] = s
	}
	return nil
}

func (f *fakeStore) DeleteSource(id int64) error {
	delete(f.sources, id)
	return nil
}

func (f *fakeStore) GetRecord(id int64) (*translation.TranslationRecord, error) {
	r, ok := f.records[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (f *fakeStore) ListRecords(userID int64) ([]translation.TranslationRecord, error) {
	var out []translation.TranslationRecord
	for _, r := range f.records {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestService_ImportSource(t *testing.T) {
	store := newFakeStore()
	svc := translation.NewTranslationService(store, &translation.StubReviewer{})

	src := translation.TranslationSource{
		Title:      "Test Import",
		SourceType: "manual",
		RawContent: "こんにちは。お元気ですか。",
	}

	result, err := svc.ImportSource(src)
	if err != nil {
		t.Fatalf("ImportSource error: %v", err)
	}
	if result == nil {
		t.Fatal("ImportSource returned nil source")
	}

	sentences, err := svc.GetFreeSentences(0, "", result.ID)
	if err != nil {
		t.Fatalf("GetFreeSentences error: %v", err)
	}
	if len(sentences) != 2 {
		t.Errorf("sentences count = %d, want 2", len(sentences))
	}
}

func TestService_SubmitTranslation(t *testing.T) {
	store := newFakeStore()
	svc := translation.NewTranslationService(store, &translation.StubReviewer{
		Feedback: translation.TranslationFeedback{
			AIScore: 88,
			GrammarExplanations: []translation.GrammarExplanation{
				{GrammarPoint: "です", Explanation: "polite copula"},
			},
			IssueDescription:     "",
			CorrectedTranslation: "你好。",
			ReferenceTranslation: "你好。",
		},
	})

	srcID, _ := store.SaveSource(translation.TranslationSource{
		Title: "Submit Test", SourceType: "manual", RawContent: "こんにちは。",
	})
	sentID, _ := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "こんにちは。",
	})

	rec, err := svc.SubmitTranslation(1, sentID, "你好。")
	if err != nil {
		t.Fatalf("SubmitTranslation error: %v", err)
	}
	if rec.RuleScore == 0 {
		t.Error("RuleScore should not be 0")
	}
	if rec.AIFeedback == nil {
		t.Error("AIFeedback should not be nil (stub reviewer)")
	} else if len(rec.AIFeedback.GrammarExplanations) != 1 {
		t.Fatalf("GrammarExplanations len = %d, want 1", len(rec.AIFeedback.GrammarExplanations))
	}
}

func TestService_SubmitTranslationRuleScoreUsesReferenceTranslation(t *testing.T) {
	tests := []struct {
		name       string
		userAnswer string
		wantScore  int
		wantExact  bool
	}{
		{name: "exact translation", userAnswer: "你好。", wantScore: 100, wantExact: true},
		{name: "unrelated translation", userAnswer: "完全不同的答案。", wantScore: 100, wantExact: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			srcID, _ := store.SaveSource(translation.TranslationSource{Title: tt.name, SourceType: "manual", RawContent: "こんにちは。"})
			sentID, _ := store.SaveSentence(translation.TranslationSentence{
				SourceID: srcID, Direction: "jp2cn", SourceText: "こんにちは。", ReferenceTranslation: "你好。",
			})

			rec, err := translation.NewTranslationService(store, nil).SubmitTranslation(1, sentID, tt.userAnswer)
			if err != nil {
				t.Fatalf("SubmitTranslation error: %v", err)
			}
			if tt.wantExact && rec.RuleScore != tt.wantScore {
				t.Errorf("RuleScore = %d, want %d for exact reference match", rec.RuleScore, tt.wantScore)
			}
			if !tt.wantExact && rec.RuleScore >= tt.wantScore {
				t.Errorf("RuleScore = %d, want less than %d for unrelated translation", rec.RuleScore, tt.wantScore)
			}
		})
	}
}

func TestService_SubmitTranslationReviewerErrorKeepsGrammarFallback(t *testing.T) {
	store := newFakeStore()
	srcID, _ := store.SaveSource(translation.TranslationSource{
		Title: "Reviewer Error", SourceType: "manual", RawContent: "雨が降っています。",
	})
	sentID, _ := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "雨が降っています。", ReferenceTranslation: "正在下雨。",
	})

	svc := translation.NewTranslationService(store, &translation.StubReviewer{Err: fmt.Errorf("AI unavailable")})
	rec, err := svc.SubmitTranslation(1, sentID, "正在下雨。")
	if err != nil {
		t.Fatalf("SubmitTranslation error: %v", err)
	}
	if rec.AIFeedback == nil {
		t.Fatal("AIFeedback should contain local fallback")
	}
	if len(rec.AIFeedback.GrammarExplanations) == 0 {
		t.Fatal("local fallback should include detected grammar")
	}
	if rec.Score != rec.RuleScore {
		t.Errorf("fallback Score = %d, RuleScore = %d, want them to match", rec.Score, rec.RuleScore)
	}
}
