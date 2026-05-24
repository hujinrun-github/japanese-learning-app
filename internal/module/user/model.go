package user

import "time"

// ResetToken 密码重置令牌
type ResetToken struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

// ForgotPasswordReq 忘记密码请求
type ForgotPasswordReq struct {
	Email string `json:"email"`
}

// ResetPasswordReq 重置密码请求
type ResetPasswordReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// UpdateProfileReq 更新个人信息请求
type UpdateProfileReq struct {
	Name       string   `json:"name"`
	Email      string   `json:"email"`
	JLPTLevels []string `json:"jlpt_levels"`
}

// UpdatePasswordReq 修改密码请求
type UpdatePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// DailyGoalsReq 每日学习目标请求
type DailyGoalsReq struct {
	Word     int `json:"word"`
	Grammar  int `json:"grammar"`
	Speaking int `json:"speaking"`
	Writing  int `json:"writing"`
}

// JLPTLevel 表示 JLPT 等级（独立定义，避免循环依赖）
type JLPTLevel string

const (
	LevelN5 JLPTLevel = "N5"
	LevelN4 JLPTLevel = "N4"
	LevelN3 JLPTLevel = "N3"
	LevelN2 JLPTLevel = "N2"
	LevelN1 JLPTLevel = "N1"
)

// User 用户账户
type User struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	JLPTLevels  []string  `json:"jlpt_levels"`
	StreakDays  int       `json:"streak_days"`
	CreatedAt   time.Time `json:"created_at"`
}

// RegisterReq 注册请求
type RegisterReq struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`   // 明文，服务端立即哈希，不持久化
	GoalLevel JLPTLevel `json:"goal_level"`
}

// LoginReq 登录请求
type LoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenResp 登录成功响应
type TokenResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// UserStats 学习统计看板数据
type UserStats struct {
	StreakDays  int                   `json:"streak_days"`
	ModuleStats map[string]ModuleStat `json:"modules"`
}

// ModuleStat 单个模块的进度统计
type ModuleStat struct {
	DueCount       int `json:"due_count"`
	MasteredCount  int `json:"mastered_count"`
	TotalCount     int `json:"total_count"`
	TodayCompleted int `json:"today_completed"`
	DailyGoal      int `json:"daily_goal"`
}
