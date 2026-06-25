package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

type batchAudioRequest struct {
	Module       string   `json:"module"` // "words", "grammar", or "speaking"
	Level        string   `json:"level,omitempty"`
	Type         string   `json:"type,omitempty"`
	Texts        []string `json:"texts,omitempty"`
	WordIDs      []int64  `json:"word_ids,omitempty"`
	Force        bool     `json:"force,omitempty"`
	Provider     string   `json:"provider"`
	TTSUrl       string   `json:"tts_url"`
	TTSModel     string   `json:"tts_model"`
	Voice        string   `json:"voice"`
	Instructions string   `json:"instructions"`
	SBVURL       string   `json:"sbv_url"`
	SBVModel     string   `json:"sbv_model"`
	SBVSpeaker   string   `json:"sbv_speaker"`
	SBVStyle     string   `json:"sbv_style"`
}

type batchAudioResponse struct {
	Module    string `json:"module"`
	Total     int    `json:"total"`
	Generated int    `json:"generated"`
	Existing  int    `json:"existing"`
	Failed    int    `json:"failed"`
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
	if req.Module == "word" && req.WordID > 0 && h.cfg.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "word audio database update is not available in PostgreSQL admin mode"})
		return
	}

	client := cli.NewTTSClient(ttsConfigFromRegenRequest(req))

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
	// Trim leading/trailing silence for word audio.
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

func (h *Handler) batchGenerateAudio(w http.ResponseWriter, r *http.Request) {
	var req batchAudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	req.Module = strings.TrimSpace(req.Module)
	req.Level = strings.TrimSpace(req.Level)
	req.Type = strings.TrimSpace(req.Type)
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	if h.cfg.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "batch audio is not available in PostgreSQL admin mode"})
		return
	}

	client := cli.NewTTSClient(ttsConfigFromBatchAudioRequest(req))
	var resp batchAudioResponse
	resp.Module = req.Module

	switch req.Module {
	case "words":
		var (
			stats cli.WordAudioStats
			err   error
		)
		if len(req.WordIDs) > 0 {
			stats, err = cli.GenerateWordAudioByIDsWithStats(h.cfg.DB, client, "./data/audio/words", req.WordIDs, req.Force, false)
		} else {
			stats, err = cli.GenerateWordAudioWithStats(h.cfg.DB, client, "./data/audio/words", req.Level, req.Force, false)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "batch word audio failed: " + err.Error()})
			return
		}
		resp.Total = stats.Total
		resp.Generated = stats.Generated
		resp.Existing = stats.Existing
		resp.Failed = stats.Failed
	case "grammar":
		texts := req.Texts
		if len(texts) == 0 {
			var err error
			texts, err = collectGrammarAudioTexts(h.cfg.DB, req.Level)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "collect grammar audio texts failed: " + err.Error()})
				return
			}
		}
		stats, err := cli.GenerateTTSFilesWithStats(client, "./data/audio/examples", texts, req.Force)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "batch grammar audio failed: " + err.Error()})
			return
		}
		resp.Total = stats.Total
		resp.Generated = stats.Generated
		resp.Existing = stats.Existing
		resp.Failed = stats.Failed
	case "speaking":
		texts := req.Texts
		if len(texts) == 0 {
			var err error
			texts, err = collectSpeakingAudioTexts(h.cfg.DB, req.Level, req.Type)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "collect speaking audio texts failed: " + err.Error()})
				return
			}
		}
		stats, err := cli.GenerateTTSFilesWithStats(client, "./data/audio/examples", texts, req.Force)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "batch speaking audio failed: " + err.Error()})
			return
		}
		resp.Total = stats.Total
		resp.Generated = stats.Generated
		resp.Existing = stats.Existing
		resp.Failed = stats.Failed
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown module"})
		return
	}

	slog.Info("batchGenerateAudio done", "module", resp.Module, "level", req.Level, "type", req.Type, "total", resp.Total, "generated", resp.Generated, "existing", resp.Existing, "failed", resp.Failed)
	writeJSON(w, http.StatusOK, resp)
}

func collectGrammarAudioTexts(db *sql.DB, level string) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if level == "" {
		rows, err = db.Query("SELECT examples_json FROM grammar_points ORDER BY id")
	} else {
		rows, err = db.Query("SELECT examples_json FROM grammar_points WHERE jlpt_level = ? ORDER BY id", level)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var texts []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var examples []struct {
			Japanese string `json:"japanese"`
		}
		if err := json.Unmarshal([]byte(raw), &examples); err != nil {
			return nil, fmt.Errorf("unmarshal grammar examples: %w", err)
		}
		for _, example := range examples {
			texts = append(texts, example.Japanese)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return texts, nil
}

func collectSpeakingAudioTexts(db *sql.DB, level, speakingType string) ([]string, error) {
	where := "WHERE text != ''"
	args := []any{}
	if level != "" {
		where += " AND jlpt_level = ?"
		args = append(args, level)
	}
	if speakingType != "" {
		where += " AND type = ?"
		args = append(args, speakingType)
	}

	rows, err := db.Query("SELECT text FROM speaking_materials "+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var texts []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		texts = append(texts, text)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return texts, nil
}

func ttsConfigFromRegenRequest(req regenRequest) cli.TTSConfig {
	return cli.TTSConfig{
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
}

func ttsConfigFromBatchAudioRequest(req batchAudioRequest) cli.TTSConfig {
	return cli.TTSConfig{
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
}
