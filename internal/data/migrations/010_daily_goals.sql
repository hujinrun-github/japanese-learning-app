-- 010_daily_goals.sql
-- Add daily_goals_json column to users table for per-module daily goal tracking.

ALTER TABLE users ADD COLUMN daily_goals_json TEXT NOT NULL DEFAULT '{}';
