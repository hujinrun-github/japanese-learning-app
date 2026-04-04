package word

import "time"

// JLPTLevel 表示 JLPT 等级
type JLPTLevel string

const (
	LevelN5 JLPTLevel = "N5"
	LevelN4 JLPTLevel = "N4"
	LevelN3 JLPTLevel = "N3"
	LevelN2 JLPTLevel = "N2"
	LevelN1 JLPTLevel = "N1"
)

// ReviewRating 用户对单词的三级评分
type ReviewRating string

const (
	RatingEasy   ReviewRating = "easy"
	RatingNormal ReviewRating = "normal"
	RatingHard   ReviewRating = "hard"
)

// WordExample 单词例句
type WordExample struct {
	Japanese string `json:"japanese"`
	Chinese  string `json:"chinese"`
}

// Word 表示词库中的一个日语单词（内容库，只读）
type Word struct {
	ID           int64        `json:"id"`
	KanjiForm    string       `json:"kanji_form"`     // 汉字写法，如「勉強」
	Reading      string       `json:"reading"`        // 假名读音，如「べんきょう」
	PartOfSpeech string       `json:"part_of_speech"` // 词性，如「名詞」
	Meaning      string       `json:"meaning"`        // 中文释义
	Examples     []WordExample `json:"examples"`
	JLPTLevel    JLPTLevel    `json:"jlpt_level"`
}

// WordRecord 用户与某个单词的学习关系（用户数据，读写）
type WordRecord struct {
	ID            int64         `json:"id"`
	UserID        int64         `json:"user_id"`
	WordID        int64         `json:"word_id"`
	MasteryLevel  int           `json:"mastery_level"` // 0~5，SM-2 重复次数
	NextReviewAt  time.Time     `json:"next_review_at"`
	EaseFactor    float64       `json:"ease_factor"` // SM-2 EF，初始 2.5
	Interval      int           `json:"interval"`    // 距下次复习的天数
	ReviewHistory []ReviewEvent `json:"review_history"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// ReviewEvent 一次评分事件记录
type ReviewEvent struct {
	Rating     ReviewRating `json:"rating"`
	ReviewedAt time.Time    `json:"reviewed_at"`
}

// WordCard 复习队列中的单张卡片（聚合 Word + WordRecord）
type WordCard struct {
	Word   Word       `json:"word"`
	Record WordRecord `json:"record"`
	IsNew  bool       `json:"is_new"` // true 表示首次学习
}
