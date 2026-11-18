package errata

import (
	"encoding/json"
	"errors"
	"fmt"
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
	mux.HandleFunc("PATCH /api/v1/errata/{id}", updateMine(store))
	mux.HandleFunc("DELETE /api/v1/errata/{id}", deleteMine(store))
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

func reportID(r *http.Request) (int64, error) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse errata report id %q: %w", raw, err)
	}
	if id <= 0 {
		return 0, errors.New("errata report id must be positive")
	}
	return id, nil
}

func optionalString(value *string) string {
	if value == nil {
		return "<unchanged>"
	}
	return *value
}

// loadOwnedPending 统一处理「只能改本人的、仍待处理的勘误」。
// ok=false 表示响应已写毕，调用方应立即返回。
func loadOwnedPending(w http.ResponseWriter, r *http.Request, store *Store, userID, id int64) (Report, bool) {
	report, found, err := store.GetByIDForUser(r.Context(), id, userID)
	if err != nil {
		slog.Error("load learner errata failed", "err", err, "user_id", userID, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not load errata report", "")
		return Report{}, false
	}
	if !found {
		slog.Debug("learner errata not found", "user_id", userID, "id", id)
		httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "errata report not found", "")
		return Report{}, false
	}
	if report.Status != Pending {
		slog.Error("learner errata modification rejected: already resolved",
			"user_id", userID, "id", id, "module", report.Module, "category", report.Category)
		httputil.WriteError(w, http.StatusConflict, "ERR_ERRATA_RESOLVED", "resolved errata report cannot be modified", "")
		return Report{}, false
	}
	return report, true
}

func updateMine(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		id, err := reportID(r)
		if err != nil {
			slog.Error("errata edit invalid id", "err", err, "user_id", userID, "id", r.PathValue("id"))
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata report id", "")
			return
		}
		var req struct {
			Category    *string `json:"category"`
			ItemLabel   *string `json:"item_label"`
			ItemKey     *string `json:"item_key"`
			Description *string `json:"description"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			slog.Error("errata edit invalid body", "err", err, "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata edit", "")
			return
		}
		if req.Category == nil && req.ItemLabel == nil && req.ItemKey == nil && req.Description == nil {
			slog.Error("errata edit empty body", "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "no editable fields provided", "")
			return
		}
		slog.Debug("errata edit requested", "user_id", userID, "id", id,
			"category", optionalString(req.Category), "item_label", optionalString(req.ItemLabel),
			"item_key", optionalString(req.ItemKey), "description", optionalString(req.Description))

		report, ready := loadOwnedPending(w, r, store, userID, id)
		if !ready {
			return
		}

		if report.Category == "audio" && req.Category != nil && *req.Category != report.Category {
			slog.Error("audio errata category change rejected", "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusConflict, "ERR_ERRATA_DELETE_RESTRICTED", "audio errata category cannot be changed by the learner", "")
			return
		}
		next := report
		if req.Category != nil {
			next.Category = *req.Category
		}
		if req.ItemLabel != nil {
			next.ItemLabel = strings.TrimSpace(*req.ItemLabel)
		}
		if req.ItemKey != nil {
			next.ItemKey = strings.TrimSpace(*req.ItemKey)
		}
		if req.Description != nil {
			next.Description = strings.TrimSpace(*req.Description)
		}

		if !validCategory(next.Category) {
			slog.Error("errata edit rejected category", "user_id", userID, "id", id, "category", next.Category)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata report category", "")
			return
		}
		allowLabel, allowKey := editableFields(next.Category)
		if req.ItemLabel != nil && !allowLabel {
			slog.Error("errata edit rejected item_label by category",
				"user_id", userID, "id", id, "module", next.Module, "category", next.Category)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "item_label is not editable for this category", "")
			return
		}
		if req.ItemKey != nil && !allowKey {
			slog.Error("errata edit rejected item_key by category",
				"user_id", userID, "id", id, "module", next.Module, "category", next.Category)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "item_key is not editable for this category", "")
			return
		}
		if next.ItemLabel == "" || utf8.RuneCountInString(next.ItemLabel) > 300 ||
			next.Description == "" || utf8.RuneCountInString(next.Description) > 1000 {
			slog.Error("errata edit rejected fields", "user_id", userID, "id", id,
				"item_label_len", utf8.RuneCountInString(next.ItemLabel), "description_len", utf8.RuneCountInString(next.Description))
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata report fields", "")
			return
		}
		// 定位键按模块校验；未提交该字段时保留原值，避免历史数据格式阻塞描述修订。
		if req.ItemKey != nil && !validItemKeyForModule(next.Module, next.ItemKey) {
			slog.Error("errata edit rejected item_key for module",
				"user_id", userID, "id", id, "module", next.Module, "item_key", next.ItemKey)
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "item_key does not match the report module", "")
			return
		}

		updated, err := store.UpdateForUser(r.Context(), next)
		if err != nil {
			slog.Error("errata edit failed", "err", err, "user_id", userID, "id", id,
				"module", next.Module, "category", next.Category)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not update errata report", "")
			return
		}
		if !updated {
			slog.Error("errata edit lost race", "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusConflict, "ERR_CONFLICT", "errata report changed, please retry", "")
			return
		}
		slog.Debug("errata edited", "user_id", userID, "id", id,
			"module", next.Module, "category", next.Category, "item_key", next.ItemKey)
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: next})
	}
}

func deleteMine(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := user.UserIDFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
			return
		}
		id, err := reportID(r)
		if err != nil {
			slog.Error("errata delete invalid id", "err", err, "user_id", userID, "id", r.PathValue("id"))
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid errata report id", "")
			return
		}
		slog.Debug("errata delete requested", "user_id", userID, "id", id)

		report, ready := loadOwnedPending(w, r, store, userID, id)
		if !ready {
			return
		}
		if !learnerDeletable(report.Category) {
			slog.Error("errata delete rejected by category",
				"user_id", userID, "id", id, "module", report.Module, "category", report.Category)
			httputil.WriteError(w, http.StatusConflict, "ERR_ERRATA_DELETE_RESTRICTED",
				"audio errata reports cannot be withdrawn by the learner", "")
			return
		}
		deleted, err := store.DeleteForUser(r.Context(), id, userID)
		if err != nil {
			slog.Error("errata delete failed", "err", err, "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "could not delete errata report", "")
			return
		}
		if !deleted {
			slog.Error("errata delete lost race", "user_id", userID, "id", id)
			httputil.WriteError(w, http.StatusConflict, "ERR_CONFLICT", "errata report changed, please retry", "")
			return
		}
		slog.Debug("errata deleted", "user_id", userID, "id", id,
			"module", report.Module, "category", report.Category)
		w.WriteHeader(http.StatusNoContent)
	}
}
