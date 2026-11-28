package translation_test

import (
	"context"
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

func (f *fakeStore) ListUnpracticedSentences(ctx context.Context, userID, sourceID int64, limit int, levels []string) ([]translation.TranslationSentence, error) {
	return nil, nil // The practice queue is covered by the database integration test.
}

func (f *fakeStore) GetDailyProgress(context.Context, int64, translation.StudyPlanTarget) (translation.DailyProgress, error) {
	return translation.DailyProgress{}, nil // Daily progress is covered by the database integration test.
}

func (f *fakeStore) SaveRecord(r translation.TranslationRecord) (int64, error) {
	r.ID = f.nextID
	f.records[f.nextID] = r
	f.nextID++
	return r.ID, nil
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

func (f *fakeStore) ListSentenceLinks(context.Context, int64) ([]translation.SentenceLink, error) {
	return nil, nil
}

func TestService_ImportSource(t *testing.T) {
	store := newFakeStore()
	svc := translation.NewTranslationService(store, nil)

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

	sentences, err := svc.GetFreeSentences(context.Background(), 0, "", result.ID, false)
	if err != nil {
		t.Fatalf("GetFreeSentences error: %v", err)
	}
	if len(sentences) != 2 {
		t.Errorf("sentences count = %d, want 2", len(sentences))
	}
}
