CREATE TABLE IF NOT EXISTS lesson_annotations (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id     BIGINT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    start_offset  INTEGER NOT NULL CHECK (start_offset >= 0),
    end_offset    INTEGER NOT NULL CHECK (end_offset > start_offset),
    quote         TEXT NOT NULL,
    comment       TEXT NOT NULL DEFAULT '',
    links_json    TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (user_id, lesson_id, start_offset, end_offset)
);
