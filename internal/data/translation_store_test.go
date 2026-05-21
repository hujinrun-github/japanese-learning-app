package data

import (
	"testing"

	"japanese-learning-app/internal/module/translation"
)

func TestTranslationStore_SaveAndGetSource(t *testing.T) {
	store := &TranslationStore{db: testDB}

	src := translation.TranslationSource{
		Title:      "Test Source",
		SourceType: "manual",
		RawContent: "今日はいい天気です。明日も晴れるでしょう。",
	}

	id, err := store.SaveSource(src)
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}
	if id == 0 {
		t.Error("SaveSource returned 0 id")
	}

	sources, err := store.ListSources()
	if err != nil {
		t.Fatalf("ListSources error: %v", err)
	}
	if len(sources) < 1 {
		t.Fatal("ListSources: no sources found")
	}

	found := false
	for _, s := range sources {
		if s.ID == id {
			found = true
			if s.Title != "Test Source" {
				t.Errorf("Title = %q, want %q", s.Title, "Test Source")
			}
			if s.SourceType != "manual" {
				t.Errorf("SourceType = %q, want %q", s.SourceType, "manual")
			}
			break
		}
	}
	if !found {
		t.Errorf("inserted source id=%d not found in ListSources", id)
	}
}

func TestTranslationStore_SaveSentence(t *testing.T) {
	store := &TranslationStore{db: testDB}

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "Sentence Test", SourceType: "manual", RawContent: "こんにちは。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	sent := translation.TranslationSentence{
		SourceID:             srcID,
		Direction:            "jp2cn",
		SourceText:           "こんにちは。",
		ReferenceTranslation: "你好。",
		Position:             0,
	}

	sentID, err := store.SaveSentence(sent)
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}
	if sentID == 0 {
		t.Error("SaveSentence returned 0 id")
	}

	sentences, err := store.ListSentencesBySource(srcID)
	if err != nil {
		t.Fatalf("ListSentencesBySource error: %v", err)
	}
	if len(sentences) != 1 {
		t.Fatalf("ListSentencesBySource count = %d, want 1", len(sentences))
	}
	if sentences[0].SourceText != "こんにちは。" {
		t.Errorf("SourceText = %q, want %q", sentences[0].SourceText, "こんにちは。")
	}
	if sentences[0].Direction != "jp2cn" {
		t.Errorf("Direction = %q, want %q", sentences[0].Direction, "jp2cn")
	}
}

func TestTranslationStore_SaveAndListRecords(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9300, "tl_record@example.com")

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "Record Test", SourceType: "manual", RawContent: "おはよう。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	sentID, err := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "おはよう。",
	})
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}

	rec := translation.TranslationRecord{
		UserID:          9300,
		SentenceID:      sentID,
		UserTranslation: "早上好。",
		Score:           90,
		RuleScore:       85,
	}

	recID, err := store.SaveRecord(rec)
	if err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}
	if recID == 0 {
		t.Error("SaveRecord returned 0 id")
	}

	records, err := store.ListRecords(9300)
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRecords count = %d, want 1", len(records))
	}
	if records[0].Score != 90 {
		t.Errorf("Score = %d, want 90", records[0].Score)
	}
	if records[0].UserTranslation != "早上好。" {
		t.Errorf("UserTranslation = %q, want %q", records[0].UserTranslation, "早上好。")
	}
}

func TestTranslationStore_GetDailyQueue(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9301, "tl_daily@example.com")

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "Daily Queue Test", SourceType: "manual", RawContent: "テスト。試験。課題。確認。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	texts := []string{"テスト。", "試験。", "課題。", "確認。"}
	for i, text := range texts {
		_, err := store.SaveSentence(translation.TranslationSentence{
			SourceID: srcID, Direction: "jp2cn", SourceText: text, Position: i,
		})
		if err != nil {
			t.Fatalf("SaveSentence %d error: %v", i, err)
		}
	}

	queue, err := store.GetDailyQueue(9301, 2)
	if err != nil {
		t.Fatalf("GetDailyQueue error: %v", err)
	}
	if len(queue) != 2 {
		t.Fatalf("GetDailyQueue count = %d, want 2", len(queue))
	}
}

