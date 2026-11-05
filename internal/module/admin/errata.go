package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/errata"
)

func (h *Handler) listErrata(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != errata.Pending && status != errata.Resolved {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	const pageSize = 20
	page := queryInt(r, "page", 1)
	if page < 1 || page > 50000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid page"})
		return
	}
	items, err := h.cfg.ErrataStore.List(r.Context(), status, pageSize+1, (page-1)*pageSize)
	if err != nil {
		slog.Error("list errata failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load errata reports"})
		return
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "has_more": hasMore})
}

func (h *Handler) updateErrata(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid report ID"})
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Status != errata.Pending && req.Status != errata.Resolved {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	found, err := h.cfg.ErrataStore.SetStatus(r.Context(), id, req.Status)
	if err != nil {
		slog.Error("update errata failed", "err", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update errata report"})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "errata report not found"})
		return
	}
	slog.Debug("errata updated", "id", id, "status", req.Status)
	w.WriteHeader(http.StatusNoContent)
}
