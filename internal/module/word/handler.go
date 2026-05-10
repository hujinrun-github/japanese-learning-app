package word

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/sm2"
)

// NoteDigestProvider is the optional interface for cross-module note enrichment.
type NoteDigestProvider interface {
	ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
}

// NoteDigest is a lightweight note reference for cross-module responses.
type NoteDigest struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// WordHandler handles HTTP requests for the word module.
type WordHandler struct {
	svc     *WordService
	noteSvc NoteDigestProvider
}

// NewWordHandler creates a WordHandler.
func NewWordHandler(svc *WordService) *WordHandler {
	return &WordHandler{svc: svc}
}

// NewWordHandlerWithNotes creates a WordHandler with optional note enrichment.
func NewWordHandlerWithNotes(svc *WordService, noteSvc NoteDigestProvider) *WordHandler {
	return &WordHandler{svc: svc, noteSvc: noteSvc}
}

// RegisterRoutes registers routes onto the provided mux.
// Routes:
//   GET  /api/v1/words/queue?level=N5   → GetReviewQueue
//   POST /api/v1/words/{id}/rate        → SubmitRating
//   POST /api/v1/words/{id}/bookmark    → Bookmark
func (h *WordHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/words/{id}", h.handleGetWord)
	mux.HandleFunc("GET /api/v1/words/queue", h.handleGetReviewQueue)
	mux.HandleFunc("POST /api/v1/words/{id}/rate", h.handleSubmitRating)
	mux.HandleFunc("POST /api/v1/words/{id}/bookmark", h.handleBookmark)
}

// handleGetReviewQueue handles GET /api/v1/words/queue?level=N5&limit=50
func (h *WordHandler) handleGetReviewQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
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
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load review queue", "")
		return
	}

	limit := parseLimitParam(r.URL.Query().Get("limit"), 50)
	if len(cards) > limit {
		cards = cards[:limit]
	}

	// Enrich examples with furigana HTML for any that don't have it pre-rendered
	for i := range cards {
		examples := cards[i].Word.Examples
		for j := range examples {
			if examples[j].FuriganaHTML == "" && examples[j].Japanese != "" {
				examples[j].FuriganaHTML = FuriganaHTML(examples[j].Japanese)
			}
		}
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: cards})
}

// handleSubmitRating handles POST /api/v1/words/{id}/rate
func (h *WordHandler) handleSubmitRating(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
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
		Rating sm2.Rating `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	if err := h.svc.SubmitRating(userID, wordID, req.Rating); err != nil {
		slog.Error("handleSubmitRating failed", "err", err, "user_id", userID, "word_id", wordID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to save rating", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// handleBookmark handles POST /api/v1/words/{id}/bookmark
func (h *WordHandler) handleBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
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
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to update bookmark", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// handleGetWord handles GET /api/v1/words/{id}
func (h *WordHandler) handleGetWord(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	wordID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid word id", "")
		return
	}

	wrd, err := h.svc.GetByID(wordID)
	if err != nil {
		slog.Error("handleGetWord failed", "err", err, "word_id", wordID)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "word not found", "")
		return
	}

	type enriched struct {
		Word
		RelatedNotes []NoteDigest `json:"related_notes"`
	}
	response := enriched{Word: *wrd}

	if h.noteSvc != nil {
		notes, err := h.noteSvc.ListByReference(userID, "word", wordID, 5)
		if err == nil {
			response.RelatedNotes = notes
		}
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: response})
}

// parseLimitParam parses a limit query parameter, clamping to [1, 200] and
// returning fallback if the value is missing or invalid.
func parseLimitParam(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	if n > 200 {
		return 200
	}
	return n
}
