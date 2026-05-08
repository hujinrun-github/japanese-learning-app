-- 006_notes.sql
-- Notes system: user-private notes with Markdown content, FTS5 search, SRS review, and linking.

CREATE TABLE IF NOT EXISTS notes (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL REFERENCES users(id),
    type                TEXT NOT NULL CHECK(type IN ('word', 'grammar', 'sentence')),
    title               TEXT NOT NULL,
    content             TEXT NOT NULL DEFAULT '',
    source_text         TEXT NOT NULL DEFAULT '',
    reference_id        INTEGER,
    reference_type      TEXT,
    tags_json           TEXT NOT NULL DEFAULT '[]',
    mastery_level       INTEGER NOT NULL DEFAULT 0,
    next_review_at      DATETIME,
    ease_factor         REAL NOT NULL DEFAULT 2.5,
    interval            INTEGER NOT NULL DEFAULT 0,
    review_history_json TEXT NOT NULL DEFAULT '[]',
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    deleted_at          DATETIME
);

CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes(user_id);
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(user_id, type);
CREATE INDEX IF NOT EXISTS idx_notes_review ON notes(user_id, next_review_at)
    WHERE next_review_at IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notes_reference ON notes(reference_type, reference_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS note_links (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    note_id         INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_note_id  INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    relation        TEXT NOT NULL DEFAULT 'related',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(note_id, target_note_id)
);

CREATE INDEX IF NOT EXISTS idx_note_links_note_id ON note_links(note_id);
CREATE INDEX IF NOT EXISTS idx_note_links_target ON note_links(target_note_id);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
    title, content, source_text,
    content=notes, content_rowid=id,
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, title, content, source_text)
    VALUES (new.id, new.title, new.content, new.source_text);
END;

CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, content, source_text)
    VALUES ('delete', old.id, old.title, old.content, old.source_text);
END;

CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, content, source_text)
    VALUES ('delete', old.id, old.title, old.content, old.source_text);
    INSERT INTO notes_fts(rowid, title, content, source_text)
    VALUES (new.id, new.title, new.content, new.source_text);
END;
