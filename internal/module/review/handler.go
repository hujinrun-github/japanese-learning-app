package review

import (
	"net/http"
	"sort"
	"time"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

// ReviewCard is a unified review queue card that can be either a word or a note.
type ReviewCard struct {
	CardType string        `json:"card_type"` // "word" | "note"
	WordCard *word.WordCard `json:"word_card,omitempty"`
	NoteCard *note.Note     `json:"note_card,omitempty"`
	IsNew    bool           `json:"is_new"`
}

// ReviewHandler serves the unified review queue.
type ReviewHandler struct {
	wordSvc *word.WordService
	noteSvc *note.NoteService
}

// NewReviewHandler creates a ReviewHandler.
func NewReviewHandler(wordSvc *word.WordService, noteSvc *note.NoteService) *ReviewHandler {
	return &ReviewHandler{wordSvc: wordSvc, noteSvc: noteSvc}
}

// RegisterRoutes registers review routes.
func (h *ReviewHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/review/queue", h.handleGetQueue)
}

func (h *ReviewHandler) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var cards []ReviewCard

	// Word cards from existing review queue
	level := word.JLPTLevel(r.URL.Query().Get("level"))
	if level == "" {
		level = word.LevelN5
	}
	wordCards, err := h.wordSvc.GetReviewQueue(userID, level)
	if err == nil {
		for _, wc := range wordCards {
			cards = append(cards, ReviewCard{
				CardType: "word",
				WordCard: &wc,
				IsNew:    wc.IsNew,
			})
		}
	}

	// Note cards due for review
	noteCards, err := h.noteSvc.GetReviewQueue(userID)
	if err == nil {
		for _, nc := range noteCards {
			ncCopy := nc
			cards = append(cards, ReviewCard{
				CardType: "note",
				NoteCard: &ncCopy,
				IsNew:    nc.NextReviewAt == nil,
			})
		}
	}

	// Sort: non-nil next_review_at first (ascending), nil (new) cards at end
	sort.Slice(cards, func(i, j int) bool {
		ti := reviewTime(cards[i])
		tj := reviewTime(cards[j])
		if ti == nil && tj == nil {
			return false
		}
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.Before(*tj)
	})

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: cards})
}

func reviewTime(c ReviewCard) *time.Time {
	if c.CardType == "word" && c.WordCard != nil {
		return &c.WordCard.Record.NextReviewAt
	}
	if c.CardType == "note" && c.NoteCard != nil {
		return c.NoteCard.NextReviewAt
	}
	return nil
}
