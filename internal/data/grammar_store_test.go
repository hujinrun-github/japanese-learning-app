package data

import (
	"slices"
	"testing"
	"time"

	"japanese-learning-app/internal/module/grammar"
)

func TestGrammarStore_GetByID_NotFound(t *testing.T) {
	store := &GrammarStore{db: testDB}

	gp, err := store.GetByID(999999)
	if err == nil {
		t.Fatal("GetByID(999999) expected error, got nil")
	}
	if gp != nil {
		t.Errorf("GetByID(999999) expected nil, got %+v", gp)
	}
}

func TestGrammarStore_InsertAndGetByID(t *testing.T) {
	store := &GrammarStore{db: testDB}

	// 插入一条语法点测试数据
	res, err := testDB.Exec(`
		INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"〜てもいい", "可以…", "动词て形", "表示许可",
		`[{"japanese":"食べてもいいです。","chinese":"可以吃。","linked_word_ids":[]}]`,
		`[{"id":1,"type":"fill_blank","prompt":"___てもいい","options":[],"answer":"食べ","explanation":"动词て形"}]`,
		"N5",
	)
	if err != nil {
		t.Fatalf("insert grammar_point error: %v", err)
	}
	id, _ := res.LastInsertId()

	gp, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID(%d) error: %v", id, err)
	}
	if gp == nil {
		t.Fatalf("GetByID(%d) returned nil", id)
	}
	if gp.Name != "〜てもいい" {
		t.Errorf("Name = %q, want %q", gp.Name, "〜てもいい")
	}
	if gp.JLPTLevel != grammar.LevelN5 {
		t.Errorf("JLPTLevel = %q, want N5", gp.JLPTLevel)
	}
	if len(gp.Examples) != 1 {
		t.Errorf("Examples len = %d, want 1", len(gp.Examples))
	}
	if len(gp.QuizQuestions) != 1 {
		t.Errorf("QuizQuestions len = %d, want 1", len(gp.QuizQuestions))
	}
}

func TestGrammarStore_ListByLevel(t *testing.T) {
	store := &GrammarStore{db: testDB}

	// 插入一个 N4 语法点
	_, err := testDB.Exec(`
		INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"〜たことがある", "曾经…", "动词た形", "表示经历",
		`[]`, `[]`, "N4",
	)
	if err != nil {
		t.Fatalf("insert N4 grammar_point error: %v", err)
	}

	gps, err := store.ListByLevel(grammar.LevelN4)
	if err != nil {
		t.Fatalf("ListByLevel(N4) error: %v", err)
	}
	if len(gps) == 0 {
		t.Error("ListByLevel(N4) returned empty slice")
	}
	for _, gp := range gps {
		if gp.JLPTLevel != grammar.LevelN4 {
			t.Errorf("grammar point %d has level %q, want N4", gp.ID, gp.JLPTLevel)
		}
	}
}

func TestGrammarStore_UpsertRecord(t *testing.T) {
	store := &GrammarStore{db: testDB}

	userID := int64(9010)
	_, _ = testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		userID, "grammartest@example.com", "hash", "N5",
	)

	// 先插入一条语法点
	res, _ := testDB.Exec(`
		INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level)
		VALUES ('test', 'test', '', '', '[]', '[]', 'N5')`)
	pointID, _ := res.LastInsertId()

	record := grammar.GrammarRecord{
		UserID:         userID,
		GrammarPointID: pointID,
		Status:         grammar.StatusLearning,
		NextReviewAt:   time.Now().Add(24 * time.Hour),
	}

	if err := store.UpsertRecord(record); err != nil {
		t.Fatalf("UpsertRecord error: %v", err)
	}

	// 更新状态
	record.Status = grammar.StatusMastered
	record.NextReviewAt = time.Now().Add(7 * 24 * time.Hour)
	if err := store.UpsertRecord(record); err != nil {
		t.Fatalf("UpsertRecord (update) error: %v", err)
	}

	got, err := store.GetRecord(userID, pointID)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if got.Status != grammar.StatusMastered {
		t.Errorf("Status = %q, want mastered", got.Status)
	}
}

