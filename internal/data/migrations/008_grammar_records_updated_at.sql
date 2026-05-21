-- 008_grammar_records_updated_at.sql
-- Add updated_at column to grammar_records for today's progress tracking.

ALTER TABLE grammar_records ADD COLUMN updated_at DATETIME;
UPDATE grammar_records SET updated_at = datetime('now') WHERE updated_at IS NULL;
