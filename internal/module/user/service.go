package user

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"time"
)

// UserStoreInterface defines data access methods required by UserService.
type UserStoreInterface interface {
	CreateUser(u User, passwordHash string) (*User, error)
	GetUserByEmail(email string) (*User, string, error) // returns (user, passwordHash, error)
	GetUserByID(id int64) (*User, error)
}

// UserService handles business logic for user registration, login and profile.
type UserService struct {
	store     UserStoreInterface
	jwtSecret string
}

// NewUserService creates a UserService instance.
func NewUserService(store UserStoreInterface, jwtSecret string) *UserService {
	return &UserService{store: store, jwtSecret: jwtSecret}
}

// Register creates a new user account.
// The password is hashed with SHA-256 before storage.
func (s *UserService) Register(req RegisterReq) (*User, error) {
	slog.Debug("UserService.Register called", "email", req.Email)

	hash := hashPassword(req.Password)

	u := User{
		Email:     req.Email,
		GoalLevel: req.GoalLevel,
		CreatedAt: time.Now(),
	}

	created, err := s.store.CreateUser(u, hash)
	if err != nil {
		slog.Error("UserService.Register: CreateUser failed", "err", err, "email", req.Email)
		return nil, fmt.Errorf("user.UserService.Register CreateUser: %w", err)
	}

	slog.Debug("UserService.Register done", "user_id", created.ID)
	return created, nil
}

// Login authenticates a user and returns a JWT token on success.
func (s *UserService) Login(req LoginReq) (TokenResp, error) {
	slog.Debug("UserService.Login called", "email", req.Email)

	u, storedHash, err := s.store.GetUserByEmail(req.Email)
	if err != nil {
		slog.Error("UserService.Login: GetUserByEmail failed", "err", err, "email", req.Email)
		return TokenResp{}, fmt.Errorf("user.UserService.Login GetUserByEmail: %w", err)
	}

	if hashPassword(req.Password) != storedHash {
		slog.Error("UserService.Login: wrong password", "email", req.Email)
		return TokenResp{}, fmt.Errorf("user.UserService.Login: invalid credentials")
	}

	token, expiresAt, err := SignToken(u.ID, s.jwtSecret, 24*time.Hour)
	if err != nil {
		slog.Error("UserService.Login: SignToken failed", "err", err)
		return TokenResp{}, fmt.Errorf("user.UserService.Login SignToken: %w", err)
	}

	slog.Debug("UserService.Login done", "user_id", u.ID)
	return TokenResp{Token: token, ExpiresAt: expiresAt, User: *u}, nil
}

// GetProfile returns the user profile by ID.
func (s *UserService) GetProfile(userID int64) (*User, error) {
	slog.Debug("UserService.GetProfile called", "user_id", userID)

	u, err := s.store.GetUserByID(userID)
	if err != nil {
		slog.Error("UserService.GetProfile: GetUserByID failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("user.UserService.GetProfile GetUserByID: %w", err)
	}

	slog.Debug("UserService.GetProfile done", "user_id", userID)
	return u, nil
}

// hashPassword returns a hex-encoded SHA-256 hash of the password.
// NOTE: For production use bcrypt is recommended; SHA-256 is used here per the
// simplicity-first principle and to avoid third-party dependencies.
func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", sum)
}
