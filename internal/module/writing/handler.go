package writing

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/httputil"
)

// WritingHandler handles HTTP requests for the writing module.
type WritingHandler struct {
	svc *WritingService
}

// NewWritingHandler creates a WritingHandler.
func NewWritingHandler(svc *WritingService) *WritingHandler {
	return &WritingHandler{svc: svc}
}

// RegisterRoutes registers writing routes.
// Routes:
//
//	GET  /api/v1/writing/queue           → GetDailyQueue
//	POST /api/v1/writing/input           → SubmitInput
//	POST /api/v1/writing/sentence        → SubmitSentence
//	GET  /api/v1/writing/records         → ListRecords
func (h *WritingHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/writing/queue", h.handleGetDailyQueue)
	mux.HandleFunc("POST /api/v1/writing/input", h.handleSubmitInput)
	mux.HandleFunc("POST /api/v1/writing/sentence", h.handleSubmitSentence)
	mux.HandleFunc("GET /api/v1/writing/records", h.handleListRecords)
}

// handleGetDailyQueue handles GET /api/v1/writing/queue
func (h *WritingHandler) handleGetDailyQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	questions, err := h.svc.GetDailyQueue(userID)
	if err != nil {
		slog.Error("handleGetDailyQueue failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: questions})
}

// submitInputRequest is the request body for POST /api/v1/writing/input.
type submitInputRequest struct {
	Question   string `json:"question"`
	UserAnswer string `json:"user_answer"`
	Expected   string `json:"expected"`
}

// handleSubmitInput handles POST /api/v1/writing/input
func (h *WritingHandler) handleSubmitInput(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req submitInputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Question == "" || req.UserAnswer == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "question and user_answer are required", "")
		return
	}

	rec, err := h.svc.SubmitInput(userID, req.Question, req.UserAnswer, req.Expected)
	if err != nil {
		slog.Error("handleSubmitInput failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec})
}

// submitSentenceRequest is the request body for POST /api/v1/writing/sentence.
type submitSentenceRequest struct {
	Question   string `json:"question"`
	UserAnswer string `json:"user_answer"`
}

// handleSubmitSentence handles POST /api/v1/writing/sentence
func (h *WritingHandler) handleSubmitSentence(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req submitSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Question == "" || req.UserAnswer == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "question and user_answer are required", "")
		return
	}

	rec, err := h.svc.SubmitSentence(userID, req.Question, req.UserAnswer)
	if err != nil {
		slog.Error("handleSubmitSentence failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec})
}

// handleListRecords handles GET /api/v1/writing/records
func (h *WritingHandler) handleListRecords(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	records, err := h.svc.ListRecords(userID)
	if err != nil {
		slog.Error("handleListRecords failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: records})
}

// contextKeyUserID is the key type for storing userID in context.
type contextKeyUserID struct{}

// userIDFromContext extracts the userID injected by AuthMiddleware.
func userIDFromContext(ctx interface{ Value(any) any }) (int64, bool) {
	v := ctx.Value(contextKeyUserID{})
	id, ok := v.(int64)
	return id, ok
}
