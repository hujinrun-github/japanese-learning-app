package writing

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

func (s *WritingService) ListDrafts(userID int64) ([]WritingDraft, error) {
	drafts, err := s.store.ListDrafts(userID)
	if err != nil {
		slog.Error("WritingService.ListDrafts failed", "user_id", userID, "err", err)
		return nil, fmt.Errorf("writing.ListDrafts: %w", err)
	}
	return drafts, nil
}

func (s *WritingService) SaveDraft(draft WritingDraft) (WritingDraft, error) {
	slog.Debug("WritingService.SaveDraft", "user_id", draft.UserID, "topic_id", draft.TopicID, "type", draft.Type, "characters", utf8.RuneCountInString(draft.UserAnswer))
	saved, err := s.store.SaveDraft(draft)
	if err != nil {
		slog.Error("WritingService.SaveDraft failed", "user_id", draft.UserID, "topic_id", draft.TopicID, "err", err)
		return WritingDraft{}, fmt.Errorf("writing.SaveDraft: %w", err)
	}
	return saved, nil
}

func (h *WritingHandler) handleListDrafts(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}
	drafts, err := h.svc.ListDrafts(userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load writing drafts", "")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: drafts})
}

func (h *WritingHandler) handleSaveDraft(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}
	var draft WritingDraft
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		slog.Debug("handleSaveDraft: invalid request", "user_id", userID, "err", err)
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid draft", "")
		return
	}
	if (draft.Type != WritingTypeEssay && draft.Type != WritingTypeSentence) || utf8.RuneCountInString(draft.UserAnswer) > 10000 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "draft type must be essay or sentence and length at most 10000 characters", "")
		return
	}
	draft.UserID = userID
	draft.TopicID = r.PathValue("id")
	for _, topic := range writingTopics() {
		if topic.ID != draft.TopicID {
			continue
		}
		saved, err := h.svc.SaveDraft(draft)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to save writing draft", "")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: saved})
		return
	}
	httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "writing topic not found", "")
}
