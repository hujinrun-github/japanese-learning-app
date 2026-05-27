package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/grammar"
)

type grammarRequest struct {
	Name            string                   `json:"name"`
	Meaning         string                   `json:"meaning"`
	ConjunctionRule string                   `json:"conjunction_rule"`
	UsageNote       string                   `json:"usage_note"`
	Examples        []grammar.GrammarExample `json:"examples"`
	QuizQuestions   []grammar.QuizQuestion   `json:"quiz_questions"`
	JLPTLevel       grammar.JLPTLevel        `json:"jlpt_level"`
}

func (h *Handler) listGrammar(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("search")
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	points, total, err := h.cfg.GrammarStore.ListAll(level, search, offset, size)
	if err != nil {
		slog.Error("listGrammar failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": points, "total": total})
}

func (h *Handler) createGrammar(w http.ResponseWriter, r *http.Request) {
	var req grammarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" || req.Meaning == "" || req.JLPTLevel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, meaning, jlpt_level are required"})
		return
	}
	gp := grammar.GrammarPoint{
		Name:            req.Name,
		Meaning:         req.Meaning,
		ConjunctionRule: req.ConjunctionRule,
		UsageNote:       req.UsageNote,
		Examples:        req.Examples,
		QuizQuestions:   req.QuizQuestions,
		JLPTLevel:       req.JLPTLevel,
	}
	id, err := h.cfg.GrammarStore.InsertPoint(gp)
	if err != nil {
		slog.Error("createGrammar failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	gp.ID = id
	writeJSON(w, http.StatusCreated, gp)
}

func (h *Handler) updateGrammar(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req grammarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	gp := grammar.GrammarPoint{
		ID:              id,
		Name:            req.Name,
		Meaning:         req.Meaning,
		ConjunctionRule: req.ConjunctionRule,
		UsageNote:       req.UsageNote,
		Examples:        req.Examples,
		QuizQuestions:   req.QuizQuestions,
		JLPTLevel:       req.JLPTLevel,
	}
	if err := h.cfg.GrammarStore.UpdatePoint(gp); err != nil {
		slog.Error("updateGrammar failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, gp)
}

func (h *Handler) deleteGrammar(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.cfg.GrammarStore.DeletePoint(id); err != nil {
		slog.Error("deleteGrammar failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
