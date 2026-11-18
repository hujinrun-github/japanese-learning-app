package grammar_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"japanese-learning-app/internal/module/grammar"
)

type grammarSearchResponse struct {
	Data struct {
		Query        string `json:"query"`
		ExactMatchID int64  `json:"exact_match_id"`
		Points       []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"points"`
	} `json:"data"`
	Code string `json:"code"`
}

func TestGrammarHandler_SearchRoute(t *testing.T) {
	bankErr := errors.New("db unavailable")
	for _, tc := range []struct {
		name       string
		q          string
		points     []grammar.GrammarPoint
		storeErr   error
		wantStatus int
		wantExact  int64
		wantCount  int
		wantCode   string
	}{
		{
			name: "exact match resolves to a detail page id",
			q:    "から",
			points: []grammar.GrammarPoint{
				{ID: 10, Name: "〜から", Meaning: "因为", JLPTLevel: grammar.LevelN5},
				{ID: 62, Name: "〜ことから", Meaning: "由于", JLPTLevel: grammar.LevelN3},
			},
			wantStatus: http.StatusOK,
			wantExact:  10,
			wantCount:  2,
		},
		{
			name: "annotation without a library entry returns related points only",
			q:    "ていた",
			points: []grammar.GrammarPoint{
				{ID: 40, Name: "〜ていただけませんか", Meaning: "能否请您", JLPTLevel: grammar.LevelN4},
			},
			wantStatus: http.StatusOK,
			wantExact:  0,
			wantCount:  1,
		},
		{
			name:       "no results",
			q:          "〜ばかり",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "missing q is rejected before touching the store",
			q:          "",
			storeErr:   bankErr,
			wantStatus: http.StatusBadRequest,
			wantCode:   "ERR_BAD_REQUEST",
		},
		{
			name:       "blank q is rejected",
			q:          "   ",
			wantStatus: http.StatusBadRequest,
			wantCode:   "ERR_BAD_REQUEST",
		},
		{
			name:       "store failure becomes a 500",
			q:          "から",
			storeErr:   bankErr,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "ERR_INTERNAL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeGrammarStore()
			for _, p := range tc.points {
				store.seedPoint(p)
			}
			store.listAllErr = tc.storeErr

			mux := http.NewServeMux()
			grammar.NewGrammarHandler(grammar.NewGrammarService(store)).RegisterRoutes(mux)

			path := "/api/v1/grammar/search?q=" + url.QueryEscape(tc.q)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var got grammarSearchResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("invalid JSON response: %v (body %s)", err, rec.Body.String())
			}
			if tc.wantCode != "" && got.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tc.wantCode)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			if got.Data.ExactMatchID != tc.wantExact {
				t.Errorf("exact_match_id = %d, want %d", got.Data.ExactMatchID, tc.wantExact)
			}
			if len(got.Data.Points) != tc.wantCount {
				t.Errorf("points count = %d, want %d", len(got.Data.Points), tc.wantCount)
			}
		})
	}
}

// The new literal /search path must not shadow the existing /{id} detail route.
func TestGrammarHandler_DetailRouteStillWorks(t *testing.T) {
	store := newFakeGrammarStore()
	store.seedPoint(grammar.GrammarPoint{
		ID: 10, Name: "〜から", Meaning: "因为", JLPTLevel: grammar.LevelN5,
	})

	mux := http.NewServeMux()
	grammar.NewGrammarHandler(grammar.NewGrammarService(store)).RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/grammar/10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Data struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got.Data.ID != 10 || got.Data.Name != "〜から" {
		t.Errorf("unexpected detail payload: %+v", got.Data)
	}
}
