package lesson

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/grammar"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

type annotationReference struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Reading string `json:"reading,omitempty"`
	Meaning string `json:"meaning"`
}

type Annotation struct {
	Start   int                   `json:"start"`
	End     int                   `json:"end"`
	Quote   string                `json:"quote"`
	Comment string                `json:"comment"`
	Links   []annotationReference `json:"links"`
}

type annotationWords interface {
	ListAll(word.JLPTLevel, string, int, int) ([]word.Word, int, error)
}

type annotationGrammar interface {
	ListAll(string, string, int, int) ([]grammar.GrammarPoint, int, error)
}

type AnnotationHandler struct {
	db      *sql.DB
	lessons *LessonService
	words   annotationWords
	grammar annotationGrammar
}

func NewAnnotationHandler(db *sql.DB, lessons *LessonService, words annotationWords, grammar annotationGrammar) *AnnotationHandler {
	return &AnnotationHandler{db: db, lessons: lessons, words: words, grammar: grammar}
}

func (h *AnnotationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/lessons/{id}/annotations", h.list)
	mux.HandleFunc("PUT /api/v1/lessons/{id}/annotations", h.save)
	mux.HandleFunc("DELETE /api/v1/lessons/{id}/annotations/{start}/{end}", h.remove)
	mux.HandleFunc("GET /api/v1/lessons/annotation-references", h.search)
}

func annotationIdentity(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return 0, 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid lesson id", "")
		return 0, 0, false
	}
	w.Header().Set("Cache-Control", "no-store")
	return uid, id, true
}

func annotationFailure(w http.ResponseWriter, err error) {
	slog.Error("lesson annotation request failed", "err", err)
	httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "lesson annotation request failed", "")
}

func (h *AnnotationHandler) list(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := annotationIdentity(w, r)
	if !ok {
		return
	}
	slog.Debug("list lesson annotations", "user_id", uid, "lesson_id", id)
	rows, err := h.db.QueryContext(r.Context(), `SELECT start_offset, end_offset, quote, comment, links_json
		FROM lesson_annotations WHERE user_id = $1 AND lesson_id = $2 ORDER BY start_offset, end_offset`, uid, id)
	if err != nil {
		annotationFailure(w, fmt.Errorf("query lesson annotations: %w", err))
		return
	}
	defer rows.Close()

	items := []Annotation{}
	for rows.Next() {
		var a Annotation
		var links string
		if err := rows.Scan(&a.Start, &a.End, &a.Quote, &a.Comment, &links); err != nil {
			annotationFailure(w, fmt.Errorf("scan lesson annotation: %w", err))
			return
		}
		if err := json.Unmarshal([]byte(links), &a.Links); err != nil {
			annotationFailure(w, fmt.Errorf("decode lesson annotation links: %w", err))
			return
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		annotationFailure(w, fmt.Errorf("read lesson annotations: %w", err))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: items})
}

