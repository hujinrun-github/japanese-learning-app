package shadowing

import (
	"context"
	"fmt"
)

const MaxEvaluationDurationMS int64 = 60_000
const ErrEvaluationBusy = "ERR_EVALUATION_BUSY"

// Evaluator bounds CPU work shared by speaking and lesson shadowing. A busy
// instance rejects work instead of retaining a queue of uploaded recordings.
type Evaluator struct{ slots chan struct{} }

func NewEvaluator(concurrency int) *Evaluator {
	return &Evaluator{slots: make(chan struct{}, max(1, concurrency))}
}

func (e *Evaluator) Evaluate(ctx context.Context, reference, recording []byte, mode EvaluationMode) (EvaluationResult, error) {
	if err := ctx.Err(); err != nil {
		return EvaluationResult{}, fmt.Errorf("evaluate audio: %w", err)
	}
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	case <-ctx.Done():
		return EvaluationResult{}, fmt.Errorf("evaluate audio: %w", ctx.Err())
	default:
		return EvaluationResult{}, &Error{Code: ErrEvaluationBusy, Message: "audio scoring is busy; please retry"}
	}
	return EvaluateWAV(reference, recording, mode)
}
