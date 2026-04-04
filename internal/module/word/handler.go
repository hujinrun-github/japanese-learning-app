package word

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
)

// WordHandler handles HTTP requests for the word module.
type WordHandler struct {
	svc *WordService
}

// NewWordHandler creates a WordHandler.
func NewWordHandler(svc *WordService) *WordHandler {
	return &WordHandler{svc: svc}
}

// RegisterRoutes registers routes onto the provided mux.
// Routes:
//   GET  /api/v1/words/queue?level=N5   → GetReviewQueue
//   POST /api/v1/words/{id}/rate        → SubmitRating
//   POST /api/v1/words/{id}/bookmark    → Bookmark
func (h *WordHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/words/queue", h.handleGetReviewQueue)
	mux.HandleFunc("POST /api/v1/words/{id}/rate", h.handleSubmitRating)
	mux.HandleFunc("POST /api/v1/words/{id}/bookmark", h.handleBookmark)
}

// handleGetReviewQueue handles GET /api/v1/words/queue?level=N5
func (h *WordHandler) handleGetReviewQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	level := JLPTLevel(r.URL.Query().Get("level"))
	if level == "" {
		level = LevelN5
	}

	cards, err := h.svc.GetReviewQueue(userID, level)
	if err != nil {
		slog.Error("handleGetReviewQueue failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: cards})
}

// handleSubmitRating handles POST /api/v1/words/{id}/rate
func (h *WordHandler) handleSubmitRating(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	wordID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid word id", "")
		return
	}

	var req struct {
		Rating ReviewRating `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	if err := h.svc.SubmitRating(userID, wordID, req.Rating); err != nil {
		slog.Error("handleSubmitRating failed", "err", err, "user_id", userID, "word_id", wordID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// handleBookmark handles POST /api/v1/words/{id}/bookmark
func (h *WordHandler) handleBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	wordID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid word id", "")
		return
	}

	if err := h.svc.Bookmark(userID, wordID); err != nil {
		slog.Error("handleBookmark failed", "err", err, "user_id", userID, "word_id", wordID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// userIDFromContext extracts the userID injected by AuthMiddleware.
// Returns (0, false) if not present.
func userIDFromContext(ctx interface{ Value(any) any }) (int64, bool) {
	v := ctx.Value(contextKeyUserID{})
	id, ok := v.(int64)
	return id, ok
}

// contextKeyUserID is the key type for storing userID in context.
type contextKeyUserID struct{}
