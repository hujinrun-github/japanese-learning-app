package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/word"
)

func TestLessonStoreListSummaries(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresLessonStoreForDB(db)

	_ = insertLessonFixture(t, db, lessonFixtureInput{
		Title:     "N5 Lesson",
		Level:     lesson.LevelN5,
		Tags:      []string{"greetings", "daily"},
		CharCount: 12,
	})
	_ = insertLessonFixture(t, db, lessonFixtureInput{
		Title:     "N4 Lesson",
		Level:     lesson.LevelN4,
		Tags:      []string{"work"},
		CharCount: 18,
	})

	summaries, err := store.ListSummaries(lesson.LevelN5)
	if err != nil {
		t.Fatalf("ListSummaries() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("ListSummaries() len = %d, want 1", len(summaries))
	}
	if summaries[0].Title != "N5 Lesson" || summaries[0].JLPTLevel != lesson.LevelN5 {
		t.Fatalf("ListSummaries() summary = %+v, want N5 lesson", summaries[0])
	}
}

func TestLessonStoreGetDetail(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresLessonStoreForDB(db)

	lessonID := insertLessonFixture(t, db, lessonFixtureInput{
		Title:            "Detail Lesson",
		Level:            lesson.LevelN5,
		Tags:             []string{"detail"},
		CharCount:        20,
		AudioURL:         "/audio/lessons/detail.wav",
		ShadowingEnabled: true,
		ShadowingVersion: 2,
		ShadowingConfig:  map[string]any{"media_url": "/audio/lessons/detail.wav", "loop_count": float64(3)},
		Sentences: []lesson.Sentence{
			{
				Index: 0,
				Tokens: []lesson.FuriganaToken{
					{Surface: "日本語", Reading: "にほんご"},
				},
				Chinese: "日语",
				StartMS: 0,
				EndMS:   3000,
			},
		},
		WordIDs: []int64{11, 22},
	})

	detail, err := store.GetDetail(lessonID)
	if err != nil {
		t.Fatalf("GetDetail() error = %v", err)
	}
	if detail == nil {
		t.Fatal("GetDetail() returned nil")
	}
	if detail.ID != lessonID {
		t.Fatalf("GetDetail() ID = %d, want %d", detail.ID, lessonID)
	}
	if len(detail.Sentences) != 1 || len(detail.WordIDs) != 2 {
		t.Fatalf("GetDetail() = %+v, want sentences and word IDs", detail)
	}
	if !detail.ShadowingEnabled {
		t.Fatal("GetDetail() ShadowingEnabled = false, want true")
	}
	if detail.ShadowingVersion != 2 {
		t.Fatalf("GetDetail() ShadowingVersion = %d, want 2", detail.ShadowingVersion)
	}
	if detail.AudioURL != "/audio/lessons/detail.wav" {
		t.Fatalf("GetDetail() AudioURL = %q, want media URL from shadowing config", detail.AudioURL)
	}
	if detail.ShadowingConfig["loop_count"] != float64(3) {
		t.Fatalf("GetDetail() ShadowingConfig = %+v, want loop_count", detail.ShadowingConfig)
	}
	if detail.Sentences[0].Tokens[0].Reading != "にほんご" {
		t.Fatalf("GetDetail() tokens = %+v, want furigana tokens", detail.Sentences[0].Tokens)
	}
}

func TestLessonStoreGetDetailNotFound(t *testing.T) {
	store := newPostgresLessonStoreForTest(t)

	detail, err := store.GetDetail(999999)
	if err == nil {
		t.Fatal("GetDetail() error = nil, want not found")
	}
	if detail != nil {
		t.Fatalf("GetDetail() detail = %+v, want nil", detail)
	}
}

func TestLessonStoreListSummariesIncludesShadowingMetadata(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresLessonStoreForDB(db)

	_ = insertLessonFixture(t, db, lessonFixtureInput{
		Title:            "Shadowing Summary Lesson",
		Level:            lesson.LevelN5,
		Tags:             []string{"shadowing"},
		CharCount:        12,
		AudioURL:         "/audio/lessons/summary.wav",
		ShadowingEnabled: true,
		ShadowingVersion: 4,
		ShadowingConfig:  map[string]any{"media_url": "/audio/lessons/summary.wav"},
	})

	summaries, err := store.ListSummaries(lesson.LevelN5)
	if err != nil {
		t.Fatalf("ListSummaries() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("ListSummaries() len = %d, want 1", len(summaries))
	}

	got := summaries[0]
	if !got.ShadowingEnabled {
		t.Fatal("ListSummaries() ShadowingEnabled = false, want true")
	}
	if got.ShadowingVersion != 4 {
		t.Fatalf("ListSummaries() ShadowingVersion = %d, want 4", got.ShadowingVersion)
	}
	if got.AudioURL != "/audio/lessons/summary.wav" {
		t.Fatalf("ListSummaries() AudioURL = %q, want media URL from shadowing config", got.AudioURL)
	}
}

func TestLessonStoreGetSentences(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresLessonStoreForDB(db)

	lessonID := insertLessonFixture(t, db, lessonFixtureInput{
		Title:     "Sentence Lesson",
		Level:     lesson.LevelN4,
		CharCount: 15,
		Sentences: []lesson.Sentence{
			{
				Index: 1,
				Tokens: []lesson.FuriganaToken{
					{Surface: "午後", Reading: "ごご"},
				},
				Chinese: "下午",
				StartMS: 3000,
				EndMS:   6000,
			},
			{
				Index: 0,
				Tokens: []lesson.FuriganaToken{
					{Surface: "午前", Reading: "ごぜん"},
				},
				Chinese: "上午",
				StartMS: 0,
				EndMS:   3000,
			},
		},
	})

	sentences, err := store.GetSentences(lessonID)
	if err != nil {
		t.Fatalf("GetSentences() error = %v", err)
	}
	if len(sentences) != 2 {
		t.Fatalf("GetSentences() len = %d, want 2", len(sentences))
	}
	if sentences[0].Index != 0 || sentences[1].Index != 1 {
		t.Fatalf("GetSentences() order = %+v, want index order", sentences)
	}
}

type lessonFixtureInput struct {
	Title            string
	Level            lesson.JLPTLevel
	Tags             []string
	CharCount        int
	AudioURL         string
	ShadowingEnabled bool
	ShadowingVersion int
	ShadowingConfig  map[string]any
	Sentences        []lesson.Sentence
	WordIDs          []int64
}

func insertLessonFixture(t *testing.T, db queryer, input lessonFixtureInput) int64 {
	t.Helper()

	ctx := context.Background()

	for _, wordID := range input.WordIDs {
		_, err := db.ExecContext(ctx,
			`INSERT INTO words (id, kanji_form, reading, part_of_speech, meaning, jlpt_level, reading_type, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
			wordID,
			fmt.Sprintf("単語%d", wordID),
			fmt.Sprintf("たんご%d", wordID),
			"名词",
			"单词",
			word.LevelN5,
			"",
		)
		if err != nil {
			t.Fatalf("insert lesson fixture word %d: %v", wordID, err)
		}
	}

	tagsJSON, err := json.Marshal(normalizeStringSlice(input.Tags))
	if err != nil {
		t.Fatalf("marshal tags: %v", err)
	}
	shadowingVersion := input.ShadowingVersion
	if shadowingVersion == 0 {
		shadowingVersion = 1
	}
	shadowingConfig := input.ShadowingConfig
	if shadowingConfig == nil {
		shadowingConfig = map[string]any{}
	}
	if input.AudioURL != "" {
		if _, ok := shadowingConfig["media_url"]; !ok {
			shadowingConfig["media_url"] = input.AudioURL
		}
	}
	shadowingConfigJSON, err := json.Marshal(shadowingConfig)
	if err != nil {
		t.Fatalf("marshal shadowing config: %v", err)
	}

	var lessonID int64
	err = db.QueryRowContext(
		ctx,
		`INSERT INTO lessons (title, jlpt_level, tags, char_count, shadowing_enabled, shadowing_version, shadowing_config_json, updated_at)
		 VALUES (
		     $1,
		     $2,
		     COALESCE((SELECT array_agg(value) FROM jsonb_array_elements_text($3::jsonb) AS value), ARRAY[]::text[]),
		     $4,
		     $5,
		     $6,
		     $7::jsonb,
		     now()
		 )
		 RETURNING id`,
		input.Title,
		input.Level,
		string(tagsJSON),
		input.CharCount,
		input.ShadowingEnabled,
		shadowingVersion,
		string(shadowingConfigJSON),
	).Scan(&lessonID)
	if err != nil {
		t.Fatalf("insert lesson fixture lesson: %v", err)
	}

	for _, sentence := range input.Sentences {
		tokensJSON, err := json.Marshal(sentence.Tokens)
		if err != nil {
			t.Fatalf("marshal lesson tokens: %v", err)
		}
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO lesson_sentences (lesson_id, position, tokens, chinese, start_ms, end_ms)
			 VALUES ($1, $2, $3::jsonb, $4, $5, $6)`,
			lessonID,
			sentence.Index,
			string(tokensJSON),
			sentence.Chinese,
			sentence.StartMS,
			sentence.EndMS,
		); err != nil {
			t.Fatalf("insert lesson sentence: %v", err)
		}
	}

	for idx, wordID := range input.WordIDs {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO lesson_words (lesson_id, word_id, position) VALUES ($1, $2, $3)`,
			lessonID,
			wordID,
			idx,
		); err != nil {
			t.Fatalf("insert lesson word: %v", err)
		}
	}

	return lessonID
}
