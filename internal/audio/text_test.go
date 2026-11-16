package audio_test

import (
	"testing"

	"japanese-learning-app/internal/audio"
)

func TestNormalizeJA(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "hiragana kept", in: "がっこうへいきます", want: "がっこうへいきます"},
		{name: "katakana to hiragana", in: "コンビニ", want: "こんびに"},
		{name: "kanji and punctuation dropped", in: "七時、です。", want: "です"},
		{name: "long vowel mark kept", in: "コーヒー", want: "こーひー"},
		{name: "mixed", in: "私はパンとコーヒーを飲みます！", want: "はぱんとこーひーをみます"},
		{name: "latin and digits dropped", in: "7時 ABC です", want: "です"},
		{name: "sokuon and n kept", in: "きって、ほんとう", want: "きってほんとう"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := audio.NormalizeJA(tt.in); got != tt.want {
				t.Errorf("NormalizeJA(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
