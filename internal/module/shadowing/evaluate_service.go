package shadowing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/lesson"
)

const maxUserAudioBytes = 2 << 20

// EvaluationService orchestrates reference lookup and local scoring. It is
// intentionally independent from the progress/attempt Service so S2 needs no
// schema changes and cannot break existing shadowing behavior.
type EvaluationService struct {
	lessons   LessonReader
	refAudio  ReferenceAudioReader
	evaluator *Evaluator
}

func NewEvaluationService(lessons LessonReader, refAudio ReferenceAudioReader, evaluator *Evaluator) *EvaluationService {
	return &EvaluationService{lessons: lessons, refAudio: refAudio, evaluator: evaluator}
}

func (s *EvaluationService) Evaluate(ctx context.Context, req EvaluationRequest) (EvaluationResult, error) {
	start := time.Now()
	slog.Debug("shadowing.EvaluationService.Evaluate called",
		"user_id", req.UserID, "lesson_id", req.LessonID, "sentence_index", req.SentenceIndex,
		"mode", req.Mode, "audio_bytes", len(req.UserAudio))
	result := newEvaluationResult(req.Mode)

	if err := validateEvaluationRequest(req); err != nil {
		slog.Error("shadowing.EvaluationService.Evaluate: invalid request", "err", err)
		return result, err
	}

	l, err := s.lessons.GetDetail(req.LessonID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, &Error{Code: ErrNotFound, Message: "lesson not found", Err: err}
	}
	if err != nil {
		return result, fmt.Errorf("shadowing.EvaluationService.Evaluate GetDetail: %w", err)
	}
	if l == nil {
		return result, &Error{Code: ErrNotFound, Message: "lesson not found"}
	}
	if !l.ShadowingEnabled {
		return result, &Error{Code: ErrShadowingDisabled, Message: "shadowing is disabled for this lesson"}
	}
	if req.ShadowingVersion != l.ShadowingVersion {
		return result, &Error{
			Code:                    ERR_SHADOWING_VERSION_STALE,
			Message:                 "shadowing version is stale",
			CurrentShadowingVersion: l.ShadowingVersion,
		}
	}
	var sentence lesson.Sentence
	found := false
	for _, candidate := range l.Sentences {
		if candidate.Index == req.SentenceIndex {
			sentence = candidate
			found = true
			break
		}
	}
	if !found {
		return result, &Error{Code: ErrShadowingEvalInvalid, Message: "sentence index not found"}
	}

	referenceWAV, err := s.refAudio.ReadSentence(ctx, l, sentence)
	if err != nil {
		return result, err
	}

	result, err = s.evaluator.Evaluate(ctx, referenceWAV, req.UserAudio, req.Mode)
	if err != nil {
		slog.Error("shadowing.EvaluationService.Evaluate: scoring failed", "err", err)
		var busy *Error
		if errors.As(err, &busy) && busy.Code == ErrEvaluationBusy {
			return result, err
		}
		return result, &Error{
			Code:    ErrShadowingAudioUnsupported,
			Message: "failed to analyze the recording",
			Err:     err,
		}
	}
	result.ShadowingVersion = req.ShadowingVersion
	result.LatencyMS = time.Since(start).Milliseconds()

	slog.Debug("shadowing.EvaluationService.Evaluate done",
		"status", result.Status, "code", result.UnscorableCode, "latency_ms", result.LatencyMS)
	return result, nil
}

func validateEvaluationRequest(req EvaluationRequest) error {
	if req.Mode != EvaluationModeShadow && req.Mode != EvaluationModeRepeat {
		return &Error{Code: ErrShadowingEvalInvalid, Message: "mode must be shadow or repeat"}
	}
	if req.LessonID <= 0 || req.ShadowingVersion <= 0 || req.SentenceIndex < 0 {
		return &Error{Code: ErrShadowingEvalInvalid, Message: "invalid lesson, version or sentence index"}
	}
	if len(req.UserAudio) == 0 || len(req.UserAudio) > maxUserAudioBytes {
		return &Error{Code: ErrShadowingEvalInvalid, Message: "audio is empty or too large"}
	}
	return nil
}
