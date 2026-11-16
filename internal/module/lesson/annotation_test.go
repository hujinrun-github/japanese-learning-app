package lesson

import "testing"

func TestAnnotationMatchesLessonText(t *testing.T) {
	sentences := []Sentence{
		{Tokens: []FuriganaToken{{Surface: "𠮷野", Reading: "よしの"}, {Surface: "です。"}}},
		{Tokens: []FuriganaToken{{Surface: "散歩", Reading: "さんぽ"}}},
	}
	for _, tt := range []struct {
		name       string
		start, end int
		quote      string
		want       bool
	}{
		{"supplementary character", 0, 2, "𠮷野", true},
		{"across sentences", 4, 6, "。散", true},
		{"stale selection", 0, 2, "吉野", false},
		{"invalid range", -1, 2, "𠮷野", false},
		{"out of bounds", 6, 8, "歩", false},
		{"empty", 1, 1, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := annotationMatchesText(sentences, tt.start, tt.end, tt.quote)
			if got != tt.want {
				t.Errorf("annotationMatchesText = %v, want %v", got, tt.want)
			}
		})
	}
}
