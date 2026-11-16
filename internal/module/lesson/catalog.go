package lesson

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"japanese-learning-app/internal/httputil"
)

type CatalogQuery struct {
	Level  JLPTLevel
	Search string
	Page   int
	Size   int
}

type CatalogItem struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	JLPTLevel JLPTLevel `json:"jlpt_level"`
	Tags      []string  `json:"tags"`
	CharCount int       `json:"char_count"`
}

type CatalogPage struct {
	Items []CatalogItem `json:"items"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

type CatalogStore interface {
	ListCatalog(context.Context, CatalogQuery) (CatalogPage, error)
}

func RegisterCatalogRoute(mux *http.ServeMux, store CatalogStore) {
	mux.HandleFunc("GET /api/v1/lessons/catalog", func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		level := JLPTLevel(params.Get("level"))
		switch level {
		case "", LevelN5, LevelN4, LevelN3, LevelN2, LevelN1:
		default:
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid JLPT level", "")
			return
		}
		page, pageOK := parseCatalogNumber(params.Get("page"), 1, 1<<31-1)
		size, sizeOK := parseCatalogNumber(params.Get("size"), 20, 100)
		if !pageOK || !sizeOK {
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid page or size", "")
			return
		}

		query := CatalogQuery{Level: level, Search: strings.TrimSpace(params.Get("q")), Page: page, Size: size}
		slog.Debug("listing lesson catalog", "level", level, "page", page, "size", size, "search", query.Search != "")
		result, err := store.ListCatalog(r.Context(), query)
		if err != nil {
			slog.Error("failed to list lesson catalog", "err", err, "level", level, "page", page)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to list lessons", "")
			return
		}
		slog.Debug("listed lesson catalog", "total", result.Total, "returned", len(result.Items))
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: result})
	})
}

func parseCatalogNumber(raw string, fallback, max int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= 1 && n <= max
}
