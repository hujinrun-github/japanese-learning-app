package shadowing

import (
	"context"
	"testing"

	"japanese-learning-app/internal/audio"
)

func TestEvaluateWAVDurationLimit(t *testing.T) {
	short, err := audio.EncodeWAV(make([]float64, 8000), 8000)
	if err != nil {
		t.Fatal(err)
	}
	long, err := audio.EncodeWAV(make([]float64, 8000*300), 8000)
	if err != nil {
		t.Fatal(err)
	}
	tooLong, err := audio.EncodeWAV(make([]float64, 8000*300+1), 8000)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		ref, user []byte
		wantError bool
	}{
		{"five-minute reference", long, short, false},
		{"five-minute recording", short, long, false},
		{"reference over five minutes", tooLong, short, true},
		{"recording over five minutes", short, tooLong, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := EvaluateWAV(tc.ref, tc.user, EvaluationModeRepeat); (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tc.wantError)
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
