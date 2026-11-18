package cli

import (
	"strings"
	"testing"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/word"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

func TestPostgresImportPreservesAudioAndStoresExamples(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	json := `[{"kanji_form":"猫","reading":"ねこ","meaning":"猫","jlpt_level":"N5","examples":[{"japanese":"猫です。","chinese":"是猫。"}]}]`
	if _, err := ImportWords(db, strings.NewReader(json)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE words SET local_audio_url='/audio/words/cat.wav'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportWords(db, strings.NewReader(json)); err != nil {
		t.Fatal(err)
	}
	s := pgdata.NewWordStore(db, store.StoreDeps{})
	items, err := s.ListByLevel(word.LevelN5)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AudioURL != "/audio/words/cat.wav" || len(items[0].Examples) != 1 || items[0].Examples[0].Chinese != "是猫。" {
		t.Fatalf("imported words=%+v", items)
	}
}
