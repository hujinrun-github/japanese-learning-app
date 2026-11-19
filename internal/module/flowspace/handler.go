package flowspace

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/integrations/flowspace", h.status)
	mux.HandleFunc("POST /api/v1/integrations/flowspace/pair", h.pair)
	mux.HandleFunc("DELETE /api/v1/integrations/flowspace", h.disconnect)
	mux.HandleFunc("POST /api/v1/integrations/flowspace/retry", h.retry)
}

func (h *Handler) identity(w http.ResponseWriter, r *http.Request) (int64, bool) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
	}
	return uid, ok
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	slog.Error("FlowSpace integration request failed", "error", err)
	httputil.WriteError(w, 502, "ERR_FLOWSPACE_INTEGRATION", err.Error(), "")
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.identity(w, r)
	if !ok {
		return
	}
	status, err := h.service.Status(r.Context(), uid)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, 200, httputil.APIResponse{Data: status})
}

func (h *Handler) pair(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.identity(w, r)
	if !ok {
		return
	}
	var input struct {
		PairingCode string `json:"pairing_code"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "配对码格式无效", "")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "配对码格式无效", "")
		return
	}
	status, err := h.service.Pair(r.Context(), uid, input.PairingCode)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, 200, httputil.APIResponse{Data: status})
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.identity(w, r)
	if !ok {
		return
	}
	if err := h.service.Disconnect(r.Context(), uid); err != nil {
		h.fail(w, err)
		return
	}
	h.status(w, r)
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.identity(w, r)
	if !ok {
		return
	}
	if err := h.service.Retry(r.Context(), uid); err != nil {
		h.fail(w, err)
		return
	}
	h.status(w, r)
}
