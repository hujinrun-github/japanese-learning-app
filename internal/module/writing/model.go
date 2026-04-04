package writing

import "time"

// WritingType 写作练习类型
type WritingType string

const (
	WritingTypeInput    WritingType = "input"    // 键盘输入练习
	WritingTypeSentence WritingType = "sentence" // 造句练习
)

// WritingQuestion 写作题目
type WritingQuestion struct {
	ID             int64       `json:"id"`
	Type           WritingType `json:"type"`
	Prompt         string      `json:"prompt"`                       // 题目提示（中文或假名）
	GrammarPointID int64       `json:"grammar_point_id,omitempty"`   // 造句题关联的语法点
	ExpectedAnswer string      `json:"-"`                            // 仅后端存储，不返回前端
}

// AIFeedback AI 批改结果
type AIFeedback struct {
	Score              int      `json:"score"`               // 0~100
	GrammarCorrect     bool     `json:"grammar_correct"`
	VocabCorrect       bool     `json:"vocab_correct"`
	IssueDescription   string   `json:"issue_description"`   // 问题说明（空表示全对）
	CorrectedSentence  string   `json:"corrected_sentence"`  // 修改后的句子
	AlternativePhrases []string `json:"alternative_phrases"` // 其他地道表达
	ReferenceAnswer    string   `json:"reference_answer"`
}

// WritingRecord 一次写作练习记录
type WritingRecord struct {
	ID          int64       `json:"id"`
	UserID      int64       `json:"user_id"`
	Type        WritingType `json:"type"`
	Question    string      `json:"question"`
	UserAnswer  string      `json:"user_answer"`
	AIFeedback  *AIFeedback `json:"ai_feedback,omitempty"` // 输入练习无 AI 反馈
	Score       int         `json:"score"`
	PracticedAt time.Time   `json:"practiced_at"`
}
