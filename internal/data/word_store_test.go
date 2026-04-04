package data

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/word"
)

func TestWordStore_GetByID(t *testing.T) {
	store := &WordStore{db: testDB}

	// 种子数据 002_seed.sql 插入了 60 条单词，id=1 应存在
	w, err := store.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID(1) unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("GetByID(1) returned nil word")
	}
	if w.ID != 1 {
		t.Errorf("GetByID(1).ID = %d, want 1", w.ID)
	}
	if w.KanjiForm == "" {
		t.Error("GetByID(1).KanjiForm is empty")
	}
	if w.JLPTLevel == "" {
		t.Error("GetByID(1).JLPTLevel is empty")
	}
}

func TestWordStore_GetByID_NotFound(t *testing.T) {
	store := &WordStore{db: testDB}

	w, err := store.GetByID(999999)
	if err == nil {
		t.Fatal("GetByID(999999) expected error, got nil")
	}
	if w != nil {
		t.Errorf("GetByID(999999) expected nil word, got %+v", w)
	}
}

func TestWordStore_ListByLevel(t *testing.T) {
	store := &WordStore{db: testDB}

	tests := []struct {
		level     word.JLPTLevel
		wantCount int // 种子数据：N5=30, N4=30
	}{
		{word.LevelN5, 30},
		{word.LevelN4, 30},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			words, total, err := store.ListByLevel(tt.level, 1, 100)
			if err != nil {
				t.Fatalf("ListByLevel(%s) unexpected error: %v", tt.level, err)
			}
			if total != tt.wantCount {
				t.Errorf("ListByLevel(%s) total = %d, want %d", tt.level, total, tt.wantCount)
			}
			if len(words) != tt.wantCount {
				t.Errorf("ListByLevel(%s) len = %d, want %d", tt.level, len(words), tt.wantCount)
			}
			for _, w := range words {
				if w.JLPTLevel != tt.level {
					t.Errorf("word %d has level %s, want %s", w.ID, w.JLPTLevel, tt.level)
				}
			}
		})
	}
}

func TestWordStore_ListByLevel_Pagination(t *testing.T) {
	store := &WordStore{db: testDB}

	// 第一页：size=10
	page1, total, err := store.ListByLevel(word.LevelN5, 1, 10)
	if err != nil {
		t.Fatalf("ListByLevel page=1 size=10 error: %v", err)
	}
	if total != 30 {
		t.Errorf("total = %d, want 30", total)
	}
	if len(page1) != 10 {
		t.Errorf("page1 len = %d, want 10", len(page1))
	}

	// 第二页
	page2, _, err := store.ListByLevel(word.LevelN5, 2, 10)
	if err != nil {
		t.Fatalf("ListByLevel page=2 error: %v", err)
	}
	if len(page2) != 10 {
		t.Errorf("page2 len = %d, want 10", len(page2))
	}

	// 两页 ID 不重叠
	ids1 := make(map[int64]bool)
	for _, w := range page1 {
		ids1[w.ID] = true
	}
	for _, w := range page2 {
		if ids1[w.ID] {
			t.Errorf("word %d appears in both page1 and page2", w.ID)
		}
	}
}

func TestWordStore_UpsertRecord(t *testing.T) {
	store := &WordStore{db: testDB}

	// 使用固定的测试用户 ID 和单词 ID（避免外键冲突，先创建用户）
	userID := int64(9001)
	wordID := int64(1)

	// 插入一条测试用户（跳过外键；测试 DB 设置了 foreign_keys=ON，需实际用户）
	// 使用 user_store 直接插入
	_, err := testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		userID, "wordtest@example.com", "hash", "N5",
	)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	record := word.WordRecord{
		UserID:       userID,
		WordID:       wordID,
		MasteryLevel: 1,
		NextReviewAt: time.Now().Add(24 * time.Hour),
		EaseFactor:   2.5,
		Interval:     1,
		UpdatedAt:    time.Now(),
	}

	if err := store.UpsertRecord(record); err != nil {
		t.Fatalf("UpsertRecord error: %v", err)
	}

	// 再次 upsert（更新）
	record.MasteryLevel = 2
	record.EaseFactor = 2.6
	if err := store.UpsertRecord(record); err != nil {
		t.Fatalf("UpsertRecord (update) error: %v", err)
	}

	// 读取并验证
	got, err := store.GetRecord(userID, wordID)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if got.MasteryLevel != 2 {
		t.Errorf("MasteryLevel = %d, want 2", got.MasteryLevel)
	}
}

func TestWordStore_ListDueRecords(t *testing.T) {
	store := &WordStore{db: testDB}

	userID := int64(9002)
	_, _ = testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		userID, "due@example.com", "hash", "N5",
	)

	// 插入一条已到期记录和一条未来记录
	dueRecord := word.WordRecord{
		UserID:       userID,
		WordID:       1,
		MasteryLevel: 0,
		NextReviewAt: time.Now().Add(-1 * time.Hour), // 已到期
		EaseFactor:   2.5,
		Interval:     0,
		UpdatedAt:    time.Now(),
	}
	futureRecord := word.WordRecord{
		UserID:       userID,
		WordID:       2,
		MasteryLevel: 3,
		NextReviewAt: time.Now().Add(7 * 24 * time.Hour), // 未来
		EaseFactor:   2.8,
		Interval:     7,
		UpdatedAt:    time.Now(),
	}

	_ = store.UpsertRecord(dueRecord)
	_ = store.UpsertRecord(futureRecord)

	due, err := store.ListDueRecords(userID, 10)
	if err != nil {
		t.Fatalf("ListDueRecords error: %v", err)
	}

	for _, r := range due {
		if r.NextReviewAt.After(time.Now()) {
			t.Errorf("ListDueRecords returned future record: next_review_at=%v", r.NextReviewAt)
		}
	}

	// 至少包含刚插入的那条到期记录
	found := false
	for _, r := range due {
		if r.UserID == userID && r.WordID == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListDueRecords did not return the due record")
	}
}

func TestWordStore_BookmarkWord(t *testing.T) {
	store := &WordStore{db: testDB}

	userID := int64(9003)
	_, _ = testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		userID, "bookmark@example.com", "hash", "N5",
	)

	// 收藏单词
	if err := store.BookmarkWord(userID, 1); err != nil {
		t.Fatalf("BookmarkWord error: %v", err)
	}

	// 幂等：重复收藏不报错
	if err := store.BookmarkWord(userID, 1); err != nil {
		t.Fatalf("BookmarkWord (idempotent) error: %v", err)
	}

	// 验证收藏已持久化
	var count int
	err := testDB.QueryRow(
		`SELECT COUNT(*) FROM word_bookmarks WHERE user_id=? AND word_id=?`, userID, 1,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query bookmark count error: %v", err)
	}
	if count != 1 {
		t.Errorf("bookmark count = %d, want 1", count)
	}
}
