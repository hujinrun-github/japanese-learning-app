package shadowing

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"japanese-learning-app/internal/module/lesson"
)

type LessonReader interface {
	GetDetail(id int64) (*lesson.Lesson, error)
}

type Store interface {
	GetProgress(userID, lessonID int64, version int) (*Progress, error)
	UpsertProgress(p Progress) error
	InsertAttempt(a Attempt) error
	ListAttemptSummary(userID, lessonID int64, version int) ([]SentenceAttemptSummary, error)
	ListCompletedSentenceIndexes(userID, lessonID int64, version int) ([]int, error)
}

type Service struct {
	lessons LessonReader
	store   Store
}

func NewService(lessons LessonReader, store Store) *Service {
	return &Service{lessons: lessons, store: store}
}

func (s *Service) GetSession(userID, lessonID int64) (*ShadowingSession, error) {
	l, err := s.loadValidLesson("GetSession", lessonID)
	if err != nil {
		return nil, err
	}

	progress, err := s.store.GetProgress(userID, lessonID, l.ShadowingVersion)
	if err != nil {
		return nil, fmt.Errorf("shadowing.Service.GetSession: %w", err)
	}
	summary, err := s.store.ListAttemptSummary(userID, lessonID, l.ShadowingVersion)
	if err != nil {
		return nil, fmt.Errorf("shadowing.Service.GetSession: %w", err)
	}
	completed, err := s.store.ListCompletedSentenceIndexes(userID, lessonID, l.ShadowingVersion)
	if err != nil {
		return nil, fmt.Errorf("shadowing.Service.GetSession: %w", err)
	}
	if summary == nil {
		summary = []SentenceAttemptSummary{}
	}
	if completed == nil {
		completed = []int{}
	}

	return &ShadowingSession{
		Lesson:                   l,
		Progress:                 progress,
		AttemptSummary:           summary,
		CompletedSentenceCount:   len(completed),
		CompletedSentenceIndexes: completed,
	}, nil
}

func (s *Service) SaveProgress(userID, lessonID int64, req ProgressRequest) (*Progress, error) {
	l, err := s.loadValidLesson("SaveProgress", lessonID)
	if err != nil {
		return nil, err
	}
	if req.Version != l.ShadowingVersion {
		return nil, &Error{
			Code:                    ERR_SHADOWING_VERSION_STALE,
			Message:                 "shadowing version is stale",
			CurrentShadowingVersion: l.ShadowingVersion,
		}
	}
	if req.LastSentenceIndex < 0 || req.LastPositionMS < 0 || !isPracticeMode(req.PracticeMode) {
		return nil, &Error{Code: ERR_SHADOWING_CONTENT_INVALID, Message: "invalid shadowing progress request"}
	}

	progress := Progress{
		UserID:            userID,
		LessonID:          lessonID,
		ShadowingVersion:  req.Version,
		LastSentenceIndex: req.LastSentenceIndex,
		LastPositionMS:    req.LastPositionMS,
		LastPracticeMode:  req.PracticeMode,
	}
	if err := s.store.UpsertProgress(progress); err != nil {
		return nil, fmt.Errorf("shadowing.Service.SaveProgress: %w", err)
	}
	return &progress, nil
}

func (s *Service) SaveAttempt(userID, lessonID int64, req AttemptRequest) error {
	l, err := s.loadValidLesson("SaveAttempt", lessonID)
	if err != nil {
		return err
	}
	if req.Version != l.ShadowingVersion {
		return &Error{
			Code:                    ERR_SHADOWING_VERSION_STALE,
			Message:                 "shadowing version is stale",
			CurrentShadowingVersion: l.ShadowingVersion,
		}
	}
	if req.PracticeMode != PracticeModeLoop && req.PracticeMode != PracticeModeRecord {
		return &Error{Code: ERR_SHADOWING_ATTEMPT_MODE_INVALID, Message: "shadowing attempt mode must be loop or record"}
	}
	if !lessonHasSentenceIndex(l, req.SentenceIndex) || req.PlaybackRate <= 0 || req.LoopCount < 0 || !validSelfScore(req.SelfScore) {
		return &Error{Code: ERR_SHADOWING_CONTENT_INVALID, Message: "invalid shadowing attempt request"}
	}

	attempt := Attempt{
		UserID:           userID,
		LessonID:         lessonID,
		ShadowingVersion: req.Version,
		SentenceIndex:    req.SentenceIndex,
		PracticeMode:     req.PracticeMode,
		PlaybackRate:     req.PlaybackRate,
		LoopCount:        req.LoopCount,
		SelfScore:        req.SelfScore,
		RecognitionText:  req.RecognitionText,
		AudioRef:         req.AudioRef,
	}
	if err := s.store.InsertAttempt(attempt); err != nil {
		return fmt.Errorf("shadowing.Service.SaveAttempt: %w", err)
	}
	return nil
}

func (s *Service) loadValidLesson(method string, lessonID int64) (*lesson.Lesson, error) {
	l, err := s.lessons.GetDetail(lessonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &Error{Code: ERR_NOT_FOUND, Message: "lesson not found", Err: err}
	}
	if err != nil {
		return nil, fmt.Errorf("shadowing.Service.%s: %w", method, err)
	}
	if l == nil {
		return nil, &Error{Code: ERR_NOT_FOUND, Message: "lesson not found"}
	}
	if !l.ShadowingEnabled {
		return nil, &Error{Code: ERR_SHADOWING_DISABLED, Message: "shadowing is disabled for this lesson"}
	}
	if strings.TrimSpace(l.AudioURL) == "" {
		return nil, &Error{Code: ERR_SHADOWING_MEDIA_MISSING, Message: "shadowing audio is missing"}
	}
	if !validSentences(l.Sentences) {
		return nil, &Error{Code: ERR_SHADOWING_CONTENT_INVALID, Message: "shadowing sentence timings are invalid"}
	}
	return l, nil
}

func validSentences(sentences []lesson.Sentence) bool {
	if len(sentences) == 0 {
		return false
	}
	for _, sentence := range sentences {
		if sentence.EndMS <= sentence.StartMS {
			return false
		}
	}
	return true
}

func isPracticeMode(mode PracticeMode) bool {
	switch mode {
	case PracticeModeNormal, PracticeModeSlow, PracticeModeLoop, PracticeModeRecord:
		return true
	default:
		return false
	}
}

func lessonHasSentenceIndex(l *lesson.Lesson, sentenceIndex int) bool {
	if sentenceIndex < 0 {
		return false
	}
	for _, sentence := range l.Sentences {
		if sentence.Index == sentenceIndex {
			return true
		}
	}
	return false
}

func validSelfScore(score *int) bool {
	return score == nil || (*score >= 0 && *score <= 100)
}
