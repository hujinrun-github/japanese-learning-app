package grammar

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// grammarSearchLimit caps a cross-level search so a broad query cannot pull
// the whole grammar library into one response.
const grammarSearchLimit = 50

// GrammarSearchHit is the public, answer-free projection of a grammar point.
// Search results must never expose quiz answers, unlike the full GrammarPoint.
type GrammarSearchHit struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Meaning   string    `json:"meaning"`
	JLPTLevel JLPTLevel `json:"jlpt_level"`
}

// GrammarSearchResult is returned by GET /api/v1/grammar/search.
// ExactMatchID is the grammar_points row whose name matches the query after
// normalization; zero means the library has no full entry for this annotation
// and the caller should show the related hits instead.
type GrammarSearchResult struct {
	Query        string             `json:"query"`
	ExactMatchID int64              `json:"exact_match_id"`
	Points       []GrammarSearchHit `json:"points"`
}

// normalizeGrammarName removes the decorative markers used differently by the
// translation question bank and the grammar library: leading placeholder
// tildes, quote wrappers, parenthesised variants and surrounding whitespace.
// Inner tildes are kept because they are part of patterns such as 〜は〜です.
func normalizeGrammarName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.TrimLeft(s, " 　〜～~「」『』\"'")
	s = strings.TrimRight(s, " 　「」『』\"'")
	if idx := strings.IndexAny(s, "（("); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// findExactMatchID returns the first grammar point whose normalized name
// equals the normalized query. Substring matches are deliberately rejected:
// 「ていた」must not link to 「〜ていただけませんか」.
func findExactMatchID(query string, points []GrammarPoint) int64 {
	normalizedQuery := normalizeGrammarName(query)
	if normalizedQuery == "" {
		return 0
	}
	for _, p := range points {
		if normalizeGrammarName(p.Name) == normalizedQuery {
			return p.ID
		}
	}
	return 0
}

// SearchByName looks up grammar points across all JLPT levels by name or
// meaning. Exact name matches are ranked first, then shorter (more specific)
// names, so the caller can jump straight to the relevant detail page.
func (s *GrammarService) SearchByName(query string) (*GrammarSearchResult, error) {
	q := strings.TrimSpace(query)
	slog.Debug("GrammarService.SearchByName called", "query", q)

	result := &GrammarSearchResult{Query: q, Points: []GrammarSearchHit{}}
	if q == "" {
		slog.Debug("GrammarService.SearchByName done: empty query", "count", 0)
		return result, nil
	}

	normalizedQuery := normalizeGrammarName(q)
	if normalizedQuery == "" {
		return result, nil
	}
	points, err := s.store.SearchCandidates(normalizedQuery, grammarSearchLimit)
	if err != nil {
		slog.Error("GrammarService.SearchByName: SearchCandidates failed", "err", err, "query", q)
		return nil, fmt.Errorf("grammar.GrammarService.SearchByName: %w", err)
	}

	sort.SliceStable(points, func(i, j int) bool {
		iExact := normalizeGrammarName(points[i].Name) == normalizedQuery
		jExact := normalizeGrammarName(points[j].Name) == normalizedQuery
		if iExact != jExact {
			return iExact
		}
		return len([]rune(points[i].Name)) < len([]rune(points[j].Name))
	})

	result.ExactMatchID = findExactMatchID(q, points)
	for _, p := range points {
		result.Points = append(result.Points, GrammarSearchHit{
			ID:        p.ID,
			Name:      p.Name,
			Meaning:   p.Meaning,
			JLPTLevel: p.JLPTLevel,
		})
	}

	slog.Debug("GrammarService.SearchByName done", "query", q,
		"count", len(result.Points), "exact_match_id", result.ExactMatchID)
	return result, nil
}
