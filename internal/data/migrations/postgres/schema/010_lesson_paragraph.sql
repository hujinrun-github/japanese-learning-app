-- Sentences carry the paragraph they belong to so lesson articles can be
-- rendered as prose paragraphs instead of one block per sentence.
-- Default 0 keeps every pre-existing row in the first paragraph, so no data
-- backfill is needed and old lessons remain readable.
ALTER TABLE lesson_sentences
    ADD COLUMN IF NOT EXISTS paragraph INTEGER NOT NULL DEFAULT 0;
