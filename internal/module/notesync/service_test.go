package notesync

import (
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/integration/notion"
	"japanese-learning-app/internal/module/note"
)

func TestServiceCreateTargetValidatesAndDefaults(t *testing.T) {
	store := newFakeSyncStore()
	service := NewService(store, &fakeNoteStore{}, func(target SyncTarget) (NotionClient, error) {
		return &fakeNotionClient{}, nil
	})

	target, err := service.CreateTarget(1, TargetRequest{Name: "日语", DataSourceID: "123456781234123412341234567890ab"})
	if err != nil {
		t.Fatalf("CreateTarget failed: %v", err)
	}
	if target.TokenEnv != notion.DefaultTokenEnv || target.TitleProperty != "Name" || target.TagsProperty != "Tags" {
		t.Fatalf("defaults not applied: %+v", target)
	}
	if target.DataSourceID != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("DataSourceID = %q", target.DataSourceID)
	}
}

func TestServicePushNoteCreatesThenUpdatesSamePage(t *testing.T) {
	store := newFakeSyncStore()
	store.targets[1] = &SyncTarget{ID: 1, UserID: 7, Type: TargetTypeNotion, Name: "Notion", DataSourceID: "source", TokenEnv: notion.DefaultTokenEnv, TitleProperty: "Name", TagsProperty: "Tags", Enabled: true}
	notes := &fakeNoteStore{notes: map[int64]*note.Note{3: {ID: 3, UserID: 7, Type: note.TypeWord, Title: "雨", Content: "雨的含义", Tags: []string{"N5"}}}}
	client := &fakeNotionClient{created: notion.Page{ID: "page-1", URL: "https://notion.so/page-1"}}
	service := NewService(store, notes, func(target SyncTarget) (NotionClient, error) { return client, nil })
	if err := service.BindNote(7, 3, 1); err != nil {
		t.Fatalf("BindNote failed: %v", err)
	}
	if _, err := service.PushNote(7, 3); err != nil {
		t.Fatalf("first PushNote failed: %v", err)
	}
	if client.createCalls != 1 || client.updateCalls != 0 {
		t.Fatalf("first calls create=%d update=%d", client.createCalls, client.updateCalls)
	}
	if _, err := service.PushNote(7, 3); err != nil {
		t.Fatalf("second PushNote failed: %v", err)
	}
	if client.createCalls != 1 || client.updateCalls != 1 || client.lastUpdatedPage != "page-1" {
		t.Fatalf("second calls create=%d update=%d page=%q", client.createCalls, client.updateCalls, client.lastUpdatedPage)
	}
}

