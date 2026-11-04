package translation

import "testing"

func TestNormalizeJSONResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain JSON", input: `{"ai_score":80}`, want: `{"ai_score":80}`},
		{name: "json code fence", input: "```json\n{\"ai_score\":80}\n```", want: `{"ai_score":80}`},
		{name: "whitespace", input: "  {\"ai_score\":80}  ", want: `{"ai_score":80}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeJSONResponse(tt.input); got != tt.want {
				t.Errorf("normalizeJSONResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}
