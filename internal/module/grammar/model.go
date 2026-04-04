package grammar

import "time"

// JLPTLevel 表示 JLPT 等级（与 word 包保持一致的独立定义，避免循环依赖）
type JLPTLevel string

const (
	LevelN5 JLPTLevel = "N5"
	LevelN4 JLPTLevel = "N4"
	LevelN3 JLPTLevel = "N3"
	LevelN2 JLPTLevel = "N2"
	LevelN1 JLPTLevel = "N1"
)

// QuizType 检验题类型
type QuizType string

const (
	QuizFillBlank   QuizType = "fill_blank"   // 填空
	QuizMultiChoice QuizType = "multi_choice" // 选择
)

// QuizQuestion 语法检验题
type QuizQuestion struct {
	ID          int64    `json:"id"`
	Type        QuizType `json:"type"`
	Prompt      string   `json:"prompt"`            // 题目（含空格标记，如「___てもいい」）
	Options     []string `json:"options,omitempty"` // 选择题选项
	Answer      string   `json:"answer"`            // 正确答案（服务端存储，响应时不返回）
	Explanation string   `json:"explanation"`       // 解析（答错后展示）
}

// GrammarExample 语法例句
type GrammarExample struct {
	Japanese    string  `json:"japanese"`
	Chinese     string  `json:"chinese"`
	LinkedWords []int64 `json:"linked_word_ids,omitempty"` // 可一键加入单词本的词汇
}

// GrammarPoint 语法点（内容库）
type GrammarPoint struct {
	ID              int64            `json:"id"`
	Name            string           `json:"name"`             // 如「〜てもいい」
	Meaning         string           `json:"meaning"`          // 中文意思
	ConjunctionRule string           `json:"conjunction_rule"` // 接续方式
	UsageNote       string           `json:"usage_note"`       // 使用场景说明
	Examples        []GrammarExample `json:"examples"`
	QuizQuestions   []QuizQuestion   `json:"quiz_questions"`
	JLPTLevel       JLPTLevel        `json:"jlpt_level"`
}

// GrammarStatus 用户对语法点的学习状态
type GrammarStatus string

const (
	StatusUnlearned GrammarStatus = "unlearned" // 未学
	StatusLearning  GrammarStatus = "learning"  // 学习中
	StatusMastered  GrammarStatus = "mastered"  // 已掌握
)

// GrammarRecord 用户语法学习记录
type GrammarRecord struct {
	ID             int64         `json:"id"`
	UserID         int64         `json:"user_id"`
	GrammarPointID int64         `json:"grammar_point_id"`
	Status         GrammarStatus `json:"status"`
	NextReviewAt   time.Time     `json:"next_review_at"`
	QuizHistory    []QuizAttempt `json:"quiz_history"`
}

// QuizAttempt 一次检验记录
type QuizAttempt struct {
	Score       int       `json:"score"`        // 本次得分（0~100）
	AttemptedAt time.Time `json:"attempted_at"`
}

// QuizSubmission 用户提交的检验答案
type QuizSubmission struct {
	QuestionID int64  `json:"question_id"`
	Answer     string `json:"answer"`
}

// QuizResult 检验结果
type QuizResult struct {
	Score   int              `json:"score"` // 0~100
	Results []QuizItemResult `json:"results"`
}

// QuizItemResult 单题结果
type QuizItemResult struct {
	QuestionID  int64  `json:"question_id"`
	Correct     bool   `json:"correct"`
	UserAnswer  string `json:"user_answer"`
	Expected    string `json:"expected"`
	Explanation string `json:"explanation"` // 答错时返回
}
