-- 013_shadowing.sql
-- Shadowing Audio MVP runtime schema for SQLite.

ALTER TABLE lessons ADD COLUMN shadowing_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE lessons ADD COLUMN video_url TEXT NOT NULL DEFAULT '';
ALTER TABLE lessons ADD COLUMN shadowing_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE lessons ADD COLUMN shadowing_config_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS lesson_shadowing_progress (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id              INTEGER NOT NULL,
    lesson_id            INTEGER NOT NULL,
    shadowing_version    INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    last_sentence_index  INTEGER NOT NULL DEFAULT 0 CHECK (last_sentence_index >= 0),
    last_position_ms     INTEGER NOT NULL DEFAULT 0 CHECK (last_position_ms >= 0),
    last_practice_mode   TEXT    NOT NULL DEFAULT 'normal',
    updated_at           DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, lesson_id, shadowing_version),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (lesson_id) REFERENCES lessons(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS lesson_shadowing_attempts (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id              INTEGER NOT NULL,
    lesson_id            INTEGER NOT NULL,
    shadowing_version    INTEGER NOT NULL DEFAULT 1 CHECK (shadowing_version >= 1),
    sentence_index       INTEGER NOT NULL CHECK (sentence_index >= 0),
    practice_mode        TEXT    NOT NULL,
    playback_rate        REAL    NOT NULL DEFAULT 1.0 CHECK (playback_rate > 0),
    loop_count           INTEGER NOT NULL DEFAULT 0 CHECK (loop_count >= 0),
    self_score           INTEGER CHECK (self_score IS NULL OR (self_score BETWEEN 0 AND 100)),
    recognition_text     TEXT    NOT NULL DEFAULT '',
    audio_ref            TEXT    NOT NULL DEFAULT '',
    created_at           DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (lesson_id) REFERENCES lessons(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lesson_shadowing_attempts_sentence
    ON lesson_shadowing_attempts (user_id, lesson_id, shadowing_version, sentence_index, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lesson_shadowing_attempts_history
    ON lesson_shadowing_attempts (user_id, lesson_id, shadowing_version, created_at DESC);
