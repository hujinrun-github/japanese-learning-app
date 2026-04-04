package summary

import "time"

// StudySession 一次学习会话（对应 study_sessions 表）
type StudySession struct {
	ID              int64      `json:"id"`
	SessionID       string     `json:"session_id"`       // UUID 字符串
	UserID          int64      `json:"user_id"`
	Module          ModuleType `json:"module"`
	DurationSeconds int        `json:"duration_seconds"`
	CompletedCount  int        `json:"completed_count"`
	StartedAt       time.Time  `json:"started_at"`
}

// ModuleType 学习模块类型
type ModuleType string

const (
	ModuleWord     ModuleType = "word"
	ModuleGrammar  ModuleType = "grammar"
	ModuleLesson   ModuleType = "lesson"
	ModuleSpeaking ModuleType = "speaking"
	ModuleWriting  ModuleType = "writing"
)

// SummaryItem 总结中的单条亮点或待改进项
type SummaryItem struct {
	Label string `json:"label"` // 对象名称（如单词、语法点名称）
	Note  string `json:"note"`  // 说明（如「连续3次评为容易」）
}

// ScoreSummary 得分概要（各模块字段不同，使用灵活 map）
// 单词：{ "reviewed": 15, "easy_rate": 0.47, "hard_count": 4 }
// 语法：{ "score": 80, "correct": 2, "total": 3 }
// 口语：{ "score": 78, "history_avg": 71 }
// 写作：{ "completed": 4, "avg_score": 82 }
type ScoreSummary map[string]any

// SessionSummary 一次练习会话的总结
type SessionSummary struct {
	ID                     int64         `json:"id"`
	UserID                 int64         `json:"user_id"`
	SessionID              string        `json:"session_id"`
	Module                 ModuleType    `json:"module"`
	ScoreSummary           ScoreSummary  `json:"score_summary"`
	Strengths              []SummaryItem `json:"strengths"`               // 亮点
	Weaknesses             []SummaryItem `json:"weaknesses"`              // 待改进
	ImprovementSuggestions []string      `json:"improvement_suggestions"` // 1~3 条建议
	GeneratedAt            time.Time     `json:"generated_at"`
}
