package postgres

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/user"
)

func TestGrammarStoreGetByID(t *testing.T) {
	store := newPostgresGrammarStoreForTest(t)

	pointID, err := store.InsertPoint(grammar.GrammarPoint{
		Name:            "てもいい",
		Meaning:         "可以",
		ConjunctionRule: "动词て形",
		UsageNote:       "表示许可",
		JLPTLevel:       grammar.LevelN5,
		Examples: []grammar.GrammarExample{
			{Japanese: "食べてもいいです。", Chinese: "可以吃。", FuriganaHTML: "たべてもいいです。", LinkedWords: []int64{1, 2}},
		},
		QuizQuestions: []grammar.QuizQuestion{
			{Type: grammar.QuizFillBlank, Prompt: "___てもいい", Answer: "食べ", Explanation: "动词て形"},
		},
	})
	if err != nil {
		t.Fatalf("InsertPoint() error = %v", err)
	}

	got, err := store.GetByID(pointID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByID() returned nil")
	}
	if got.Name != "てもいい" {
		t.Fatalf("GetByID() name = %q, want %q", got.Name, "てもいい")
	}
	if len(got.Examples) != 1 || got.Examples[0].FuriganaHTML == "" {
		t.Fatalf("GetByID() examples = %+v, want furigana and linked words", got.Examples)
	}
	if len(got.QuizQuestions) != 1 || got.QuizQuestions[0].Prompt == "" {
		t.Fatalf("GetByID() quiz = %+v, want prompt preserved", got.QuizQuestions)
	}
}

func TestGrammarStoreListByLevel(t *testing.T) {
	store := newPostgresGrammarStoreForTest(t)

	_, err := store.InsertPoint(grammar.GrammarPoint{Name: "A", Meaning: "A", JLPTLevel: grammar.LevelN4})
	if err != nil {
		t.Fatalf("InsertPoint(N4) error = %v", err)
	}
	_, err = store.InsertPoint(grammar.GrammarPoint{Name: "B", Meaning: "B", JLPTLevel: grammar.LevelN5})
	if err != nil {
		t.Fatalf("InsertPoint(N5) error = %v", err)
	}

	points, err := store.ListByLevel(grammar.LevelN4)
	if err != nil {
		t.Fatalf("ListByLevel() error = %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("ListByLevel() len = %d, want 1", len(points))
	}
	if points[0].JLPTLevel != grammar.LevelN4 {
		t.Fatalf("ListByLevel() level = %s, want %s", points[0].JLPTLevel, grammar.LevelN4)
	}
}

func TestGrammarStoreListByLevelWithStatus(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresGrammarStoreForDB(db)
	userStore := newPostgresUserStoreForDB(db)

	createdUser, err := userStore.CreateUser(user.User{Name: "Grammar User", Email: "grammar-status@example.com"}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	pointID, err := store.InsertPoint(grammar.GrammarPoint{Name: "たことがある", Meaning: "曾经", JLPTLevel: grammar.LevelN4})
	if err != nil {
		t.Fatalf("InsertPoint() error = %v", err)
	}
	err = store.UpsertRecord(grammar.GrammarRecord{
		UserID:         createdUser.ID,
		GrammarPointID: pointID,
		Status:         grammar.StatusMastered,
		NextReviewAt:   time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
		QuizHistory: []grammar.QuizAttempt{
			{Score: 90, AttemptedAt: time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)},
		},
	})
	if err != nil {
		t.Fatalf("UpsertRecord() error = %v", err)
	}

	items, err := store.ListByLevelWithStatus(createdUser.ID, grammar.LevelN4)
	if err != nil {
		t.Fatalf("ListByLevelWithStatus() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListByLevelWithStatus() len = %d, want 1", len(items))
	}
	if items[0].UserStatus != grammar.StatusMastered || items[0].LastQuizScore != 90 {
		t.Fatalf("ListByLevelWithStatus() item = %+v, want mastered/90", items[0])
	}
}

func TestGrammarStoreUpsertRecordAndListDueRecords(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresGrammarStoreForDB(db)
	userStore := newPostgresUserStoreForDB(db)

	createdUser, err := userStore.CreateUser(user.User{Name: "Grammar Reviewer", Email: "grammar-reviewer@example.com"}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	pointID, err := store.InsertPoint(grammar.GrammarPoint{Name: "ながら", Meaning: "一边", JLPTLevel: grammar.LevelN5})
	if err != nil {
		t.Fatalf("InsertPoint() error = %v", err)
	}

	err = store.UpsertRecord(grammar.GrammarRecord{
		UserID:         createdUser.ID,
		GrammarPointID: pointID,
		Status:         grammar.StatusLearning,
		NextReviewAt:   time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC),
		QuizHistory: []grammar.QuizAttempt{
			{Score: 60, AttemptedAt: time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)},
		},
	})
	if err != nil {
		t.Fatalf("UpsertRecord() error = %v", err)
	}

	record, err := store.GetRecord(createdUser.ID, pointID)
	if err != nil {
		t.Fatalf("GetRecord() error = %v", err)
	}
	if record == nil || record.Status != grammar.StatusLearning {
		t.Fatalf("GetRecord() record = %+v, want learning", record)
	}

	dueRecords, err := store.ListDueRecords(createdUser.ID)
	if err != nil {
		t.Fatalf("ListDueRecords() error = %v", err)
	}
	if len(dueRecords) != 1 {
		t.Fatalf("ListDueRecords() len = %d, want 1", len(dueRecords))
	}
}

