package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/store"
)

type UserStore struct {
	db       queryer
	now      func() time.Time
	location *time.Location
}

func NewUserStore(db queryer, deps store.StoreDeps) *UserStore {
	return &UserStore{
		db:       db,
		now:      currentTimeFunc(deps.Clock),
		location: appLocation(deps.AppTimezone),
	}
}

func (s *UserStore) CreateUser(u user.User, passwordHash string) (*user.User, error) {
	levels := normalizeJLPTLevels(u.JLPTLevels)
	levelsJSON, err := json.Marshal(levels)
	if err != nil {
		return nil, fmt.Errorf("postgres.UserStore.CreateUser marshal jlpt levels: %w", err)
	}

	var createdAt any
	if !u.CreatedAt.IsZero() {
		createdAt = u.CreatedAt.UTC()
	}

	var id int64
	err = s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO users (email, name, password_hash, goal_level, jlpt_levels, streak_days, created_at)
		 VALUES (
			$1,
			$2,
			$3,
			$4,
			COALESCE(
				(SELECT array_agg(value) FROM jsonb_array_elements_text($5::jsonb) AS value),
				ARRAY['N5']::text[]
			),
			$6,
			COALESCE($7, now())
		 )
		 RETURNING id`,
		u.Email,
		u.Name,
		passwordHash,
		goalLevel(levels),
		string(levelsJSON),
		u.StreakDays,
		createdAt,
	).Scan(&id)
	if err != nil {
		return nil, wrapUserWriteError("CreateUser", err)
	}

	created, err := s.GetUserByID(id)
	if err != nil {
		return nil, fmt.Errorf("postgres.UserStore.CreateUser get created user: %w", err)
	}

	return created, nil
}

func (s *UserStore) GetUserByEmail(email string) (*user.User, string, error) {
	record, err := s.getUserRecordBy("email", email)
	if err != nil {
		return nil, "", err
	}
	return &record.user, record.passwordHash, nil
}

func (s *UserStore) GetUserByID(id int64) (*user.User, error) {
	record, err := s.getUserRecordBy("id", id)
	if err != nil {
		return nil, err
	}
	return &record.user, nil
}

func (s *UserStore) GetStats(userID int64) (*user.UserStats, error) {
	stats := &user.UserStats{ModuleStats: make(map[string]user.ModuleStat)}

	var streakDays int
	var dailyGoalsJSON string
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT streak_days, daily_goals_json::text FROM users WHERE id = $1`,
		userID,
	).Scan(&streakDays, &dailyGoalsJSON)
	if err != nil {
		return nil, fmt.Errorf("postgres.UserStore.GetStats user: %w", translateError(err))
	}

	stats.StreakDays = streakDays
	goals := parseDailyGoals(dailyGoalsJSON)

	now := s.now().In(s.location)
	start, end := dayWindow(now)

	wordTotal, err := s.countRows(`SELECT COUNT(*) FROM words`)
	if err != nil {
		return nil, err
	}
	wordDue, err := s.countRows(
		`SELECT COUNT(*) FROM word_records WHERE user_id = $1 AND next_review_at <= $2`,
		userID, now.UTC(),
	)
	if err != nil {
		return nil, err
	}
	wordMastered, err := s.countRows(
		`SELECT COUNT(*) FROM word_records WHERE user_id = $1 AND mastery_level >= 5`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	wordToday, err := s.countRows(
		`SELECT COUNT(*) FROM word_records WHERE user_id = $1 AND updated_at >= $2 AND updated_at < $3`,
		userID, start.UTC(), end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	stats.ModuleStats["word"] = user.ModuleStat{
		DueCount:       wordDue,
		MasteredCount:  wordMastered,
		TotalCount:     wordTotal,
		TodayCompleted: wordToday,
		DailyGoal:      goals["word"],
	}

	grammarTotal, err := s.countRows(`SELECT COUNT(*) FROM grammar_points`)
	if err != nil {
		return nil, err
	}
	grammarDue, err := s.countRows(
		`SELECT COUNT(*) FROM grammar_records WHERE user_id = $1 AND next_review_at <= $2`,
		userID, now.UTC(),
	)
	if err != nil {
		return nil, err
	}
	grammarMastered, err := s.countRows(
		`SELECT COUNT(*) FROM grammar_records WHERE user_id = $1 AND status = 'mastered'`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	grammarToday, err := s.countRows(
		`SELECT COUNT(*) FROM grammar_records WHERE user_id = $1 AND updated_at >= $2 AND updated_at < $3`,
		userID, start.UTC(), end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	stats.ModuleStats["grammar"] = user.ModuleStat{
		DueCount:       grammarDue,
		MasteredCount:  grammarMastered,
		TotalCount:     grammarTotal,
		TodayCompleted: grammarToday,
		DailyGoal:      goals["grammar"],
	}

	speakingTotal, err := s.countRows(`SELECT COUNT(*) FROM speaking_materials`)
	if err != nil {
		return nil, err
	}
	speakingMastered, err := s.countRows(
		`SELECT COUNT(*) FROM speaking_records WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	speakingToday, err := s.countRows(
		`SELECT COUNT(*) FROM speaking_records WHERE user_id = $1 AND practiced_at >= $2 AND practiced_at < $3`,
		userID, start.UTC(), end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	stats.ModuleStats["speaking"] = user.ModuleStat{
		DueCount:       0,
		MasteredCount:  speakingMastered,
		TotalCount:     speakingTotal,
		TodayCompleted: speakingToday,
		DailyGoal:      goals["speaking"],
	}

	writingTotal, err := s.countRows(`SELECT COUNT(*) FROM writing_questions`)
	if err != nil {
		return nil, err
	}
	writingMastered, err := s.countRows(
		`SELECT COUNT(*) FROM writing_records WHERE user_id = $1 AND score >= 70`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	writingToday, err := s.countRows(
		`SELECT COUNT(*) FROM writing_records WHERE user_id = $1 AND practiced_at >= $2 AND practiced_at < $3`,
		userID, start.UTC(), end.UTC(),
	)
	if err != nil {
		return nil, err
	}
	stats.ModuleStats["writing"] = user.ModuleStat{
		DueCount:       0,
		MasteredCount:  writingMastered,
		TotalCount:     writingTotal,
		TodayCompleted: writingToday,
		DailyGoal:      goals["writing"],
	}

	return stats, nil
}

func (s *UserStore) UpdateDailyGoals(userID int64, goals map[string]int) error {
	jsonBytes, err := json.Marshal(goals)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.UpdateDailyGoals marshal: %w", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`UPDATE users SET daily_goals_json = $2::jsonb WHERE id = $1`,
		userID,
		string(jsonBytes),
	)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.UpdateDailyGoals: %w", translateError(err))
	}

	return nil
}

func (s *UserStore) GetDailyGoals(userID int64) (map[string]int, error) {
	var jsonStr string
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT daily_goals_json::text FROM users WHERE id = $1`,
		userID,
	).Scan(&jsonStr)
	if err != nil {
		return nil, fmt.Errorf("postgres.UserStore.GetDailyGoals: %w", translateError(err))
	}

	return parseDailyGoals(jsonStr), nil
}

func (s *UserStore) GetUserIDByEmail(email string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT id FROM users WHERE email = $1`,
		email,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.UserStore.GetUserIDByEmail: %w", translateError(err))
	}
	return id, nil
}

func (s *UserStore) CreateResetToken(token string, userID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO password_reset_tokens (token, user_id, expires_at, used) VALUES ($1, $2, $3, FALSE)`,
		token,
		userID,
		expiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.CreateResetToken: %w", translateError(err))
	}
	return nil
}

func (s *UserStore) GetResetToken(token string) (*user.ResetToken, error) {
	var rt user.ResetToken
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT token, user_id, expires_at, used FROM password_reset_tokens WHERE token = $1`,
		token,
	).Scan(&rt.Token, &rt.UserID, &rt.ExpiresAt, &rt.Used)
	if err != nil {
		return nil, fmt.Errorf("postgres.UserStore.GetResetToken: %w", translateError(err))
	}
	return &rt, nil
}

func (s *UserStore) MarkTokenUsed(token string) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE password_reset_tokens SET used = TRUE WHERE token = $1`,
		token,
	)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.MarkTokenUsed: %w", translateError(err))
	}
	return nil
}

