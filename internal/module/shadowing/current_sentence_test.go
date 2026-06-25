package shadowing

import (
	"testing"

	"japanese-learning-app/internal/module/lesson"
)

func TestCurrentSentenceIndex(t *testing.T) {
	sentences := []lesson.Sentence{
		{Index: 10, StartMS: 1000, EndMS: 2000},
		{Index: 11, StartMS: 3000, EndMS: 4000},
		{Index: 12, StartMS: 4500, EndMS: 5500},
	}

	tests := []struct {
		name          string
		sentences     []lesson.Sentence
		currentTimeMS int64
		wantIndex     int
		wantOK        bool
	}{
		{
			name:          "empty list",
			sentences:     nil,
			currentTimeMS: 1000,
			wantIndex:     -1,
			wantOK:        false,
		},
		{
			name:          "before first sentence returns first index",
			sentences:     sentences,
			currentTimeMS: 500,
			wantIndex:     10,
			wantOK:        true,
		},
		{
			name:          "exact start uses that sentence",
			sentences:     sentences,
			currentTimeMS: 3000,
			wantIndex:     11,
			wantOK:        true,
		},
		{
			name:          "inside sentence returns that sentence",
			sentences:     sentences,
			currentTimeMS: 3500,
			wantIndex:     11,
			wantOK:        true,
		},
		{
			name:          "gap returns previous valid sentence",
			sentences:     sentences,
			currentTimeMS: 2500,
			wantIndex:     10,
			wantOK:        true,
		},
		{
			name:          "after last sentence returns last valid index",
			sentences:     sentences,
			currentTimeMS: 9000,
			wantIndex:     12,
			wantOK:        true,
		},
		{
			name: "invalid sentences are skipped",
			sentences: []lesson.Sentence{
				{Index: 1, StartMS: 0, EndMS: 0},
				{Index: 2, StartMS: 1000, EndMS: 900},
				{Index: 3, StartMS: 2000, EndMS: 3000},
			},
			currentTimeMS: 1500,
			wantIndex:     3,
			wantOK:        true,
		},
		{
			name: "all invalid sentences return not found",
			sentences: []lesson.Sentence{
				{Index: 1, StartMS: 0, EndMS: 0},
				{Index: 2, StartMS: 1000, EndMS: 999},
			},
			currentTimeMS: 1500,
			wantIndex:     -1,
			wantOK:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotOK := CurrentSentenceIndex(tt.sentences, tt.currentTimeMS)
			if gotIndex != tt.wantIndex || gotOK != tt.wantOK {
				t.Fatalf("CurrentSentenceIndex() = (%d, %v), want (%d, %v)", gotIndex, gotOK, tt.wantIndex, tt.wantOK)
			}
		})
	}
}
