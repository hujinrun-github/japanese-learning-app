-- 012_words_verb_type.sql
-- 动词自他标签独立字段：自动词 / 他动词 / 自他两用，非动词与未标注为空字符串。
-- 线上词库原先把这些信息混写在 part_of_speech 中（「自动词」「自他動2」「ナ形・名・自動3」等）。
-- 使用 IF NOT EXISTS 保证对已手工加列的生产库幂等。

ALTER TABLE words ADD COLUMN IF NOT EXISTS verb_type TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN words.verb_type IS '动词自他标签：自动词 / 他动词 / 自他两用；非动词为空';
