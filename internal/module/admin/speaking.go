package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/module/speaking"
)

type speakingRequest struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	AudioURL  string `json:"audio_url"`
	JLPTLevel string `json:"jlpt_level"`
}

func (h *Handler) listSpeaking(w http.ResponseWriter, r *http.Request) {
	practiceType := r.URL.Query().Get("type")
	level := r.URL.Query().Get("level")
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 20)
	offset := (page - 1) * size

	materials, total, err := h.cfg.SpeakingStore.ListAll(practiceType, level, offset, size)
	if err != nil {
		slog.Error("listSpeaking failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": materials, "total": total})
}

func (h *Handler) createSpeaking(w http.ResponseWriter, r *http.Request) {
	var req speakingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Title == "" || req.Type == "" || req.JLPTLevel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title, type, jlpt_level are required"})
		return
	}
	m := speaking.SpeakingMaterial{
		Type:      req.Type,
		Title:     req.Title,
		Text:      req.Text,
		AudioURL:  req.AudioURL,
		JLPTLevel: req.JLPTLevel,
	}
	id, err := h.cfg.SpeakingStore.InsertMaterial(m)
	if err != nil {
		slog.Error("createSpeaking failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	m.ID = id
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) updateSpeaking(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req speakingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	m := speaking.SpeakingMaterial{
		ID:        id,
		Type:      req.Type,
		Title:     req.Title,
		Text:      req.Text,
		AudioURL:  req.AudioURL,
		JLPTLevel: req.JLPTLevel,
	}
	if err := h.cfg.SpeakingStore.UpdateMaterial(m); err != nil {
		slog.Error("updateSpeaking failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) deleteSpeaking(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.cfg.SpeakingStore.DeleteMaterial(id); err != nil {
		slog.Error("deleteSpeaking failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
