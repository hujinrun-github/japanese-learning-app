package favorite

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

var ErrNotFound = errors.New("favorite target not found")

type Target struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type Item struct {
	Target
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	JLPTLevel string `json:"jlpt_level"`
	Path      string `json:"path"`
}

type Store interface {
	List(context.Context, int64) ([]Item, error)
	Set(context.Context, int64, Target, bool) (*Item, error)
}

func RegisterRoutes(mux *http.ServeMux, store Store) {
	mux.HandleFunc("GET /api/v1/favorites", func(w http.ResponseWriter, r *http.Request) {
		uid, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		items, err := store.List(r.Context(), uid)
		if err != nil {
			slog.Error("load favorites", "user_id", uid, "err", err)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load favorites", "")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: items})
	})

	change := func(saved bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			uid, ok := user.UserIDFromContext(r.Context())
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
				return
			}
			target := Target{Kind: r.PathValue("kind"), Key: r.PathValue("key")}
			if !target.valid() {
				httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid favorite target", "")
				return
			}
			item, err := store.Set(r.Context(), uid, target, saved)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "favorite target not found", "")
					return
				}
				slog.Error("update favorite", "user_id", uid, "kind", target.Kind, "key", target.Key, "err", err)
				httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to update favorite", "")
				return
			}
			if saved {
				httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: item})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("PUT /api/v1/favorites/{kind}/{key}", change(true))
	mux.HandleFunc("DELETE /api/v1/favorites/{kind}/{key}", change(false))
}

func (t Target) valid() bool {
	switch t.Kind {
	case "word", "grammar", "lesson", "listening", "speaking", "shadowing", "translation", "note":
		return validNumber(t.Key, 1)
	case "listening_sentence":
		id, position, ok := strings.Cut(t.Key, ":")
		return ok && validNumber(id, 1) && validNumber(position, 0)
	case "writing":
		return len(t.Key) > 0 && len(t.Key) <= 100 && strings.IndexFunc(t.Key, func(r rune) bool {
			return !('a' <= r && r <= 'z' || '0' <= r && r <= '9' || r == '-')
		}) < 0
	default:
		return false
	}
}

func validNumber(value string, minimum int64) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n >= minimum && strconv.FormatInt(n, 10) == value
}
