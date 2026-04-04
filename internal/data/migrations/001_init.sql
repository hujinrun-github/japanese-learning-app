-- 001_init.sql
-- 建表 DDL：日语学习应用全量表结构
-- 执行顺序：内容库（只读）→ 用户表 → 用户学习数据

-- ============================================================
-- 内容库（管理员通过 CLI 维护，用户 API 只读）
-- ============================================================

CREATE TABLE IF NOT EXISTS words (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    kanji_form      TEXT    NOT NULL,               -- 汉字写法，如「勉強」
    reading         TEXT    NOT NULL,               -- 假名读音，如「べんきょう」
    part_of_speech  TEXT    NOT NULL DEFAULT '',    -- 词性，如「名詞」
    meaning         TEXT    NOT NULL,               -- 中文释义
    examples_json   TEXT    NOT NULL DEFAULT '[]',  -- []WordExample JSON
    jlpt_level      TEXT    NOT NULL                -- "N5" | "N4" | "N3" | "N2" | "N1"
);

CREATE INDEX IF NOT EXISTS idx_words_jlpt_level ON words (jlpt_level);

CREATE TABLE IF NOT EXISTS grammar_points (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    name                TEXT    NOT NULL,               -- 如「〜てもいい」
    meaning             TEXT    NOT NULL,               -- 中文意思
    conjunction_rule    TEXT    NOT NULL DEFAULT '',    -- 接续方式
    usage_note          TEXT    NOT NULL DEFAULT '',    -- 使用场景说明
    examples_json       TEXT    NOT NULL DEFAULT '[]',  -- []GrammarExample JSON
    quiz_questions_json TEXT    NOT NULL DEFAULT '[]',  -- []QuizQuestion JSON（含答案）
    jlpt_level          TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_grammar_points_jlpt ON grammar_points (jlpt_level);

CREATE TABLE IF NOT EXISTS lessons (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    title                   TEXT    NOT NULL,
    content_furigana_json   TEXT    NOT NULL DEFAULT '[]',  -- []Sentence JSON（含振り仮名）
    translation_json        TEXT    NOT NULL DEFAULT '[]',  -- 备用全文翻译
    jlpt_level              TEXT    NOT NULL,
    tags_json               TEXT    NOT NULL DEFAULT '[]',  -- []string
    audio_url               TEXT    NOT NULL DEFAULT '',
    sentence_timestamps_json TEXT   NOT NULL DEFAULT '[]',  -- 句子时间戳（冗余，加速查询）
    char_count              INTEGER NOT NULL DEFAULT 0,
    word_ids_json           TEXT    NOT NULL DEFAULT '[]'   -- []int64 可加入单词本的词汇 ID
);

CREATE INDEX IF NOT EXISTS idx_lessons_jlpt ON lessons (jlpt_level);

CREATE TABLE IF NOT EXISTS speaking_materials (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    type        TEXT    NOT NULL,               -- "shadow" | "free"
    title       TEXT    NOT NULL DEFAULT '',
    text        TEXT    NOT NULL,               -- 朗读文本
    audio_url   TEXT    NOT NULL DEFAULT '',    -- 参考音频 URL
    jlpt_level  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_speaking_materials_type_level
    ON speaking_materials (type, jlpt_level);

CREATE TABLE IF NOT EXISTS writing_questions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    type              TEXT    NOT NULL,               -- "input" | "sentence"
    prompt            TEXT    NOT NULL,               -- 题目提示
    expected_answer   TEXT    NOT NULL,               -- 标准答案（不返回前端）
    grammar_point_id  INTEGER NOT NULL DEFAULT 0,     -- 造句题关联的语法点（0 表示无关联）
    jlpt_level        TEXT    NOT NULL DEFAULT 'N5',
    FOREIGN KEY (grammar_point_id) REFERENCES grammar_points(id) ON DELETE SET DEFAULT
);

-- ============================================================
-- 用户账户
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    email           TEXT    NOT NULL UNIQUE,
    password_hash   TEXT    NOT NULL,
    goal_level      TEXT    NOT NULL DEFAULT 'N5',  -- 学习目标 JLPT 等级
    streak_days     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);

-- ============================================================
-- 用户学习数据（读写频繁）
-- ============================================================

CREATE TABLE IF NOT EXISTS word_records (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    word_id             INTEGER NOT NULL,
    mastery_level       INTEGER NOT NULL DEFAULT 0,         -- SM-2 重复次数（0~5）
    next_review_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    ease_factor         REAL    NOT NULL DEFAULT 2.5,       -- SM-2 EF
    interval            INTEGER NOT NULL DEFAULT 0,         -- 距下次复习天数
    review_history_json TEXT    NOT NULL DEFAULT '[]',      -- []ReviewEvent JSON
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, word_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (word_id) REFERENCES words(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_word_records_due
    ON word_records (user_id, next_review_at);

CREATE TABLE IF NOT EXISTS word_bookmarks (
    user_id INTEGER NOT NULL,
    word_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, word_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (word_id) REFERENCES words(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS grammar_records (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    grammar_point_id    INTEGER NOT NULL,
    status              TEXT    NOT NULL DEFAULT 'unlearned', -- "unlearned"|"learning"|"mastered"
    next_review_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    quiz_history_json   TEXT    NOT NULL DEFAULT '[]',        -- []QuizAttempt JSON
    UNIQUE (user_id, grammar_point_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (grammar_point_id) REFERENCES grammar_points(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_grammar_records_due
    ON grammar_records (user_id, next_review_at);

CREATE TABLE IF NOT EXISTS speaking_records (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    type            TEXT    NOT NULL,                           -- "shadow" | "free"
    material_id     INTEGER NOT NULL,
    score           INTEGER NOT NULL DEFAULT 0,                 -- 0~100
    audio_ref       TEXT    NOT NULL DEFAULT '',                -- 录音文件路径
    practiced_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (material_id) REFERENCES speaking_materials(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_speaking_records_user
    ON speaking_records (user_id, practiced_at DESC);

CREATE TABLE IF NOT EXISTS writing_records (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    type            TEXT    NOT NULL,                       -- "input" | "sentence"
    question        TEXT    NOT NULL,
    user_answer     TEXT    NOT NULL DEFAULT '',
    ai_feedback_json TEXT   NOT NULL DEFAULT 'null',        -- AIFeedback JSON | null
    score           INTEGER NOT NULL DEFAULT 0,
    practiced_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_writing_records_user
    ON writing_records (user_id, practiced_at DESC);

CREATE TABLE IF NOT EXISTS study_sessions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id       TEXT    NOT NULL UNIQUE,               -- UUID 字符串
    user_id          INTEGER NOT NULL,
    module           TEXT    NOT NULL,                      -- "word"|"grammar"|"lesson"|"speaking"|"writing"
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    completed_count  INTEGER NOT NULL DEFAULT 0,
    started_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_study_sessions_user
    ON study_sessions (user_id, started_at DESC);

CREATE TABLE IF NOT EXISTS session_summaries (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id              TEXT    NOT NULL UNIQUE,
    user_id                 INTEGER NOT NULL,
    module                  TEXT    NOT NULL,
    score_summary_json      TEXT    NOT NULL DEFAULT '{}',
    strengths_json          TEXT    NOT NULL DEFAULT '[]',
    weaknesses_json         TEXT    NOT NULL DEFAULT '[]',
    suggestions_json        TEXT    NOT NULL DEFAULT '[]',
    generated_at            DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES study_sessions(session_id) ON DELETE CASCADE
);
