CREATE TABLE practice_knowledge_links (
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    listening_material_id BIGINT REFERENCES lessons(id) ON DELETE CASCADE,
    listening_sentence_id BIGINT REFERENCES lesson_sentences(id) ON DELETE CASCADE,
    listening_question_id TEXT REFERENCES listening_questions(id) ON DELETE CASCADE,
    speaking_material_id BIGINT REFERENCES speaking_materials(id) ON DELETE CASCADE,
    writing_topic_id TEXT REFERENCES writing_topics(id) ON DELETE CASCADE,
    writing_question_id BIGINT REFERENCES writing_questions(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    word_id BIGINT REFERENCES words(id) ON DELETE CASCADE,
    grammar_point_id BIGINT REFERENCES grammar_points(id) ON DELETE CASCADE,
    hint BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (source_kind, source_id, position),
    UNIQUE NULLS NOT DISTINCT (source_kind, source_id, word_id, grammar_point_id),
    CHECK (num_nonnulls(listening_material_id, listening_sentence_id, listening_question_id,
                        speaking_material_id, writing_topic_id, writing_question_id) = 1),
    CHECK ((CASE source_kind
        WHEN 'listening-material' THEN source_id = listening_material_id::TEXT
        WHEN 'listening-sentence' THEN source_id = listening_sentence_id::TEXT
        WHEN 'listening-question' THEN source_id = listening_question_id
        WHEN 'speaking-material' THEN source_id = speaking_material_id::TEXT
        WHEN 'writing-topic' THEN source_id = writing_topic_id
        WHEN 'writing-question' THEN source_id = writing_question_id::TEXT
    END) IS TRUE),
    CHECK ((word_id IS NULL) <> (grammar_point_id IS NULL)),
    CHECK (NOT hint OR (grammar_point_id IS NOT NULL AND source_kind IN ('writing-topic', 'writing-question')))
);

CREATE INDEX idx_practice_links_listening_material ON practice_knowledge_links (listening_material_id) WHERE listening_material_id IS NOT NULL;
CREATE INDEX idx_practice_links_listening_sentence ON practice_knowledge_links (listening_sentence_id) WHERE listening_sentence_id IS NOT NULL;
CREATE INDEX idx_practice_links_listening_question ON practice_knowledge_links (listening_question_id) WHERE listening_question_id IS NOT NULL;
CREATE INDEX idx_practice_links_speaking_material ON practice_knowledge_links (speaking_material_id) WHERE speaking_material_id IS NOT NULL;
CREATE INDEX idx_practice_links_writing_topic ON practice_knowledge_links (writing_topic_id) WHERE writing_topic_id IS NOT NULL;
CREATE INDEX idx_practice_links_writing_question ON practice_knowledge_links (writing_question_id) WHERE writing_question_id IS NOT NULL;
CREATE INDEX idx_practice_links_word ON practice_knowledge_links (word_id) WHERE word_id IS NOT NULL;
CREATE INDEX idx_practice_links_grammar ON practice_knowledge_links (grammar_point_id) WHERE grammar_point_id IS NOT NULL;
