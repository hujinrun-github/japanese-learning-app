package data

import (
	"testing"
	"time"

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

func TestNoteStore_Search(t *testing.T) {
	insertTestUser(t, 4, "note_search@example.com")

	db := testDB
	store := NewNoteStore(db)

	n1 := &note.Note{UserID: 4, Type: note.TypeWord, Title: "雨", Content: "あめ、ame", SourceText: "雨が降っている"}
	n2 := &note.Note{UserID: 4, Type: note.TypeGrammar, Title: "～ている", Content: "持续体", SourceText: "降っている"}
	n3 := &note.Note{UserID: 4, Type: note.TypeSentence, Title: "hello", Content: "not japanese"}
	for _, n := range []*note.Note{n1, n2, n3} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	t.Run("search by title", func(t *testing.T) {
		results, err := store.Search(4, "雨", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len = %d, want 1", len(results))
		}
		if results[0].Title != "雨" {
			t.Errorf("Title = %q", results[0].Title)
		}
	})

	t.Run("search by content", func(t *testing.T) {
		results, err := store.Search(4, "持续体", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len = %d, want 1", len(results))
		}
	})

	t.Run("search by source_text", func(t *testing.T) {
		results, err := store.Search(4, "降っている", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("len = %d, want 2 (matches both 雨 and ～ている source_text)", len(results))
		}
	})

	t.Run("no results", func(t *testing.T) {
		results, err := store.Search(4, "nonexistent", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("len = %d, want 0", len(results))
		}
	})
}

func TestNoteStore_Update(t *testing.T) {
	insertTestUser(t, 10, "note_update@example.com")

	db := testDB
	store := NewNoteStore(db)

	n := &note.Note{UserID: 10, Type: note.TypeWord, Title: "雨", Content: "old content", Tags: []string{"N5"}}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	n.Title = "雨（更新）"
	n.Content = "new content"
	n.Tags = []string{"N5", "天气"}

	if err := store.Update(n); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := store.GetByID(10, n.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Title != "雨（更新）" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Content != "new content" {
		t.Errorf("Content = %q", got.Content)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(got.Tags))
	}
}

func TestNoteStore_SoftDelete(t *testing.T) {
	insertTestUser(t, 11, "note_softdelete@example.com")

	db := testDB
	store := NewNoteStore(db)

	n := &note.Note{UserID: 11, Type: note.TypeWord, Title: "to delete", Content: "x"}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := store.SoftDelete(11, n.ID); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// GetByID should return error for soft-deleted note
	_, err := store.GetByID(11, n.ID)
	if err == nil {
		t.Error("expected error for soft-deleted note")
	}

	// List should not include soft-deleted note
	notes, total, err := store.List(11, note.NoteListParams{Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	_ = notes
}

func TestNoteStore_Links(t *testing.T) {
	insertTestUser(t, 20, "note_links@example.com")

	store := NewNoteStore(testDB)

	wordNote := &note.Note{UserID: 20, Type: note.TypeWord, Title: "雨", Content: "ame"}
	grammarNote := &note.Note{UserID: 20, Type: note.TypeGrammar, Title: "～ている", Content: "持续体"}
	sentenceNote := &note.Note{UserID: 20, Type: note.TypeSentence, Title: "雨が降っている", Content: "正在下雨"}
	for _, n := range []*note.Note{wordNote, grammarNote, sentenceNote} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	t.Run("add link", func(t *testing.T) {
		link, err := store.AddLink(20, grammarNote.ID, wordNote.ID, note.RelationUsesWord)
		if err != nil {
			t.Fatalf("AddLink failed: %v", err)
		}
		if link.ID == 0 {
			t.Error("link ID not set")
		}
		if link.Relation != note.RelationUsesWord {
			t.Errorf("relation = %q", link.Relation)
		}
	})

	t.Run("duplicate link", func(t *testing.T) {
		_, err := store.AddLink(20, grammarNote.ID, wordNote.ID, note.RelationUsesWord)
		if err == nil {
			t.Error("expected error for duplicate link")
		}
	})

	store.AddLink(20, sentenceNote.ID, wordNote.ID, note.RelationContext)
	store.AddLink(20, sentenceNote.ID, grammarNote.ID, note.RelationUsesGrammar)

	t.Run("outgoing links", func(t *testing.T) {
		links, err := store.GetOutgoingLinks(20, sentenceNote.ID)
		if err != nil {
			t.Fatalf("GetOutgoingLinks failed: %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("len = %d, want 2", len(links))
		}
	})

	t.Run("incoming links", func(t *testing.T) {
		links, err := store.GetIncomingLinks(20, wordNote.ID)
		if err != nil {
			t.Fatalf("GetIncomingLinks failed: %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("len = %d, want 2 (grammar uses_word + sentence context)", len(links))
		}
	})

	t.Run("remove link", func(t *testing.T) {
		links, _ := store.GetOutgoingLinks(20, sentenceNote.ID)
		if err := store.RemoveLink(20, links[0].ID); err != nil {
			t.Fatalf("RemoveLink failed: %v", err)
		}
		remaining, _ := store.GetOutgoingLinks(20, sentenceNote.ID)
		if len(remaining) != 1 {
			t.Errorf("len = %d, want 1 after removal", len(remaining))
		}
	})
}

func TestNoteStore_ListByReference(t *testing.T) {
	insertTestUser(t, 5, "note_listbyref@example.com")

	db := testDB
	store := NewNoteStore(db)

	wordID := int64(42)
	refType := "word"

	n1 := &note.Note{UserID: 5, Type: note.TypeWord, Title: "雨 note", Content: "x",
		ReferenceID: &wordID, ReferenceType: &refType}
	n2 := &note.Note{UserID: 5, Type: note.TypeSentence, Title: "sentence about 雨", Content: "y",
		ReferenceID: &wordID, ReferenceType: &refType}
	n3 := &note.Note{UserID: 5, Type: note.TypeWord, Title: "other", Content: "z"}
	for _, n := range []*note.Note{n1, n2, n3} {
		if err := store.Create(n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	digests, err := store.ListByReference(5, "word", wordID, 10)
	if err != nil {
		t.Fatalf("ListByReference failed: %v", err)
	}
	if len(digests) != 2 {
		t.Fatalf("len = %d, want 2", len(digests))
	}
}

func TestNoteStore_ListTags(t *testing.T) {
	insertTestUser(t, 6, "note_listtags@example.com")

	db := testDB
	store := NewNoteStore(db)

	n1 := &note.Note{UserID: 6, Type: note.TypeWord, Title: "a", Content: "x", Tags: []string{"N5", "动词"}}
	n2 := &note.Note{UserID: 6, Type: note.TypeGrammar, Title: "b", Content: "y", Tags: []string{"N5", "易错"}}
	if err := store.Create(n1); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := store.Create(n2); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tags, err := store.ListTags(6)
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("len = %d, want 3", len(tags))
	}
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	for _, want := range []string{"N5", "动词", "易错"} {
		if !tagSet[want] {
			t.Errorf("missing tag %q", want)
		}
	}
}

func TestNoteStore_SRS(t *testing.T) {
	insertTestUser(t, 100, "note_srs@example.com")

	db := testDB
	store := NewNoteStore(db)

	n := &note.Note{UserID: 100, Type: note.TypeWord, Title: "雨", Content: "ame"}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	t.Run("promote", func(t *testing.T) {
		if err := store.Promote(100, n.ID); err != nil {
			t.Fatalf("Promote failed: %v", err)
		}
		got, _ := store.GetByID(100, n.ID)
		if got.NextReviewAt == nil {
			t.Error("NextReviewAt should be set after promote")
		}
		if got.MasteryLevel != 0 {
			t.Errorf("MasteryLevel = %d, want 0", got.MasteryLevel)
		}
	})

	t.Run("demote", func(t *testing.T) {
		if err := store.Demote(100, n.ID); err != nil {
			t.Fatalf("Demote failed: %v", err)
		}
		got, _ := store.GetByID(100, n.ID)
		if got.NextReviewAt != nil {
			t.Error("NextReviewAt should be nil after demote")
		}
	})

	t.Run("save review", func(t *testing.T) {
		store.Promote(100, n.ID)
		got, _ := store.GetByID(100, n.ID)
		got.MasteryLevel = 2
		got.EaseFactor = 2.5
		got.Interval = 6
		now := time.Now()
		got.NextReviewAt = &now

		if err := store.SaveReview(100, n.ID, *got); err != nil {
			t.Fatalf("SaveReview failed: %v", err)
		}
		updated, _ := store.GetByID(100, n.ID)
		if updated.MasteryLevel != 2 {
			t.Errorf("MasteryLevel = %d, want 2", updated.MasteryLevel)
		}
	})

	t.Run("list due notes", func(t *testing.T) {
		store.Promote(100, n.ID)
		due, err := store.ListDueNotes(100)
		if err != nil {
			t.Fatalf("ListDueNotes failed: %v", err)
		}
		found := false
		for _, dn := range due {
			if dn.ID == n.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("promoted note should appear in due notes")
		}
	})

	t.Run("list archived", func(t *testing.T) {
		n2 := &note.Note{UserID: 100, Type: note.TypeWord, Title: "graduated", Content: "x"}
		store.Create(n2)
		// Directly update to simulate graduation (mastery >= 5, next_review_at = NULL)
		db.Exec(`UPDATE notes SET mastery_level = 5, next_review_at = NULL WHERE id = ?`, n2.ID)

		archived, total, err := store.ListArchived(100, note.NoteListParams{Offset: 0, Limit: 10, Sort: "created_at", Order: "asc"})
		if err != nil {
			t.Fatalf("ListArchived failed: %v", err)
		}
		if total < 1 {
			t.Errorf("total = %d, want >= 1", total)
		}
		_ = archived
	})
}