func (h *AnnotationHandler) save(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := annotationIdentity(w, r)
	if !ok {
		return
	}
	var a Annotation
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&a); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid annotation", "")
		return
	}
	a.Comment = strings.TrimSpace(a.Comment)
	if a.Start < 0 || a.End <= a.Start || strings.TrimSpace(a.Quote) == "" ||
		utf8.RuneCountInString(a.Comment) > 2000 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid annotation range, comment, or references", "")
		return
	}
	type referenceKey struct {
		kind string
		id   int64
	}
	seen := make(map[referenceKey]bool, len(a.Links))
	for i, link := range a.Links {
		key := referenceKey{link.Kind, link.ID}
		if (link.Kind != "word" && link.Kind != "grammar") || link.ID < 1 || seen[key] {
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid annotation reference", "")
			return
		}
		seen[key] = true
		var err error
		switch link.Kind {
		case "word":
			err = h.db.QueryRowContext(r.Context(), `SELECT kanji_form, reading, meaning FROM words WHERE id = $1`, link.ID).
				Scan(&link.Title, &link.Reading, &link.Meaning)
		case "grammar":
			link.Reading = ""
			err = h.db.QueryRowContext(r.Context(), `SELECT name, meaning FROM grammar_points WHERE id = $1`, link.ID).
				Scan(&link.Title, &link.Meaning)
		}
		if errors.Is(err, sql.ErrNoRows) {
			httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "linked vocabulary or grammar entry is unavailable", "")
			return
		}
		if err != nil {
			annotationFailure(w, fmt.Errorf("resolve annotation reference: %w", err))
			return
		}
		a.Links[i] = link
	}

	lesson, err := h.lessons.GetDetail(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "lesson not found", "")
			return
		}
		annotationFailure(w, fmt.Errorf("load lesson text for annotation: %w", err))
		return
	}
	if !annotationMatchesText(lesson.Sentences, a.Start, a.End, a.Quote) {
		httputil.WriteError(w, http.StatusConflict, "ERR_STALE_ANNOTATION", "selection no longer matches lesson text", "")
		return
	}
	if a.Links == nil {
		a.Links = []annotationReference{}
	}
	links, err := json.Marshal(a.Links)
	if err != nil {
		annotationFailure(w, fmt.Errorf("encode annotation links: %w", err))
		return
	}
	_, err = h.db.ExecContext(r.Context(), `INSERT INTO lesson_annotations
		(user_id, lesson_id, start_offset, end_offset, quote, comment, links_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, lesson_id, start_offset, end_offset)
		DO UPDATE SET quote = excluded.quote, comment = excluded.comment, links_json = excluded.links_json`,
		uid, id, a.Start, a.End, a.Quote, a.Comment, string(links))
	if err != nil {
		annotationFailure(w, fmt.Errorf("save lesson annotation: %w", err))
		return
	}
	slog.Debug("saved lesson annotation", "user_id", uid, "lesson_id", id, "start", a.Start, "end", a.End)
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: a})
}

func (h *AnnotationHandler) remove(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := annotationIdentity(w, r)
	if !ok {
		return
	}
	start, startErr := strconv.Atoi(r.PathValue("start"))
	end, endErr := strconv.Atoi(r.PathValue("end"))
	if startErr != nil || endErr != nil || start < 0 || end <= start {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid annotation range", "")
		return
	}
	_, err := h.db.ExecContext(r.Context(), `DELETE FROM lesson_annotations
		WHERE user_id = $1 AND lesson_id = $2 AND start_offset = $3 AND end_offset = $4`, uid, id, start, end)
	if err != nil {
		annotationFailure(w, fmt.Errorf("remove lesson annotation: %w", err))
		return
	}
	slog.Debug("removed lesson annotation", "user_id", uid, "lesson_id", id, "start", start, "end", end)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AnnotationHandler) search(w http.ResponseWriter, r *http.Request) {
	if _, ok := user.UserIDFromContext(r.Context()); !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || utf8.RuneCountInString(q) > 80 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid search query", "")
		return
	}
	kind := r.URL.Query().Get("kind")
	results := []annotationReference{}
	switch kind {
	case "word":
		words, _, err := h.words.ListAll("", q, 0, 20)
		if err != nil {
			annotationFailure(w, fmt.Errorf("search annotation words: %w", err))
			return
		}
		for _, item := range words {
			results = append(results, annotationReference{Kind: kind, ID: item.ID, Title: item.KanjiForm, Reading: item.Reading, Meaning: item.Meaning})
		}
	case "grammar":
		points, _, err := h.grammar.ListAll("", q, 0, 20)
		if err != nil {
			annotationFailure(w, fmt.Errorf("search annotation grammar: %w", err))
			return
		}
		for _, item := range points {
			results = append(results, annotationReference{Kind: kind, ID: item.ID, Title: item.Name, Meaning: item.Meaning})
		}
	default:
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid reference kind", "")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: results})
}

func annotationMatchesText(sentences []Sentence, start, end int, quote string) bool {
	var text strings.Builder
	for _, sentence := range sentences {
		for _, token := range sentence.Tokens {
			text.WriteString(token.Surface)
		}
	}
	chars := []rune(text.String())
	return start >= 0 && end > start && end <= len(chars) && string(chars[start:end]) == quote
}
