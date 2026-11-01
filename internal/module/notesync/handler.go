package notesync

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/note-sync/targets", h.handleListTargets)
	mux.HandleFunc("POST /api/v1/note-sync/targets", h.handleCreateTarget)
	mux.HandleFunc("PUT /api/v1/note-sync/targets/{id}", h.handleUpdateTarget)
	mux.HandleFunc("DELETE /api/v1/note-sync/targets/{id}", h.handleDeleteTarget)
	mux.HandleFunc("POST /api/v1/note-sync/targets/test", h.handleTestTarget)
	mux.HandleFunc("GET /api/v1/notes/{id}/sync-binding", h.handleGetBinding)
	mux.HandleFunc("PUT /api/v1/notes/{id}/sync-binding", h.handlePutBinding)
	mux.HandleFunc("DELETE /api/v1/notes/{id}/sync-binding", h.handleDeleteBinding)
	mux.HandleFunc("GET /api/v1/notes/{id}/sync-state", h.handleGetState)
	mux.HandleFunc("POST /api/v1/note-sync/notes/{id}/push", h.handlePushNote)
	mux.HandleFunc("POST /api/v1/note-sync/targets/{id}/push", h.handlePushTarget)
	mux.HandleFunc("POST /api/v1/note-sync/targets/{id}/pull", h.handlePullTarget)
	mux.HandleFunc("GET /api/v1/note-sync/targets/{id}/deletions", h.handleListDeletions)
	mux.HandleFunc("POST /api/v1/note-sync/targets/{id}/deletions/{noteID}/confirm", h.handleConfirmDeletion)
	mux.HandleFunc("POST /api/v1/note-sync/targets/{id}/deletions/{noteID}/restore", h.handleRestoreDeletion)
}

func (h *Handler) userID(r *http.Request) (int64, bool) { return user.UserIDFromContext(r.Context()) }

func (h *Handler) handleListTargets(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targets, err := h.service.ListTargets(uid)
	if err != nil {
		writeInternal(w, "failed to list Notion targets", err)
		return
	}
	httpJSON(w, http.StatusOK, targets)
}

func (h *Handler) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	var req TargetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, "invalid Notion target")
		return
	}
	target, err := h.service.CreateTarget(uid, req)
	if err != nil {
		writeServiceError(w, err, "failed to create Notion target")
		return
	}
	httpJSON(w, http.StatusCreated, target)
}

func (h *Handler) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	var req TargetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, "invalid Notion target")
		return
	}
	target, err := h.service.UpdateTarget(uid, targetID, req)
	if err != nil {
		writeServiceError(w, err, "failed to update Notion target")
		return
	}
	httpJSON(w, http.StatusOK, target)
}

func (h *Handler) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	if err := h.service.DeleteTarget(uid, targetID); err != nil {
		writeServiceError(w, err, "failed to delete Notion target")
		return
	}
	httpJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleTestTarget(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	var req TargetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, "invalid Notion target")
		return
	}
	if err := h.service.TestTarget(uid, req); err != nil {
		writeServiceError(w, err, "Notion connection test failed")
		return
	}
	httpJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) handleGetBinding(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	noteID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	binding, err := h.service.GetBinding(uid, noteID)
	if err != nil {
		writeServiceError(w, err, "failed to load note binding")
		return
	}
	httpJSON(w, http.StatusOK, binding)
}

func (h *Handler) handlePutBinding(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	noteID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	var req BindingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, "invalid note binding")
		return
	}
	if req.TargetID == nil {
		err = h.service.ClearBinding(uid, noteID)
	} else {
		err = h.service.BindNote(uid, noteID, *req.TargetID)
	}
	if err != nil {
		writeServiceError(w, err, "failed to update note binding")
		return
	}
	binding, err := h.service.GetBinding(uid, noteID)
	if err != nil {
		writeServiceError(w, err, "failed to load note binding")
		return
	}
	httpJSON(w, http.StatusOK, binding)
}

func (h *Handler) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	noteID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	if err := h.service.ClearBinding(uid, noteID); err != nil {
		writeServiceError(w, err, "failed to remove note binding")
		return
	}
	httpJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleGetState(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	noteID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	state, err := h.service.GetState(uid, noteID)
	if err != nil {
		writeServiceError(w, err, "failed to load note sync state")
		return
	}
	httpJSON(w, http.StatusOK, state)
}

func (h *Handler) handlePushNote(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	noteID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	state, err := h.service.PushNote(uid, noteID)
	if err != nil {
		writeServiceError(w, err, "failed to sync note to Notion")
		return
	}
	httpJSON(w, http.StatusOK, state)
}

func (h *Handler) handlePushTarget(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	result, err := h.service.PushTarget(uid, targetID)
	if err != nil {
		writeServiceError(w, err, "failed to push notes to Notion")
		return
	}
	httpJSON(w, http.StatusOK, result)
}

func (h *Handler) handlePullTarget(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	result, err := h.service.PullTarget(uid, targetID)
	if err != nil {
		writeServiceError(w, err, "failed to pull notes from Notion")
		return
	}
	httpJSON(w, http.StatusOK, result)
}

func (h *Handler) handleListDeletions(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	items, err := h.service.ListPendingDeletes(uid, targetID)
	if err != nil {
		writeServiceError(w, err, "failed to list Notion deletions")
		return
	}
	httpJSON(w, http.StatusOK, items)
}

func (h *Handler) handleConfirmDeletion(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	noteID, err := pathID(r, "noteID")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	if err := h.service.ConfirmDelete(uid, targetID, noteID); err != nil {
		writeServiceError(w, err, "failed to confirm Notion deletion")
		return
	}
	httpJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleRestoreDeletion(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	targetID, err := pathID(r, "id")
	if err != nil {
		writeBadRequest(w, "invalid target id")
		return
	}
	noteID, err := pathID(r, "noteID")
	if err != nil {
		writeBadRequest(w, "invalid note id")
		return
	}
	state, err := h.service.RestorePendingDelete(uid, targetID, noteID)
	if err != nil {
		writeServiceError(w, err, "failed to restore Notion page")
		return
	}
	httpJSON(w, http.StatusOK, state)
}

func decodeJSON(r *http.Request, value any) error { return json.NewDecoder(r.Body).Decode(value) }

func pathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

func writeUnauthorized(w http.ResponseWriter) {
	httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
}
func writeBadRequest(w http.ResponseWriter, message string) {
	httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", message, "")
}
func writeInternal(w http.ResponseWriter, message string, err error) {
	slog.Error(message, "err", err)
	httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", message, "")
}
func httpJSON(w http.ResponseWriter, status int, data any) {
	httputil.WriteJSON(w, status, httputil.APIResponse{Data: data})
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) {
	slog.Error(fallback, "err", err)
	status, code, message := http.StatusInternalServerError, "ERR_INTERNAL", fallback
	switch {
	case errors.Is(err, ErrTargetNotFound), errors.Is(err, ErrBindingNotFound), errors.Is(err, ErrStateNotFound):
		status, code, message = http.StatusNotFound, "ERR_NOT_FOUND", "Notion sync resource not found"
	case errors.Is(err, ErrTargetInUse), errors.Is(err, ErrIdentityLocked):
		status, code, message = http.StatusConflict, "ERR_CONFLICT", err.Error()
	default:
		status, code, message = http.StatusBadRequest, "ERR_NOTION_SYNC", err.Error()
	}
	httputil.WriteError(w, status, code, message, "")
}
