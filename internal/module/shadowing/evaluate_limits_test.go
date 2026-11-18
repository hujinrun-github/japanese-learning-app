package shadowing

import (
	"context"
	"testing"

	"japanese-learning-app/internal/audio"
)

func TestEvaluateWAVRejectsLongAudio(t *testing.T) {
	short, err := audio.EncodeWAV(make([]float64, 8000), 8000)
	if err != nil {
		t.Fatal(err)
	}
	long, err := audio.EncodeWAV(make([]float64, 8000*61), 8000)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		ref, user []byte
	}{
		{"reference", long, short}, {"recording", short, long},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := EvaluateWAV(tc.ref, tc.user, EvaluationModeRepeat); err == nil {
				t.Fatal("audio longer than 60 seconds must be rejected before feature extraction")
			}
		})
	}
}

func TestEvaluatorConcurrencyAndCancellation(t *testing.T) {
	e := NewEvaluator(1)
	e.slots <- struct{}{}
	if _, err := e.Evaluate(context.Background(), nil, nil, EvaluationModeRepeat); err == nil {
		t.Fatal("busy evaluator accepted another request")
	}
	<-e.slots
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Evaluate(ctx, nil, nil, EvaluationModeRepeat); err == nil {
		t.Fatal("cancelled evaluation accepted")
	}
	if len(e.slots) != 0 {
		t.Fatal("evaluation slot leaked")
	}
}