func (s *UserStore) UpdatePassword(userID int64, newPasswordHash string) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE users SET password_hash = $2 WHERE id = $1`,
		userID,
		newPasswordHash,
	)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.UpdatePassword: %w", translateError(err))
	}
	return nil
}

func (s *UserStore) UpdateUser(id int64, name, email string, jlptLevels []string) error {
	levels := normalizeJLPTLevels(jlptLevels)
	levelsJSON, err := json.Marshal(levels)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.UpdateUser marshal jlpt levels: %w", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`UPDATE users
		 SET name = $2,
		     email = $3,
		     goal_level = $4,
		     jlpt_levels = COALESCE(
		         (SELECT array_agg(value) FROM jsonb_array_elements_text($5::jsonb) AS value),
		         ARRAY['N5']::text[]
		     )
		 WHERE id = $1`,
		id,
		name,
		email,
		goalLevel(levels),
		string(levelsJSON),
	)
	if err != nil {
		return wrapUserWriteError("UpdateUser", err)
	}
	return nil
}

func (s *UserStore) ListAllUsers(offset, limit int) ([]user.User, int, error) {
	total, err := s.countRows(`SELECT COUNT(*) FROM users`)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, name, email, password_hash, COALESCE(array_to_json(jlpt_levels)::text, '[]'), streak_days, created_at
		 FROM users
		 ORDER BY id DESC
		 LIMIT $1 OFFSET $2`,
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.UserStore.ListAllUsers query: %w", translateError(err))
	}
	defer rows.Close()

	var users []user.User
	for rows.Next() {
		record, err := scanUserRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, record.user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("postgres.UserStore.ListAllUsers rows: %w", translateError(err))
	}

	return users, total, nil
}

