package data

import (
	"testing"

	"japanese-learning-app/internal/module/user"
)

func TestUserStore_Create(t *testing.T) {
	store := &UserStore{db: testDB}

	u, err := store.Create("newuser@example.com", "hashedpwd", user.LevelN5)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if u == nil {
		t.Fatal("Create returned nil user")
	}
	if u.ID == 0 {
		t.Error("Create returned user with ID=0")
	}
	if u.Email != "newuser@example.com" {
		t.Errorf("Email = %q, want %q", u.Email, "newuser@example.com")
	}
	if u.GoalLevel != user.LevelN5 {
		t.Errorf("GoalLevel = %q, want N5", u.GoalLevel)
	}
}

func TestUserStore_Create_DuplicateEmail(t *testing.T) {
	store := &UserStore{db: testDB}

	email := "duplicate@example.com"
	_, err := store.Create(email, "hash1", user.LevelN5)
	if err != nil {
		t.Fatalf("first Create error: %v", err)
	}

	_, err = store.Create(email, "hash2", user.LevelN4)
	if err == nil {
		t.Fatal("second Create with duplicate email expected error, got nil")
	}
}

func TestUserStore_GetByEmail(t *testing.T) {
	store := &UserStore{db: testDB}

	email := "getbyemail@example.com"
	created, err := store.Create(email, "securehash", user.LevelN4)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := store.GetByEmail(email)
	if err != nil {
		t.Fatalf("GetByEmail error: %v", err)
	}
	if got == nil {
		t.Fatal("GetByEmail returned nil")
	}
	if got.ID != created.ID {
		t.Errorf("GetByEmail ID = %d, want %d", got.ID, created.ID)
	}
	if got.Email != email {
		t.Errorf("GetByEmail Email = %q, want %q", got.Email, email)
	}
}

func TestUserStore_GetByEmail_NotFound(t *testing.T) {
	store := &UserStore{db: testDB}

	got, err := store.GetByEmail("nonexistent@example.com")
	if err == nil {
		t.Fatal("GetByEmail(nonexistent) expected error, got nil")
	}
	if got != nil {
		t.Errorf("GetByEmail(nonexistent) expected nil, got %+v", got)
	}
}

func TestUserStore_GetByID(t *testing.T) {
	store := &UserStore{db: testDB}

	created, err := store.Create("getbyid@example.com", "hash", user.LevelN3)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	got, err := store.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetByID ID = %d, want %d", got.ID, created.ID)
	}
}

func TestUserStore_UpdateStreak(t *testing.T) {
	store := &UserStore{db: testDB}

	created, err := store.Create("streak@example.com", "hash", user.LevelN5)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if err := store.UpdateStreak(created.ID, 5); err != nil {
		t.Fatalf("UpdateStreak error: %v", err)
	}

	got, err := store.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after UpdateStreak error: %v", err)
	}
	if got.StreakDays != 5 {
		t.Errorf("StreakDays = %d, want 5", got.StreakDays)
	}
}
