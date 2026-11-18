package grammar

import "testing"

func TestNormalizeGrammarName(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"plain", "から", "から"},
		{"leading wave dash", "〜から", "から"},
		{"fullwidth tilde", "～から", "から"},
		{"ascii tilde", "~から", "から"},
		{"multiple leading tildes", "〜〜から", "から"},
		{"japanese quotes", "「ていた」", "ていた"},
		{"surrounding spaces", "  〜ています  ", "ています"},
		{"fullwidth variant suffix", "〜そうだ（様態）", "そうだ"},
		{"ascii variant suffix", "〜そうだ(伝聞)", "そうだ"},
		{"inner wave dash is kept", "〜は〜です", "は〜です"},
		{"empty", "", ""},
		{"only decoration", "〜「」", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeGrammarName(tc.in); got != tc.want {
				t.Errorf("normalizeGrammarName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFindExactMatchID(t *testing.T) {
	defaultPoints := []GrammarPoint{
		{ID: 10, Name: "〜から"},
		{ID: 62, Name: "〜ことから"},
		{ID: 7, Name: "〜ています"},
	}
	for _, tc := range []struct {
		name   string
		query  string
		points []GrammarPoint
		want   int64
	}{
		{"annotated name matches library entry", "〜から", defaultPoints, 10},
		{"bare name matches library entry", "から", defaultPoints, 10},
		{"quoted name matches library entry", "「から」", defaultPoints, 10},
		{"variant suffix matches library entry", "そうだ（様態）", []GrammarPoint{{ID: 37, Name: "〜そうだ（様態）"}}, 37},
		{"exact name that is a substring of another", "ことから", defaultPoints, 62},
		{"no exact match", "〜ばかり", defaultPoints, 0},
		{"empty query", "", defaultPoints, 0},
		{"decoration only", "〜", defaultPoints, 0},
		// A bank annotation must not link to an unrelated longer grammar point.
		{"inflected form is not an exact match", "ていた", []GrammarPoint{{ID: 40, Name: "〜ていただけませんか"}}, 0},
		{"no candidates", "から", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := findExactMatchID(tc.query, tc.points); got != tc.want {
				t.Errorf("findExactMatchID(%q) = %d, want %d", tc.query, got, tc.want)
			}
		})
	}
}
