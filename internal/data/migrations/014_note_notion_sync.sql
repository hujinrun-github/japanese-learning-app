-- Notion synchronization metadata. Secrets stay in environment variables.
CREATE TABLE IF NOT EXISTS note_sync_targets (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id               INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type                  TEXT NOT NULL CHECK(type = 'notion'),
    name                  TEXT NOT NULL,
    data_source_id        TEXT NOT NULL,
    token_env             TEXT NOT NULL DEFAULT 'JLA_NOTION_TOKEN',
    title_property        TEXT NOT NULL DEFAULT 'Name',
    tags_property         TEXT NOT NULL DEFAULT 'Tags',
    flow_id_property      TEXT NOT NULL DEFAULT '',
    sync_tags_json        TEXT NOT NULL DEFAULT '[]',
    enabled               INTEGER NOT NULL DEFAULT 1,
    auto_push             INTEGER NOT NULL DEFAULT 0,
    created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at            DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_note_sync_targets_user_identity
    ON note_sync_targets(user_id, data_source_id);

CREATE TABLE IF NOT EXISTS note_sync_bindings (
    note_id       INTEGER PRIMARY KEY REFERENCES notes(id) ON DELETE CASCADE,
    target_id     INTEGER NOT NULL REFERENCES note_sync_targets(id) ON DELETE RESTRICT,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_note_sync_bindings_target
    ON note_sync_bindings(target_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS note_sync_states (
    note_id           INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    target_id         INTEGER NOT NULL REFERENCES note_sync_targets(id) ON DELETE CASCADE,
    external_page_id  TEXT NOT NULL,
    external_url      TEXT NOT NULL DEFAULT '',
    local_hash        TEXT NOT NULL DEFAULT '',
    external_hash     TEXT NOT NULL DEFAULT '',
    last_direction    TEXT CHECK(last_direction IN ('push', 'pull', 'delete', 'restore')),
    last_synced_at    DATETIME,
    last_error        TEXT NOT NULL DEFAULT '',
    pending_delete    INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY(note_id, target_id),
    UNIQUE(target_id, external_page_id)
);

CREATE INDEX IF NOT EXISTS idx_note_sync_states_pending_delete
    ON note_sync_states(target_id, pending_delete, updated_at DESC);
