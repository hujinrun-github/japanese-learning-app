package shadowing

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

type ServiceInterface interface {
	GetSession(userID, lessonID int64) (*ShadowingSession, error)
	SaveProgress(userID, lessonID int64, req ProgressRequest) (*Progress, error)
	SaveAttempt(userID, lessonID int64, req AttemptRequest) error
}

type Handler struct {
	svc ServiceInterface
}

func NewHandler(svc ServiceInterface) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/lessons/{id}/shadowing", h.handleGetSession)
	mux.HandleFunc("POST /api/v1/lessons/{id}/shadowing/progress", h.handleSaveProgress)
	mux.HandleFunc("POST /api/v1/lessons/{id}/shadowing/attempts", h.handleSaveAttempt)
}

func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	userID, lessonID, ok := h.readRequestScope(w, r)
	if !ok {
		return
	}

	session, err := h.svc.GetSession(userID, lessonID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: session})
}

func (h *Handler) handleSaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, lessonID, ok := h.readRequestScope(w, r)
	if !ok {
		return
	}

	var req ProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", "")
		return
	}

	progress, err := h.svc.SaveProgress(userID, lessonID, req)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: progress})
}

func (h *Handler) handleSaveAttempt(w http.ResponseWriter, r *http.Request) {
	userID, lessonID, ok := h.readRequestScope(w, r)
	if !ok {
		return
	}

	var req AttemptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JSON body", "")
		return
	}

	if err := h.svc.SaveAttempt(userID, lessonID, req); err != nil {
		h.writeServiceError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]bool{"ok": true}})
}

func (h *Handler) readRequestScope(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, ErrUnauthorized, "unauthorized", "")
		return 0, 0, false
	}

	lessonID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid lesson id", "")
		return 0, 0, false
	}

	return userID, lessonID, true
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var shadowingErr *Error
	if !errors.As(err, &shadowingErr) {
		slog.Error("shadowing handler failed", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to handle shadowing request", "")
		return
	}

	status := statusForErrorCode(shadowingErr.Code)
	if shadowingErr.Code == ErrShadowingVersionStale {
		httputil.WriteErrorWithDetails(w, status, shadowingErr.Code, shadowingErr.Error(), "", map[string]any{
			"current_shadowing_version": shadowingErr.CurrentShadowingVersion,
		})
		return
	}

	httputil.WriteError(w, status, shadowingErr.Code, shadowingErr.Error(), "")
}

func statusForErrorCode(code string) int {
	switch code {
	case ErrUnauthorized:
		return http.StatusUnauthorized
	case ErrNotFound:
		return http.StatusNotFound
	case ErrShadowingDisabled:
		return http.StatusForbidden
	case ErrShadowingVersionStale:
		return http.StatusConflict
	case ErrShadowingMediaMissing, ErrShadowingContentInvalid, ErrShadowingAttemptModeInvalid:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
