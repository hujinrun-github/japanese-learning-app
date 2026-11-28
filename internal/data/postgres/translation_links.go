package postgres

import (
	"context"
	"fmt"

	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/store"
)

func (s *TranslationStore) ListSentenceLinks(ctx context.Context, sentenceID int64) ([]translation.SentenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT CASE WHEN links.word_id IS NOT NULL THEN 'word' ELSE 'grammar' END,
		       COALESCE(links.word_id, links.grammar_point_id),
		       COALESCE(NULLIF(words.kanji_form, ''), words.reading, points.name),
		       COALESCE(words.reading, ''),
		       COALESCE(words.meaning, points.meaning),
		       COALESCE(words.jlpt_level, points.jlpt_level)
		FROM translation_sentence_links links
		LEFT JOIN words ON words.id = links.word_id
		LEFT JOIN grammar_points points ON points.id = links.grammar_point_id
		WHERE links.sentence_id = $1
		ORDER BY links.position`, sentenceID)
	if err != nil {
		return nil, fmt.Errorf("query translation sentence links: %w", translateError(err))
	}
	defer rows.Close()

	links := []translation.SentenceLink{}
	for rows.Next() {
		var link translation.SentenceLink
		if err := rows.Scan(&link.Kind, &link.ID, &link.Title, &link.Reading, &link.Meaning, &link.JLPTLevel); err != nil {
			return nil, fmt.Errorf("scan translation sentence link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read translation sentence links: %w", translateError(err))
	}
	return links, nil
}

func (s *TranslationStore) ReplaceSentenceLinks(ctx context.Context, sentenceID int64, links []translation.SentenceLinkRef) error {
	if len(links) > translation.MaxSentenceLinks {
		return fmt.Errorf("too many translation sentence links: %w", store.ErrConstraint)
	}
	err := withQueryerTx(ctx, s.db, func(tx queryer) error {
		var lockedID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM translation_sentences WHERE id = $1 FOR UPDATE`, sentenceID).Scan(&lockedID); err != nil {
			return fmt.Errorf("lock translation sentence: %w", translateError(err))
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM translation_sentence_links WHERE sentence_id = $1`, sentenceID); err != nil {
			return fmt.Errorf("clear translation sentence links: %w", translateError(err))
		}
		for position, link := range links {
			var wordID, grammarID *int64
			switch link.Kind {
			case translation.SentenceLinkWord:
				wordID = &link.ID
			case translation.SentenceLinkGrammar:
				grammarID = &link.ID
			default:
				return fmt.Errorf("unknown translation link kind %q: %w", link.Kind, store.ErrConstraint)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO translation_sentence_links (sentence_id, position, word_id, grammar_point_id)
				VALUES ($1, $2, $3, $4)`, sentenceID, position, wordID, grammarID); err != nil {
				return fmt.Errorf("add %s link %d: %w", link.Kind, link.ID, translateError(err))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("replace translation sentence %d links: %w", sentenceID, err)
	}
	return nil
}
