package user

import "time"

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
	ID         int64     `json:"id"`
	Email      string    `json:"email"`
	GoalLevel  JLPTLevel `json:"goal_level"` // 学习目标等级
	StreakDays int       `json:"streak_days"` // 连续学习天数
	CreatedAt  time.Time `json:"created_at"`
}

// RegisterReq 注册请求
type RegisterReq struct {
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
	StreakDays   int                   `json:"streak_days"`
	TotalMinutes int                   `json:"total_minutes"`
	ModuleStats  map[string]ModuleStat `json:"module_stats"`
}

// ModuleStat 单个模块的使用统计
type ModuleStat struct {
	SessionCount    int    `json:"session_count"`
	TotalMinutes    int    `json:"total_minutes"`
	LastPracticedAt string `json:"last_practiced_at,omitempty"`
}
