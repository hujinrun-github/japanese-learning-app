-- 015_user_streak.sql
-- Adds the anchor the daily streak is computed against (see the PostgreSQL
-- migration 007_user_streak.sql for the rationale).
-- SQLite has no ADD COLUMN IF NOT EXISTS; the migration runner tolerates the
-- duplicate-column error when this has already been applied.
-- Stored as a TEXT 'YYYY-MM-DD' local calendar day.
ALTER TABLE users ADD COLUMN last_study_date TEXT;
