package postgres

import (
	"testing"

	"japanese-learning-app/internal/module/word"
)

func TestWordStoreVerbTypeRoundTrip(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresWordStoreForDB(db)

	tests := []struct {
		name    string
		kanji   string
		initial string
		updated string
	}{
		{name: "transitive", kanji: "開ける", initial: word.VerbTypeTransitive, updated: word.VerbTypeIntransitive},
		{name: "intransitive", kanji: "開く", initial: word.VerbTypeIntransitive, updated: word.VerbTypeBoth},
		{name: "both", kanji: "吹く", initial: word.VerbTypeBoth, updated: word.VerbTypeTransitive},
		{name: "default empty then filled", kanji: "猫", initial: "", updated: word.VerbTypeTransitive},
		{name: "cleared", kanji: "犬", initial: word.VerbTypeBoth, updated: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := store.InsertWord(word.Word{
				KanjiForm: tt.kanji,
				Reading:   "てすと",
				Meaning:   "测试",
				JLPTLevel: word.LevelN5,
				VerbType:  tt.initial,
			})
			if err != nil {
				t.Fatalf("InsertWord() error = %v", err)
			}

			got, err := store.GetByID(id)
			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}
			if got.VerbType != tt.initial {
				t.Fatalf("GetByID().VerbType = %q, want %q", got.VerbType, tt.initial)
			}

			items, _, err := store.ListAll("", "", 0, 100)
			if err != nil {
				t.Fatalf("ListAll() error = %v", err)
			}
			var listed *word.Word
			for i := range items {
				if items[i].ID == id {
					listed = &items[i]
					break
				}
			}
			if listed == nil {
				t.Fatalf("ListAll() did not return word %d", id)
			}
			if listed.VerbType != tt.initial {
				t.Fatalf("ListAll().VerbType = %q, want %q", listed.VerbType, tt.initial)
			}

			got.VerbType = tt.updated
			if err := store.UpdateWord(*got); err != nil {
				t.Fatalf("UpdateWord() error = %v", err)
			}
			after, err := store.GetByID(id)
			if err != nil {
				t.Fatalf("GetByID() after update error = %v", err)
			}
			if after.VerbType != tt.updated {
				t.Fatalf("GetByID().VerbType after update = %q, want %q", after.VerbType, tt.updated)
			}
		})
	}
}
