-- 004_words_unique_kanji_reading.sql
-- Add a UNIQUE constraint on (kanji_form, reading) to support idempotent CLI imports.
-- INSERT OR IGNORE will silently skip rows that already exist.

CREATE UNIQUE INDEX IF NOT EXISTS idx_words_kanji_reading
    ON words (kanji_form, reading);
