package data

import (
	"context"
	"testing"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/testdb"
)

func TestSpeakingLessonsLeaveReadingCatalogAndKeepSource(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	_, err := db.Exec(`INSERT INTO speaking_materials (id, type, title, text, jlpt_level)
		VALUES (1, 'shadow', '駅で', '駅はどこですか。あちらです。', 'N5');
		INSERT INTO lessons (id, title, jlpt_level, speaking_material_id) VALUES
		(1, '私の一日', 'N5', NULL), (2, '駅で', 'N5', 1);
		INSERT INTO lesson_sentences (lesson_id, position, chinese) VALUES (2, 0, '车站对话')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, search string
		want         int
	}{
		{"all reading", "", 1},
		{"dialogue search", "駅で", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := NewLessonCatalog(db).ListCatalog(context.Background(), lesson.CatalogQuery{Page: 1, Size: 20, Search: tt.search})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != tt.want || len(page.Items) != tt.want {
				t.Fatalf("catalog = %+v, want %d readings", page, tt.want)
			}
		})
	}
	store := pgdata.NewLessonStore(db)
	items, err := store.ListSummaries(lesson.LevelN5)
	if err != nil || len(items) != 1 || items[0].ID != 1 {
		t.Fatalf("legacy reading list = %+v, err = %v", items, err)
	}
	source, err := store.GetDetail(2)
	if err != nil || len(source.Sentences) != 1 || source.Sentences[0].Chinese != "车站对话" {
		t.Fatalf("preserved dialogue source = %+v, err = %v", source, err)
	}
}
