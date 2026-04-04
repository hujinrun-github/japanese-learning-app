package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/user"
)

// UserStoreInterface defines the data access methods required by UserService.
type UserStoreInterface interface {
	Create(email, passwordHash string, goalLevel user.JLPTLevel) (*user.User, error)
	GetByEmail(email string) (*user.User, error)
	GetByID(id int64) (*user.User, error)
	GetPasswordHash(email string) (string, error)
	UpdateStreak(userID int64, streakDays int) error
}

// PasswordHasher abstracts password hashing and verification (allows fake in tests).
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) bool
}

// UserService handles business logic for user registration, login, and profile.
type UserService struct {
	store  UserStoreInterface
	hasher PasswordHasher
}

// NewUserService creates a UserService instance.
func NewUserService(store UserStoreInterface, hasher PasswordHasher) *UserService {
	return &UserService{store: store, hasher: hasher}
}

// Register creates a new user account. Returns the created user.
func (s *UserService) Register(email, password string, goalLevel user.JLPTLevel) (*user.User, error) {
	slog.Debug("UserService.Register called", "email", email, "goal_level", goalLevel)

	hash, err := s.hasher.Hash(password)
	if err != nil {
		slog.Error("UserService.Register: failed to hash password", "err", err)
		return nil, fmt.Errorf("service.UserService.Register hash: %w", err)
	}

	u, err := s.store.Create(email, hash, goalLevel)
	if err != nil {
		slog.Error("UserService.Register: failed to create user", "err", err, "email", email)
		return nil, fmt.Errorf("service.UserService.Register create: %w", err)
	}

	slog.Debug("UserService.Register done", "user_id", u.ID, "email", email)
	return u, nil
}

// Login authenticates a user and returns a token response.
// Returns error if the email is not found or the password is wrong.
func (s *UserService) Login(email, password string) (*user.TokenResp, error) {
	slog.Debug("UserService.Login called", "email", email)

	hash, err := s.store.GetPasswordHash(email)
	if err != nil {
		slog.Error("UserService.Login: user not found or db error", "email", email, "err", err)
		// Return generic message to avoid email enumeration
		return nil, fmt.Errorf("service.UserService.Login: invalid credentials")
	}

	if !s.hasher.Verify(password, hash) {
		slog.Error("UserService.Login: wrong password", "email", email)
		return nil, fmt.Errorf("service.UserService.Login: invalid credentials")
	}

	u, err := s.store.GetByEmail(email)
	if err != nil {
		slog.Error("UserService.Login: failed to get user after auth", "err", err)
		return nil, fmt.Errorf("service.UserService.Login get user: %w", err)
	}

	token, err := generateToken()
	if err != nil {
		slog.Error("UserService.Login: failed to generate token", "err", err)
		return nil, fmt.Errorf("service.UserService.Login generate token: %w", err)
	}

	resp := &user.TokenResp{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User:      *u,
	}

	slog.Debug("UserService.Login done", "user_id", u.ID, "email", email)
	return resp, nil
}

// GetByID returns a user by ID.
func (s *UserService) GetByID(id int64) (*user.User, error) {
	slog.Debug("UserService.GetByID called", "user_id", id)

	u, err := s.store.GetByID(id)
	if err != nil {
		slog.Error("UserService.GetByID: failed", "err", err, "user_id", id)
		return nil, fmt.Errorf("service.UserService.GetByID: %w", err)
	}

	slog.Debug("UserService.GetByID done", "user_id", id)
	return u, nil
}

// UpdateStreak updates the user's consecutive study streak.
func (s *UserService) UpdateStreak(userID int64, streakDays int) error {
	slog.Debug("UserService.UpdateStreak called", "user_id", userID, "streak_days", streakDays)

	if err := s.store.UpdateStreak(userID, streakDays); err != nil {
		slog.Error("UserService.UpdateStreak: failed", "err", err)
		return fmt.Errorf("service.UserService.UpdateStreak: %w", err)
	}

	slog.Debug("UserService.UpdateStreak done", "user_id", userID, "streak_days", streakDays)
	return nil
}

// generateToken generates a 32-byte random hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generateToken: %w", err)
	}
	return hex.EncodeToString(b), nil
}
