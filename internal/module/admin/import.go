package admin

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strconv"

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

	raw, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var inserted int
	switch module {
	case "words":
		inserted, err = cli.ImportWords(h.cfg.DB, bytes.NewReader(raw))
	case "grammar":
		inserted, err = cli.ImportGrammar(h.cfg.DB, bytes.NewReader(raw))
	case "speaking":
		inserted, err = cli.ImportSpeaking(h.cfg.DB, bytes.NewReader(raw))
	case "writing":
		inserted, err = cli.ImportWriting(h.cfg.DB, bytes.NewReader(raw))
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown module"})
		return
	}
	if err != nil {
		slog.Error("bulkImport failed", "module", module, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Auto-generate TTS audio if requested
	ttsProvider := r.FormValue("tts_provider")
	if ttsProvider != "" {
		ttsCfg := cli.TTSConfig{
			Provider:     ttsProvider,
			TTSUrl:       r.FormValue("tts_url"),
			TTSModel:     r.FormValue("tts_model"),
			Voice:        r.FormValue("voice"),
			Instructions: r.FormValue("instructions"),
			SBVURL:       r.FormValue("sbv_url"),
			SBVModel:     r.FormValue("sbv_model"),
			SBVSpeaker:   r.FormValue("sbv_speaker"),
			SBVStyle:     r.FormValue("sbv_style"),
		}
		client := cli.NewTTSClient(ttsCfg)
		force, _ := strconv.ParseBool(r.FormValue("tts_force"))

		switch module {
		case "words":
			if _, genErr := cli.GenerateWordAudio(h.cfg.DB, client, "./data/audio/words", "", force, false); genErr != nil {
				slog.Error("bulkImport TTS words failed", "err", genErr)
			}
		case "grammar":
			sentences, _ := cli.ExtractGrammarSentences(raw)
			if len(sentences) > 0 {
				if _, genErr := cli.GenerateTTSFilesWithStats(client, "./data/audio/examples", sentences, force); genErr != nil {
					slog.Error("bulkImport TTS grammar failed", "err", genErr)
				}
			}
		case "speaking":
			sentences, _ := cli.ExtractSpeakingSentences(raw)
			if len(sentences) > 0 {
				if _, genErr := cli.GenerateTTSFilesWithStats(client, "./data/audio/examples", sentences, force); genErr != nil {
					slog.Error("bulkImport TTS speaking failed", "err", genErr)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"inserted": inserted})
}
