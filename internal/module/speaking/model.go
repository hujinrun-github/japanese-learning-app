package speaking

import "time"

// PracticeType 口语练习类型
type PracticeType string

const (
	PracticeTypeShadow PracticeType = "shadow" // 影子跟读
	PracticeTypeFree   PracticeType = "free"   // 自由朗读
)

// SentenceAnnotation 评分后对单个句子的标注
type SentenceAnnotation struct {
	SentenceIndex  int    `json:"sentence_index"`
	Score          int    `json:"score"`            // 0~100，该句得分
	NeedsAttention bool   `json:"needs_attention"`  // 是否需要注意
	Note           string `json:"note,omitempty"`   // 提示说明
}

// ScoreResult 口语评分结果
type ScoreResult struct {
	OverallScore int                  `json:"overall_score"` // 0~100
	Annotations  []SentenceAnnotation `json:"annotations"`
	FeedbackMS   int64                `json:"feedback_ms"` // 评分耗时（毫秒，用于监控 SC-005）
}

// SpeakingRecord 一次口语练习记录
type SpeakingRecord struct {
	ID          int64        `json:"id"`
	UserID      int64        `json:"user_id"`
	Type        PracticeType `json:"type"`
	MaterialID  int64        `json:"material_id"`
	Score       int          `json:"score"`
	AudioRef    string       `json:"audio_ref"`   // 录音文件存储路径
	PracticedAt time.Time    `json:"practiced_at"`
}
