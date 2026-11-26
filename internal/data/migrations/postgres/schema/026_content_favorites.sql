CREATE TABLE content_favorites (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('grammar', 'lesson', 'listening', 'listening_sentence', 'speaking', 'shadowing', 'writing', 'translation', 'note')),
    target_key TEXT NOT NULL,
    PRIMARY KEY (user_id, kind, target_key)
);

CREATE VIEW favorite_content AS
SELECT 'word'::text AS kind, id::text AS target_key, COALESCE(NULLIF(kanji_form,''), reading) AS title,
    concat_ws(' · ', NULLIF(reading,''), NULLIF(meaning,'')) AS detail, jlpt_level,
    '/search/words/' || id AS path, NULL::bigint AS owner_id
FROM words
UNION ALL
SELECT 'grammar', id::text, name, meaning, jlpt_level, '/grammar/' || id, NULL FROM grammar_points
UNION ALL
SELECT 'lesson', id::text, title, '', jlpt_level, '/lesson/' || id, NULL FROM lessons
UNION ALL
SELECT 'listening', l.id::text, l.title, '', l.jlpt_level, '/listening/' || l.id, NULL
FROM lessons l WHERE EXISTS (SELECT 1 FROM lesson_sentences s WHERE s.lesson_id = l.id)
UNION ALL
SELECT 'listening_sentence', l.id || ':' || s.position,
    COALESCE((SELECT string_agg(token->>'surface', '' ORDER BY ord) FROM jsonb_array_elements(s.tokens) WITH ORDINALITY AS t(token, ord)), l.title),
    concat_ws(' · ', l.title, NULLIF(s.chinese,'')), l.jlpt_level, '/listening/' || l.id || '?sentence=' || s.position, NULL
FROM lesson_sentences s JOIN lessons l ON l.id = s.lesson_id
UNION ALL
SELECT 'speaking', id::text, COALESCE(NULLIF(title,''), left(text,100)), left(text,180), jlpt_level, '/speaking?material_id=' || id, NULL
FROM speaking_materials
UNION ALL
SELECT 'shadowing', id::text, title, '', jlpt_level, '/lesson/' || id || '/shadowing', NULL
FROM lessons WHERE shadowing_enabled
UNION ALL
SELECT 'writing', id, title, essay_prompt, jlpt_level, '/writing/practice?topic_id=' || id, NULL FROM writing_topics
UNION ALL
SELECT 'translation', id::text, source_text, '', jlpt_level, '/translation/practice?sentence_id=' || id || '&from=favorites', NULL FROM translation_sentences
UNION ALL
SELECT 'note', id::text, title, left(content,180), '', '/notes/' || id, user_id FROM notes WHERE deleted_at IS NULL;
