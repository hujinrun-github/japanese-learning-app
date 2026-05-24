package speaking

// levenshteinDistance computes the edit distance between two rune slices.
func levenshteinDistance(a, b []rune) int {
	n, m := len(a), len(b)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	// Use two rows to save memory: O(min(n,m)) space.
	prev := make([]int, m+1)
	curr := make([]int, m+1)
	for j := 0; j <= m; j++ {
		prev[j] = j
	}

	for i := 1; i <= n; i++ {
		curr[0] = i
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[m]
}

// ComputeCER returns the Character Error Rate between reference and recognized text.
// CER = levenshtein_distance / len(reference), clamped to [0, 1].
// Returns 0 when both strings are empty, 1.0 when reference is empty with non-empty recognized.
func ComputeCER(reference, recognized string) float64 {
	refRunes := []rune(reference)
	recRunes := []rune(recognized)

	if len(refRunes) == 0 {
		if len(recRunes) == 0 {
			return 0
		}
		return 1.0
	}

	dist := levenshteinDistance(refRunes, recRunes)
	cer := float64(dist) / float64(len(refRunes))
	if cer > 1.0 {
		return 1.0
	}
	return cer
}

// ScoreFromCER maps CER [0, 1] to a 0-100 score. CER=0 → 100, CER=1+ → 0.
func ScoreFromCER(cer float64) int {
	score := int(100 - cer*100)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}
