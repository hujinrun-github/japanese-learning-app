package translation

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// TranslationHandler handles HTTP requests for the translation module.
type TranslationHandler struct {
	svc *TranslationService
}

// NewTranslationHandler creates a TranslationHandler.
func NewTranslationHandler(svc *TranslationService) *TranslationHandler {
	return &TranslationHandler{svc: svc}
}

// RegisterRoutes registers translation routes.
func (h *TranslationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/translation/queue", h.handleGetDailyQueue)
	mux.HandleFunc("GET /api/v1/translation/sentences", h.handleGetSentences)
	mux.HandleFunc("POST /api/v1/translation/submit", h.handleSubmit)
	mux.HandleFunc("GET /api/v1/translation/records/{id}", h.handleGetRecord)
	mux.HandleFunc("GET /api/v1/translation/records", h.handleListRecords)
	mux.HandleFunc("POST /api/v1/translation/sources/import", h.handleImportURL)
	mux.HandleFunc("POST /api/v1/translation/sources", h.handleImportSource)
	mux.HandleFunc("GET /api/v1/translation/sources", h.handleListSources)
}

// getUserID extracts user ID from request context.
func getUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "user not authenticated", r.Header.Get("X-Request-ID"))
		return 0, false
	}
	return userID, true
}

// handleGetDailyQueue returns today's practice sentences for the user.
func (h *TranslationHandler) handleGetDailyQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}
	count := 5
	if n, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && n > 0 && n <= 20 {
		count = n
	}
	sentences, err := h.svc.GetDailyQueue(userID, count)
	if err != nil {
		slog.Error("handleGetDailyQueue failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sentences, RequestID: r.Header.Get("X-Request-ID")})
}

// handleGetSentences returns sentences optionally filtered by direction and source.
func (h *TranslationHandler) handleGetSentences(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}
	var sourceID int64
	if s := r.URL.Query().Get("source_id"); s != "" {
		sourceID, _ = strconv.ParseInt(s, 10, 64)
	}
	sentences, err := h.svc.GetFreeSentences(userID, r.URL.Query().Get("direction"), sourceID)
	if err != nil {
		slog.Error("handleGetSentences failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sentences, RequestID: r.Header.Get("X-Request-ID")})
}

type submitRequest struct {
	SentenceID      int64  `json:"sentence_id"`
	UserTranslation string `json:"user_translation"`
}

// handleSubmit accepts a user's translation and returns the scored record.
func (h *TranslationHandler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}

	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", r.Header.Get("X-Request-ID"))
		return
	}
	if req.SentenceID == 0 || req.UserTranslation == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "sentence_id and user_translation are required", r.Header.Get("X-Request-ID"))
		return
	}

	rec, err := h.svc.SubmitTranslation(userID, req.SentenceID, req.UserTranslation)
	if err != nil {
		slog.Error("handleSubmit failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec, RequestID: r.Header.Get("X-Request-ID")})
}

// handleGetRecord returns a single translation record by ID.
func (h *TranslationHandler) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid record id", r.Header.Get("X-Request-ID"))
		return
	}
	rec, err := h.svc.GetResult(id)
	if err != nil {
		slog.Error("handleGetRecord failed", "err", err, "id", id)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "record not found", r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec, RequestID: r.Header.Get("X-Request-ID")})
}

// handleListRecords returns all translation records for the authenticated user.
func (h *TranslationHandler) handleListRecords(w http.ResponseWriter, r *http.Request) {
	userID, ok := getUserID(w, r)
	if !ok {
		return
	}
	records, err := h.svc.ListRecords(userID)
	if err != nil {
		slog.Error("handleListRecords failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: records, RequestID: r.Header.Get("X-Request-ID")})
}

type importSourceRequest struct {
	Title      string `json:"title"`
	SourceType string `json:"source_type"`
	Content    string `json:"content"`
	SourceURL  string `json:"source_url"`
}

// handleImportSource imports a manually pasted text source.
func (h *TranslationHandler) handleImportSource(w http.ResponseWriter, r *http.Request) {
	var req importSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", r.Header.Get("X-Request-ID"))
		return
	}
	if req.Content == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "content is required", r.Header.Get("X-Request-ID"))
		return
	}
	if req.SourceType == "" {
		req.SourceType = "manual"
	}

	src := TranslationSource{
		Title:      req.Title,
		SourceType: req.SourceType,
		SourceURL:  req.SourceURL,
		RawContent: req.Content,
	}

	result, err := h.svc.ImportSource(src)
	if err != nil {
		slog.Error("handleImportSource failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: result, RequestID: r.Header.Get("X-Request-ID")})
}

type importURLRequest struct {
	URL string `json:"url"`
}

// handleImportURL fetches content from a URL, extracts text from HTML, and imports it.
func (h *TranslationHandler) handleImportURL(w http.ResponseWriter, r *http.Request) {
	var req importURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", r.Header.Get("X-Request-ID"))
		return
	}
	if req.URL == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "url is required", r.Header.Get("X-Request-ID"))
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(req.URL)
	if err != nil {
		slog.Error("handleImportURL fetch failed", "url", req.URL, "err", err)
		httputil.WriteError(w, http.StatusBadRequest, "ERR_IMPORT_FAILED", "failed to fetch URL: "+err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_IMPORT_FAILED", "URL returned status "+resp.Status, r.Header.Get("X-Request-ID"))
		return
	}

	html, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	text := extractTextFromHTML(string(html))

	if text == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_IMPORT_FAILED", "no text content found at URL", r.Header.Get("X-Request-ID"))
		return
	}

	src := TranslationSource{
		Title:      "Imported from URL",
		SourceType: "url",
		SourceURL:  req.URL,
		RawContent: text,
	}

	result, err := h.svc.ImportSource(src)
	if err != nil {
		slog.Error("handleImportURL import failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: result, RequestID: r.Header.Get("X-Request-ID")})
}

// extractTextFromHTML strips HTML tags and returns the plain text content.
func extractTextFromHTML(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// handleListSources returns all translation sources.
func (h *TranslationHandler) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.svc.ListSources()
	if err != nil {
		slog.Error("handleListSources failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", err.Error(), r.Header.Get("X-Request-ID"))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: sources, RequestID: r.Header.Get("X-Request-ID")})
}
