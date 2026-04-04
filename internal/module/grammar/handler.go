package grammar

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
)

// GrammarHandler handles HTTP requests for the grammar module.
type GrammarHandler struct {
	svc *GrammarService
}

// NewGrammarHandler creates a GrammarHandler.
func NewGrammarHandler(svc *GrammarService) *GrammarHandler {
	return &GrammarHandler{svc: svc}
}

// RegisterRoutes registers grammar routes onto the provided mux.
// Routes:
//   GET  /api/v1/grammar?level=N5              → ListByLevel
//   GET  /api/v1/grammar/{id}                  → GetPoint
//   POST /api/v1/grammar/{id}/quiz             → ScoreQuiz
func (h *GrammarHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/grammar", h.handleListByLevel)
	mux.HandleFunc("GET /api/v1/grammar/{id}", h.handleGetPoint)
	mux.HandleFunc("POST /api/v1/grammar/{id}/quiz", h.handleScoreQuiz)
}

func (h *GrammarHandler) handleListByLevel(w http.ResponseWriter, r *http.Request) {
	level := JLPTLevel(r.URL.Query().Get("level"))
	if level == "" {
		level = LevelN5
	}

	points, err := h.svc.ListByLevel(level)
	if err != nil {
		slog.Error("handleListByLevel failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: points})
}

func (h *GrammarHandler) handleGetPoint(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id", "")
		return
	}

	p, err := h.svc.GetPoint(id)
	if err != nil {
		slog.Error("handleGetPoint failed", "err", err, "grammar_point_id", id)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "grammar point not found", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: p})
}

func (h *GrammarHandler) handleScoreQuiz(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	grammarPointID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid id", "")
		return
	}

	var submissions []QuizSubmission
	if err := json.NewDecoder(r.Body).Decode(&submissions); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	result, err := h.svc.ScoreQuiz(userID, grammarPointID, submissions)
	if err != nil {
		slog.Error("handleScoreQuiz failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: result})
}

// contextKeyUserID is the key type for storing userID in context.
type contextKeyUserID struct{}

// userIDFromContext extracts the userID injected by AuthMiddleware.
func userIDFromContext(ctx interface{ Value(any) any }) (int64, bool) {
	v := ctx.Value(contextKeyUserID{})
	id, ok := v.(int64)
	return id, ok
}
