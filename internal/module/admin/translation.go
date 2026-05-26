package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/translation"
)

type translationSentenceRequest struct {
	SourceID             int64  `json:"source_id"`
	Direction            string `json:"direction"`
	SourceText           string `json:"source_text"`
	ReferenceTranslation string `json:"reference_translation"`
	Position             int    `json:"position"`
}

func (h *Handler) listTranslation(w http.ResponseWriter, r *http.Request) {
	sourceID := int64(queryInt(r, "source_id", 0))
	direction := r.URL.Query().Get("direction")
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	sentences, total, err := h.cfg.TranslationStore.ListAllSentences(sourceID, direction, offset, size)
	if err != nil {
		slog.Error("listTranslation failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sentences, "total": total})
}

func (h *Handler) createTranslation(w http.ResponseWriter, r *http.Request) {
	var req translationSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.SourceText == "" || req.Direction == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_text, direction are required"})
		return
	}
	sent := translation.TranslationSentence{
		SourceID:             req.SourceID,
		Direction:            req.Direction,
		SourceText:           req.SourceText,
		ReferenceTranslation: req.ReferenceTranslation,
		Position:             req.Position,
	}
	id, err := h.cfg.TranslationStore.SaveSentence(sent)
	if err != nil {
		slog.Error("createTranslation failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sent.ID = id
	writeJSON(w, http.StatusCreated, sent)
}

func (h *Handler) updateTranslation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req translationSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	sent := translation.TranslationSentence{
		ID:                   id,
		SourceID:             req.SourceID,
		Direction:            req.Direction,
		SourceText:           req.SourceText,
		ReferenceTranslation: req.ReferenceTranslation,
		Position:             req.Position,
	}
	if err := h.cfg.TranslationStore.UpdateSentence(sent); err != nil {
		slog.Error("updateTranslation failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sent)
}

func (h *Handler) deleteTranslation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.cfg.TranslationStore.DeleteSentence(id); err != nil {
		slog.Error("deleteTranslation failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
