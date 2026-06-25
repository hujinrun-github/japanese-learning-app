package postgres

import (
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
	storepkg "japanese-learning-app/internal/store"
)

func TestUserStoreCreateUser(t *testing.T) {
	cases := []struct {
		name       string
		input      user.User
		wantLevels []string
	}{
		{
			name: "explicit levels",
			input: user.User{
				Name:       "N4 User",
				Email:      "n4@example.com",
				JLPTLevels: []string{"N4"},
			},
			wantLevels: []string{"N4"},
		},
		{
			name: "default level",
			input: user.User{
				Name:  "Default User",
				Email: "default@example.com",
			},
			wantLevels: []string{"N5"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newPostgresUserStoreForTest(t)

			created, err := store.CreateUser(tc.input, "hashedpwd")
			if err != nil {
				t.Fatalf("CreateUser() error = %v", err)
			}
			if created == nil {
				t.Fatal("CreateUser() returned nil user")
			}
			if created.ID == 0 {
				t.Fatal("CreateUser() returned ID = 0")
			}
			if created.Email != tc.input.Email {
				t.Fatalf("CreateUser() email = %q, want %q", created.Email, tc.input.Email)
			}
			if len(created.JLPTLevels) != len(tc.wantLevels) || created.JLPTLevels[0] != tc.wantLevels[0] {
				t.Fatalf("CreateUser() levels = %v, want %v", created.JLPTLevels, tc.wantLevels)
			}
		})
	}
}

func TestUserStoreCreateUserDuplicateEmail(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	_, err := store.CreateUser(user.User{
		Name:       "First User",
		Email:      "duplicate@example.com",
		JLPTLevels: []string{"N5"},
	}, "hash1")
	if err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	_, err = store.CreateUser(user.User{
		Name:       "Second User",
		Email:      "duplicate@example.com",
		JLPTLevels: []string{"N4"},
	}, "hash2")
	if !errors.Is(err, user.ErrEmailTaken) {
		t.Fatalf("second CreateUser() error = %v, want ErrEmailTaken", err)
	}
}

func TestUserStoreGetUserByEmail(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	created, err := store.CreateUser(user.User{
		Name:       "Lookup User",
		Email:      "lookup@example.com",
		JLPTLevels: []string{"N3"},
	}, "securehash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, hash, err := store.GetUserByEmail("lookup@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetUserByEmail() returned nil user")
	}
	if got.ID != created.ID {
		t.Fatalf("GetUserByEmail() ID = %d, want %d", got.ID, created.ID)
	}
	if hash != "securehash" {
		t.Fatalf("GetUserByEmail() hash = %q, want %q", hash, "securehash")
	}
}

func TestUserStoreGetUserByEmailNotFound(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	got, _, err := store.GetUserByEmail("missing@example.com")
	if got != nil {
		t.Fatalf("GetUserByEmail() user = %+v, want nil", got)
	}
	if !errors.Is(err, storepkg.ErrNotFound) {
		t.Fatalf("GetUserByEmail() error = %v, want ErrNotFound", err)
	}
}

func TestUserStoreGetUserByID(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	created, err := store.CreateUser(user.User{
		Name:       "ByID User",
		Email:      "byid@example.com",
		JLPTLevels: []string{"N2"},
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	got, err := store.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("GetUserByID() user = %+v, want ID %d", got, created.ID)
	}
}

func TestUserStorePasswordResetLifecycle(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	created, err := store.CreateUser(user.User{
		Name:       "Reset User",
		Email:      "reset@example.com",
		JLPTLevels: []string{"N5"},
	}, "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	if err := store.CreateResetToken("token-123", created.ID, expiresAt); err != nil {
		t.Fatalf("CreateResetToken() error = %v", err)
	}

	token, err := store.GetResetToken("token-123")
	if err != nil {
		t.Fatalf("GetResetToken() error = %v", err)
	}
	if token.UserID != created.ID {
		t.Fatalf("GetResetToken() user ID = %d, want %d", token.UserID, created.ID)
	}
	if token.Used {
		t.Fatal("GetResetToken() used = true, want false")
	}

	if err := store.MarkTokenUsed("token-123"); err != nil {
		t.Fatalf("MarkTokenUsed() error = %v", err)
	}

	token, err = store.GetResetToken("token-123")
	if err != nil {
		t.Fatalf("GetResetToken() after MarkTokenUsed error = %v", err)
	}
	if !token.Used {
		t.Fatal("GetResetToken() used = false, want true")
	}
}

func TestUserStoreUpdatePasswordNotFound(t *testing.T) {
	store := newPostgresUserStoreForTest(t)

	err := store.UpdatePassword(999999, "new-hash")
	if !errors.Is(err, storepkg.ErrNotFound) {
		t.Fatalf("UpdatePassword() error = %v, want ErrNotFound", err)
	}
}