func TestServicePushTargetClassifiesCreatedAndUpdated(t *testing.T) {
	store := newFakeSyncStore()
	store.targets[1] = &SyncTarget{ID: 1, UserID: 7, Type: TargetTypeNotion, Name: "Notion", DataSourceID: "source", TokenEnv: notion.DefaultTokenEnv, Enabled: true}
	notes := &fakeNoteStore{notes: map[int64]*note.Note{
		3: {ID: 3, UserID: 7, Type: note.TypeWord, Title: "雨"},
		4: {ID: 4, UserID: 7, Type: note.TypeSentence, Title: "晴れ"},
	}}
	client := &fakeNotionClient{created: notion.Page{ID: "page-new"}}
	service := NewService(store, notes, func(SyncTarget) (NotionClient, error) { return client, nil })
	if err := service.BindNote(7, 3, 1); err != nil {
		t.Fatalf("BindNote(3) failed: %v", err)
	}
	if err := service.BindNote(7, 4, 1); err != nil {
		t.Fatalf("BindNote(4) failed: %v", err)
	}
	store.states[stateKey(4, 1)] = &SyncState{NoteID: 4, TargetID: 1, ExternalPageID: "page-existing"}

	result, err := service.PushTarget(7, 1)
	if err != nil {
		t.Fatalf("PushTarget failed: %v", err)
	}
	if result.Created != 1 || result.Updated != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestServiceRestorePendingDeleteRestoresPageAndLocalNote(t *testing.T) {
	store := newFakeSyncStore()
	store.targets[1] = &SyncTarget{ID: 1, UserID: 7, Type: TargetTypeNotion, Name: "Notion", DataSourceID: "source", TokenEnv: notion.DefaultTokenEnv, Enabled: true}
	notes := &fakeNoteStore{notes: map[int64]*note.Note{3: {ID: 3, UserID: 7, Type: note.TypeWord, Title: "雨"}}, deleted: map[int64]*note.Note{}}
	client := &fakeNotionClient{created: notion.Page{ID: "page-1", URL: "https://notion.so/page-1"}}
	service := NewService(store, notes, func(SyncTarget) (NotionClient, error) { return client, nil })
	if err := service.BindNote(7, 3, 1); err != nil {
		t.Fatalf("BindNote failed: %v", err)
	}
	now := time.Now()
	store.states[stateKey(3, 1)] = &SyncState{NoteID: 3, TargetID: 1, ExternalPageID: "page-1", PendingDelete: true, UpdatedAt: now}
	if err := notes.SoftDelete(7, 3); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}
	state, err := service.RestorePendingDelete(7, 1, 3)
	if err != nil {
		t.Fatalf("RestorePendingDelete failed: %v", err)
	}
	if client.restoreCalls != 1 || state.PendingDelete || state.LastDirection != "restore" {
		t.Fatalf("restore state = %+v, restore calls=%d", state, client.restoreCalls)
	}
	if _, err := notes.GetByID(7, 3); err != nil {
		t.Fatalf("local note was not restored: %v", err)
	}
}

type fakeSyncStore struct {
	targets  map[int64]*SyncTarget
	bindings map[int64]*SyncBinding
	states   map[string]*SyncState
}

