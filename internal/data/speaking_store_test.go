package data

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
)

func TestSpeakingStore_SaveAndGetRecord(t *testing.T) {
	store := &SpeakingStore{db: testDB}

	insertTestUser(t, 9200, "speaking_save@example.com")

	// 先插入一条 speaking_material
	res, err := testDB.Exec(`
		INSERT INTO speaking_materials (type, title, text, audio_url, jlpt_level)
		VALUES (?, ?, ?, ?, ?)`,
		"shadow", "Test Material", "日本語のテキスト", "https://example.com/audio.mp3", "N5",
	)
	if err != nil {
		t.Fatalf("insert speaking_material error: %v", err)
	}
	materialID, _ := res.LastInsertId()

	record := speaking.SpeakingRecord{
		UserID:      9200,
		Type:        speaking.PracticeTypeShadow,
		MaterialID:  materialID,
		Score:       85,
		AudioRef:    "/data/audio/test.webm",
		PracticedAt: time.Now(),
	}

	if err := store.SaveRecord(record); err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}

	// 查询验证
	var count int
	err = testDB.QueryRow(
		`SELECT COUNT(*) FROM speaking_records WHERE user_id=? AND material_id=?`,
		9200, materialID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query count error: %v", err)
	}
	if count != 1 {
		t.Errorf("speaking_records count = %d, want 1", count)
	}
}

func TestSpeakingStore_ListRecords_OrderedByPracticedAt(t *testing.T) {
	store := &SpeakingStore{db: testDB}

	insertTestUser(t, 9201, "speaking_list@example.com")

	// 插入 speaking_material
	res, err := testDB.Exec(`
		INSERT INTO speaking_materials (type, title, text, audio_url, jlpt_level)
		VALUES ('free', 'List Test', 'text', '', 'N4')`)
	if err != nil {
		t.Fatalf("insert speaking_material error: %v", err)
	}
	materialID, _ := res.LastInsertId()

	now := time.Now()
	for i := 0; i < 3; i++ {
		r := speaking.SpeakingRecord{
			UserID:      9201,
			Type:        speaking.PracticeTypeFree,
			MaterialID:  materialID,
			Score:       60 + i*10,
			AudioRef:    "/data/audio/test.webm",
			PracticedAt: now.Add(time.Duration(i) * time.Hour),
		}
		if err := store.SaveRecord(r); err != nil {
			t.Fatalf("SaveRecord %d error: %v", i, err)
		}
	}

	records, err := store.ListRecords(9201)
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

func TestSpeakingStore_GetRecord(t *testing.T) {
	store := &SpeakingStore{db: testDB}

	insertTestUser(t, 9202, "speaking_get@example.com")

	res, err := testDB.Exec(`
		INSERT INTO speaking_materials (type, title, text, audio_url, jlpt_level)
		VALUES ('shadow', 'Get Test', 'text', '', 'N5')`)
	if err != nil {
		t.Fatalf("insert speaking_material error: %v", err)
	}
	materialID, _ := res.LastInsertId()

	record := speaking.SpeakingRecord{
		UserID:      9202,
		Type:        speaking.PracticeTypeShadow,
		MaterialID:  materialID,
		Score:       72,
		AudioRef:    "/data/audio/get_test.webm",
		PracticedAt: time.Now(),
	}
	if err := store.SaveRecord(record); err != nil {
		t.Fatalf("SaveRecord error: %v", err)
	}

	// 获取刚插入的 ID
	var id int64
	err = testDB.QueryRow(
		`SELECT id FROM speaking_records WHERE user_id=? AND material_id=? ORDER BY id DESC LIMIT 1`,
		9202, materialID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("query id error: %v", err)
	}

	got, err := store.GetRecord(id)
	if err != nil {
		t.Fatalf("GetRecord(%d) error: %v", id, err)
	}
	if got == nil {
		t.Fatal("GetRecord returned nil")
	}
	if got.Score != 72 {
		t.Errorf("Score = %d, want 72", got.Score)
	}
	if got.AudioRef != "/data/audio/get_test.webm" {
		t.Errorf("AudioRef = %q, want /data/audio/get_test.webm", got.AudioRef)
	}
}
