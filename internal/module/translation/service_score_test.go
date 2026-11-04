package translation

import "testing"

func TestComputeRuleScoreForReference(t *testing.T) {
	tests := []struct {
		name      string
		direction string
		source    string
		answer    string
		reference string
		minScore  int
		maxScore  int
	}{
		{
			name:      "exact answer",
			direction: "jp2cn",
			source:    "こんにちは。",
			answer:    "你好。",
			reference: "你好。",
			minScore:  100,
			maxScore:  100,
		},
		{
			name:      "formatting differences are ignored",
			direction: "jp2cn",
			source:    "こんにちは。",
			answer:    "你 好！",
			reference: "你好。",
			minScore:  100,
			maxScore:  100,
		},
		{
			name:      "partial answer",
			direction: "jp2cn",
			source:    "こんにちは。",
			answer:    "你们好。",
			reference: "你好。",
			minScore:  1,
			maxScore:  99,
		},
		{
			name:      "unrelated answer",
			direction: "jp2cn",
			source:    "こんにちは。",
			answer:    "完全不同的答案。",
			reference: "你好。",
			minScore:  0,
			maxScore:  50,
		},
		{
			name:      "missing answer",
			direction: "cn2jp",
			source:    "你好。",
			answer:    "   ",
			reference: "こんにちは。",
			minScore:  0,
			maxScore:  0,
		},
		{
			name:      "no reference stays conservative",
			direction: "cn2jp",
			source:    "你好。",
			answer:    "こんにちは。",
			minScore:  1,
			maxScore:  75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeRuleScoreForReference(tt.direction, tt.source, tt.answer, tt.reference)
			if got < tt.minScore || got > tt.maxScore {
				t.Errorf("computeRuleScoreForReference() = %d, want %d..%d", got, tt.minScore, tt.maxScore)
			}
		})
	}
}
