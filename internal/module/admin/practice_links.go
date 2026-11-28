package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/module/practicelinks"
	"japanese-learning-app/internal/store"
)

func (h *Handler) listPracticeLinkSources(w http.ResponseWriter, r *http.Request) {
	const size = 20
	page := queryInt(r, "page", 1)
	query := strings.TrimSpace(r.URL.Query().Get("search"))
	if page < 1 || page > 10000 || utf8.RuneCountInString(query) > 100 || strings.ContainsRune(query, 0) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid practice catalog page or search"})
		return
	}
	items, err := h.cfg.PracticeLinks.ListSources(r.Context(), r.PathValue("kind"), query, (page-1)*size, size+1)
	if err != nil {
		writePracticeLinksError(w, r, err)
		return
	}
	hasMore := len(items) > size
	if hasMore {
		items = items[:size]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "has_more": hasMore})
}

func (h *Handler) listPracticeLinks(w http.ResponseWriter, r *http.Request) {
	target := practicelinks.Target{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
	if err := h.cfg.PracticeLinks.SourceExists(r.Context(), target); err != nil {
		writePracticeLinksError(w, r, err)
		return
	}
	links, err := h.cfg.PracticeLinks.ListLinks(r.Context(), target, false)
	if err != nil {
		writePracticeLinksError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

func (h *Handler) replacePracticeLinks(w http.ResponseWriter, r *http.Request) {
	target := practicelinks.Target{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
	var req struct {
		Links []practicelinks.Selection `json:"links"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil || req.Links == nil || len(req.Links) > practicelinks.MaxLinks {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "links must be an array with at most 30 entries"})
		return
	}
	seen := make(map[practicelinks.Ref]bool, len(req.Links))
	for _, link := range req.Links {
		if link.ID <= 0 || (link.Kind != practicelinks.Word && link.Kind != practicelinks.Grammar) || seen[link.Ref] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "links must contain distinct word or grammar IDs"})
			return
		}
		seen[link.Ref] = true
	}
	if err := h.cfg.PracticeLinks.ReplaceLinks(r.Context(), target, req.Links); err != nil {
		writePracticeLinksError(w, r, err)
		return
	}
	slog.DebugContext(r.Context(), "updated practice knowledge links", "kind", target.Kind, "id", target.ID, "count", len(req.Links))
	w.WriteHeader(http.StatusNoContent)
}

func writePracticeLinksError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "practice knowledge links request failed", "kind", r.PathValue("kind"), "id", r.PathValue("id"), "err", err)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "practice item not found"})
	case errors.Is(err, store.ErrConstraint), errors.Is(err, store.ErrDuplicate):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid practice item, dictionary entry or hint selection"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load or save practice links"})
	}
}
