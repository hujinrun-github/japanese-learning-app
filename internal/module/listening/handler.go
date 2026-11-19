package listening

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

type Handler struct{ store Store }

func NewHandler(store Store) *Handler { return &Handler{store: store} }
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/listening/materials", h.list)
	mux.HandleFunc("GET /api/v1/listening/materials/{id}", h.session)
	mux.HandleFunc("PUT /api/v1/listening/materials/{id}/progress", h.progress)
	mux.HandleFunc("POST /api/v1/listening/materials/{id}/attempts", h.attempt)
	mux.HandleFunc("POST /api/v1/listening/materials/{id}/complete", h.complete)
}
func (h *Handler) scope(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return 0, 0, false
	}
	id := int64(0)
	if raw := r.PathValue("id"); raw != "" {
		var err error
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "材料 ID 无效", "")
			return 0, 0, false
		}
	}
	return uid, id, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "请求内容无效", "")
		return false
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "请求只能包含一个对象", "")
		return false
	}
	return true
}
func writeResult(w http.ResponseWriter, v any, err error) {
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			httputil.WriteError(w, e.Status, e.Code, e.Message, "")
		} else {
			slog.Error("listening request failed", "err", err)
			httputil.WriteError(w, 500, "ERR_INTERNAL", "听力训练暂不可用", "")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, 200, httputil.APIResponse{Data: v})
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := h.scope(w, r)
	if !ok {
		return
	}
	v, err := h.store.ListMaterials(r.Context(), uid, r.URL.Query().Get("level"), r.URL.Query().Get("status"))
	writeResult(w, v, err)
}
func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	v, err := h.store.GetSession(r.Context(), uid, id)
	writeResult(w, v, err)
}
func (h *Handler) progress(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	var req ProgressRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.store.SaveProgress(r.Context(), uid, id, req)
	writeResult(w, v, err)
}
func (h *Handler) attempt(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	var req AttemptRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.store.SubmitAttempt(r.Context(), uid, id, req)
	writeResult(w, v, err)
}
func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	v, err := h.store.Complete(r.Context(), uid, id)
	writeResult(w, v, err)
}
