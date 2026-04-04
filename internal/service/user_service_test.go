package service_test

import (
	"errors"
	"fmt"
	"testing"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/service"
)

// --- fakes ---

type fakeUserStore struct {
	byEmail map[string]*user.User
	byID    map[int64]*user.User
	nextID  int64
	// stores hashed passwords keyed by email
	passwords map[string]string
}

func (f *fakeUserStore) Create(email, passwordHash string, goalLevel user.JLPTLevel) (*user.User, error) {
	if _, exists := f.byEmail[email]; exists {
		return nil, fmt.Errorf("duplicate email: %w", errors.New("UNIQUE constraint failed"))
	}
	f.nextID++
	u := &user.User{ID: f.nextID, Email: email, GoalLevel: goalLevel}
	f.byEmail[email] = u
	f.byID[f.nextID] = u
	f.passwords[email] = passwordHash
	return u, nil
}

func (f *fakeUserStore) GetByEmail(email string) (*user.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return nil, fmt.Errorf("user %q: %w", email, errors.New("not found"))
	}
	return u, nil
}

func (f *fakeUserStore) GetByID(id int64) (*user.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, fmt.Errorf("user %d: %w", id, errors.New("not found"))
	}
	return u, nil
}

func (f *fakeUserStore) GetPasswordHash(email string) (string, error) {
	hash, ok := f.passwords[email]
	if !ok {
		return "", fmt.Errorf("user %q: %w", email, errors.New("not found"))
	}
	return hash, nil
}

func (f *fakeUserStore) UpdateStreak(userID int64, streakDays int) error {
	u, ok := f.byID[userID]
	if !ok {
		return fmt.Errorf("user %d not found", userID)
	}
	u.StreakDays = streakDays
	return nil
}

// fakePasswordHasher used in user service tests
type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}

func (fakeHasher) Verify(password, hash string) bool {
	return hash == "hashed:"+password
}

// --- tests ---

func TestUserService_Register(t *testing.T) {
	store := &fakeUserStore{
		byEmail:   map[string]*user.User{},
		byID:      map[int64]*user.User{},
		passwords: map[string]string{},
	}
	svc := service.NewUserService(store, fakeHasher{})

	u, err := svc.Register("alice@example.com", "password123", user.LevelN5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", u.Email)
	}
	if u.GoalLevel != user.LevelN5 {
		t.Errorf("expected goal level N5, got %s", u.GoalLevel)
	}
}

func TestUserService_Register_DuplicateEmail(t *testing.T) {
	store := &fakeUserStore{
		byEmail:   map[string]*user.User{},
		byID:      map[int64]*user.User{},
		passwords: map[string]string{},
	}
	svc := service.NewUserService(store, fakeHasher{})

	if _, err := svc.Register("alice@example.com", "pass1", user.LevelN5); err != nil {
		t.Fatalf("first register unexpected error: %v", err)
	}
	if _, err := svc.Register("alice@example.com", "pass2", user.LevelN4); err == nil {
		t.Error("expected error on duplicate email registration")
	}
}

func TestUserService_Login(t *testing.T) {
	store := &fakeUserStore{
		byEmail:   map[string]*user.User{},
		byID:      map[int64]*user.User{},
		passwords: map[string]string{},
	}
	svc := service.NewUserService(store, fakeHasher{})

	if _, err := svc.Register("bob@example.com", "mypass", user.LevelN4); err != nil {
		t.Fatalf("register unexpected error: %v", err)
	}

	resp, err := svc.Login("bob@example.com", "mypass")
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.User.Email != "bob@example.com" {
		t.Errorf("expected email bob@example.com, got %s", resp.User.Email)
	}
}

func TestUserService_Login_WrongPassword(t *testing.T) {
	store := &fakeUserStore{
		byEmail:   map[string]*user.User{},
		byID:      map[int64]*user.User{},
		passwords: map[string]string{},
	}
	svc := service.NewUserService(store, fakeHasher{})

	if _, err := svc.Register("charlie@example.com", "correct", user.LevelN5); err != nil {
		t.Fatalf("register unexpected error: %v", err)
	}

	if _, err := svc.Login("charlie@example.com", "wrongpass"); err == nil {
		t.Error("expected error on wrong password")
	}
}

func TestUserService_GetByID(t *testing.T) {
	store := &fakeUserStore{
		byEmail:   map[string]*user.User{},
		byID:      map[int64]*user.User{},
		passwords: map[string]string{},
	}
	svc := service.NewUserService(store, fakeHasher{})

	u, _ := svc.Register("dave@example.com", "pass", user.LevelN3)
	got, err := svc.GetByID(u.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Email != "dave@example.com" {
		t.Errorf("expected dave@example.com, got %s", got.Email)
	}
}
