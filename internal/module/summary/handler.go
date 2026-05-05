package summary

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// SummaryHandler handles HTTP requests for the summary module.
type SummaryHandler struct {
	svc *SummaryService
}

// NewSummaryHandler creates a SummaryHandler.
func NewSummaryHandler(svc *SummaryService) *SummaryHandler {
	return &SummaryHandler{svc: svc}
}

// RegisterRoutes registers summary routes.
// Routes:
//
//	POST /api/v1/summary/sessions        → RecordSession
//	POST /api/v1/summary/generate        → GenerateSummary
//	GET  /api/v1/summary                 → ListSummaries
func (h *SummaryHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/summary/sessions", h.handleRecordSession)
	mux.HandleFunc("POST /api/v1/summary/generate", h.handleGenerateSummary)
	mux.HandleFunc("GET /api/v1/summary", h.handleListSummaries)
}

// handleRecordSession handles POST /api/v1/summary/sessions
func (h *SummaryHandler) handleRecordSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var session StudySession
	if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	session.UserID = userID

	if err := h.svc.RecordSession(session); err != nil {
		slog.Error("handleRecordSession failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to record study session", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: session})
}

// generateSummaryRequest is the request body for POST /api/v1/summary/generate.
type generateSummaryRequest struct {
	SessionID    string       `json:"session_id"`
	Module       ModuleType   `json:"module"`
	ScoreSummary ScoreSummary `json:"score_summary"`
}

// handleGenerateSummary handles POST /api/v1/summary/generate
func (h *SummaryHandler) handleGenerateSummary(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req generateSummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.SessionID == "" || req.Module == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "session_id and module are required", "")
		return
	}

	sum, err := h.svc.GenerateSummary(userID, req.SessionID, req.Module, req.ScoreSummary)
	if err != nil {
		slog.Error("handleGenerateSummary failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to generate summary", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sum})
}

// handleListSummaries handles GET /api/v1/summary
func (h *SummaryHandler) handleListSummaries(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	summaries, err := h.svc.ListSummaries(userID)
	if err != nil {
		slog.Error("handleListSummaries failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load summaries", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: summaries})
}
