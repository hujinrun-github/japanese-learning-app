package mistake

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

const (
	allModules  = "all"
	allStatus   = "all"
	defaultSize = 20
	maxSize     = 100
)

// RegisterRoutes wires the learner-facing mistake book endpoints. Callers must
// mount /api/v1/mistakes (and its subtree) behind AuthMiddleware.
func RegisterRoutes(mux *http.ServeMux, store *Store) {
	mux.HandleFunc("POST /api/v1/mistakes", create(store))
	mux.HandleFunc("GET /api/v1/mistakes", list(store))
	mux.HandleFunc("PATCH /api/v1/mistakes/{id}", updateStatus(store))
}

func create(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		var in Input
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			slog.Error("mistake create invalid body", "err", err, "user_id", userID)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid mistake", "")
			return
		}
		in.Prompt = strings.TrimSpace(in.Prompt)
		in.UserAnswer = strings.TrimSpace(in.UserAnswer)
		in.CorrectAnswer = strings.TrimSpace(in.CorrectAnswer)
		in.Explanation = strings.TrimSpace(in.Explanation)
		in.SourcePath = strings.TrimSpace(in.SourcePath)
		slog.Debug("mistake create requested", "user_id", userID, "module", in.Module, "item_key", in.ItemKey)

		if !validModule(in.Module) || !validItemKey(in.ItemKey) || !validText(in.Prompt, 1, 500) ||
			!validText(in.UserAnswer, 0, 500) || !validText(in.CorrectAnswer, 0, 500) ||
			!validText(in.Explanation, 0, 1000) || !validSourcePath(in.SourcePath) {
			slog.Error("mistake create rejected fields", "user_id", userID, "module", in.Module, "item_key", in.ItemKey)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid mistake fields", "")
			return
		}

		id, err := store.Save(r.Context(), userID, in)
		if err != nil {
			slog.Error("mistake create failed", "err", err, "user_id", userID, "module", in.Module, "item_key", in.ItemKey)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not save mistake", "")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: map[string]int64{"id": id}})
	}
}

func list(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		query := r.URL.Query()
		module := query.Get("module")
		if module == "" || module == allModules {
			module = ""
		} else if !validModule(module) {
			slog.Error("mistake list invalid module", "user_id", userID, "module", module)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid module", "")
			return
		}

		status := query.Get("status")
		switch status {
		case "", Unmastered, Mastered:
			if status == "" {
				status = Unmastered
			}
		case allStatus:
			status = ""
		default:
			slog.Error("mistake list invalid status", "user_id", userID, "status", status)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid status", "")
			return
		}

		page, err := parsePositiveInt(query.Get("page"), 1, 50000)
		if err != nil {
			slog.Error("mistake list invalid page", "err", err, "user_id", userID, "page", query.Get("page"))
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid page", "")
			return
		}
		size, err := parsePositiveInt(query.Get("size"), defaultSize, maxSize)
		if err != nil {
			slog.Error("mistake list invalid size", "err", err, "user_id", userID, "size", query.Get("size"))
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid size", "")
			return
		}
		slog.Debug("mistake list requested", "user_id", userID, "module", module, "status", status, "page", page, "size", size)

		items, total, err := store.List(r.Context(), userID, module, status, size, (page-1)*size)
		if err != nil {
			slog.Error("mistake list failed", "err", err, "user_id", userID, "page", page)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not load mistakes", "")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: Page{
			Items: items, Total: total, Page: page, Size: size,
		}})
	}
}

func updateStatus(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			slog.Error("mistake update invalid id", "err", err, "user_id", userID, "id", r.PathValue("id"))
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid mistake id", "")
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
			slog.Error("mistake update invalid body", "err", err, "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid status body", "")
			return
		}
		if !validStatus(body.Status) {
			slog.Error("mistake update rejected status", "user_id", userID, "id", id, "status", body.Status)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid status", "")
			return
		}
		slog.Debug("mistake update requested", "user_id", userID, "id", id, "status", body.Status)

		updated, err := store.SetStatus(r.Context(), userID, id, body.Status)
		if err != nil {
			slog.Error("mistake update failed", "err", err, "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not update mistake", "")
			return
		}
		if !updated {
			httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "mistake not found", "")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]any{
			"id": id, "status": body.Status,
		}})
	}
}

func parsePositiveInt(raw string, fallback, max int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("mistake.parsePositiveInt: %w", err)
	}
	if value < 1 || value > max {
		return 0, fmt.Errorf("mistake.parsePositiveInt: value %d outside 1..%d", value, max)
	}
	return value, nil
}

func validModule(module string) bool {
	switch module {
	case "word", "grammar", "writing", "translation":
		return true
	default:
		return false
	}
}

func validStatus(status string) bool {
	return status == Unmastered || status == Mastered
}

func validText(value string, min, max int) bool {
	length := utf8.RuneCountInString(value)
	return length >= min && length <= max
}

func validItemKey(key string) bool {
	return key != "" && len(key) <= 120 && strings.IndexFunc(key, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == ':' || r == '-' || r == '_')
	}) == -1
}

func validSourcePath(path string) bool {
	return path == "" || (len(path) <= 200 && strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//"))
}
