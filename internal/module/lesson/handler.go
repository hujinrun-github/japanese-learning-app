package lesson

import (
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
)

// LessonHandler handles HTTP requests for the lesson module.
type LessonHandler struct {
	svc *LessonService
}

// NewLessonHandler creates a LessonHandler.
func NewLessonHandler(svc *LessonService) *LessonHandler {
	return &LessonHandler{svc: svc}
}

// RegisterRoutes registers lesson routes onto the provided mux.
// Routes:
//   GET  /api/v1/lessons?level=N5         → ListSummaries
//   GET  /api/v1/lessons/{id}             → GetDetail
//   GET  /api/v1/lessons/{id}/sentences   → GetSentences
func (h *LessonHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/lessons", h.handleListSummaries)
	mux.HandleFunc("GET /api/v1/lessons/{id}", h.handleGetDetail)
	mux.HandleFunc("GET /api/v1/lessons/{id}/sentences", h.handleGetSentences)
}

func (h *LessonHandler) handleListSummaries(w http.ResponseWriter, r *http.Request) {
	level := JLPTLevel(r.URL.Query().Get("level"))
	if level == "" {
		level = LevelN5
	}

	summaries, err := h.svc.ListSummaries(level)
	if err != nil {
		slog.Error("handleListSummaries failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: summaries})
}

func (h *LessonHandler) handleGetDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id", "")
		return
	}

	l, err := h.svc.GetDetail(id)
	if err != nil {
		slog.Error("handleGetDetail failed", "err", err, "lesson_id", id)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "lesson not found", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: l})
}

func (h *LessonHandler) handleGetSentences(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id", "")
		return
	}

	sentences, err := h.svc.GetSentences(id)
	if err != nil {
		slog.Error("handleGetSentences failed", "err", err, "lesson_id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sentences})
}
