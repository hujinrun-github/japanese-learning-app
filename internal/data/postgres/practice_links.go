package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"japanese-learning-app/internal/module/practicelinks"
	"japanese-learning-app/internal/store"
)

type PracticeLinkStore struct{ db queryer }

func NewPracticeLinkStore(db queryer) *PracticeLinkStore { return &PracticeLinkStore{db: db} }

type practiceSource struct {
	table, column, catalog string
	numeric                bool
}

const listeningSourceMedia = `(l.audio_object_id IS NOT NULL OR l.video_object_id IS NOT NULL
	OR COALESCE(l.shadowing_config_json->>'audio_url','') <> ''
	OR COALESCE(l.shadowing_config_json->>'video_url','') <> ''
	OR COALESCE(l.shadowing_config_json->>'media_url','') <> '')`

func practiceSourceFor(kind string) (practiceSource, error) {
	switch kind {
	case practicelinks.ListeningMaterial:
		return practiceSource{"lessons", "listening_material_id", `
			SELECT l.id::text, l.title, '', l.jlpt_level, false FROM lessons l
			WHERE EXISTS (SELECT 1 FROM lesson_sentences WHERE lesson_id = l.id) AND ` + listeningSourceMedia, true}, nil
	case practicelinks.ListeningSentence:
		return practiceSource{"lesson_sentences", "listening_sentence_id", `
			SELECT line.id::text,
			       COALESCE(NULLIF((SELECT string_agg(token.value->>'surface', '' ORDER BY token.position)
			                         FROM jsonb_array_elements(line.tokens) WITH ORDINALITY AS token(value, position)), ''), line.chinese),
			       l.title || ' · #' || (line.position + 1)::text, l.jlpt_level, false
			FROM lesson_sentences line JOIN lessons l ON l.id = line.lesson_id WHERE ` + listeningSourceMedia, true}, nil
	case practicelinks.ListeningQuestion:
		return practiceSource{"listening_questions", "listening_question_id", `
			SELECT q.id, q.prompt, l.title || ' · #' || (q.position + 1)::text, l.jlpt_level, false
			FROM listening_questions q JOIN lessons l ON l.id = q.lesson_id WHERE q.published`, false}, nil
	case practicelinks.SpeakingMaterial:
		return practiceSource{"speaking_materials", "speaking_material_id", `
			SELECT id::text, COALESCE(NULLIF(title,''),text), type, jlpt_level, false FROM speaking_materials`, true}, nil
	case practicelinks.WritingTopic:
		return practiceSource{"writing_topics", "writing_topic_id", `
			SELECT id, title, sentence_prompt, jlpt_level, true FROM writing_topics`, false}, nil
	case practicelinks.WritingQuestion:
		return practiceSource{"writing_questions", "writing_question_id", `
			SELECT id::text, prompt, type, jlpt_level, type = 'sentence' FROM writing_questions`, true}, nil
	default:
		return practiceSource{}, fmt.Errorf("unsupported practice source %q: %w", kind, store.ErrConstraint)
	}
}

func (source practiceSource) key(id string) (any, error) {
	if source.numeric {
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number <= 0 || strconv.FormatInt(number, 10) != id {
			return nil, fmt.Errorf("invalid numeric practice source ID: %w", store.ErrConstraint)
		}
		return number, nil
	}
	if id == "" || len(id) > 160 || strings.ContainsRune(id, 0) {
		return nil, fmt.Errorf("invalid practice source ID: %w", store.ErrConstraint)
	}
	return id, nil
}

func (s *PracticeLinkStore) ListSources(ctx context.Context, kind, query string, offset, limit int) ([]practicelinks.Source, error) {
	source, err := practiceSourceFor(kind)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, context, level, can_hint FROM (`+source.catalog+`) AS sources(id,title,context,level,can_hint)
		WHERE $1 = '' OR strpos(lower(id),lower($1)) > 0 OR strpos(lower(title),lower($1)) > 0 OR strpos(lower(context),lower($1)) > 0
		ORDER BY title, id LIMIT $2 OFFSET $3`, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query %s practice sources: %w", kind, translateError(err))
	}
	defer rows.Close()
	sources := []practicelinks.Source{}
	for rows.Next() {
		var item practicelinks.Source
		if err := rows.Scan(&item.ID, &item.Title, &item.Context, &item.Level, &item.CanHint); err != nil {
			return nil, fmt.Errorf("scan practice source: %w", err)
		}
		sources = append(sources, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read practice sources: %w", translateError(err))
	}
	return sources, nil
}

func (s *PracticeLinkStore) SourceExists(ctx context.Context, target practicelinks.Target) error {
	source, err := practiceSourceFor(target.Kind)
	if err != nil {
		return err
	}
	key, err := source.key(target.ID)
	if err != nil {
		return err
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT true FROM `+source.table+` WHERE id = $1`, key).Scan(&exists); err != nil {
		return fmt.Errorf("check practice source: %w", translateError(err))
	}
	return nil
}

const practiceEntryFields = `
	CASE WHEN links.word_id IS NOT NULL THEN 'word' ELSE 'grammar' END,
	COALESCE(links.word_id, links.grammar_point_id),
	COALESCE(NULLIF(words.kanji_form, ''), words.reading, points.name),
	COALESCE(words.reading, ''), COALESCE(words.meaning, points.meaning),
	COALESCE(words.jlpt_level, points.jlpt_level), links.hint`