func (s *UserStore) DeleteUser(id int64) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`DELETE FROM users WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("postgres.UserStore.DeleteUser: %w", translateError(err))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres.UserStore.DeleteUser rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("postgres.UserStore.DeleteUser: %w", errors.Join(store.ErrNotFound, sql.ErrNoRows))
	}

	return nil
}

type userRecord struct {
	user         user.User
	passwordHash string
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *UserStore) getUserRecordBy(field string, value any) (*userRecord, error) {
	var query string
	switch field {
	case "email":
		query = `SELECT id, name, email, password_hash, COALESCE(array_to_json(jlpt_levels)::text, '[]'), streak_days, created_at FROM users WHERE email = $1`
	case "id":
		query = `SELECT id, name, email, password_hash, COALESCE(array_to_json(jlpt_levels)::text, '[]'), streak_days, created_at FROM users WHERE id = $1`
	default:
		return nil, fmt.Errorf("postgres.UserStore.getUserRecordBy: unsupported field %q", field)
	}

	record, err := scanUserRecord(s.db.QueryRowContext(context.Background(), query, value))
	if err != nil {
		return nil, err
	}
	return record, nil
}

func scanUserRecord(scanner rowScanner) (*userRecord, error) {
	var record userRecord
	var jlptLevelsJSON string
	err := scanner.Scan(
		&record.user.ID,
		&record.user.Name,
		&record.user.Email,
		&record.passwordHash,
		&jlptLevelsJSON,
		&record.user.StreakDays,
		&record.user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.scanUserRecord: %w", translateError(err))
	}

	record.user.JLPTLevels = parseJLPTLevels(jlptLevelsJSON)
	return &record, nil
}

func (s *UserStore) countRows(query string, args ...any) (int, error) {
	var count int
	err := s.db.QueryRowContext(context.Background(), query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("postgres.UserStore.countRows: %w", translateError(err))
	}
	return count, nil
}

func dayWindow(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 1)
}

func normalizeJLPTLevels(levels []string) []string {
	if len(levels) == 0 {
		return []string{string(user.LevelN5)}
	}
	return levels
}

func goalLevel(levels []string) string {
	normalized := normalizeJLPTLevels(levels)
	return normalized[0]
}

func parseJLPTLevels(jsonStr string) []string {
	var levels []string
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &levels); err == nil && len(levels) > 0 {
			return levels
		}
	}
	return []string{string(user.LevelN5)}
}

func parseDailyGoals(jsonStr string) map[string]int {
	goals := map[string]int{
		"word":     20,
		"grammar":  5,
		"speaking": 3,
		"writing":  3,
	}

	if jsonStr == "" || jsonStr == "{}" {
		return goals
	}

	var stored map[string]int
	if err := json.Unmarshal([]byte(jsonStr), &stored); err != nil {
		return goals
	}

	for key, value := range stored {
		goals[key] = value
	}

	return goals
}

func wrapUserWriteError(op string, err error) error {
	translated := translateError(err)
	if errors.Is(translated, store.ErrDuplicate) {
		return fmt.Errorf("postgres.UserStore.%s: %w", op, errors.Join(user.ErrEmailTaken, translated))
	}
	return fmt.Errorf("postgres.UserStore.%s: %w", op, translated)
}
