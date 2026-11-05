package errata

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

func RegisterRoutes(mux *http.ServeMux, store *Store) {
	mux.HandleFunc("POST /api/v1/errata", submit(store))
	mux.HandleFunc("GET /api/v1/errata", listMine(store))
}

func submit(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		var req struct {
			Module      string `json:"module"`
			ItemKey     string `json:"item_key"`
			ItemLabel   string `json:"item_label"`
			Category    string `json:"category"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			slog.Debug("errata submit invalid body", "err", err, "user_id", userID)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata report", "")
			return
		}
		report := Report{
			UserID: userID, Module: req.Module, ItemKey: req.ItemKey,
			ItemLabel: strings.TrimSpace(req.ItemLabel), Category: req.Category,
			Description: strings.TrimSpace(req.Description),
		}
		if !validModule(report.Module) || !validCategory(report.Category) || !validItemKey(report.ItemKey) ||
			report.ItemLabel == "" || utf8.RuneCountInString(report.ItemLabel) > 300 ||
			report.Description == "" || utf8.RuneCountInString(report.Description) > 1000 {
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata report fields", "")
			return
		}
		id, err := store.Save(r.Context(), report)
		if err != nil {
			slog.Error("errata submit failed", "err", err, "user_id", userID, "module", report.Module, "item_key", report.ItemKey)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not save errata report", "")
			return
		}
		slog.Debug("errata submitted", "id", id, "module", report.Module, "item_key", report.ItemKey)
		httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: map[string]int64{"id": id}})
	}
}

func listMine(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		page := 1
		if value := r.URL.Query().Get("page"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 || parsed > 50000 {
				httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid page", "")
				return
			}
			page = parsed
		}
		const pageSize = 20
		items, err := store.ListByUser(r.Context(), userID, pageSize+1, (page-1)*pageSize)
		if err != nil {
			slog.Error("list learner errata failed", "err", err, "user_id", userID, "page", page)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not load errata reports", "")
			return
		}
		hasMore := len(items) > pageSize
		if hasMore {
			items = items[:pageSize]
		}
		slog.Debug("listed learner errata", "user_id", userID, "page", page, "count", len(items))
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: struct {
			Items   []Report `json:"items"`
			HasMore bool     `json:"has_more"`
		}{Items: items, HasMore: hasMore}})
	}
}

func validModule(module string) bool {
	switch module {
	case "word", "grammar", "lesson", "speaking", "shadowing", "writing", "translation":
		return true
	default:
		return false
	}
}

func validCategory(category string) bool {
	switch category {
	case "prompt", "answer", "explanation", "audio", "other":
		return true
	default:
		return false
	}
}

func validItemKey(key string) bool {
	return key != "" && len(key) <= 80 && strings.IndexFunc(key, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ':' || r == '-' || r == '_')
	}) == -1
}
