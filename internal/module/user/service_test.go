package user_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
)

// --- fake store ---

type fakeUserStore struct {
	users       map[string]*user.User // keyed by email
	nextID      int64
	hashes      map[int64]string // userID → password hash
	resetTokens map[string]*user.ResetToken
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		users:       make(map[string]*user.User),
		hashes:      make(map[int64]string),
		resetTokens: make(map[string]*user.ResetToken),
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

func (f *fakeUserStore) GetUserIDByEmail(email string) (int64, error) {
	u, ok := f.users[email]
	if !ok {
		return 0, sql.ErrNoRows
	}
	return u.ID, nil
}

func (f *fakeUserStore) CreateResetToken(token string, userID int64, expiresAt time.Time) error {
	f.resetTokens[token] = &user.ResetToken{Token: token, UserID: userID, ExpiresAt: expiresAt}
	return nil
}

func (f *fakeUserStore) GetResetToken(token string) (*user.ResetToken, error) {
	rt, ok := f.resetTokens[token]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return rt, nil
}

func (f *fakeUserStore) MarkTokenUsed(token string) error {
	if rt, ok := f.resetTokens[token]; ok {
		rt.Used = true
	}
	return nil
}

func (f *fakeUserStore) UpdatePassword(userID int64, newPasswordHash string) error {
	f.hashes[userID] = newPasswordHash
	return nil
}

// --- tests ---

func TestUserService_Register(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

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
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	req := user.RegisterReq{Email: "bob@example.com", Password: "pw", GoalLevel: user.LevelN5}
	_, _ = svc.Register(req)
	_, err := svc.Register(req)
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

func TestUserService_Login(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

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
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	_, _ = svc.Register(user.RegisterReq{Email: "dave@example.com", Password: "correct", GoalLevel: user.LevelN5})

	_, err := svc.Login(user.LoginReq{Email: "dave@example.com", Password: "wrong"})
	if err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestUserService_Login_UnknownEmail(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	_, err := svc.Login(user.LoginReq{Email: "nobody@example.com", Password: "pw"})
	if err == nil {
		t.Error("expected error for unknown email")
	}
}

func TestUserService_GetProfile(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	u, _ := svc.Register(user.RegisterReq{Email: "eve@example.com", Password: "pw", GoalLevel: user.LevelN2})

	profile, err := svc.GetProfile(u.ID)
	if err != nil {
		t.Fatalf("GetProfile error: %v", err)
	}
	if profile.ID != u.ID {
		t.Errorf("expected ID=%d, got %d", u.ID, profile.ID)
	}
}

func TestUserService_ForgotPassword_UnknownEmail(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	// Should succeed silently even for unknown email (anti-enumeration)
	err := svc.ForgotPassword("unknown@example.com")
	if err != nil {
		t.Errorf("expected nil for unknown email, got %v", err)
	}
}

func TestUserService_ForgotPassword_KnownEmail(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	_, _ = svc.Register(user.RegisterReq{Email: "frank@example.com", Password: "pw", GoalLevel: user.LevelN5})

	err := svc.ForgotPassword("frank@example.com")
	if err != nil {
		t.Errorf("ForgotPassword error: %v", err)
	}
	if len(store.resetTokens) != 1 {
		t.Errorf("expected 1 reset token, got %d", len(store.resetTokens))
	}
}

func TestUserService_ResetPassword(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	u, _ := svc.Register(user.RegisterReq{Email: "grace@example.com", Password: "oldpw", GoalLevel: user.LevelN5})

	// Directly insert a valid token
	token := "validtoken123"
	_ = store.CreateResetToken(token, u.ID, time.Now().Add(30*time.Minute))

	err := svc.ResetPassword(token, "newpassword")
	if err != nil {
		t.Fatalf("ResetPassword error: %v", err)
	}

	// Verify login works with new password
	_, err = svc.Login(user.LoginReq{Email: "grace@example.com", Password: "newpassword"})
	if err != nil {
		t.Errorf("Login with new password failed: %v", err)
	}
}

func TestUserService_ResetPassword_ExpiredToken(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	u, _ := svc.Register(user.RegisterReq{Email: "henry@example.com", Password: "pw", GoalLevel: user.LevelN5})

	// Insert an already-expired token
	token := "expiredtoken"
	_ = store.CreateResetToken(token, u.ID, time.Now().Add(-1*time.Minute))

	err := svc.ResetPassword(token, "newpassword")
	if !errors.Is(err, user.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestUserService_ResetPassword_UsedToken(t *testing.T) {
	store := newFakeUserStore()
	svc := user.NewUserService(store, "jwt-secret", &user.StubMailer{}, "http://localhost")

	u, _ := svc.Register(user.RegisterReq{Email: "iris@example.com", Password: "pw", GoalLevel: user.LevelN5})

	token := "usedtoken"
	_ = store.CreateResetToken(token, u.ID, time.Now().Add(30*time.Minute))
	_ = store.MarkTokenUsed(token)

	err := svc.ResetPassword(token, "newpassword")
	if !errors.Is(err, user.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}
