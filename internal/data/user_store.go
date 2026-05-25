package data

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/user"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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
func (s *UserStore) Create(name, email, passwordHash string, jlptLevelsJSON string) (*user.User, error) {
	slog.Debug("UserStore.Create called", "email", email, "name", name)

	res, err := s.db.Exec(
		`INSERT INTO users (name, email, password_hash, jlpt_levels) VALUES (?, ?, ?, ?)`,
		name, email, passwordHash, jlptLevelsJSON,
	)
	if err != nil {
		slog.Error("failed to insert user", "err", err, "email", email)
		if isUniqueConstraintError(err) {
			return nil, fmt.Errorf("data.UserStore.Create: %w", user.ErrEmailTaken)
		}
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
		`SELECT id, name, email, jlpt_levels, streak_days, created_at FROM users WHERE email = ?`, email,
	)

	var u user.User
	var createdAt string
	var jlptLevelsJSON string
	err := row.Scan(&u.ID, &u.Name, &u.Email, &jlptLevelsJSON, &u.StreakDays, &createdAt)
	if err == sql.ErrNoRows {
		slog.Error("user not found by email", "email", email)
		return nil, fmt.Errorf("data.UserStore.GetByEmail %q: %w", email, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan user", "err", err, "email", email)
		return nil, fmt.Errorf("data.UserStore.GetByEmail: %w", err)
	}

	u.JLPTLevels = parseJLPTLevels(jlptLevelsJSON)
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
		`SELECT id, name, email, jlpt_levels, streak_days, created_at FROM users WHERE id = ?`, id,
	)

	var u user.User
	var createdAt string
	var jlptLevelsJSON string
	err := row.Scan(&u.ID, &u.Name, &u.Email, &jlptLevelsJSON, &u.StreakDays, &createdAt)
	if err == sql.ErrNoRows {
		slog.Error("user not found by id", "user_id", id)
		return nil, fmt.Errorf("data.UserStore.GetByID %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan user", "err", err, "user_id", id)
		return nil, fmt.Errorf("data.UserStore.GetByID: %w", err)
	}

	u.JLPTLevels = parseJLPTLevels(jlptLevelsJSON)
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

// CreateResetToken stores a password-reset token for the given user.
func (s *UserStore) CreateResetToken(token string, userID int64, expiresAt time.Time) error {
	slog.Debug("UserStore.CreateResetToken called", "user_id", userID)

	_, err := s.db.Exec(
		`INSERT INTO password_reset_tokens (token, user_id, expires_at, used) VALUES (?, ?, ?, 0)`,
		token, userID, expiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		slog.Error("failed to insert reset token", "err", err, "user_id", userID)
		return fmt.Errorf("data.UserStore.CreateResetToken: %w", err)
	}

	slog.Debug("UserStore.CreateResetToken done", "user_id", userID)
	return nil
}

// GetResetToken returns the reset token row, or sql.ErrNoRows if not found.
func (s *UserStore) GetResetToken(token string) (*user.ResetToken, error) {
	slog.Debug("UserStore.GetResetToken called", "token", token)

	row := s.db.QueryRow(
		`SELECT token, user_id, expires_at, used FROM password_reset_tokens WHERE token = ?`, token,
	)
	var rt user.ResetToken
	var expiresAtStr string
	var usedInt int
	err := row.Scan(&rt.Token, &rt.UserID, &expiresAtStr, &usedInt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("data.UserStore.GetResetToken %q: %w", token, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan reset token", "err", err)
		return nil, fmt.Errorf("data.UserStore.GetResetToken: %w", err)
	}

	rt.ExpiresAt, err = parseSQLiteTime(expiresAtStr)
	if err != nil {
		return nil, fmt.Errorf("data.UserStore.GetResetToken parse expires_at: %w", err)
	}
	rt.Used = usedInt != 0

	slog.Debug("UserStore.GetResetToken done", "user_id", rt.UserID)
	return &rt, nil
}

// MarkTokenUsed marks a reset token as consumed.
func (s *UserStore) MarkTokenUsed(token string) error {
	slog.Debug("UserStore.MarkTokenUsed called", "token", token)

	_, err := s.db.Exec(`UPDATE password_reset_tokens SET used = 1 WHERE token = ?`, token)
	if err != nil {
		slog.Error("failed to mark token used", "err", err)
		return fmt.Errorf("data.UserStore.MarkTokenUsed: %w", err)
	}

	slog.Debug("UserStore.MarkTokenUsed done")
	return nil
}

// UpdatePassword sets a new password hash for the given user.
func (s *UserStore) UpdatePassword(userID int64, newPasswordHash string) error {
	slog.Debug("UserStore.UpdatePassword called", "user_id", userID)

	_, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, newPasswordHash, userID)
	if err != nil {
		slog.Error("failed to update password", "err", err, "user_id", userID)
		return fmt.Errorf("data.UserStore.UpdatePassword: %w", err)
	}

	slog.Debug("UserStore.UpdatePassword done", "user_id", userID)
	return nil
}

// UpdateUser updates the user's name, email, and jlpt_levels.
func (s *UserStore) UpdateUser(id int64, name, email, jlptLevelsJSON string) error {
	slog.Debug("UserStore.UpdateUser called", "user_id", id)

	_, err := s.db.Exec(
		`UPDATE users SET name = ?, email = ?, jlpt_levels = ? WHERE id = ?`,
		name, email, jlptLevelsJSON, id,
	)
	if err != nil {
		slog.Error("failed to update user", "err", err, "user_id", id)
		if isUniqueConstraintError(err) {
			return fmt.Errorf("data.UserStore.UpdateUser: %w", user.ErrEmailTaken)
		}
		return fmt.Errorf("data.UserStore.UpdateUser: %w", err)
	}

	slog.Debug("UserStore.UpdateUser done", "user_id", id)
	return nil
}

// GetStats returns the user's learning stats across all modules.
func (s *UserStore) GetStats(userID int64) (*user.UserStats, error) {
	slog.Debug("UserStore.GetStats called", "user_id", userID)

	stats := &user.UserStats{ModuleStats: make(map[string]user.ModuleStat)}

	var streakDays int
	var dailyGoalsJSON string
	if err := s.db.QueryRow(
		`SELECT streak_days, daily_goals_json FROM users WHERE id = ?`, userID,
	).Scan(&streakDays, &dailyGoalsJSON); err != nil {
		return nil, fmt.Errorf("data.UserStore.GetStats streak_days: %w", err)
	}
	stats.StreakDays = streakDays
	goals := parseDailyGoals(dailyGoalsJSON)

	// word
	wordTotal, wordDue, wordMastered := s.countWords(userID)
	stats.ModuleStats["word"] = user.ModuleStat{
		DueCount: wordDue, MasteredCount: wordMastered, TotalCount: wordTotal,
		TodayCompleted: s.countTodayCompleted(userID, "word"), DailyGoal: goals["word"],
	}

	// grammar
	grammarTotal, grammarDue, grammarMastered := s.countGrammar(userID)
	stats.ModuleStats["grammar"] = user.ModuleStat{
		DueCount: grammarDue, MasteredCount: grammarMastered, TotalCount: grammarTotal,
		TodayCompleted: s.countTodayCompleted(userID, "grammar"), DailyGoal: goals["grammar"],
	}

	// speaking
	speakingTotal, speakingMastered := s.countSpeaking(userID)
	stats.ModuleStats["speaking"] = user.ModuleStat{
		DueCount: 0, MasteredCount: speakingMastered, TotalCount: speakingTotal,
		TodayCompleted: s.countTodayCompleted(userID, "speaking"), DailyGoal: goals["speaking"],
	}

	// writing
	writingTotal, writingMastered := s.countWriting(userID)
	stats.ModuleStats["writing"] = user.ModuleStat{
		DueCount: 0, MasteredCount: writingMastered, TotalCount: writingTotal,
		TodayCompleted: s.countTodayCompleted(userID, "writing"), DailyGoal: goals["writing"],
	}

	slog.Debug("UserStore.GetStats done", "user_id", userID)
	return stats, nil
}

func (s *UserStore) countWords(userID int64) (total, due, mastered int) {
	s.db.QueryRow(`SELECT COUNT(*) FROM words`).Scan(&total)
	s.db.QueryRow(
		`SELECT COUNT(*) FROM word_records WHERE user_id = ? AND next_review_at <= datetime('now')`,
		userID,
	).Scan(&due)
	s.db.QueryRow(
		`SELECT COUNT(*) FROM word_records WHERE user_id = ? AND mastery_level >= 5`,
		userID,
	).Scan(&mastered)
	return
}

func (s *UserStore) countGrammar(userID int64) (total, due, mastered int) {
	s.db.QueryRow(`SELECT COUNT(*) FROM grammar_points`).Scan(&total)
	s.db.QueryRow(
		`SELECT COUNT(*) FROM grammar_records WHERE user_id = ? AND next_review_at <= datetime('now')`,
		userID,
	).Scan(&due)
	s.db.QueryRow(
		`SELECT COUNT(*) FROM grammar_records WHERE user_id = ? AND status = 'mastered'`,
		userID,
	).Scan(&mastered)
	return
}

func (s *UserStore) countSpeaking(userID int64) (total, mastered int) {
	s.db.QueryRow(`SELECT COUNT(*) FROM speaking_materials`).Scan(&total)
	s.db.QueryRow(
		`SELECT COUNT(*) FROM speaking_records WHERE user_id = ?`,
		userID,
	).Scan(&mastered)
	return
}

func (s *UserStore) countWriting(userID int64) (total, mastered int) {
	s.db.QueryRow(`SELECT COUNT(*) FROM writing_questions`).Scan(&total)
	s.db.QueryRow(
		`SELECT COUNT(*) FROM writing_records WHERE user_id = ? AND score >= 70`,
		userID,
	).Scan(&mastered)
	return
}

func defaultDailyGoals() map[string]int {
	return map[string]int{"word": 20, "grammar": 5, "speaking": 3, "writing": 3}
}

func (s *UserStore) countTodayCompleted(userID int64, module string) int {
	var count int
	switch module {
	case "word":
		s.db.QueryRow(
			`SELECT COUNT(*) FROM word_records
			 WHERE user_id = ? AND date(updated_at) = date('now')`,
			userID,
		).Scan(&count)
	case "grammar":
		s.db.QueryRow(
			`SELECT COUNT(*) FROM grammar_records
			 WHERE user_id = ? AND date(updated_at) = date('now')`,
			userID,
		).Scan(&count)
	case "speaking":
		s.db.QueryRow(
			`SELECT COUNT(*) FROM speaking_records
			 WHERE user_id = ? AND date(practiced_at) = date('now')`,
			userID,
		).Scan(&count)
	case "writing":
		s.db.QueryRow(
			`SELECT COUNT(*) FROM writing_records
			 WHERE user_id = ? AND date(practiced_at) = date('now')`,
			userID,
		).Scan(&count)
	default:
		s.db.QueryRow(
			`SELECT COALESCE(SUM(completed_count), 0)
			 FROM study_sessions
			 WHERE user_id = ? AND module = ? AND date(started_at) = date('now')`,
			userID, module,
		).Scan(&count)
	}
	return count
}

func (s *UserStore) GetDailyGoals(userID int64) (map[string]int, error) {
	slog.Debug("UserStore.GetDailyGoals called", "user_id", userID)
	var jsonStr string
	if err := s.db.QueryRow(
		`SELECT daily_goals_json FROM users WHERE id = ?`, userID,
	).Scan(&jsonStr); err != nil {
		slog.Error("failed to query daily_goals_json", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.UserStore.GetDailyGoals: %w", err)
	}
	goals := parseDailyGoals(jsonStr)
	slog.Debug("UserStore.GetDailyGoals done", "user_id", userID)
	return goals, nil
}

func (s *UserStore) UpdateDailyGoals(userID int64, goals map[string]int) error {
	slog.Debug("UserStore.UpdateDailyGoals called", "user_id", userID)
	jsonBytes, err := json.Marshal(goals)
	if err != nil {
		slog.Error("failed to marshal daily goals", "err", err)
		return fmt.Errorf("data.UserStore.UpdateDailyGoals marshal: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE users SET daily_goals_json = ? WHERE id = ?`,
		string(jsonBytes), userID,
	)
	if err != nil {
		slog.Error("failed to update daily_goals_json", "err", err, "user_id", userID)
		return fmt.Errorf("data.UserStore.UpdateDailyGoals exec: %w", err)
	}
	slog.Debug("UserStore.UpdateDailyGoals done", "user_id", userID)
	return nil
}

func parseDailyGoals(jsonStr string) map[string]int {
	goals := defaultDailyGoals()
	if jsonStr == "" || jsonStr == "{}" {
		return goals
	}
	var stored map[string]int
	if err := json.Unmarshal([]byte(jsonStr), &stored); err != nil {
		slog.Warn("failed to parse daily_goals_json, using defaults", "err", err)
		return goals
	}
	for k, v := range stored {
		goals[k] = v
	}
	return goals
}

// isUniqueConstraintError reports whether err is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// ListAllUsers 分页查询所有用户，返回用户列表和总数。
func (s *UserStore) ListAllUsers(offset, limit int) ([]user.User, int, error) {
	slog.Debug("UserStore.ListAllUsers called", "offset", offset, "limit", limit)

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("data.UserStore.ListAllUsers count: %w", err)
	}

	rows, err := s.db.Query(
		`SELECT id, name, email, jlpt_levels, streak_days, created_at FROM users ORDER BY id LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("data.UserStore.ListAllUsers query: %w", err)
	}
	defer rows.Close()

	var users []user.User
	for rows.Next() {
		var u user.User
		var createdAt string
		var jlptLevelsJSON string
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &jlptLevelsJSON, &u.StreakDays, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("data.UserStore.ListAllUsers scan: %w", err)
		}
		u.JLPTLevels = parseJLPTLevels(jlptLevelsJSON)
		u.CreatedAt, err = parseSQLiteTime(createdAt)
		if err != nil {
			return nil, 0, fmt.Errorf("data.UserStore.ListAllUsers parse created_at: %w", err)
		}
		users = append(users, u)
	}

	slog.Debug("UserStore.ListAllUsers done", "count", len(users), "total", total)
	return users, total, rows.Err()
}

// parseJLPTLevels parses a JSON array string like '["N5","N4"]' into a []string.
// Returns []string{"N5"} for empty or unparseable input.
func parseJLPTLevels(jsonStr string) []string {
	if jsonStr == "" {
		return []string{"N5"}
	}
	var levels []string
	if err := json.Unmarshal([]byte(jsonStr), &levels); err != nil {
		return []string{"N5"}
	}
	if len(levels) == 0 {
		return []string{"N5"}
	}
	return levels
}
