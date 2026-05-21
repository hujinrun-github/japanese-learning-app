-- 009_translation.sql
-- Create translation module tables: sources, sentences, and practice records.

CREATE TABLE translation_sources (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    title          TEXT    NOT NULL,
    source_type    TEXT    NOT NULL CHECK (source_type IN ('manual', 'url', 'api')),
    source_url     TEXT    NOT NULL DEFAULT '',
    api_endpoint   TEXT    NOT NULL DEFAULT '',
    raw_content    TEXT    NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE translation_sentences (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id             INTEGER NOT NULL REFERENCES translation_sources(id) ON DELETE CASCADE,
    direction             TEXT    NOT NULL CHECK (direction IN ('cn2jp', 'jp2cn')),
    source_text           TEXT    NOT NULL,
    reference_translation TEXT    NOT NULL DEFAULT '',
    position              INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE translation_records (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER  NOT NULL REFERENCES users(id),
    sentence_id       INTEGER  NOT NULL REFERENCES translation_sentences(id),
    user_translation  TEXT     NOT NULL,
    score             INTEGER  NOT NULL DEFAULT 0,
    rule_score        INTEGER  NOT NULL DEFAULT 0,
    ai_feedback_json  TEXT     NOT NULL DEFAULT '',
    practiced_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_translation_sentences_source ON translation_sentences(source_id);
CREATE INDEX idx_translation_records_user ON translation_records(user_id);
CREATE INDEX idx_translation_records_sentence ON translation_records(sentence_id);
