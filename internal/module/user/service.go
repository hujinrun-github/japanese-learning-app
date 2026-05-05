package user

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrEmailTaken is returned when a registration attempt uses an already-registered email.
var ErrEmailTaken = errors.New("email already registered")

// ErrTokenInvalid is returned when the reset token is not found, expired, or already used.
var ErrTokenInvalid = errors.New("invalid or expired password reset token")

// UserStoreInterface defines data access methods required by UserService.
type UserStoreInterface interface {
	CreateUser(u User, passwordHash string) (*User, error)
	GetUserByEmail(email string) (*User, string, error) // returns (user, passwordHash, error)
	GetUserByID(id int64) (*User, error)
	// Password reset methods
	GetUserIDByEmail(email string) (int64, error)
	CreateResetToken(token string, userID int64, expiresAt time.Time) error
	GetResetToken(token string) (*ResetToken, error)
	MarkTokenUsed(token string) error
	UpdatePassword(userID int64, newPasswordHash string) error
}

// UserService handles business logic for user registration, login and profile.
type UserService struct {
	store      UserStoreInterface
	jwtSecret  string
	mailer     Mailer
	appBaseURL string
}

// NewUserService creates a UserService instance.
func NewUserService(store UserStoreInterface, jwtSecret string, mailer Mailer, appBaseURL string) *UserService {
	return &UserService{store: store, jwtSecret: jwtSecret, mailer: mailer, appBaseURL: appBaseURL}
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

// ForgotPassword generates a reset token and sends a reset email.
// If the email is not registered, we silently succeed to avoid user enumeration.
func (s *UserService) ForgotPassword(email string) error {
	slog.Debug("UserService.ForgotPassword called", "email", email)

	userID, err := s.store.GetUserIDByEmail(email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Silently succeed — don't reveal whether the email is registered.
			slog.Info("UserService.ForgotPassword: email not found, silently ignoring", "email", email)
			return nil
		}
		slog.Error("UserService.ForgotPassword: GetUserIDByEmail failed", "err", err, "email", email)
		return fmt.Errorf("user.UserService.ForgotPassword GetUserIDByEmail: %w", err)
	}

	token, err := generateToken()
	if err != nil {
		slog.Error("UserService.ForgotPassword: generateToken failed", "err", err)
		return fmt.Errorf("user.UserService.ForgotPassword generateToken: %w", err)
	}

	expiresAt := time.Now().Add(30 * time.Minute)
	if err := s.store.CreateResetToken(token, userID, expiresAt); err != nil {
		slog.Error("UserService.ForgotPassword: CreateResetToken failed", "err", err, "user_id", userID)
		return fmt.Errorf("user.UserService.ForgotPassword CreateResetToken: %w", err)
	}

	resetURL := s.appBaseURL + "/reset-password?token=" + token
	if err := s.mailer.SendPasswordReset(email, resetURL); err != nil {
		slog.Error("UserService.ForgotPassword: SendPasswordReset failed", "err", err, "email", email)
		return fmt.Errorf("user.UserService.ForgotPassword SendPasswordReset: %w", err)
	}

	slog.Info("UserService.ForgotPassword done", "user_id", userID)
	return nil
}

// ResetPassword validates the token and sets a new password.
func (s *UserService) ResetPassword(token, newPassword string) error {
	slog.Debug("UserService.ResetPassword called")

	rt, err := s.store.GetResetToken(token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("UserService.ResetPassword: token not found")
			return ErrTokenInvalid
		}
		slog.Error("UserService.ResetPassword: GetResetToken failed", "err", err)
		return fmt.Errorf("user.UserService.ResetPassword GetResetToken: %w", err)
	}

	if rt.Used {
		slog.Warn("UserService.ResetPassword: token already used", "user_id", rt.UserID)
		return ErrTokenInvalid
	}
	if time.Now().After(rt.ExpiresAt) {
		slog.Warn("UserService.ResetPassword: token expired", "user_id", rt.UserID, "expires_at", rt.ExpiresAt)
		return ErrTokenInvalid
	}

	newHash := hashPassword(newPassword)
	if err := s.store.UpdatePassword(rt.UserID, newHash); err != nil {
		slog.Error("UserService.ResetPassword: UpdatePassword failed", "err", err, "user_id", rt.UserID)
		return fmt.Errorf("user.UserService.ResetPassword UpdatePassword: %w", err)
	}

	if err := s.store.MarkTokenUsed(token); err != nil {
		slog.Error("UserService.ResetPassword: MarkTokenUsed failed", "err", err)
		return fmt.Errorf("user.UserService.ResetPassword MarkTokenUsed: %w", err)
	}

	slog.Info("UserService.ResetPassword done", "user_id", rt.UserID)
	return nil
}

// generateToken creates a 32-byte cryptographically random hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generateToken: %w", err)
	}
	return hex.EncodeToString(b), nil
}