func newFakeSyncStore() *fakeSyncStore {
	return &fakeSyncStore{targets: map[int64]*SyncTarget{}, bindings: map[int64]*SyncBinding{}, states: map[string]*SyncState{}}
}
func (f *fakeSyncStore) CreateTarget(t *SyncTarget) error {
	t.ID = int64(len(f.targets) + 1)
	f.targets[t.ID] = t
	return nil
}
func (f *fakeSyncStore) GetTarget(uid, id int64) (*SyncTarget, error) {
	t, ok := f.targets[id]
	if !ok || t.UserID != uid {
		return nil, ErrTargetNotFound
	}
	copy := *t
	return &copy, nil
}
func (f *fakeSyncStore) ListTargets(uid int64) ([]SyncTarget, error) {
	var out []SyncTarget
	for _, t := range f.targets {
		if t.UserID == uid {
			out = append(out, *t)
		}
	}
	return out, nil
}
func (f *fakeSyncStore) UpdateTarget(t *SyncTarget) error { f.targets[t.ID] = t; return nil }
func (f *fakeSyncStore) DeleteTarget(uid, id int64) error { delete(f.targets, id); return nil }
func (f *fakeSyncStore) CountBindings(uid, tid int64) (int, error) {
	count := 0
	for _, b := range f.bindings {
		t, _ := f.GetTarget(uid, b.TargetID)
		if t != nil && b.TargetID == tid {
			count++
		}
	}
	return count, nil
}
func (f *fakeSyncStore) GetBinding(uid, nid int64) (*SyncBinding, error) {
	b, ok := f.bindings[nid]
	if !ok {
		return nil, ErrBindingNotFound
	}
	if _, err := f.GetTarget(uid, b.TargetID); err != nil {
		return nil, err
	}
	copy := *b
	return &copy, nil
}
func (f *fakeSyncStore) SetBinding(uid, nid, tid int64) error {
	if _, err := f.GetTarget(uid, tid); err != nil {
		return err
	}
	f.bindings[nid] = &SyncBinding{NoteID: nid, TargetID: tid, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return nil
}
func (f *fakeSyncStore) ClearBinding(uid, nid int64) error { delete(f.bindings, nid); return nil }
func (f *fakeSyncStore) GetState(uid, nid, tid int64) (*SyncState, error) {
	s, ok := f.states[stateKey(nid, tid)]
	if !ok {
		return nil, ErrStateNotFound
	}
	copy := *s
	return &copy, nil
}
func (f *fakeSyncStore) GetStateByPage(uid, tid int64, page string) (*SyncState, error) {
	for _, s := range f.states {
		if s.TargetID == tid && s.ExternalPageID == page {
			copy := *s
			return &copy, nil
		}
	}
	return nil, ErrStateNotFound
}
func (f *fakeSyncStore) SaveState(s *SyncState) error {
	copy := *s
	f.states[stateKey(s.NoteID, s.TargetID)] = &copy
	return nil
}
func (f *fakeSyncStore) ListBoundNoteIDs(uid, tid int64) ([]int64, error) {
	var out []int64
	for nid, b := range f.bindings {
		if b.TargetID == tid {
			out = append(out, nid)
		}
	}
	return out, nil
}
func (f *fakeSyncStore) ListStates(uid, tid int64) ([]SyncState, error) {
	var out []SyncState
	for _, s := range f.states {
		if s.TargetID == tid {
			out = append(out, *s)
		}
	}
	return out, nil
}

type fakeNoteStore struct {
	notes   map[int64]*note.Note
	deleted map[int64]*note.Note
}

func (f *fakeNoteStore) Create(n *note.Note) error {
	if f.notes == nil {
		f.notes = map[int64]*note.Note{}
	}
	n.ID = int64(len(f.notes) + 1)
	f.notes[n.ID] = n
	return nil
}
func (f *fakeNoteStore) GetByID(uid, id int64) (*note.Note, error) {
	n, ok := f.notes[id]
	if !ok || n.UserID != uid {
		return nil, errors.New("not found")
	}
	copy := *n
	return &copy, nil
}
func (f *fakeNoteStore) Update(n *note.Note) error { f.notes[n.ID] = n; return nil }
func (f *fakeNoteStore) SoftDelete(uid, id int64) error {
	if f.deleted == nil {
		f.deleted = map[int64]*note.Note{}
	}
	if n, ok := f.notes[id]; ok {
		f.deleted[id] = n
		delete(f.notes, id)
	}
	return nil
}
func (f *fakeNoteStore) Restore(uid, id int64) error {
	if n, ok := f.deleted[id]; ok {
		f.notes[id] = n
		delete(f.deleted, id)
	}
	return nil
}

type fakeNotionClient struct {
	created                  notion.Page
	createCalls, updateCalls int
	lastUpdatedPage          string
	restoreCalls             int
}

func (f *fakeNotionClient) TestDataSource(string) error                       { return nil }
func (f *fakeNotionClient) QueryDataSource(string) ([]notion.Page, error)     { return nil, nil }
func (f *fakeNotionClient) RetrievePageBlocks(string) ([]notion.Block, error) { return nil, nil }
func (f *fakeNotionClient) CreatePage(notion.PageInput) (notion.Page, error) {
	f.createCalls++
	return f.created, nil
}
func (f *fakeNotionClient) UpdatePage(id string, _ notion.PageInput) (notion.Page, error) {
	f.updateCalls++
	f.lastUpdatedPage = id
	return notion.Page{ID: id, URL: "https://notion.so/" + id}, nil
}
func (f *fakeNotionClient) ReplacePageBlocks(string, []notion.Block) error { return nil }
func (f *fakeNotionClient) RestorePage(string) error {
	f.restoreCalls++
	return nil
}

func stateKey(noteID, targetID int64) string {
	return string(rune(noteID)) + ":" + string(rune(targetID))
}
