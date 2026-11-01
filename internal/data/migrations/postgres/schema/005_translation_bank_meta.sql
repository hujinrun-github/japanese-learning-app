-- 005_translation_bank_meta.sql
-- Adds provenance + JLPT metadata to the translation tables so an external
-- question bank (e.g. the 言叶日课 / yanye-rike bank) can be imported without
-- losing level, topic, tags or grammar points.
-- All columns are additive with defaults, so existing rows stay valid.

ALTER TABLE translation_sources
    ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT '';

ALTER TABLE translation_sentences
    ADD COLUMN IF NOT EXISTS jlpt_level TEXT NOT NULL DEFAULT '';

ALTER TABLE translation_sentences
    ADD COLUMN IF NOT EXISTS topic TEXT NOT NULL DEFAULT '';

ALTER TABLE translation_sentences
    ADD COLUMN IF NOT EXISTS external_id TEXT NOT NULL DEFAULT '';

ALTER TABLE translation_sentences
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT ARRAY[]::text[];

ALTER TABLE translation_sentences
    ADD COLUMN IF NOT EXISTS points TEXT[] NOT NULL DEFAULT ARRAY[]::text[];

CREATE INDEX IF NOT EXISTS idx_translation_sentences_level
    ON translation_sentences (jlpt_level);

CREATE INDEX IF NOT EXISTS idx_translation_sentences_external_id
    ON translation_sentences (external_id);
