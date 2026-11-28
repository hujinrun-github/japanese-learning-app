package listening

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/practicelinks"
	"japanese-learning-app/internal/module/user"
)

type KnowledgeLinkStore interface {
	ListListeningLinks(ctx context.Context, lessonID int64, includeContent, includeQuestions bool) (practicelinks.ListeningLinks, error)
}

type Handler struct {
	store Store
	links KnowledgeLinkStore
}

func NewHandler(store Store) *Handler                         { return &Handler{store: store} }
func (h *Handler) SetKnowledgeLinks(links KnowledgeLinkStore) { h.links = links }
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/listening/materials", h.list)
	mux.HandleFunc("GET /api/v1/listening/materials/{id}", h.session)
	mux.HandleFunc("GET /api/v1/listening/materials/{id}/knowledge-links", h.knowledgeLinks)
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

func (h *Handler) knowledgeLinks(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := h.scope(w, r)
	if !ok {
		return
	}
	cycle, err := strconv.Atoi(r.URL.Query().Get("cycle"))
	stage := r.URL.Query().Get("stage")
	if err != nil || cycle < 1 || (stage != "blind" && stage != "intensive" && stage != "retest" && stage != "completed") {
		writeResult(w, nil, &Error{Status: http.StatusBadRequest, Code: "ERR_BAD_REQUEST", Message: "训练阶段或轮次无效"})
		return
	}
	session, err := h.store.GetSession(r.Context(), uid, id)
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	content, questions := knowledgeVisibility(session, stage, cycle)
	if h.links == nil || (!content && !questions) {
		writeResult(w, practicelinks.ListeningLinks{}, nil)
		return
	}
	links, err := h.links.ListListeningLinks(r.Context(), id, content, questions)
	writeResult(w, links, err)
}

func knowledgeVisibility(session Session, stage string, cycle int) (content, questions bool) {
	if session.Progress.Cycle != cycle {
		return false, false
	}
	var blind, retest bool
	for _, attempt := range session.Attempts {
		blind = blind || attempt.Phase == "blind"
		retest = retest || attempt.Phase == "retest"
	}
	if session.Progress.Stage == "retest" && !retest {
		return false, false
	}
	switch stage {
	case "intensive":
		content = session.Progress.Stage == "intensive" || session.Progress.Stage == "completed" || retest
		return content, content && blind
	case "blind":
		return false, blind
	case "retest", "completed":
		return false, retest
	default:
		return false, false
	}
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
