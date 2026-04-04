package data

import (
	"database/sql"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/user"
)

// UserStore 实现用户数据访问，对应 users 表。
type UserStore struct {
	db *sql.DB
}

// NewUserStore 创建 UserStore 实例。
func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// Create 创建新用户，返回创建后的用户数据。邮箱重复时返回 error。
func (s *UserStore) Create(email, passwordHash string, goalLevel user.JLPTLevel) (*user.User, error) {
	slog.Debug("UserStore.Create called", "email", email, "goal_level", goalLevel)

	res, err := s.db.Exec(
		`INSERT INTO users (email, password_hash, goal_level) VALUES (?, ?, ?)`,
		email, passwordHash, goalLevel,
	)
	if err != nil {
		slog.Error("failed to insert user", "err", err, "email", email)
		return nil, fmt.Errorf("data.UserStore.Create: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		slog.Error("failed to get last insert id", "err", err)
		return nil, fmt.Errorf("data.UserStore.Create last insert id: %w", err)
	}

	u, err := s.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("data.UserStore.Create get after insert: %w", err)
	}

	slog.Debug("UserStore.Create done", "user_id", id, "email", email)
	return u, nil
}

// GetByEmail 按邮箱查询用户，不存在时返回 error。
func (s *UserStore) GetByEmail(email string) (*user.User, error) {
	slog.Debug("UserStore.GetByEmail called", "email", email)

	row := s.db.QueryRow(
		`SELECT id, email, goal_level, streak_days, created_at FROM users WHERE email = ?`, email,
	)

	var u user.User
	var createdAt string
	err := row.Scan(&u.ID, &u.Email, &u.GoalLevel, &u.StreakDays, &createdAt)
	if err == sql.ErrNoRows {
		slog.Error("user not found by email", "email", email)
		return nil, fmt.Errorf("data.UserStore.GetByEmail %q: %w", email, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan user", "err", err, "email", email)
		return nil, fmt.Errorf("data.UserStore.GetByEmail: %w", err)
	}

	u.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("data.UserStore.GetByEmail parse created_at: %w", err)
	}

	slog.Debug("UserStore.GetByEmail done", "user_id", u.ID, "email", email)
	return &u, nil
}

// GetByID 按 ID 查询用户，不存在时返回 error。
func (s *UserStore) GetByID(id int64) (*user.User, error) {
	slog.Debug("UserStore.GetByID called", "user_id", id)

	row := s.db.QueryRow(
		`SELECT id, email, goal_level, streak_days, created_at FROM users WHERE id = ?`, id,
	)

	var u user.User
	var createdAt string
	err := row.Scan(&u.ID, &u.Email, &u.GoalLevel, &u.StreakDays, &createdAt)
	if err == sql.ErrNoRows {
		slog.Error("user not found by id", "user_id", id)
		return nil, fmt.Errorf("data.UserStore.GetByID %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan user", "err", err, "user_id", id)
		return nil, fmt.Errorf("data.UserStore.GetByID: %w", err)
	}

	u.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("data.UserStore.GetByID parse created_at: %w", err)
	}

	slog.Debug("UserStore.GetByID done", "user_id", id)
	return &u, nil
}

// GetPasswordHash 按邮箱查询用户密码哈希，用于登录验证。
func (s *UserStore) GetPasswordHash(email string) (string, error) {
	slog.Debug("UserStore.GetPasswordHash called", "email", email)

	var hash string
	err := s.db.QueryRow(`SELECT password_hash FROM users WHERE email = ?`, email).Scan(&hash)
	if err == sql.ErrNoRows {
		slog.Error("user not found for password hash", "email", email)
		return "", fmt.Errorf("data.UserStore.GetPasswordHash %q: %w", email, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to query password_hash", "err", err, "email", email)
		return "", fmt.Errorf("data.UserStore.GetPasswordHash: %w", err)
	}

	slog.Debug("UserStore.GetPasswordHash done", "email", email)
	return hash, nil
}

// UpdateStreak 更新用户的连续学习天数。
func (s *UserStore) UpdateStreak(userID int64, streakDays int) error {
	slog.Debug("UserStore.UpdateStreak called", "user_id", userID, "streak_days", streakDays)

	_, err := s.db.Exec(
		`UPDATE users SET streak_days = ? WHERE id = ?`, streakDays, userID,
	)
	if err != nil {
		slog.Error("failed to update streak", "err", err, "user_id", userID)
		return fmt.Errorf("data.UserStore.UpdateStreak: %w", err)
	}

	slog.Debug("UserStore.UpdateStreak done", "user_id", userID, "streak_days", streakDays)
	return nil
}
