package translation_test

import (
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
	}
}
