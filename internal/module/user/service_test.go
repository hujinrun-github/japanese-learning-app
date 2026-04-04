package user_test

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/module/user"
)

// --- fake store ---

type fakeUserStore struct {
	users  map[string]*user.User // keyed by email
	nextID int64
	hashes map[int64]string // userID → password hash
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		users:  make(map[string]*user.User),
		hashes: make(map[int64]string),
	}
}

func (f *fakeUserStore) CreateUser(u user.User, passwordHash string) (*user.User, error) {
	if _, exists := f.users[u.Email]; exists {
		return nil, errors.New("email already taken")
	}
	f.nextID++
	u.ID = f.nextID
	cp := u
	f.users[u.Email] = &cp
	f.hashes[u.ID] = passwordHash
	return &cp, nil
}

func (f *fakeUserStore) GetUserByEmail(email string) (*user.User, string, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, "", errors.New("user not found")
	}
	return u, f.hashes[u.ID], nil
}

func (f *fakeUserStore) GetUserByID(id int64) (*user.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, errors.New("user not found")
}

// --- tests ---

func TestUserService_Register(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret")

	req := user.RegisterReq{
		Email:     "alice@example.com",
		Password:  "password123",
		GoalLevel: user.LevelN3,
	}
	u, err := svc.Register(req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if u.Email != req.Email {
		t.Errorf("expected email=%s, got %s", req.Email, u.Email)
	}
	if u.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestUserService_Register_DuplicateEmail(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret")

	req := user.RegisterReq{Email: "bob@example.com", Password: "pw", GoalLevel: user.LevelN5}
	_, _ = svc.Register(req)
	_, err := svc.Register(req)
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

func TestUserService_Login(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret")

	req := user.RegisterReq{Email: "carol@example.com", Password: "mypassword", GoalLevel: user.LevelN4}
	_, _ = svc.Register(req)

	resp, err := svc.Login(user.LoginReq{Email: "carol@example.com", Password: "mypassword"})
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.User.Email != "carol@example.com" {
		t.Errorf("expected email=carol@example.com, got %s", resp.User.Email)
	}
}

func TestUserService_Login_WrongPassword(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret")

	_, _ = svc.Register(user.RegisterReq{Email: "dave@example.com", Password: "correct", GoalLevel: user.LevelN5})

	_, err := svc.Login(user.LoginReq{Email: "dave@example.com", Password: "wrong"})
	if err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestUserService_Login_UnknownEmail(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret")

	_, err := svc.Login(user.LoginReq{Email: "nobody@example.com", Password: "pw"})
	if err == nil {
		t.Error("expected error for unknown email")
	}
}

func TestUserService_GetProfile(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret")

	u, _ := svc.Register(user.RegisterReq{Email: "eve@example.com", Password: "pw", GoalLevel: user.LevelN2})

	profile, err := svc.GetProfile(u.ID)
	if err != nil {
		t.Fatalf("GetProfile error: %v", err)
	}
	if profile.ID != u.ID {
		t.Errorf("expected ID=%d, got %d", u.ID, profile.ID)
	}
}
