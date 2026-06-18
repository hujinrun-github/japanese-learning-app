package shadowing

import (
	"fmt"
	"time"

	"japanese-learning-app/internal/module/lesson"
)

type PracticeMode string

const (
	PracticeModeNormal PracticeMode = "normal"
	PracticeModeSlow   PracticeMode = "slow"
	PracticeModeLoop   PracticeMode = "loop"
	PracticeModeRecord PracticeMode = "record"
)

const (
	ERR_UNAUTHORIZED                   = "ERR_UNAUTHORIZED"
	ERR_NOT_FOUND                      = "ERR_NOT_FOUND"
	ERR_SHADOWING_DISABLED             = "ERR_SHADOWING_DISABLED"
	ERR_SHADOWING_MEDIA_MISSING        = "ERR_SHADOWING_MEDIA_MISSING"
	ERR_SHADOWING_CONTENT_INVALID      = "ERR_SHADOWING_CONTENT_INVALID"
	ERR_SHADOWING_VERSION_STALE        = "ERR_SHADOWING_VERSION_STALE"
	ERR_SHADOWING_ATTEMPT_MODE_INVALID = "ERR_SHADOWING_ATTEMPT_MODE_INVALID"
)

const (
	ErrUnauthorized                = ERR_UNAUTHORIZED
	ErrNotFound                    = ERR_NOT_FOUND
	ErrShadowingDisabled           = ERR_SHADOWING_DISABLED
	ErrShadowingMediaMissing       = ERR_SHADOWING_MEDIA_MISSING
	ErrShadowingContentInvalid     = ERR_SHADOWING_CONTENT_INVALID
	ErrShadowingVersionStale       = ERR_SHADOWING_VERSION_STALE
	ErrShadowingAttemptModeInvalid = ERR_SHADOWING_ATTEMPT_MODE_INVALID
)

type Error struct {
	Code                    string `json:"code"`
	Message                 string `json:"message"`
	CurrentShadowingVersion int    `json:"current_shadowing_version,omitempty"`
	Err                     error  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Progress struct {
	ID                int64        `json:"id"`
	UserID            int64        `json:"user_id"`
	LessonID          int64        `json:"lesson_id"`
	ShadowingVersion  int          `json:"shadowing_version"`
	LastSentenceIndex int          `json:"last_sentence_index"`
	LastPositionMS    int64        `json:"last_position_ms"`
	LastPracticeMode  PracticeMode `json:"last_practice_mode"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type Attempt struct {
	ID               int64        `json:"id"`
	UserID           int64        `json:"user_id"`
	LessonID         int64        `json:"lesson_id"`
	ShadowingVersion int          `json:"shadowing_version"`
	SentenceIndex    int          `json:"sentence_index"`
	PracticeMode     PracticeMode `json:"practice_mode"`
	PlaybackRate     float64      `json:"playback_rate"`
	LoopCount        int          `json:"loop_count"`
	SelfScore        *int         `json:"self_score"`
	RecognitionText  string       `json:"recognition_text"`
	AudioRef         string       `json:"audio_ref"`
	CreatedAt        time.Time    `json:"created_at"`
}

type SentenceAttemptSummary struct {
	SentenceIndex int  `json:"sentence_index"`
	AttemptCount  int  `json:"attempt_count"`
	BestScore     *int `json:"best_score"`
	LastScore     *int `json:"last_score"`
}

type ShadowingSession struct {
	Lesson                   *lesson.Lesson           `json:"lesson"`
	Progress                 *Progress                `json:"progress,omitempty"`
	AttemptSummary           []SentenceAttemptSummary `json:"attempt_summary"`
	CompletedSentenceIndexes []int                    `json:"completed_sentence_indexes"`
}

type ProgressRequest struct {
	Version           int          `json:"version"`
	LastSentenceIndex int          `json:"last_sentence_index"`
	LastPositionMS    int64        `json:"last_position_ms"`
	PracticeMode      PracticeMode `json:"practice_mode"`
}

type AttemptRequest struct {
	Version         int          `json:"version"`
	SentenceIndex   int          `json:"sentence_index"`
	PracticeMode    PracticeMode `json:"practice_mode"`
	PlaybackRate    float64      `json:"playback_rate"`
	LoopCount       int          `json:"loop_count"`
	SelfScore       *int         `json:"self_score"`
	RecognitionText string       `json:"recognition_text"`
	AudioRef        string       `json:"audio_ref"`
}