func TestGrammarStoreAdminCRUDAndListAllRecords(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresGrammarStoreForDB(db)
	userStore := newPostgresUserStoreForDB(db)

	pointID, err := store.InsertPoint(grammar.GrammarPoint{
		Name:            "ように",
		Meaning:         "为了",
		ConjunctionRule: "动词辞书形",
		UsageNote:       "表示目的",
		JLPTLevel:       grammar.LevelN4,
		Examples: []grammar.GrammarExample{
			{Japanese: "忘れないように。", Chinese: "为了不忘记。"},
		},
		QuizQuestions: []grammar.QuizQuestion{
			{Type: grammar.QuizMultiChoice, Prompt: "选择正确用法", Options: []string{"A", "B"}, Answer: "A", Explanation: "A is correct"},
		},
	})
	if err != nil {
		t.Fatalf("InsertPoint() error = %v", err)
	}

	items, total, err := store.ListAll(string(grammar.LevelN4), "ように", 0, 20)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("ListAll() total/len = %d/%d, want 1/1", total, len(items))
	}

	err = store.UpdatePoint(grammar.GrammarPoint{
		ID:              pointID,
		Name:            "ように",
		Meaning:         "以便",
		ConjunctionRule: "动词辞书形",
		UsageNote:       "表示目的或变化结果",
		JLPTLevel:       grammar.LevelN4,
		Examples: []grammar.GrammarExample{
			{Japanese: "見えるように。", Chinese: "以便看见。", FuriganaHTML: "みえるように。"},
		},
		QuizQuestions: []grammar.QuizQuestion{
			{Type: grammar.QuizFillBlank, Prompt: "___ように", Answer: "見える", Explanation: "动词辞书形"},
		},
	})
	if err != nil {
		t.Fatalf("UpdatePoint() error = %v", err)
	}

	updated, err := store.GetByID(pointID)
	if err != nil {
		t.Fatalf("GetByID(updated) error = %v", err)
	}
	if updated.Meaning != "以便" || len(updated.QuizQuestions) != 1 {
		t.Fatalf("GetByID(updated) = %+v, want updated point", updated)
	}

	createdUser, err := userStore.CreateUser(user.User{Name: "Grammar Records User", Email: "grammar-records@example.com"}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	err = store.UpsertRecord(grammar.GrammarRecord{
		UserID:         createdUser.ID,
		GrammarPointID: pointID,
		Status:         grammar.StatusMastered,
		NextReviewAt:   time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertRecord() error = %v", err)
	}

	records, total, err := store.ListAllRecords(createdUser.ID, 0, 20)
	if err != nil {
		t.Fatalf("ListAllRecords() error = %v", err)
	}
	if total != 1 || len(records) != 1 {
		t.Fatalf("ListAllRecords() total/len = %d/%d, want 1/1", total, len(records))
	}

	if err := store.DeletePoint(pointID); err != nil {
		t.Fatalf("DeletePoint() error = %v", err)
	}
}
