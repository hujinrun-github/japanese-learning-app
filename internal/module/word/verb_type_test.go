package word

import "testing"

func TestNormalizeVerbType(t *testing.T) {
	tests := []struct {
		name string
		pos  string
		want string
	}{
		{name: "empty part of speech", pos: "", want: ""},
		{name: "plain chinese verb", pos: "动词", want: ""},
		{name: "plain japanese verb", pos: "動詞", want: ""},
		{name: "ichidan verb", pos: "一段动词", want: ""},
		{name: "sahen verb", pos: "サ变动词", want: ""},
		{name: "zahen verb", pos: "ザ变动词", want: ""},
		{name: "noun", pos: "名詞", want: ""},
		{name: "intransitive chinese", pos: "自动词", want: VerbTypeIntransitive},
		{name: "transitive chinese", pos: "他动词", want: VerbTypeTransitive},
		{name: "intransitive japanese", pos: "自動詞", want: VerbTypeIntransitive},
		{name: "transitive japanese", pos: "他動詞", want: VerbTypeTransitive},
		{name: "both godan", pos: "自他動1", want: VerbTypeBoth},
		{name: "both ichidan", pos: "自他動2", want: VerbTypeBoth},
		{name: "both sahen", pos: "自他動3", want: VerbTypeBoth},
		{name: "compound intransitive", pos: "ナ形・名・自動3", want: VerbTypeIntransitive},
		{name: "compound intransitive with adverb", pos: "ナ形・副・名・自動3", want: VerbTypeIntransitive},
		{name: "compound both", pos: "ナ形・名・自他動3", want: VerbTypeBoth},
		{name: "compound transitive", pos: "ナ形・他動3・名", want: VerbTypeTransitive},
		{name: "adjectival noun is not a verb", pos: "形容動詞", want: ""},
		{name: "interjection is not a verb", pos: "感動詞", want: ""},
		{name: "sahen noun is not a verb", pos: "名詞・サ変", want: ""},
		{name: "auxiliary verb is not a verb", pos: "助动词", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeVerbType(tt.pos); got != tt.want {
				t.Fatalf("NormalizeVerbType(%q) = %q, want %q", tt.pos, got, tt.want)
			}
		})
	}
}
