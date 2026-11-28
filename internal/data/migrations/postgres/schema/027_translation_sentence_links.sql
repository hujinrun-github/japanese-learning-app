CREATE TABLE translation_sentence_links (
    sentence_id BIGINT NOT NULL REFERENCES translation_sentences(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    word_id BIGINT REFERENCES words(id) ON DELETE CASCADE,
    grammar_point_id BIGINT REFERENCES grammar_points(id) ON DELETE CASCADE,
    PRIMARY KEY (sentence_id, position),
    CHECK ((word_id IS NULL) <> (grammar_point_id IS NULL))
);

CREATE UNIQUE INDEX idx_translation_sentence_links_word
    ON translation_sentence_links (word_id, sentence_id) WHERE word_id IS NOT NULL;
CREATE UNIQUE INDEX idx_translation_sentence_links_grammar
    ON translation_sentence_links (grammar_point_id, sentence_id) WHERE grammar_point_id IS NOT NULL;