func TestGrammarStore_ListDueRecords(t *testing.T) {
	store := &GrammarStore{db: testDB}

	userID := int64(9011)
	_, _ = testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		userID, "grammardue@example.com", "hash", "N4",
	)

	res1, _ := testDB.Exec(`INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level) VALUES ('due1','','','','[]','[]','N5')`)
	res2, _ := testDB.Exec(`INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level) VALUES ('future1','','','','[]','[]','N5')`)
	pointID1, _ := res1.LastInsertId()
	pointID2, _ := res2.LastInsertId()

	// 到期记录
	dueRec := grammar.GrammarRecord{
		UserID:         userID,
		GrammarPointID: pointID1,
		Status:         grammar.StatusLearning,
		NextReviewAt:   time.Now().Add(-2 * time.Hour),
	}
	// 未来记录
	futureRec := grammar.GrammarRecord{
		UserID:         userID,
		GrammarPointID: pointID2,
		Status:         grammar.StatusLearning,
		NextReviewAt:   time.Now().Add(48 * time.Hour),
	}
	_ = store.UpsertRecord(dueRec)
	_ = store.UpsertRecord(futureRec)

	due, err := store.ListDueRecords(userID)
	if err != nil {
		t.Fatalf("ListDueRecords error: %v", err)
	}

	for _, r := range due {
		if r.NextReviewAt.After(time.Now()) {
			t.Errorf("ListDueRecords returned future record: next_review_at=%v", r.NextReviewAt)
		}
	}

	found := false
	for _, r := range due {
		if r.UserID == userID && r.GrammarPointID == pointID1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListDueRecords did not return the due record")
	}
}

// The translation → grammar jump depends on ListAll's fuzzy search. Exercise it
// against a real SQLite database so SQL/ORDER BY/column issues are not hidden
// by the in-memory fake used in the grammar module tests.
func TestGrammarStore_ListAll_Search(t *testing.T) {
	store := &GrammarStore{db: testDB}

	fixtures := []struct{ name, meaning, level string }{
		{"ZZSEARCH-から", "检索甲", "N5"},
		{"ZZSEARCH-ことから", "检索乙", "N4"},
		{"ZZSEARCH-ばかり", "检索丙", "N3"},
	}
	for _, f := range fixtures {
		_, err := testDB.Exec(`
			INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, examples_json, quiz_questions_json, jlpt_level)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			f.name, f.meaning, "", "", "[]", "[]", f.level,
		)
		if err != nil {
			t.Fatalf("insert fixture %q: %v", f.name, err)
		}
	}
	t.Cleanup(func() {
		if _, err := testDB.Exec(`DELETE FROM grammar_points WHERE name LIKE 'ZZSEARCH-%'`); err != nil {
			t.Errorf("cleanup fixtures: %v", err)
		}
	})

	for _, tc := range []struct {
		name      string
		level     string
		search    string
		offset    int
		limit     int
		wantTotal int
		wantLen   int
		wantNames []string // nil skips the name assertion
	}{
		{"cross-level name search", "", "ZZSEARCH", 0, 50, 3, 3,
			[]string{"ZZSEARCH-から", "ZZSEARCH-ことから", "ZZSEARCH-ばかり"}},
		{"meaning search", "", "检索乙", 0, 50, 1, 1, []string{"ZZSEARCH-ことから"}},
		{"level filter", "N4", "ZZSEARCH", 0, 50, 1, 1, []string{"ZZSEARCH-ことから"}},
		{"pagination", "", "ZZSEARCH", 1, 1, 3, 1, nil},
		{"no match", "", "ZZSEARCH-存在しない", 0, 50, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			points, total, err := store.ListAll(tc.level, tc.search, tc.offset, tc.limit)
			if err != nil {
				t.Fatalf("ListAll error: %v", err)
			}
			if total != tc.wantTotal {
				t.Errorf("total = %d, want %d", total, tc.wantTotal)
			}
			if len(points) != tc.wantLen {
				t.Errorf("len(points) = %d, want %d", len(points), tc.wantLen)
			}
			if tc.wantNames == nil {
				return
			}
			var got []string
			for _, p := range points {
				got = append(got, p.Name)
			}
			slices.Sort(got)
			slices.Sort(tc.wantNames)
			if !slices.Equal(got, tc.wantNames) {
				t.Errorf("names = %v, want %v", got, tc.wantNames)
			}
		})
	}

	// End-to-end: service + real store, no fakes. Mirrors the two outcomes of
	// clicking a grammar tag in translation practice.
	svc := grammar.NewGrammarService(store)
	t.Run("service resolves an exact annotation to a detail page id", func(t *testing.T) {
		result, err := svc.SearchByName("ZZSEARCH-から")
		if err != nil {
			t.Fatalf("SearchByName error: %v", err)
		}
		if result.ExactMatchID <= 0 {
			t.Errorf("exact_match_id = %d, want a real grammar_points id", result.ExactMatchID)
		}
		if len(result.Points) != 1 || result.Points[0].Name != "ZZSEARCH-から" {
			t.Errorf("points = %+v, want only ZZSEARCH-から", result.Points)
		}
	})
	t.Run("service leaves an unknown annotation on the result list", func(t *testing.T) {
		result, err := svc.SearchByName("ZZSEARCH")
		if err != nil {
			t.Fatalf("SearchByName error: %v", err)
		}
		if result.ExactMatchID != 0 {
			t.Errorf("exact_match_id = %d, want 0", result.ExactMatchID)
		}
		if len(result.Points) != 3 {
			t.Errorf("points len = %d, want 3 related hits", len(result.Points))
		}
	})
}
