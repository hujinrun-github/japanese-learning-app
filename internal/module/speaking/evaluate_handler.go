package speaking

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/shadowing"
)

const maxSpeakingAudioBytes = 2 << 20

const (
	ErrSpeakingEvalInvalid      = "ERR_SPEAKING_EVAL_INVALID"
	ErrSpeakingReferenceMissing = "ERR_SPEAKING_REFERENCE_MISSING"
	ErrSpeakingAudioUnsupported = "ERR_SPEAKING_AUDIO_UNSUPPORTED"
)

// EvaluationHandler scores a speaking recording with the same local prosody
// engine lesson shadowing uses. The reference audio is the material's audio_url,
// or the content-addressed TTS file that "播放参考" plays for the same text.
type EvaluationHandler struct {
	svc       *SpeakingService
	audioRoot string
	loader    shadowing.ObjectLoader
	evaluator *shadowing.Evaluator
}

// NewEvaluationHandler creates an EvaluationHandler. audioRoot is the directory
// served under /audio; loader is optional and only needed for object-storage
// audio URLs.
func NewEvaluationHandler(svc *SpeakingService, audioRoot string, loader shadowing.ObjectLoader, evaluator *shadowing.Evaluator) *EvaluationHandler {
	return &EvaluationHandler{svc: svc, audioRoot: audioRoot, loader: loader, evaluator: evaluator}
}

// RegisterRoutes registers POST /api/v1/speaking/evaluate.
func (h *EvaluationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/speaking/evaluate", h.handleEvaluate)
}

func (h *EvaluationHandler) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	slog.Debug("speaking.EvaluationHandler called", "audio_root", h.audioRoot)

	r.Body = http.MaxBytesReader(w, r.Body, maxSpeakingAudioBytes+64<<10)
	if err := r.ParseMultipartForm(maxSpeakingAudioBytes); err != nil {
		slog.Error("speaking.EvaluationHandler: invalid multipart form", "err", err)
		httputil.WriteError(w, http.StatusBadRequest, ErrSpeakingEvalInvalid, "invalid multipart form", "")
		return
	}

	file, _, err := r.FormFile("audio")
	if err != nil {
		slog.Error("speaking.EvaluationHandler: audio part missing", "err", err)
		httputil.WriteError(w, http.StatusBadRequest, ErrSpeakingEvalInvalid, "audio part is required", "")
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("speaking.EvaluationHandler: close upload failed", "err", err)
		}
	}()

	userAudio, err := io.ReadAll(io.LimitReader(file, maxSpeakingAudioBytes+1))
	if err != nil {
		slog.Error("speaking.EvaluationHandler: read audio failed", "err", err)
		httputil.WriteError(w, http.StatusBadRequest, ErrSpeakingEvalInvalid, "failed to read audio part", "")
		return
	}
	if len(userAudio) == 0 || len(userAudio) > maxSpeakingAudioBytes {
		slog.Error("speaking.EvaluationHandler: audio is empty or too large", "bytes", len(userAudio))
		httputil.WriteError(w, http.StatusBadRequest, ErrSpeakingEvalInvalid, "audio is empty or too large", "")
		return
	}

	materialID, err := strconv.ParseInt(r.FormValue("material_id"), 10, 64)
	if err != nil || materialID <= 0 {
		slog.Error("speaking.EvaluationHandler: invalid material id", "material_id", r.FormValue("material_id"), "err", err)
		httputil.WriteError(w, http.StatusBadRequest, ErrSpeakingEvalInvalid, "invalid material id", "")
		return
	}

	mode := shadowing.EvaluationMode(r.FormValue("mode"))
	if mode != "" && mode != shadowing.EvaluationModeShadow && mode != shadowing.EvaluationModeRepeat {
		slog.Error("speaking.EvaluationHandler: invalid mode", "mode", mode)
		httputil.WriteError(w, http.StatusBadRequest, ErrSpeakingEvalInvalid, "mode must be shadow or repeat", "")
		return
	}

	material, err := h.svc.GetMaterialByID(materialID)
	if err != nil {
		slog.Error("speaking.EvaluationHandler: load material failed", "err", err, "material_id", materialID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load material", "")
		return
	}
	if material == nil {
		slog.Error("speaking.EvaluationHandler: material not found", "material_id", materialID)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "material not found", "")
		return
	}
	if mode == "" {
		if material.Type == string(PracticeTypeShadow) {
			mode = shadowing.EvaluationModeShadow
		} else {
			mode = shadowing.EvaluationModeRepeat
		}
	}

	refURL := referenceAudioURL(*material)
	slog.Debug("speaking.EvaluationHandler: evaluating",
		"material_id", materialID, "mode", mode, "reference_url", refURL, "audio_bytes", len(userAudio))

	refWAV, err := shadowing.ReadAudioURL(r.Context(), h.audioRoot, h.loader, refURL)
	if err != nil {
		slog.Error("speaking.EvaluationHandler: reference audio unavailable", "err", err,
			"material_id", materialID, "reference_url", refURL)
		httputil.WriteError(w, http.StatusUnprocessableEntity, ErrSpeakingReferenceMissing,
			"reference audio is unavailable for this material", "")
		return
	}

	result, err := h.evaluator.Evaluate(r.Context(), refWAV, userAudio, mode)
	if err != nil {
		slog.Error("speaking.EvaluationHandler: scoring failed", "err", err, "material_id", materialID)
		var busy *shadowing.Error
		if errors.As(err, &busy) && busy.Code == shadowing.ErrEvaluationBusy {
			httputil.WriteError(w, http.StatusServiceUnavailable, busy.Code, busy.Message, "")
			return
		}
		httputil.WriteError(w, http.StatusUnprocessableEntity, ErrSpeakingAudioUnsupported,
			"failed to analyze the recording", "")
		return
	}
	result.LatencyMS = time.Since(start).Milliseconds()

	slog.Debug("speaking.EvaluationHandler done",
		"material_id", materialID, "status", result.Status, "code", result.UnscorableCode,
		"latency_ms", result.LatencyMS)
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: result})
}

// referenceAudioURL returns the material's configured reference audio, or the
// content-addressed TTS file that speakExample() plays for the same text.
func referenceAudioURL(m SpeakingMaterial) string {
	if u := strings.TrimSpace(m.AudioURL); u != "" {
		return u
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(m.Text)))[:16]
	return "/audio/examples/" + hash + ".wav"
}
