package admin

import (
	"log/slog"
	"net/http"
)

func (h *Handler) listRecords(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	userID := int64(queryInt(r, "user_id", 0))
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	switch module {
	case "word":
		records, total, err := h.cfg.WordStore.ListAllRecords(userID, offset, size)
		if err != nil {
			slog.Error("listRecords word failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": total})
	case "grammar":
		records, total, err := h.cfg.GrammarStore.ListAllRecords(userID, offset, size)
		if err != nil {
			slog.Error("listRecords grammar failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": total})
	case "speaking":
		records, total, err := h.cfg.SpeakingStore.ListAllRecords(userID, offset, size)
		if err != nil {
			slog.Error("listRecords speaking failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": total})
	case "writing":
		records, total, err := h.cfg.WritingStore.ListAllRecords(userID, offset, size)
		if err != nil {
			slog.Error("listRecords writing failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": total})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown module: " + module})
	}
}
