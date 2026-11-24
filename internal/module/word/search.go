package word

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
)

const wordSearchLimit = 50

// SearchHit contains the dictionary fields needed to preview a word.
type SearchHit struct {
	ID           int64     `json:"id"`
	KanjiForm    string    `json:"kanji_form"`
	Reading      string    `json:"reading"`
	Meaning      string    `json:"meaning"`
	PartOfSpeech string    `json:"part_of_speech"`
	JLPTLevel    JLPTLevel `json:"jlpt_level"`
}

func (s *WordService) Search(ctx context.Context, query string) ([]SearchHit, error) {
	slog.DebugContext(ctx, "searching dictionary words", "query", query)
	hits, err := s.store.Search(ctx, query, wordSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("word.WordService.Search: %w", err)
	}
	slog.DebugContext(ctx, "dictionary word search completed", "count", len(hits))
	return hits, nil
}

func (h *WordHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || strings.ContainsRune(query, 0) || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 100 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "q must contain 1 to 100 valid text characters", "")
		return
	}

	hits, err := h.svc.Search(r.Context(), query)
	if err != nil {
		slog.ErrorContext(r.Context(), "dictionary word search failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to search words", "")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: hits})
}
