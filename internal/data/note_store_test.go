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