const practiceEntryJoins = `
	LEFT JOIN words ON words.id = links.word_id
	LEFT JOIN grammar_points points ON points.id = links.grammar_point_id`

func practiceEntryScan(link *practicelinks.Link) []any {
	return []any{&link.Kind, &link.ID, &link.Title, &link.Reading, &link.Meaning, &link.JLPTLevel, &link.Hint}
}

func (s *PracticeLinkStore) ListLinks(ctx context.Context, target practicelinks.Target, hintsOnly bool) ([]practicelinks.Link, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+practiceEntryFields+` FROM practice_knowledge_links links `+practiceEntryJoins+`
		WHERE links.source_kind = $1 AND links.source_id = $2 AND (NOT $3 OR links.hint)
		ORDER BY links.position`, target.Kind, target.ID, hintsOnly)
	if err != nil {
		return nil, fmt.Errorf("query practice links: %w", translateError(err))
	}
	defer rows.Close()
	links := []practicelinks.Link{}
	for rows.Next() {
		var link practicelinks.Link
		if err := rows.Scan(practiceEntryScan(&link)...); err != nil {
			return nil, fmt.Errorf("scan practice link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read practice links: %w", translateError(err))
	}
	return links, nil
}

func (s *PracticeLinkStore) ReplaceLinks(ctx context.Context, target practicelinks.Target, links []practicelinks.Selection) error {
	if len(links) > practicelinks.MaxLinks {
		return fmt.Errorf("too many practice links: %w", store.ErrConstraint)
	}
	source, err := practiceSourceFor(target.Kind)
	if err != nil {
		return err
	}
	key, err := source.key(target.ID)
	if err != nil {
		return err
	}
	err = withQueryerTx(ctx, s.db, func(tx queryer) error {
		canHintExpression := "false"
		if target.Kind == practicelinks.WritingTopic {
			canHintExpression = "true"
		} else if target.Kind == practicelinks.WritingQuestion {
			canHintExpression = "type = 'sentence'"
		}
		var canHint bool
		if err := tx.QueryRowContext(ctx, `SELECT `+canHintExpression+` FROM `+source.table+` WHERE id = $1 FOR NO KEY UPDATE`, key).Scan(&canHint); err != nil {
			return fmt.Errorf("lock practice source: %w", translateError(err))
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM practice_knowledge_links WHERE source_kind = $1 AND source_id = $2`, target.Kind, target.ID); err != nil {
			return fmt.Errorf("clear practice links: %w", translateError(err))
		}
		for position, entry := range links {
			var wordID, grammarID *int64
			switch entry.Kind {
			case practicelinks.Word:
				wordID = &entry.ID
			case practicelinks.Grammar:
				grammarID = &entry.ID
			default:
				return fmt.Errorf("unknown practice entry kind: %w", store.ErrConstraint)
			}
			if entry.Hint && (!canHint || grammarID == nil) {
				return fmt.Errorf("only sentence-writing grammar links can be hints: %w", store.ErrConstraint)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO practice_knowledge_links (source_kind, source_id, `+source.column+`, position, word_id, grammar_point_id, hint)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`, target.Kind, target.ID, key, position, wordID, grammarID, entry.Hint); err != nil {
				return fmt.Errorf("add practice %s link %d: %w", entry.Kind, entry.ID, translateError(err))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("replace %s %s practice links: %w", target.Kind, target.ID, err)
	}
	return nil
}

func (s *PracticeLinkStore) ListListeningLinks(ctx context.Context, lessonID int64, includeContent, includeQuestions bool) (practicelinks.ListeningLinks, error) {
	out := practicelinks.ListeningLinks{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT links.source_kind, links.source_id, COALESCE(line.position,0), `+practiceEntryFields+`
		FROM practice_knowledge_links links
		LEFT JOIN lesson_sentences line ON line.id = links.listening_sentence_id
		LEFT JOIN listening_questions question ON question.id = links.listening_question_id
		`+practiceEntryJoins+`
		WHERE ($2 AND (links.listening_material_id = $1 OR line.lesson_id = $1))
		   OR ($3 AND question.lesson_id = $1 AND question.published)
		ORDER BY links.position`, lessonID, includeContent, includeQuestions)
	if err != nil {
		return out, fmt.Errorf("query listening knowledge links: %w", translateError(err))
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id string
		var sentenceIndex int
		var link practicelinks.Link
		if err := rows.Scan(append([]any{&kind, &id, &sentenceIndex}, practiceEntryScan(&link)...)...); err != nil {
			return out, fmt.Errorf("scan listening knowledge link: %w", err)
		}
		switch kind {
		case practicelinks.ListeningMaterial:
			out.Material = append(out.Material, link)
		case practicelinks.ListeningSentence:
			if out.Sentences == nil {
				out.Sentences = make(map[int][]practicelinks.Link)
			}
			out.Sentences[sentenceIndex] = append(out.Sentences[sentenceIndex], link)
		case practicelinks.ListeningQuestion:
			if out.Questions == nil {
				out.Questions = make(map[string][]practicelinks.Link)
			}
			out.Questions[id] = append(out.Questions[id], link)
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read listening knowledge links: %w", translateError(err))
	}
	return out, nil
}
