package data

import (
	"testing"

	"japanese-learning-app/internal/module/lesson"
)

func insertTestLesson(t *testing.T, title string, level lesson.JLPTLevel, tags string) int64 {
	t.Helper()
	res, err := testDB.Exec(`
		INSERT INTO lessons (title, content_furigana_json, translation_json, jlpt_level, tags_json, audio_url, sentence_timestamps_json, char_count, word_ids_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title,
		`[{"index":0,"tokens":[{"surface":"日本語","reading":"にほんご"}],"chinese":"日语","start_ms":0,"end_ms":3000}]`,
		`[]`,
		string(level),
		tags,
		"https://example.com/audio.mp3",
		`[]`,
		20,
		`[1,2,3]`,
	)
	if err != nil {
		t.Fatalf("insertTestLesson error: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestLessonStore_ListSummaries_ByLevel(t *testing.T) {
	store := &LessonStore{db: testDB}

	insertTestLesson(t, "N5 Lesson A", lesson.LevelN5, `["greetings","daily"]`)
	insertTestLesson(t, "N5 Lesson B", lesson.LevelN5, `["travel"]`)
	insertTestLesson(t, "N4 Lesson A", lesson.LevelN4, `["work"]`)

	n5s, err := store.ListSummaries(lesson.LevelN5, "")
	if err != nil {
		t.Fatalf("ListSummaries(N5, \"\") error: %v", err)
	}
	for _, s := range n5s {
		if s.JLPTLevel != lesson.LevelN5 {
			t.Errorf("ListSummaries(N5) returned lesson with level %q", s.JLPTLevel)
		}
	}
	if len(n5s) < 2 {
		t.Errorf("ListSummaries(N5) len = %d, want >= 2", len(n5s))
	}
}

func TestLessonStore_ListSummaries_ByTag(t *testing.T) {
	store := &LessonStore{db: testDB}

	insertTestLesson(t, "N5 Tagged", lesson.LevelN5, `["tagfilter"]`)
	insertTestLesson(t, "N5 No Tag", lesson.LevelN5, `["other"]`)

	tagged, err := store.ListSummaries(lesson.LevelN5, "tagfilter")
	if err != nil {
		t.Fatalf("ListSummaries(N5, tagfilter) error: %v", err)
	}
	for _, s := range tagged {
		// 每个返回的 lesson 应包含该 tag
		found := false
		for _, tag := range s.Tags {
			if tag == "tagfilter" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("lesson %d (%q) does not have tag 'tagfilter'", s.ID, s.Title)
		}
	}
}

func TestLessonStore_GetDetail(t *testing.T) {
	store := &LessonStore{db: testDB}

	id := insertTestLesson(t, "Detail Test", lesson.LevelN5, `["test"]`)

	detail, err := store.GetDetail(id)
	if err != nil {
		t.Fatalf("GetDetail(%d) error: %v", id, err)
	}
	if detail == nil {
		t.Fatalf("GetDetail(%d) returned nil", id)
	}
	if detail.ID != id {
		t.Errorf("GetDetail ID = %d, want %d", detail.ID, id)
	}
	if len(detail.Sentences) == 0 {
		t.Error("GetDetail returned lesson with no sentences")
	}
	if len(detail.WordIDs) == 0 {
		t.Error("GetDetail returned lesson with no WordIDs")
	}
}

func TestLessonStore_GetDetail_NotFound(t *testing.T) {
	store := &LessonStore{db: testDB}

	detail, err := store.GetDetail(999999)
	if err == nil {
		t.Fatal("GetDetail(999999) expected error, got nil")
	}
	if detail != nil {
		t.Errorf("GetDetail(999999) expected nil, got %+v", detail)
	}
}

func TestLessonStore_GetSentences(t *testing.T) {
	store := &LessonStore{db: testDB}

	id := insertTestLesson(t, "Sentences Test", lesson.LevelN4, `["sentences"]`)

	sentences, err := store.GetSentences(id)
	if err != nil {
		t.Fatalf("GetSentences(%d) error: %v", id, err)
	}
	if len(sentences) == 0 {
		t.Error("GetSentences returned empty slice")
	}
	// Verify sentences are in index order
	for i := 1; i < len(sentences); i++ {
		if sentences[i].Index < sentences[i-1].Index {
			t.Errorf("GetSentences not ordered: index[%d]=%d, index[%d]=%d",
				i-1, sentences[i-1].Index, i, sentences[i].Index)
		}
	}
}
