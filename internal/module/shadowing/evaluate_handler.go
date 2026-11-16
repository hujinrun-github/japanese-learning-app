package shadowing

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// EvaluationHandler exposes the S2 local scoring endpoint.
type EvaluationHandler struct {
	svc *EvaluationService
}

func NewEvaluationHandler(svc *EvaluationService) *EvaluationHandler {
	return &EvaluationHandler{svc: svc}
}

func (h *EvaluationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/lessons/{id}/shadowing/evaluations", h.handleEvaluate)
}

func (h *EvaluationHandler) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	lessonID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || lessonID <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid lesson id", "")
		return
	}
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		// S2 does not persist results, so identity is only used for diagnostics.
		slog.Debug("shadowing.EvaluationHandler: no authenticated user in context")
		userID = 0
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUserAudioBytes+64<<10)
	if err := r.ParseMultipartForm(maxUserAudioBytes); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid multipart form", "")
		return
	}
	file, _, err := r.FormFile("audio")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "audio part is required", "")
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("shadowing.EvaluationHandler: close upload failed", "err", err)
		}
	}()
	audioBytes, err := io.ReadAll(io.LimitReader(file, maxUserAudioBytes+1))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "failed to read audio part", "")
		return
	}

	mode := EvaluationMode(r.FormValue("mode"))
	if mode == "" {
		mode = EvaluationModeShadow
	}
	version, convErr := strconv.Atoi(r.FormValue("shadowing_version"))
	sentenceIndex, indexErr := strconv.Atoi(r.FormValue("sentence_index"))
	if convErr != nil || indexErr != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid numeric form fields", "")
		return
	}

	req := EvaluationRequest{
		UserID:           userID,
		LessonID:         lessonID,
		ShadowingVersion: version,
		SentenceIndex:    sentenceIndex,
		Mode:             mode,
		UserAudio:        audioBytes,
	}
	result, err := h.svc.Evaluate(r.Context(), req)
	if err != nil {
		writeShadowingError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: result})
}
