package postgres

import (
	"context"
	"fmt"
	"strings"

	"japanese-learning-app/internal/module/writing"
)

func (s *WritingStore) ListDrafts(userID int64) ([]writing.WritingDraft, error) {
	rows, err := s.db.QueryContext(context.Background(), `SELECT topic_id, type, user_answer, updated_at FROM writing_drafts WHERE user_id = $1 ORDER BY updated_at DESC, topic_id, type`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres.WritingStore.ListDrafts: %w", translateError(err))
	}
	defer rows.Close()
	drafts := make([]writing.WritingDraft, 0)
	for rows.Next() {
		draft := writing.WritingDraft{UserID: userID}
		if err := rows.Scan(&draft.TopicID, &draft.Type, &draft.UserAnswer, &draft.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres.WritingStore.ListDrafts scan: %w", err)
		}
		drafts = append(drafts, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.WritingStore.ListDrafts rows: %w", err)
	}
	return drafts, nil
}

func (s *WritingStore) SaveDraft(draft writing.WritingDraft) (writing.WritingDraft, error) {
	draft.UpdatedAt = s.now().UTC()
	if strings.TrimSpace(draft.UserAnswer) == "" {
		_, err := s.db.ExecContext(context.Background(), `DELETE FROM writing_drafts WHERE user_id = $1 AND topic_id = $2 AND type = $3`, draft.UserID, draft.TopicID, draft.Type)
		if err != nil {
			return writing.WritingDraft{}, fmt.Errorf("postgres.WritingStore.SaveDraft clear: %w", translateError(err))
		}
		return draft, nil
	}
	_, err := s.db.ExecContext(context.Background(), `INSERT INTO writing_drafts (user_id, topic_id, type, user_answer, updated_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, topic_id, type) DO UPDATE SET user_answer = EXCLUDED.user_answer, updated_at = EXCLUDED.updated_at`,
		draft.UserID, draft.TopicID, draft.Type, draft.UserAnswer, draft.UpdatedAt)
	if err != nil {
		return writing.WritingDraft{}, fmt.Errorf("postgres.WritingStore.SaveDraft: %w", translateError(err))
	}
	return draft, nil
}
