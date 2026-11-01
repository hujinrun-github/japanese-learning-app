-- 007_user_streak.sql
-- Adds the anchor the daily streak is computed against.
--
-- users.streak_days alone cannot tell "studied today" from "studied yesterday":
-- without a date there is no way to know whether the streak should advance,
-- hold, or reset. last_study_date records the learner's local calendar day of
-- the most recent check-in, and RecordStudyDay advances the streak from it.
-- Nullable + no default: an existing row simply has no check-in yet.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS last_study_date DATE;
