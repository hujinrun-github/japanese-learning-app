package speaking

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// SpeakingHandler handles HTTP requests for the speaking module.
type SpeakingHandler struct {
	svc *SpeakingService
}

// NewSpeakingHandler creates a SpeakingHandler.
func NewSpeakingHandler(svc *SpeakingService) *SpeakingHandler {
	return &SpeakingHandler{svc: svc}
}

// RegisterRoutes registers speaking routes.
// Routes:
//   POST /api/v1/speaking/practice          → Practice (multipart/form-data)
//   GET  /api/v1/speaking/records           → ListRecords
func (h *SpeakingHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/speaking/practice", h.handlePractice)
	mux.HandleFunc("GET /api/v1/speaking/records", h.handleListRecords)
}

// handlePractice handles POST /api/v1/speaking/practice
// Expects multipart/form-data with fields: type, material_id, reference_audio, user_audio
func (h *SpeakingHandler) handlePractice(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid multipart form", "")
		return
	}

	practiceType := PracticeType(r.FormValue("type"))
	if practiceType == "" {
		practiceType = PracticeTypeShadow
	}

	materialIDStr := r.FormValue("material_id")
	materialID, err := strconv.ParseInt(materialIDStr, 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid material_id", "")
		return
	}

	refAudio, err := readFormFile(r, "reference_audio")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "missing reference_audio", "")
		return
	}

	userAudio, err := readFormFile(r, "user_audio")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "missing user_audio", "")
		return
	}

	result, err := h.svc.Practice(userID, practiceType, materialID, refAudio, userAudio)
	if err != nil {
		slog.Error("handlePractice failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to process speaking practice", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: result})
}

// handleListRecords handles GET /api/v1/speaking/records
func (h *SpeakingHandler) handleListRecords(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	records, err := h.svc.ListRecords(userID)
	if err != nil {
		slog.Error("handleListRecords failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load speaking records", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: records})
}

// readFormFile reads all bytes from the named multipart file field.
func readFormFile(r *http.Request, field string) ([]byte, error) {
	f, _, err := r.FormFile(field)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
