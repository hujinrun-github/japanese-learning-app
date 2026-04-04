-- 003_fix_writing_questions.sql
-- 修复 writing_questions.grammar_point_id：改为无 FK 约束的软引用
-- 0 或 NULL 均表示无关联语法点
-- SQLite 不支持 ALTER COLUMN，重建表

PRAGMA foreign_keys = OFF;

CREATE TABLE writing_questions_new (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    type              TEXT    NOT NULL,
    prompt            TEXT    NOT NULL,
    expected_answer   TEXT    NOT NULL,
    grammar_point_id  INTEGER NOT NULL DEFAULT 0,  -- 0 表示无关联语法点，无 FK 约束
    jlpt_level        TEXT    NOT NULL DEFAULT 'N5'
);

INSERT INTO writing_questions_new
    SELECT id, type, prompt, expected_answer, grammar_point_id, jlpt_level
    FROM writing_questions;

DROP TABLE writing_questions;
ALTER TABLE writing_questions_new RENAME TO writing_questions;

PRAGMA foreign_keys = ON;
