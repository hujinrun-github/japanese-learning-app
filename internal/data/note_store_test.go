package data

import (
	"testing"

	"japanese-learning-app/internal/module/note"
)

func TestNoteStore_CreateAndGetByID(t *testing.T) {
	insertTestUser(t, 1, "note_test@example.com")

	store := NewNoteStore(testDB)
	n := &note.Note{
		UserID:     1,
		Type:       note.TypeWord,
		Title:      "雨",
		Content:    "あめ、雨。**音读**：う。",
		SourceText: "雨が降っている",
		Tags:       []string{"N5", "天气"},
	}

	err := store.Create(n)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if n.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	got, err := store.GetByID(1, n.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Title != "雨" {
		t.Errorf("Title = %q, want %q", got.Title, "雨")
	}
	if got.Type != note.TypeWord {
		t.Errorf("Type = %q, want %q", got.Type, note.TypeWord)
	}
	if got.Content != "あめ、雨。**音读**：う。" {
		t.Errorf("Content = %q", got.Content)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(got.Tags))
	}
	if got.MasteryLevel != 0 {
		t.Errorf("MasteryLevel = %d, want 0", got.MasteryLevel)
	}
	if got.NextReviewAt != nil {
		t.Error("NextReviewAt should be nil for new note")
	}
	if got.EaseFactor != 2.5 {
		t.Errorf("EaseFactor = %f, want 2.5", got.EaseFactor)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestNoteStore_List(t *testing.T) {
	insertTestUser(t, 2, "note_list@example.com")
	insertTestUser(t, 3, "other_user@example.com")

	db := testDB
	store := NewNoteStore(db)

	// Create test notes
	n1 := &note.Note{UserID: 2, Type: note.TypeWord, Title: "雨", Content: "ame", Tags: []string{"N5", "天气"}}
	n2 := &note.Note{UserID: 2, Type: note.TypeGrammar, Title: "～ている", Content: "持续体", Tags: []string{"N5"}}
	n3 := &note.Note{UserID: 3, Type: note.TypeWord, Title: "other user", Content: "should not appear"}
	for _, n := range []*note.Note{n1, n2, n3} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List all for user 2
	t.Run("list all", func(t *testing.T) {
		notes, total, err := store.List(2, note.NoteListParams{Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
		if len(notes) != 2 {
			t.Fatalf("len = %d, want 2", len(notes))
		}
	})

	// Filter by type
	t.Run("filter by type", func(t *testing.T) {
		notes, total, err := store.List(2, note.NoteListParams{Type: note.TypeWord, Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if notes[0].Type != note.TypeWord {
			t.Errorf("type = %q, want word", notes[0].Type)
		}
	})

	// Filter by tag
	t.Run("filter by tag", func(t *testing.T) {
		notes, total, err := store.List(2, note.NoteListParams{Tag: "天气", Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if notes[0].Title != "雨" {
			t.Errorf("title = %q, want 雨", notes[0].Title)
		}
	})

	// Pagination
	t.Run("pagination", func(t *testing.T) {
		notes, total, err := store.List(2, note.NoteListParams{Offset: 0, Limit: 1, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2 (total ignores pagination)", total)
		}
		if len(notes) != 1 {
			t.Errorf("len = %d, want 1", len(notes))
		}
	})
}
