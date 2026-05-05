-- 005_password_reset_tokens.sql
-- 密码重置令牌表：用于「忘记密码」功能

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token      TEXT    NOT NULL PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    used       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
