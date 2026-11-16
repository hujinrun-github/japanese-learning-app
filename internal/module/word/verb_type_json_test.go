package word

import (
	"encoding/json"
	"testing"
)

func TestWordJSONIncludesVerbType(t *testing.T) {
	tests := []struct {
		name     string
		verbType string
		want     string
	}{
		{name: "transitive", verbType: VerbTypeTransitive, want: "他动词"},
		{name: "intransitive", verbType: VerbTypeIntransitive, want: "自动词"},
		{name: "both", verbType: VerbTypeBoth, want: "自他两用"},
		{name: "empty", verbType: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(Word{
				KanjiForm: "開ける",
				VerbType:  tt.verbType,
			})
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			got, ok := fields["verb_type"]
			if !ok {
				t.Fatalf("json is missing verb_type: %s", raw)
			}
			if got != tt.want {
				t.Fatalf("verb_type = %v, want %q", got, tt.want)
			}
		})
	}
}
