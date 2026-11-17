-- Mistake book: one row per (user, module, question). Re-answering the same
-- question wrong again updates the existing row instead of inserting a duplicate.
-- Every statement is idempotent because migrations are replayed on each startup.
CREATE TABLE IF NOT EXISTS mistakes (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module         TEXT NOT NULL CHECK (module IN ('word', 'grammar', 'writing', 'translation')),
    item_key       TEXT NOT NULL,
    prompt         TEXT NOT NULL,
    user_answer    TEXT NOT NULL DEFAULT '',
    correct_answer TEXT NOT NULL DEFAULT '',
    explanation    TEXT NOT NULL DEFAULT '',
    source_path    TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'unmastered' CHECK (status IN ('unmastered', 'mastered')),
    wrong_count    INTEGER NOT NULL DEFAULT 1,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    mastered_at    DATETIME,
    UNIQUE (user_id, module, item_key)
);

CREATE INDEX IF NOT EXISTS idx_mistakes_user_status ON mistakes(user_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_mistakes_user_module ON mistakes(user_id, module, status);
