package data

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/writing"
)

func seedWritingQuestions(t *testing.T) {
	t.Helper()
	// 插入 5 道写作题（input type），确保 GetDailyQueue 可返回 3~5 道
	for i := 0; i < 5; i++ {
		_, err := testDB.Exec(`
			INSERT OR IGNORE INTO writing_questions (type, prompt, expected_answer, grammar_point_id, jlpt_level)
			VALUES (?, ?, ?, ?, ?)`,
			"input",
			"请输入假名：たべる",
			"たべる",
			0,
			"N5",
		)
		if err != nil {
			t.Fatalf("seed writing_question error: %v", err)
		}
	}
}

func TestWritingStore_GetDailyQueue(t *testing.T) {
	store := &WritingStore{db: testDB}

	seedWritingQuestions(t)

	insertTestUser(t, 9300, "writing_queue@example.com")

	queue, err := store.GetDailyQueue(9300)
	if err != nil {
		t.Fatalf("GetDailyQueue error: %v", err)
	}
	if len(queue) < 3 {
		t.Errorf("GetDailyQueue len = %d, want >= 3", len(queue))
	}
	if len(queue) > 5 {
		t.Errorf("GetDailyQueue len = %d, want <= 5", len(queue))
	}

	// 验证 ExpectedAnswer 不暴露（json:"-"，但 Go 层面应有值）
	for _, q := range queue {
		if q.ID == 0 {
			t.Error("GetDailyQueue returned question with ID=0")
		}
		if q.Prompt == "" {
			t.Error("GetDailyQueue returned question with empty Prompt")
		}
	}
}

func TestWritingStore_SaveRecord(t *testing.T) {
	store := &WritingStore{db: testDB}

	insertTestUser(t, 9301, "writing_save@example.com")

	record := writing.WritingRecord{
		UserID:      9301,
		Type:        writing.WritingTypeInput,
		Question:    "请输入：たべる",
		UserAnswer:  "たべる",
		AIFeedback:  nil,
		Score:       100,
		PracticedAt: time.Now(),
	}

	if err := store.SaveRecord(record); err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}

	// 验证持久化
	var count int
	err := testDB.QueryRow(
		`SELECT COUNT(*) FROM writing_records WHERE user_id=?`, 9301,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query count error: %v", err)
	}
	if count != 1 {
		t.Errorf("writing_records count = %d, want 1", count)
	}
}

func TestWritingStore_SaveRecord_WithAIFeedback(t *testing.T) {
	store := &WritingStore{db: testDB}

	insertTestUser(t, 9302, "writing_ai@example.com")

	feedback := &writing.AIFeedback{
		Score:              85,
		GrammarCorrect:     true,
		VocabCorrect:       false,
		IssueDescription:   "用词不够自然",
		CorrectedSentence:  "日本語を話すことができます。",
		AlternativePhrases: []string{"日本語が話せます。"},
		ReferenceAnswer:    "日本語を話すことができます。",
	}

	record := writing.WritingRecord{
		UserID:      9302,
		Type:        writing.WritingTypeSentence,
		Question:    "翻译：我会说日语。",
		UserAnswer:  "日本語を話すことできます。",
		AIFeedback:  feedback,
		Score:       85,
		PracticedAt: time.Now(),
	}

	if err := store.SaveRecord(record); err != nil {
		t.Fatalf("SaveRecord with feedback error: %v", err)
	}

	records, err := store.ListRecords(9302)
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRecords len = %d, want 1", len(records))
	}

	got := records[0]
	if got.AIFeedback == nil {
		t.Fatal("ListRecords returned record with nil AIFeedback")
	}
	if got.AIFeedback.Score != 85 {
		t.Errorf("AIFeedback.Score = %d, want 85", got.AIFeedback.Score)
	}
}

func TestWritingStore_ListRecords_OrderedByPracticedAt(t *testing.T) {
	store := &WritingStore{db: testDB}

	insertTestUser(t, 9303, "writing_list@example.com")

	now := time.Now()
	for i := 0; i < 3; i++ {
		r := writing.WritingRecord{
			UserID:      9303,
			Type:        writing.WritingTypeInput,
			Question:    "question",
			UserAnswer:  "answer",
			Score:       50 + i*10,
			PracticedAt: now.Add(time.Duration(i) * time.Hour),
		}
		if err := store.SaveRecord(r); err != nil {
			t.Fatalf("SaveRecord %d error: %v", i, err)
		}
	}

	records, err := store.ListRecords(9303)
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if len(records) < 3 {
		t.Errorf("ListRecords len = %d, want >= 3", len(records))
	}

	// 验证按 practiced_at 倒序
	for i := 1; i < len(records); i++ {
		if records[i].PracticedAt.After(records[i-1].PracticedAt) {
			t.Errorf("ListRecords not in descending order at index %d", i)
		}
	}
}
