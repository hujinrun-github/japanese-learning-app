package data

import (
	"testing"
	"time"

	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/module/notesync"
)

func TestNoteSyncStoreTargetBindingAndState(t *testing.T) {
	insertTestUser(t, 9101, "note_sync_store@example.com")
	store := NewNoteSyncStore(testDB)

	cases := []struct {
		name   string
		target notesync.SyncTarget
	}{
		{name: "notion target", target: notesync.SyncTarget{UserID: 9101, Type: notesync.TargetTypeNotion, Name: "Japanese Notes", DataSourceID: "source-1", TokenEnv: "JLA_NOTION_TOKEN", TitleProperty: "Name", TagsProperty: "Tags", SyncTags: []string{"日语"}, Enabled: true}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.target
			if err := store.CreateTarget(&target); err != nil {
				t.Fatalf("CreateTarget failed: %v", err)
			}
			if target.ID == 0 {
				t.Fatal("CreateTarget did not set ID")
			}
			got, err := store.GetTarget(9101, target.ID)
			if err != nil {
				t.Fatalf("GetTarget failed: %v", err)
			}
			if got.Name != target.Name || len(got.SyncTags) != 1 || !got.Enabled {
				t.Fatalf("target round trip = %+v", got)
			}

			local := &note.Note{UserID: 9101, Type: note.TypeWord, Title: "雨", Content: "ame"}
			if err := NewNoteStore(testDB).Create(local); err != nil {
				t.Fatalf("create note failed: %v", err)
			}
			if err := store.SetBinding(9101, local.ID, target.ID); err != nil {
				t.Fatalf("SetBinding failed: %v", err)
			}
			binding, err := store.GetBinding(9101, local.ID)
			if err != nil || binding.TargetID != target.ID {
				t.Fatalf("GetBinding = %+v, err=%v", binding, err)
			}

			now := time.Now().UTC()
			state := &notesync.SyncState{NoteID: local.ID, TargetID: target.ID, ExternalPageID: "page-1", ExternalURL: "https://notion.so/page-1", LocalHash: "local", ExternalHash: "external", LastDirection: "push", LastSyncedAt: &now, UpdatedAt: now}
			if err := store.SaveState(state); err != nil {
				t.Fatalf("SaveState failed: %v", err)
			}
			gotState, err := store.GetStateByPage(9101, target.ID, "page-1")
			if err != nil || gotState.NoteID != local.ID || gotState.ExternalURL == "" {
				t.Fatalf("GetStateByPage = %+v, err=%v", gotState, err)
			}
		})
	}
}
