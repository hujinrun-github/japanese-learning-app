package speaking

import (
	"encoding/json"
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
//
//	POST /api/v1/speaking/practice           → record a self-rated practice
//	GET  /api/v1/speaking/records            → list user's practice records
//	GET  /api/v1/speaking/materials          → list practice materials
//	GET  /api/v1/speaking/materials/{id}     → get a single material
func (h *SpeakingHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/speaking/practice", h.handlePractice)
	mux.HandleFunc("GET /api/v1/speaking/records", h.handleListRecords)
	mux.HandleFunc("GET /api/v1/speaking/materials", h.handleListMaterials)
	mux.HandleFunc("GET /api/v1/speaking/materials/{id}", h.handleGetMaterial)
}

type practiceRequest struct {
	Type       string `json:"type"`
	MaterialID int64  `json:"material_id"`
	Score      int    `json:"score"`
}

// handlePractice handles POST /api/v1/speaking/practice
// Accepts JSON body with type, material_id, and score fields.
func (h *SpeakingHandler) handlePractice(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req practiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", "")
		return
	}

	practiceType := PracticeType(req.Type)
	if practiceType == "" {
		practiceType = PracticeTypeShadow
	}

	if req.Score < 0 || req.Score > 100 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "score must be 0-100", "")
		return
	}

	record, err := h.svc.RecordPractice(userID, practiceType, req.MaterialID, req.Score)
	if err != nil {
		slog.Error("handlePractice failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to save practice record", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: record})
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

// handleListMaterials handles GET /api/v1/speaking/materials
func (h *SpeakingHandler) handleListMaterials(w http.ResponseWriter, r *http.Request) {
	practiceType := r.URL.Query().Get("type")
	level := r.URL.Query().Get("level")

	materials, err := h.svc.ListMaterials(practiceType, level)
	if err != nil {
		slog.Error("handleListMaterials failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load materials", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: materials})
}

// handleGetMaterial handles GET /api/v1/speaking/materials/{id}
func (h *SpeakingHandler) handleGetMaterial(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid material id", "")
		return
	}

	material, err := h.svc.GetMaterialByID(id)
	if err != nil {
		slog.Error("handleGetMaterial failed", "err", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load material", "")
		return
	}
	if material == nil {
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "material not found", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: material})
}
