package note_test

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/module/note"
)

type fakeNoteStore struct {
	notes  map[int64]*note.Note
	links  []note.NoteLink
	nextID int64
	linkID int64
}

func newFakeNoteStore() *fakeNoteStore {
	return &fakeNoteStore{notes: make(map[int64]*note.Note)}
}

func (f *fakeNoteStore) Create(n *note.Note) error {
	f.nextID++
	n.ID = f.nextID
	f.notes[n.ID] = n
	return nil
}

func (f *fakeNoteStore) GetByID(userID, noteID int64) (*note.Note, error) {
	n, ok := f.notes[noteID]
	if !ok || n.UserID != userID {
		return nil, errors.New("not found")
	}
	return n, nil
}

func (f *fakeNoteStore) List(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	var result []note.Note
	for _, n := range f.notes {
		if n.UserID == userID {
			result = append(result, *n)
		}
	}
	return result, len(result), nil
}

func (f *fakeNoteStore) Update(n *note.Note) error {
	if _, ok := f.notes[n.ID]; !ok {
		return errors.New("not found")
	}
	f.notes[n.ID] = n
	return nil
}

func (f *fakeNoteStore) SoftDelete(userID, noteID int64) error {
	delete(f.notes, noteID)
	return nil
}

func (f *fakeNoteStore) Search(userID int64, query string, limit int) ([]note.Note, error) {
	return nil, nil
}

func (f *fakeNoteStore) AddLink(userID, noteID, targetNoteID int64, relation note.LinkRelation) (*note.NoteLink, error) {
	f.linkID++
	l := &note.NoteLink{ID: f.linkID, NoteID: noteID, TargetNoteID: targetNoteID, Relation: relation}
	f.links = append(f.links, *l)
	return l, nil
}

func (f *fakeNoteStore) RemoveLink(userID, linkID int64) error {
	for i, l := range f.links {
		if l.ID == linkID {
			f.links = append(f.links[:i], f.links[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (f *fakeNoteStore) GetOutgoingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	var result []note.NoteLink
	for _, l := range f.links {
		if l.NoteID == noteID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (f *fakeNoteStore) GetIncomingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	var result []note.NoteLink
	for _, l := range f.links {
		if l.TargetNoteID == noteID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (f *fakeNoteStore) Promote(userID, noteID int64) error  { return nil }
func (f *fakeNoteStore) Demote(userID, noteID int64) error   { return nil }
func (f *fakeNoteStore) SaveReview(userID, noteID int64, n note.Note) error { return nil }
func (f *fakeNoteStore) ListDueNotes(userID int64) ([]note.Note, error)      { return nil, nil }
func (f *fakeNoteStore) ListArchived(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	return nil, 0, nil
}
func (f *fakeNoteStore) ListByReference(userID int64, refType string, refID int64, limit int) ([]note.NoteDigest, error) {
	return nil, nil
}
func (f *fakeNoteStore) ListTags(userID int64) ([]string, error) { return nil, nil }

func TestNoteService_CreateAndGetDetail(t *testing.T) {
	store := newFakeNoteStore()
	svc := note.NewNoteService(store)

	n := &note.Note{UserID: 1, Type: note.TypeWord, Title: "雨", Content: "ame"}
	if err := svc.Create(n); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if n.ID == 0 {
		t.Error("ID not set after create")
	}

	svc.AddLink(1, n.ID, 99, note.RelationRelated)
	svc.AddLink(1, 99, n.ID, note.RelationContext)

	detail, err := svc.GetDetail(1, n.ID)
	if err != nil {
		t.Fatalf("GetDetail failed: %v", err)
	}
	if detail.Title != "雨" {
		t.Errorf("Title = %q", detail.Title)
	}
	if len(detail.OutgoingLinks) != 1 {
		t.Errorf("OutgoingLinks len = %d, want 1", len(detail.OutgoingLinks))
	}
	if len(detail.IncomingLinks) != 1 {
		t.Errorf("IncomingLinks len = %d, want 1", len(detail.IncomingLinks))
	}
}
