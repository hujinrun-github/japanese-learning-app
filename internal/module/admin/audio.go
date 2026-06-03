package admin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"japanese-learning-app/internal/cli"
)

type regenRequest struct {
	Text         string `json:"text"`
	Module       string `json:"module"` // "word" or "example"
	WordID       int64  `json:"word_id,omitempty"`
	Provider     string `json:"provider"`
	TTSUrl       string `json:"tts_url"`
	TTSModel     string `json:"tts_model"`
	Voice        string `json:"voice"`
	Instructions string `json:"instructions"`
	SBVURL       string `json:"sbv_url"`
	SBVModel     string `json:"sbv_model"`
	SBVSpeaker   string `json:"sbv_speaker"`
	SBVStyle     string `json:"sbv_style"`
}

func (h *Handler) regenerateAudio(w http.ResponseWriter, r *http.Request) {
	var req regenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}

	cfg := cli.TTSConfig{
		Provider:     req.Provider,
		TTSUrl:       req.TTSUrl,
		TTSModel:     req.TTSModel,
		Voice:        req.Voice,
		Instructions: req.Instructions,
		SBVURL:       req.SBVURL,
		SBVModel:     req.SBVModel,
		SBVSpeaker:   req.SBVSpeaker,
		SBVStyle:     req.SBVStyle,
	}
	client := cli.NewTTSClient(cfg)

	audio, err := client.Synthesize(context.Background(), req.Text)
	if err != nil {
		slog.Error("regenerateAudio synthesis failed", "text", req.Text, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "TTS synthesis failed: " + err.Error()})
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Text)))[:16]
	filename := hash + ".wav"

	var outDir string
	switch req.Module {
	case "word":
		outDir = "./data/audio/words"
	default:
		outDir = "./data/audio/examples"
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mkdir failed: " + err.Error()})
		return
	}

	path := filepath.Join(outDir, filename)
		// Trim leading/trailing silence for word audio
		if req.Module == "word" {
			audio = cli.TrimWAVSilence(audio, 0.06)
		}
	if err := os.WriteFile(path, audio, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write file failed: " + err.Error()})
		return
	}

	// For words, update audio_url in database
	if req.Module == "word" && req.WordID > 0 {
		if _, dbErr := h.cfg.DB.Exec("UPDATE words SET audio_url = ? WHERE id = ?", filename, req.WordID); dbErr != nil {
			slog.Error("regenerateAudio failed to update word audio_url", "word_id", req.WordID, "err", dbErr)
		}
	}

	slog.Info("regenerateAudio done", "text", req.Text, "module", req.Module, "filename", filename)
	writeJSON(w, http.StatusOK, map[string]string{
		"filename":    filename,
		"audio_url":   "/audio/" + req.Module + "s/" + filename,
		"module_path": req.Module + "s",
	})
}
