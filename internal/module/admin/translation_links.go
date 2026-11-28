package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/store"
)

func (h *Handler) listTranslationLinks(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid translation sentence ID"})
		return
	}
	sentence, err := h.cfg.TranslationStore.GetSentenceByID(id)
	if err != nil {
		writeTranslationLinksError(w, r, id, err)
		return
	}
	if sentence == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "translation sentence not found"})
		return
	}
	links, err := h.cfg.TranslationStore.ListSentenceLinks(r.Context(), id)
	if err != nil {
		writeTranslationLinksError(w, r, id, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

func (h *Handler) replaceTranslationLinks(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid translation sentence ID"})
		return
	}
	var req struct {
		Links []translation.SentenceLinkRef `json:"links"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil || req.Links == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "links must be an array"})
		return
	}
	if len(req.Links) > translation.MaxSentenceLinks {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("select at most %d entries", translation.MaxSentenceLinks)})
		return
	}
	seen := make(map[translation.SentenceLinkRef]bool, len(req.Links))
	for _, link := range req.Links {
		if link.ID <= 0 || (link.Kind != translation.SentenceLinkWord && link.Kind != translation.SentenceLinkGrammar) || seen[link] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "links must contain distinct word or grammar IDs"})
			return
		}
		seen[link] = true
	}
	if err := h.cfg.TranslationStore.ReplaceSentenceLinks(r.Context(), id, req.Links); err != nil {
		writeTranslationLinksError(w, r, id, err)
		return
	}
	slog.DebugContext(r.Context(), "updated translation sentence links", "sentence_id", id, "count", len(req.Links))
	w.WriteHeader(http.StatusNoContent)
}

func writeTranslationLinksError(w http.ResponseWriter, r *http.Request, id int64, err error) {
	slog.ErrorContext(r.Context(), "translation sentence links failed", "method", r.Method, "sentence_id", id, "err", err)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "translation sentence not found"})
	case errors.Is(err, store.ErrConstraint), errors.Is(err, store.ErrDuplicate):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a selected word or grammar point is no longer available"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load or save translation links"})
	}
}
