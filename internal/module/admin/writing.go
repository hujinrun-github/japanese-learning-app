package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/writing"
)

type writingRequest struct {
	Type           writing.WritingType `json:"type"`
	Prompt         string              `json:"prompt"`
	ExpectedAnswer string              `json:"expected_answer"`
	GrammarPointID int64               `json:"grammar_point_id"`
	JLPTLevel      string              `json:"jlpt_level"`
}

func (h *Handler) listWriting(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	qtype := r.URL.Query().Get("type")
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	questions, total, err := h.cfg.WritingStore.ListAllQuestions(level, qtype, offset, size)
	if err != nil {
		slog.Error("listWriting failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": questions, "total": total})
}

func (h *Handler) createWriting(w http.ResponseWriter, r *http.Request) {
	var req writingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Prompt == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt, type are required"})
		return
	}
	q := writing.WritingQuestion{
		Type:           req.Type,
		Prompt:         req.Prompt,
		ExpectedAnswer: req.ExpectedAnswer,
		GrammarPointID: req.GrammarPointID,
		JLPTLevel:      req.JLPTLevel,
	}
	id, err := h.cfg.WritingStore.InsertQuestion(q)
	if err != nil {
		slog.Error("createWriting failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	q.ID = id
	writeJSON(w, http.StatusCreated, q)
}

func (h *Handler) updateWriting(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req writingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	q := writing.WritingQuestion{
		ID:             id,
		Type:           req.Type,
		Prompt:         req.Prompt,
		ExpectedAnswer: req.ExpectedAnswer,
		GrammarPointID: req.GrammarPointID,
		JLPTLevel:      req.JLPTLevel,
	}
	if err := h.cfg.WritingStore.UpdateQuestion(q); err != nil {
		slog.Error("updateWriting failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (h *Handler) deleteWriting(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.cfg.WritingStore.DeleteQuestion(id); err != nil {
		slog.Error("deleteWriting failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
