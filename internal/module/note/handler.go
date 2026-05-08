package note

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/sm2"
)

// NoteHandler handles HTTP requests for the note module.
type NoteHandler struct {
	svc *NoteService
}

// NewNoteHandler creates a NoteHandler.
func NewNoteHandler(svc *NoteService) *NoteHandler {
	return &NoteHandler{svc: svc}
}

// RegisterRoutes registers note routes onto the provided mux.
func (h *NoteHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/notes", h.handleList)
	mux.HandleFunc("POST /api/v1/notes", h.handleCreate)
	mux.HandleFunc("GET /api/v1/notes/search", h.handleSearch)
	mux.HandleFunc("GET /api/v1/notes/tags", h.handleListTags)
	mux.HandleFunc("GET /api/v1/notes/archive", h.handleListArchived)
	mux.HandleFunc("GET /api/v1/notes/review-queue", h.handleGetReviewQueue)
	mux.HandleFunc("GET /api/v1/notes/{id}", h.handleGetDetail)
	mux.HandleFunc("PUT /api/v1/notes/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/notes/{id}", h.handleDelete)
	mux.HandleFunc("POST /api/v1/notes/{id}/links", h.handleAddLink)
	mux.HandleFunc("DELETE /api/v1/notes/{id}/links/{linkId}", h.handleRemoveLink)
	mux.HandleFunc("POST /api/v1/notes/{id}/promote", h.handlePromote)
	mux.HandleFunc("DELETE /api/v1/notes/{id}/promote", h.handleDemote)
	mux.HandleFunc("POST /api/v1/notes/{id}/review", h.handleReview)
	mux.HandleFunc("POST /api/v1/notes/{id}/recycle", h.handleRecycle)
}

func (h *NoteHandler) userID(r *http.Request) (int64, bool) {
	return user.UserIDFromContext(r.Context())
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handleList(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	params := NoteListParams{
		Type:  NoteType(r.URL.Query().Get("type")),
		Tag:   r.URL.Query().Get("tag"),
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}
	if params.Sort == "" {
		params.Sort = "updated_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	params.Offset = (page - 1) * size
	params.Limit = size

	notes, total, err := h.svc.List(uid, params)
	if err != nil {
		slog.Error("handleList failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list notes", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]interface{}{
		"items": notes, "total": total, "page": page, "size": size,
	}})
}

func (h *NoteHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var n Note
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	n.UserID = uid

	if err := h.svc.Create(&n); err != nil {
		slog.Error("handleCreate failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to create note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: n})
}

func (h *NoteHandler) handleGetDetail(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	detail, err := h.svc.GetDetail(uid, noteID)
	if err != nil {
		slog.Error("handleGetDetail failed", "err", err)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "note not found", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: detail})
}

func (h *NoteHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	var n Note
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	n.ID = noteID
	n.UserID = uid

	if err := h.svc.Update(&n); err != nil {
		slog.Error("handleUpdate failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to update note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Delete(uid, noteID); err != nil {
		slog.Error("handleDelete failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to delete note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// ── Search ────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "missing q parameter", "")
		return
	}

	notes, err := h.svc.Search(uid, query, 50)
	if err != nil {
		slog.Error("handleSearch failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "search failed", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: notes})
}

// ── Links ─────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handleAddLink(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	var req struct {
		TargetNoteID int64        `json:"target_note_id"`
		Relation     LinkRelation `json:"relation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	link, err := h.svc.AddLink(uid, noteID, req.TargetNoteID, req.Relation)
	if err != nil {
		slog.Error("handleAddLink failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to create link", "")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: link})
}

func (h *NoteHandler) handleRemoveLink(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	linkID, err := strconv.ParseInt(r.PathValue("linkId"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid link id", "")
		return
	}

	if err := h.svc.RemoveLink(uid, linkID); err != nil {
		slog.Error("handleRemoveLink failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to remove link", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

// ── SRS ───────────────────────────────────────────────────────────────────────

func (h *NoteHandler) handlePromote(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Promote(uid, noteID); err != nil {
		slog.Error("handlePromote failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to promote note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleDemote(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Demote(uid, noteID); err != nil {
		slog.Error("handleDemote failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to demote note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleReview(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	var req struct {
		Rating sm2.Rating `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}

	if err := h.svc.SubmitRating(uid, noteID, req.Rating); err != nil {
		slog.Error("handleReview failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to record review", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleRecycle(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	noteID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid note id", "")
		return
	}

	if err := h.svc.Recycle(uid, noteID); err != nil {
		slog.Error("handleRecycle failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to recycle note", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{"status": "ok"}})
}

func (h *NoteHandler) handleGetReviewQueue(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	notes, err := h.svc.GetReviewQueue(uid)
	if err != nil {
		slog.Error("handleGetReviewQueue failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load review queue", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: notes})
}

func (h *NoteHandler) handleListArchived(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	params := NoteListParams{
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}
	if params.Sort == "" {
		params.Sort = "updated_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	params.Offset = (page - 1) * size
	params.Limit = size

	notes, total, err := h.svc.ListArchived(uid, params)
	if err != nil {
		slog.Error("handleListArchived failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list archived notes", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]interface{}{
		"items": notes, "total": total, "page": page, "size": size,
	}})
}

func (h *NoteHandler) handleListTags(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	tags, err := h.svc.ListTags(uid)
	if err != nil {
		slog.Error("handleListTags failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list tags", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: tags})
}
