package grammar_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"japanese-learning-app/internal/module/grammar"
)

func TestGrammarService_SearchByName(t *testing.T) {
	bankErr := errors.New("db unavailable")
	for _, tc := range []struct {
		name      string
		query     string
		points    []grammar.GrammarPoint
		storeErr  error
		wantExact int64
		wantIDs   []int64
		wantQuery string
		wantErr   bool
	}{
		{
			name:      "quoted annotation finds library entry",
			query:     "「から」",
			points:    []grammar.GrammarPoint{{ID: 10, Name: "〜から", JLPTLevel: grammar.LevelN5}},
			wantExact: 10, wantIDs: []int64{10}, wantQuery: "「から」",
		},
		{
			name:      "fullwidth tilde finds wave-dash library entry",
			query:     "～から",
			points:    []grammar.GrammarPoint{{ID: 10, Name: "〜から", JLPTLevel: grammar.LevelN5}},
			wantExact: 10, wantIDs: []int64{10}, wantQuery: "～から",
		},
		{
			name:  "exact match is found and ranked first",
			query: "から",
			points: []grammar.GrammarPoint{
				{ID: 62, Name: "〜ことから", Meaning: "由于", JLPTLevel: grammar.LevelN3},
				{ID: 10, Name: "〜から", Meaning: "因为", JLPTLevel: grammar.LevelN5},
				{ID: 63, Name: "〜からこそ", Meaning: "正因为", JLPTLevel: grammar.LevelN3},
			},
			// The two non-exact names are equally long, so stable sorting keeps
			// their original (id) order. Only the exact match is promoted.
			wantExact: 10,
			wantIDs:   []int64{10, 62, 63},
			wantQuery: "から",
		},
		{
			name:  "shorter name ranks ahead of a longer substring match",
			query: "から",
			points: []grammar.GrammarPoint{
				{ID: 62, Name: "〜ということから", Meaning: "由于", JLPTLevel: grammar.LevelN3},
				{ID: 63, Name: "〜からこそ", Meaning: "正因为", JLPTLevel: grammar.LevelN3},
			},
			wantIDs:   []int64{63, 62},
			wantQuery: "から",
		},
		{
			name:      "query is trimmed",
			query:     "  〜から  ",
			points:    []grammar.GrammarPoint{{ID: 10, Name: "〜から", Meaning: "因为", JLPTLevel: grammar.LevelN5}},
			wantExact: 10,
			wantIDs:   []int64{10},
			wantQuery: "〜から",
		},
		{
			name:      "annotation without a library entry still returns related points",
			query:     "ていた",
			points:    []grammar.GrammarPoint{{ID: 40, Name: "〜ていただけませんか", Meaning: "能否请您", JLPTLevel: grammar.LevelN4}},
			wantExact: 0,
			wantIDs:   []int64{40},
			wantQuery: "ていた",
		},
		{
			name:      "empty query returns an empty result without touching the store",
			query:     "   ",
			storeErr:  bankErr,
			wantExact: 0,
			wantIDs:   []int64{},
			wantQuery: "",
		},
		{
			name:      "no candidates",
			query:     "〜ばかり",
			wantIDs:   []int64{},
			wantQuery: "〜ばかり",
		},
		{
			name:     "store error is wrapped",
			query:    "から",
			storeErr: bankErr,
			wantErr:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeGrammarStore()
			for _, p := range tc.points {
				store.seedPoint(p)
			}
			store.listAllErr = tc.storeErr

			svc := grammar.NewGrammarService(store)
			result, err := svc.SearchByName(tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !errors.Is(err, bankErr) {
					t.Errorf("error should wrap the store error, got %v", err)
				}
				if !strings.Contains(err.Error(), "SearchByName") {
					t.Errorf("error should name the failing operation, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Query != tc.wantQuery {
				t.Errorf("query = %q, want %q", result.Query, tc.wantQuery)
			}
			if result.ExactMatchID != tc.wantExact {
				t.Errorf("exact_match_id = %d, want %d", result.ExactMatchID, tc.wantExact)
			}
			var gotIDs []int64
			for _, hit := range result.Points {
				gotIDs = append(gotIDs, hit.ID)
			}
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Errorf("ids = %v, want %v", gotIDs, tc.wantIDs)
			}
			if result.Points == nil {
				t.Error("points should be an empty slice, not nil")
			}
		})
	}
}

func TestGrammarService_SearchByName_DoesNotLeakQuizAnswers(t *testing.T) {
	store := newFakeGrammarStore()
	store.seedPoint(grammar.GrammarPoint{
		ID:        10,
		Name:      "〜から",
		Meaning:   "因为",
		JLPTLevel: grammar.LevelN5,
		QuizQuestions: []grammar.QuizQuestion{
			{ID: 1, Answer: "secret-answer", Explanation: "secret-explanation"},
		},
	})

	result, err := grammar.NewGrammarService(store).SearchByName("から")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Points) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Points))
	}
	hit := result.Points[0]
	if hit.ID != 10 || hit.Name != "〜から" || hit.Meaning != "因为" || hit.JLPTLevel != grammar.LevelN5 {
		t.Errorf("unexpected hit: %+v", hit)
	}
}
