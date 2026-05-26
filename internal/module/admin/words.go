package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/word"
)

type wordRequest struct {
	KanjiForm         string             `json:"kanji_form"`
	Reading           string             `json:"reading"`
	Meaning           string             `json:"meaning"`
	PartOfSpeech      string             `json:"part_of_speech"`
	JLPTLevel         word.JLPTLevel     `json:"jlpt_level"`
	Examples          []word.WordExample `json:"examples"`
	ReadingType       string             `json:"reading_type"`
	AutoFill          bool               `json:"auto_fill"`
	GenerateExamples  bool               `json:"generate_examples"`
}

func (h *Handler) listWords(w http.ResponseWriter, r *http.Request) {
	level := word.JLPTLevel(r.URL.Query().Get("level"))
	search := r.URL.Query().Get("search")
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	words, total, err := h.cfg.WordStore.ListAll(level, search, offset, size)
	if err != nil {
		slog.Error("listWords failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": words, "total": total})
}

func (h *Handler) createWord(w http.ResponseWriter, r *http.Request) {
	var req wordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.KanjiForm == "" || req.Meaning == "" || req.JLPTLevel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kanji_form, meaning, jlpt_level are required"})
		return
	}
	wd := word.Word{
		KanjiForm:    req.KanjiForm,
		Reading:      req.Reading,
		Meaning:      req.Meaning,
		PartOfSpeech: req.PartOfSpeech,
		JLPTLevel:    req.JLPTLevel,
		Examples:     req.Examples,
		ReadingType:  req.ReadingType,
	}

	if req.AutoFill {
		autoFillWord(&wd)
	}

	if req.GenerateExamples && h.cfg.AIAPIKey != "" {
		gen := NewExampleGenerator(h.cfg.AIAPIKey, h.cfg.AIAPIEndpoint)
		examples, err := gen.GenerateExamples(wd)
		if err != nil {
			slog.Warn("createWord: failed to generate examples, continuing without", "err", err)
		} else {
			wd.Examples = examples
		}
	}

	id, err := h.cfg.WordStore.InsertWord(wd)
	if err != nil {
		slog.Error("createWord failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	wd.ID = id
	writeJSON(w, http.StatusCreated, wd)
}

func (h *Handler) updateWord(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req wordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	wd := word.Word{
		ID:           id,
		KanjiForm:    req.KanjiForm,
		Reading:      req.Reading,
		Meaning:      req.Meaning,
		PartOfSpeech: req.PartOfSpeech,
		JLPTLevel:    req.JLPTLevel,
		Examples:     req.Examples,
		ReadingType:  req.ReadingType,
	}

	if req.AutoFill {
		autoFillWord(&wd)
	}

	if req.GenerateExamples && h.cfg.AIAPIKey != "" {
		gen := NewExampleGenerator(h.cfg.AIAPIKey, h.cfg.AIAPIEndpoint)
		examples, err := gen.GenerateExamples(wd)
		if err != nil {
			slog.Warn("updateWord: failed to generate examples, continuing without", "err", err)
		} else {
			wd.Examples = examples
		}
	}

	if err := h.cfg.WordStore.UpdateWord(wd); err != nil {
		slog.Error("updateWord failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, wd)
}

func (h *Handler) deleteWord(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.cfg.WordStore.DeleteWord(id); err != nil {
		slog.Error("deleteWord failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
