package lesson

// JLPTLevel 表示 JLPT 等级（独立定义，避免循环依赖）
type JLPTLevel string

const (
	LevelN5 JLPTLevel = "N5"
	LevelN4 JLPTLevel = "N4"
	LevelN3 JLPTLevel = "N3"
	LevelN2 JLPTLevel = "N2"
	LevelN1 JLPTLevel = "N1"
)

// FuriganaToken 振り仮名标注单元
// 对于有汉字的词：{ Surface: "勉強", Reading: "べんきょう" }
// 对于假名直接：  { Surface: "です", Reading: "" }
type FuriganaToken struct {
	Surface string `json:"surface"` // 显示文字
	Reading string `json:"reading"` // 假名读音（空字符串表示无需标注）
}

// Sentence 课文中的一个句子
type Sentence struct {
	Index   int             `json:"index"`
	Tokens  []FuriganaToken `json:"tokens"`   // 振り仮名分词结果
	Chinese string          `json:"chinese"`  // 中文翻译
	StartMS int64           `json:"start_ms"` // 音频开始时间（毫秒）
	EndMS   int64           `json:"end_ms"`   // 音频结束时间（毫秒）
}

// LessonSummary 课文列表项（不含全文内容，减少传输量）
type LessonSummary struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	JLPTLevel JLPTLevel `json:"jlpt_level"`
	Tags      []string  `json:"tags"`
	CharCount int       `json:"char_count"`
	AudioURL  string    `json:"audio_url"`
}

// Lesson 课文详情
type Lesson struct {
	LessonSummary
	Sentences []Sentence `json:"sentences"`
	WordIDs   []int64    `json:"word_ids"` // 课文中可加入单词本的词汇 ID 列表
}
