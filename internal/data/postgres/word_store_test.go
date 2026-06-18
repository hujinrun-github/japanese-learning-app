package postgres

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

func TestWordStoreGetByID(t *testing.T) {
	store := newPostgresWordStoreForTest(t)

	wordID, err := store.InsertWord(word.Word{
		KanjiForm:    "勉強",
		Reading:      "べんきょう",
		PartOfSpeech: "名词",
		Meaning:      "学习",
		JLPTLevel:    word.LevelN5,
		ReadingType:  "2",
		Examples: []word.WordExample{
			{Japanese: "日本語を勉強します。", Chinese: "学习日语。", FuriganaHTML: "にほんごをべんきょうします。"},
		},
	})
	if err != nil {
		t.Fatalf("InsertWord() error = %v", err)
	}

	got, err := store.GetByID(wordID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByID() returned nil word")
	}
	if got.ID != wordID {
		t.Fatalf("GetByID() ID = %d, want %d", got.ID, wordID)
	}
	if got.ReadingType != "2" {
		t.Fatalf("GetByID() reading type = %q, want %q", got.ReadingType, "2")
	}
	if len(got.Examples) != 1 || got.Examples[0].FuriganaHTML == "" {
		t.Fatalf("GetByID() examples = %+v, want furigana_html preserved", got.Examples)
	}
}

func TestWordStoreListByLevel(t *testing.T) {
	store := newPostgresWordStoreForTest(t)

	_, err := store.InsertWord(word.Word{
		KanjiForm: "水", Reading: "みず", Meaning: "水", JLPTLevel: word.LevelN5,
	})
	if err != nil {
		t.Fatalf("InsertWord(N5) error = %v", err)
	}
	_, err = store.InsertWord(word.Word{
		KanjiForm: "準備", Reading: "じゅんび", Meaning: "准备", JLPTLevel: word.LevelN4,
	})
	if err != nil {
		t.Fatalf("InsertWord(N4) error = %v", err)
	}

	words, err := store.ListByLevel(word.LevelN5)
	if err != nil {
		t.Fatalf("ListByLevel() error = %v", err)
	}
	if len(words) != 1 {
		t.Fatalf("ListByLevel() len = %d, want 1", len(words))
	}
	if words[0].JLPTLevel != word.LevelN5 {
		t.Fatalf("ListByLevel() level = %s, want %s", words[0].JLPTLevel, word.LevelN5)
	}
}

func TestWordStoreUpsertRecordAndListDueRecords(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresWordStoreForDB(db)
	userStore := newPostgresUserStoreForDB(db)

	createdUser, err := userStore.CreateUser(user.User{
		Name:  "Word Reviewer",
		Email: "reviewer@example.com",
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	wordID, err := store.InsertWord(word.Word{
		KanjiForm: "今日", Reading: "きょう", Meaning: "今天", JLPTLevel: word.LevelN5,
	})
	if err != nil {
		t.Fatalf("InsertWord() error = %v", err)
	}

	err = store.UpsertRecord(word.WordRecord{
		UserID:        createdUser.ID,
		WordID:        wordID,
		MasteryLevel:  2,
		NextReviewAt:  time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC),
		EaseFactor:    2.6,
		Interval:      3,
		ReviewHistory: nil,
		UpdatedAt:     time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertRecord() error = %v", err)
	}

	record, err := store.GetRecord(createdUser.ID, wordID)
	if err != nil {
		t.Fatalf("GetRecord() error = %v", err)
	}
	if record == nil || record.MasteryLevel != 2 {
		t.Fatalf("GetRecord() record = %+v, want mastery level 2", record)
	}

	dueRecords, err := store.ListDueRecords(createdUser.ID)
	if err != nil {
		t.Fatalf("ListDueRecords() error = %v", err)
	}
	if len(dueRecords) != 1 {
		t.Fatalf("ListDueRecords() len = %d, want 1", len(dueRecords))
	}
}

func TestWordStoreBookmarkWordIsIdempotent(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresWordStoreForDB(db)
	userStore := newPostgresUserStoreForDB(db)

	createdUser, err := userStore.CreateUser(user.User{
		Name:  "Bookmark User",
		Email: "bookmark@example.com",
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	wordID, err := store.InsertWord(word.Word{
		KanjiForm: "猫", Reading: "ねこ", Meaning: "猫", JLPTLevel: word.LevelN5,
	})
	if err != nil {
		t.Fatalf("InsertWord() error = %v", err)
	}

	if err := store.BookmarkWord(createdUser.ID, wordID); err != nil {
		t.Fatalf("BookmarkWord(first) error = %v", err)
	}
	if err := store.BookmarkWord(createdUser.ID, wordID); err != nil {
		t.Fatalf("BookmarkWord(second) error = %v", err)
	}
}

func TestWordStoreAdminCRUDAndListAllRecords(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresWordStoreForDB(db)
	userStore := newPostgresUserStoreForDB(db)

	wordID, err := store.InsertWord(word.Word{
		KanjiForm:    "出発",
		Reading:      "しゅっぱつ",
		PartOfSpeech: "名词",
		Meaning:      "出发",
		JLPTLevel:    word.LevelN4,
		ReadingType:  "1",
		Examples: []word.WordExample{
			{Japanese: "明日出発します。", Chinese: "明天出发。"},
		},
	})
	if err != nil {
		t.Fatalf("InsertWord() error = %v", err)
	}

	items, total, err := store.ListAll(word.LevelN4, "出発", 0, 20)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("ListAll() total/len = %d/%d, want 1/1", total, len(items))
	}

	err = store.UpdateWord(word.Word{
		ID:           wordID,
		KanjiForm:    "出発",
		Reading:      "しゅっぱつ",
		PartOfSpeech: "名词",
		Meaning:      "启程",
		JLPTLevel:    word.LevelN4,
		ReadingType:  "3",
		Examples: []word.WordExample{
			{Japanese: "午後に出発します。", Chinese: "下午启程。", FuriganaHTML: "ごごにしゅっぱつします。"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateWord() error = %v", err)
	}

	updated, err := store.GetByID(wordID)
	if err != nil {
		t.Fatalf("GetByID(updated) error = %v", err)
	}
	if updated.Meaning != "启程" || updated.ReadingType != "3" {
		t.Fatalf("GetByID(updated) = %+v, want updated meaning and reading type", updated)
	}

	createdUser, err := userStore.CreateUser(user.User{
		Name:  "Admin Records User",
		Email: "records@example.com",
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := store.UpsertRecord(word.WordRecord{
		UserID:       createdUser.ID,
		WordID:       wordID,
		MasteryLevel: 1,
		NextReviewAt: time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC),
		EaseFactor:   2.5,
		Interval:     1,
		UpdatedAt:    time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertRecord() error = %v", err)
	}

	records, total, err := store.ListAllRecords(createdUser.ID, 0, 20)
	if err != nil {
		t.Fatalf("ListAllRecords() error = %v", err)
	}
	if total != 1 || len(records) != 1 {
		t.Fatalf("ListAllRecords() total/len = %d/%d, want 1/1", total, len(records))
	}

	if err := store.DeleteWord(wordID); err != nil {
		t.Fatalf("DeleteWord() error = %v", err)
	}
}
