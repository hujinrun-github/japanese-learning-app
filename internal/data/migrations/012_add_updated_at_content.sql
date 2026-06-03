-- 012_add_updated_at_content.sql
-- Add updated_at columns to content tables so they can be sorted by last-updated time.
-- Column is nullable; INSERT and UPDATE queries in application code always set it explicitly.
-- Existing rows get the current timestamp.

ALTER TABLE words ADD COLUMN updated_at DATETIME;
ALTER TABLE grammar_points ADD COLUMN updated_at DATETIME;
ALTER TABLE speaking_materials ADD COLUMN updated_at DATETIME;
ALTER TABLE writing_questions ADD COLUMN updated_at DATETIME;
ALTER TABLE translation_sentences ADD COLUMN updated_at DATETIME;

UPDATE words SET updated_at = datetime('now') WHERE updated_at IS NULL;
UPDATE grammar_points SET updated_at = datetime('now') WHERE updated_at IS NULL;
UPDATE speaking_materials SET updated_at = datetime('now') WHERE updated_at IS NULL;
UPDATE writing_questions SET updated_at = datetime('now') WHERE updated_at IS NULL;
UPDATE translation_sentences SET updated_at = datetime('now') WHERE updated_at IS NULL;
