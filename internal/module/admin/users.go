package admin

import (
	"log/slog"
	"net/http"
)

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	users, total, err := h.cfg.UserStore.ListAllUsers(offset, size)
	if err != nil {
		slog.Error("listUsers failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users, "total": total})
}

func (h *Handler) getUserStats(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	stats, err := h.cfg.UserStore.GetStats(id)
	if err != nil {
		slog.Error("getUserStats failed", "err", err, "user_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
