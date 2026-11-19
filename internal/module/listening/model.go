// Package listening implements blind listening, intensive review and retesting.
package listening

import (
	"context"
	"time"

	"japanese-learning-app/internal/module/lesson"
)

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string      { return e.Message }
func Invalid(message string) error  { return &Error{422, "ERR_LISTENING_INVALID", message} }
func Conflict(message string) error { return &Error{409, "ERR_LISTENING_CONFLICT", message} }
func NotFound() error               { return &Error{404, "ERR_NOT_FOUND", "听力材料不存在或尚不可用"} }

type Answer struct {
	QuestionID  string `json:"question_id"`
	OptionIndex *int   `json:"option_index"`
}
type DraftAnswers struct {
	Phase    string   `json:"phase"`
	Answers  []Answer `json:"answers"`
	Assisted *bool    `json:"assisted,omitempty"`
}
type Question struct {
	ID                    string   `json:"id"`
	Type                  string   `json:"type"`
	Prompt                string   `json:"prompt"`
	Options               []string `json:"options"`
	CorrectOptionIndex    int      `json:"-"`
	Explanation           string   `json:"-"`
	EvidenceSentenceIndex int      `json:"-"`
	Version               int      `json:"-"`
}
type Progress struct {
	Stage          string        `json:"stage"`
	SentenceIndex  int           `json:"sentence_index"`
	PositionMS     int64         `json:"position_ms"`
	Bookmarks      []int         `json:"bookmarks"`
	DraftAnswers   *DraftAnswers `json:"draft_answers,omitempty"`
	Cycle          int           `json:"cycle"`
	ReviewStep     int           `json:"review_step"`
	CompletedCount int           `json:"completed_count"`
	DueAt          *time.Time    `json:"due_at"`
}
type ProgressRequest struct {
	ExpectedCycle int           `json:"expected_cycle"`
	Restart       bool          `json:"restart,omitempty"`
	Stage         string        `json:"stage"`
	SentenceIndex int           `json:"sentence_index"`
	PositionMS    int64         `json:"position_ms"`
	Bookmarks     []int         `json:"bookmarks"`
	DraftAnswers  *DraftAnswers `json:"draft_answers,omitempty"`
}
type AttemptRequest struct {
	Phase          string   `json:"phase"`
	Answers        []Answer `json:"answers"`
	Assisted       bool     `json:"assisted"`
	IdempotencyKey string   `json:"idempotency_key"`
}
type AnswerResult struct {
	QuestionID            string `json:"question_id"`
	OptionIndex           int    `json:"option_index"`
	CorrectOptionIndex    int    `json:"correct_option_index"`
	Correct               bool   `json:"correct"`
	Explanation           string `json:"explanation"`
	EvidenceSentenceIndex int    `json:"evidence_sentence_index"`
}
type AttemptResult struct {
	ID           int64          `json:"id"`
	Cycle        int            `json:"cycle"`
	Phase        string         `json:"phase"`
	CorrectCount int            `json:"correct_count"`
	Total        int            `json:"total"`
	Assisted     bool           `json:"assisted"`
	Results      []AnswerResult `json:"results"`
	CreatedAt    time.Time      `json:"created_at"`
}
type Session struct {
	AudioSource   string          `json:"audio_source"`
	Lesson        *lesson.Lesson  `json:"lesson"`
	Questions     []Question      `json:"questions"`
	Progress      Progress        `json:"progress"`
	Attempts      []AttemptResult `json:"attempts"`
	TrainingReady bool            `json:"training_ready"`
}
type Summary struct {
	AudioSource   string           `json:"audio_source"`
	ID            int64            `json:"id"`
	Title         string           `json:"title"`
	JLPTLevel     lesson.JLPTLevel `json:"jlpt_level"`
	AudioURL      string           `json:"audio_url"`
	VideoURL      string           `json:"video_url"`
	DurationMS    int64            `json:"duration_ms"`
	SentenceCount int              `json:"sentence_count"`
	QuestionCount int              `json:"question_count"`
	Status        string           `json:"status"`
	TrainingReady bool             `json:"training_ready"`
	Progress      Progress         `json:"progress"`
}
type Stats struct {
	Total     int `json:"total"`
	New       int `json:"new"`
	Practiced int `json:"practiced"`
	Due       int `json:"due"`
}
type Materials struct {
	Materials []Summary `json:"materials"`
	Stats     Stats     `json:"stats"`
}
type Store interface {
	ListMaterials(context.Context, int64, string, string) (Materials, error)
	GetSession(context.Context, int64, int64) (Session, error)
	SaveProgress(context.Context, int64, int64, ProgressRequest) (Progress, error)
	SubmitAttempt(context.Context, int64, int64, AttemptRequest) (AttemptResult, error)
	Complete(context.Context, int64, int64) (Progress, error)
}