func TestTranslationStore_GetSentenceByID(t *testing.T) {
	store := &TranslationStore{db: testDB}

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "GetSentence Test", SourceType: "manual", RawContent: "テスト。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	sentID, err := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "テスト。",
	})
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}

	got, err := store.GetSentenceByID(sentID)
	if err != nil {
		t.Fatalf("GetSentenceByID(%d) error: %v", sentID, err)
	}
	if got == nil {
		t.Fatal("GetSentenceByID returned nil for existing sentence")
	}
	if got.SourceText != "テスト。" {
		t.Errorf("SourceText = %q, want %q", got.SourceText, "テスト。")
	}

	// Not found should return nil, nil
	got, err = store.GetSentenceByID(99999)
	if err != nil {
		t.Fatalf("GetSentenceByID(99999) error: %v", err)
	}
	if got != nil {
		t.Errorf("GetSentenceByID should return nil for nonexistent sentence, got %+v", got)
	}
}

func TestTranslationStore_GetRecord(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9302, "tl_get_record@example.com")

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "GetRecord Test", SourceType: "manual", RawContent: "今日。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}

	sentID, err := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "今日。",
	})
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}

	recID, err := store.SaveRecord(translation.TranslationRecord{
		UserID: 9302, SentenceID: sentID, UserTranslation: "今天。", Score: 95, RuleScore: 90,
	})
	if err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}

	got, err := store.GetRecord(recID)
	if err != nil {
		t.Fatalf("GetRecord(%d) error: %v", recID, err)
	}
	if got == nil {
		t.Fatal("GetRecord returned nil")
	}
	if got.Score != 95 {
		t.Errorf("Score = %d, want 95", got.Score)
	}

	// Not found should return error
	_, err = store.GetRecord(99999)
	if err == nil {
		t.Error("GetRecord(99999) should return error for nonexistent record")
	}
}

func TestTranslationStore_UpdateRecordFeedback(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9303, "tl_feedback@example.com")

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "Feedback Test", SourceType: "manual", RawContent: "テスト。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}
	sentID, err := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "テスト。",
	})
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}

	recID, err := store.SaveRecord(translation.TranslationRecord{
		UserID: 9303, SentenceID: sentID, UserTranslation: "测试。", Score: 50, RuleScore: 40,
	})
	if err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}

	err = store.UpdateRecordFeedback(recID, 85, `{"ai_score":85,"issue_description":"good"}`)
	if err != nil {
		t.Fatalf("UpdateRecordFeedback error: %v", err)
	}

	updated, err := store.GetRecord(recID)
	if err != nil {
		t.Fatalf("GetRecord after update error: %v", err)
	}
	if updated.Score != 85 {
		t.Errorf("Score after update = %d, want 85", updated.Score)
	}
	if updated.AIFeedback == nil {
		t.Fatal("AIFeedback should not be nil after update")
	}
	if updated.AIFeedback.AIScore != 85 {
		t.Errorf("AIFeedback.AIScore = %d, want 85", updated.AIFeedback.AIScore)
	}
}

func TestTranslationStore_SaveRecordWithAIFeedback(t *testing.T) {
	store := &TranslationStore{db: testDB}

	insertTestUser(t, 9304, "tl_ai_feedback@example.com")

	srcID, err := store.SaveSource(translation.TranslationSource{
		Title: "AI Feedback Test", SourceType: "manual", RawContent: "おはよう。",
	})
	if err != nil {
		t.Fatalf("SaveSource error: %v", err)
	}
	sentID, err := store.SaveSentence(translation.TranslationSentence{
		SourceID: srcID, Direction: "jp2cn", SourceText: "おはよう。",
	})
	if err != nil {
		t.Fatalf("SaveSentence error: %v", err)
	}

	feedback := &translation.TranslationFeedback{
		AIScore:              88,
		IssueDescription:     "translation is good",
		CorrectedTranslation: "早上好。",
		ReferenceTranslation: "早上好。",
		GrammarExplanations: []translation.GrammarExplanation{
			{GrammarPoint: "は", Explanation: "topic marker"},
		},
	}

	recID, err := store.SaveRecord(translation.TranslationRecord{
		UserID: 9304, SentenceID: sentID, UserTranslation: "早好。", Score: 88, RuleScore: 80,
		AIFeedback: feedback,
	})
	if err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}
	if recID == 0 {
		t.Error("SaveRecord returned 0")
	}

	got, err := store.GetRecord(recID)
	if err != nil {
		t.Fatalf("GetRecord(%d) error: %v", recID, err)
	}
	if got.AIFeedback == nil {
		t.Fatal("AIFeedback is nil")
	}
	if got.AIFeedback.AIScore != 88 {
		t.Errorf("AIFeedback.AIScore = %d, want 88", got.AIFeedback.AIScore)
	}
	if len(got.AIFeedback.GrammarExplanations) != 1 {
		t.Errorf("GrammarExplanations len = %d, want 1", len(got.AIFeedback.GrammarExplanations))
	}
}
