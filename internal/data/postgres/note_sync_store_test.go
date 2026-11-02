package postgres

import (
	"testing"

	"japanese-learning-app/internal/store"
)

func TestNoteSyncStoreListTargetsReturnsEmptyCollection(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	noteSyncStore := NewNoteSyncStore(db, store.StoreDeps{})

	targets, err := noteSyncStore.ListTargets(999)
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if targets == nil {
		t.Fatal("ListTargets() = nil, want an empty collection")
	}
	if len(targets) != 0 {
		t.Fatalf("len(ListTargets()) = %d, want 0", len(targets))
	}
}
