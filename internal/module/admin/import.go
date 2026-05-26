package admin

import (
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/cli"
)

func (h *Handler) bulkImport(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	var inserted int
	switch module {
	case "words":
		inserted, err = cli.ImportWords(h.cfg.DB, file)
	case "grammar":
		inserted, err = cli.ImportGrammar(h.cfg.DB, file)
	case "speaking":
		inserted, err = cli.ImportSpeaking(h.cfg.DB, file)
	case "writing":
		inserted, err = cli.ImportWriting(h.cfg.DB, file)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown module"})
		return
	}
	if err != nil {
		slog.Error("bulkImport failed", "module", module, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"inserted": inserted})
}
