CREATE TABLE IF NOT EXISTS errata_reports (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module      TEXT NOT NULL,
    item_key    TEXT NOT NULL,
    item_label  TEXT NOT NULL,
    category    TEXT NOT NULL,
    description TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved')),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_errata_reports_status ON errata_reports(status, id);
CREATE INDEX IF NOT EXISTS idx_errata_reports_user ON errata_reports(user_id);
