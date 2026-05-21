-- 009_user_jlpt_levels.sql
-- Add name column, add jlpt_levels JSON array (replacing single goal_level for multi-select).

ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN jlpt_levels TEXT NOT NULL DEFAULT '["N5"]';
UPDATE users SET jlpt_levels = json_array(goal_level) WHERE jlpt_levels = '["N5"]';
